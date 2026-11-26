package reach

// collectScript is the constant PowerShell program that reads the facts;
// no user data is ever put into it. Output is one JSON object with the
// lists Profiles, Firewall, Rules and Owner; a list with a single item is
// still an array here, but ParseFacts accepts a bare object too.
//
// Everything is read through COM interfaces, because the NetSecurity
// cmdlets (Get-NetFirewallRule and friends) need administrator rights even
// for reading, while the tray app runs unelevated:
//
//   - Networks come from the Network List Manager, the same source
//     Get-NetConnectionProfile wraps, which any user may read.
//   - Firewall profiles and rules come from HNetCfg.FwPolicy2, the public
//     read API behind the firewall control panel. Reading needs no
//     elevation; only writing does (the fixer elevates for that).
//
// The trade-off: the COM object lists the local policy store only, so
// rules pushed by domain group policy are invisible here. Rules are
// narrowed to those that could concern TCP 631 (TCP or any protocol, local
// port "Any" or starting with a digit); the classifier does the exact
// match. Each section is read independently: one failure empties only its
// own list, and the classifier treats missing facts as "unknown" rather
// than guessing.
const collectScript = `[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)

$profiles = @()
try {
  $nlm = [Activator]::CreateInstance([type]::GetTypeFromCLSID([guid]'DCB00C01-570F-4A9B-8D69-199FDBA5723B'))
  $profiles = @($nlm.GetNetworks(1) | ForEach-Object {
    $cat = 'Public'
    if ($_.GetCategory() -eq 1) { $cat = 'Private' }
    elseif ($_.GetCategory() -eq 2) { $cat = 'DomainAuthenticated' }
    [pscustomobject]@{
      InterfaceAlias = [string]$_.GetName()
      NetworkCategory = $cat
      IPv4Connectivity = $(if ($_.IsConnectedToInternet) { 'Internet' } else { 'LocalNetwork' })
    }
  })
} catch { }

$firewall = @()
$rules = @()
try {
  $fw = New-Object -ComObject HNetCfg.FwPolicy2
  $firewall = @(@(1, 'Domain'), @(2, 'Private'), @(4, 'Public') | ForEach-Object {
    $inbound = 'Block'
    if ($fw.DefaultInboundAction($_[0]) -eq 1) { $inbound = 'Allow' }
    [pscustomobject]@{
      Name = $_[1]
      Enabled = [string]$fw.FirewallEnabled($_[0])
      DefaultInboundAction = $inbound
    }
  })
  $rules = @($fw.Rules | Where-Object { $_.Direction -eq 1 -and $_.Enabled -and ($_.Protocol -eq 6 -or $_.Protocol -eq 256) } | ForEach-Object {
    $rule = $_
    $ports = @('Any')
    if (-not [string]::IsNullOrWhiteSpace($rule.LocalPorts) -and $rule.LocalPorts -ne '*') {
      $ports = @($rule.LocalPorts -split ',' | ForEach-Object { $_.Trim() })
    }
    $addrs = @('Any')
    if (-not [string]::IsNullOrWhiteSpace($rule.RemoteAddresses) -and $rule.RemoteAddresses -ne '*') {
      $addrs = @($rule.RemoteAddresses -split ',' | ForEach-Object { $_.Trim() })
    }
    $relevant = $false
    foreach ($pt in $ports) { if ($pt -eq 'Any' -or $pt -match '^\d') { $relevant = $true } }
    if ($relevant) {
      $action = 'Block'
      if ($rule.Action -eq 1) { $action = 'Allow' }
      [pscustomobject]@{
        Name = [string]$rule.Name
        DisplayName = [string]$rule.Name
        Action = $action
        Profile = [int]$rule.Profiles
        Protocol = [string]$rule.Protocol
        LocalPort = $ports
        RemoteAddress = $addrs
        Program = [string]$rule.ApplicationName
      }
    }
  })
} catch { }

$owner = @()
try {
  $owner = @(Get-NetTCPConnection -LocalPort 631 -State Listen -ErrorAction SilentlyContinue | ForEach-Object {
    $proc = Get-Process -Id $_.OwningProcess -ErrorAction SilentlyContinue
    [pscustomobject]@{ ProcessName = [string]$proc.ProcessName; ProcessId = [int]$_.OwningProcess }
  })
} catch { }

[pscustomobject]@{ Profiles = $profiles; Firewall = $firewall; Rules = $rules; Owner = $owner } | ConvertTo-Json -Depth 5 -Compress
`
