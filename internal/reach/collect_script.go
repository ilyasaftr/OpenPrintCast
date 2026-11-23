package reach

// collectScript is the constant PowerShell program that reads the facts;
// no user data is ever put into it. Output is one JSON object with the
// lists Profiles, Firewall, Rules and Owner; a list with a single item is
// still an array here, but ParseFacts accepts a bare object too. Rules are
// narrowed to those that could concern TCP 631 (TCP or any protocol, local
// port "Any" or starting with a digit); the classifier does the exact match.
//
// The filters are fetched in bulk and joined on InstanceID, because asking
// for the filters rule by rule takes minutes on a PC with many rules.
const collectScript = `$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
$store = 'ActiveStore'
$ports = @{}
Get-NetFirewallPortFilter -All -PolicyStore $store | ForEach-Object { $ports[$_.InstanceID] = $_ }
$addrs = @{}
Get-NetFirewallAddressFilter -All -PolicyStore $store | ForEach-Object { $addrs[$_.InstanceID] = $_ }
$apps = @{}
Get-NetFirewallApplicationFilter -All -PolicyStore $store | ForEach-Object { $apps[$_.InstanceID] = $_ }
$profiles = @(Get-NetConnectionProfile | ForEach-Object {
  [pscustomobject]@{ InterfaceAlias = [string]$_.InterfaceAlias; NetworkCategory = [string]$_.NetworkCategory; IPv4Connectivity = [string]$_.IPv4Connectivity }
})
$firewall = @(Get-NetFirewallProfile -PolicyStore $store | ForEach-Object {
  [pscustomobject]@{ Name = [string]$_.Name; Enabled = [string]$_.Enabled; DefaultInboundAction = [string]$_.DefaultInboundAction }
})
$rules = @(Get-NetFirewallRule -Direction Inbound -Enabled True -PolicyStore $store | ForEach-Object {
  $p = $ports[$_.InstanceID]
  $tcp = $p -and (@('TCP', '6', 'Any', '256') -contains [string]$p.Protocol)
  $port = $tcp -and (@($p.LocalPort | Where-Object { ([string]$_) -eq 'Any' -or ([string]$_) -match '^\d' }).Count -gt 0)
  if ($port) {
    [pscustomobject]@{
      Name = [string]$_.Name; DisplayName = [string]$_.DisplayName; Action = [string]$_.Action; Profile = [int]$_.Profile
      Protocol = [string]$p.Protocol; LocalPort = @($p.LocalPort | ForEach-Object { [string]$_ })
      RemoteAddress = @($addrs[$_.InstanceID].RemoteAddress | ForEach-Object { [string]$_ })
      Program = [string]$apps[$_.InstanceID].Program
    }
  }
})
$owner = @(Get-NetTCPConnection -LocalPort 631 -State Listen -ErrorAction SilentlyContinue | ForEach-Object {
  $proc = Get-Process -Id $_.OwningProcess -ErrorAction SilentlyContinue
  [pscustomobject]@{ ProcessName = [string]$proc.ProcessName; ProcessId = [int]$_.OwningProcess }
})
[pscustomobject]@{ Profiles = $profiles; Firewall = $firewall; Rules = $rules; Owner = $owner } | ConvertTo-Json -Depth 5 -Compress
`
