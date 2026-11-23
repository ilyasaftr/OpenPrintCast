#define MyAppName "OpenPrintCast"
; Displayed version (the tag without the leading "v"). CI passes
; /DMyAppVersion=... /DMyAppNumericVersion=... ; these defaults only apply to a
; manual compile. VersionInfo* require a purely numeric a.b.c.d.
; MyArch is the CPU the installer is for, amd64 or arm64; it picks the
; program file (dist\OpenPrintCast-<arch>.exe), the architectures the setup
; accepts and the setup file name.
#ifndef MyAppVersion
  #define MyAppVersion "0.0.1"
#endif
#ifndef MyAppNumericVersion
  #define MyAppNumericVersion "0.0.1.0"
#endif
#ifndef MyArch
  #define MyArch "amd64"
#endif
#if MyArch == "arm64"
  #define MyArchAllowed "arm64"
#else
  #define MyArchAllowed "x64compatible"
#endif
#define MyAppPublisher "ilyasaftr"
#define MyAppExeName "OpenPrintCast.exe"
#define MyAppDir "C:\Program Files\OpenPrintCast"

[Setup]
AppId={{B6701802-1912-48E8-89AB-66A6242ACC1B}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
DefaultDirName={#MyAppDir}
SetupIconFile=assets\icon.ico
UninstallDisplayIcon={app}\{#MyAppExeName}
VersionInfoVersion={#MyAppNumericVersion}
VersionInfoProductVersion={#MyAppNumericVersion}
VersionInfoProductName={#MyAppName}
VersionInfoCompany={#MyAppPublisher}
OutputDir=dist
OutputBaseFilename=OpenPrintCast-Setup-{#MyAppVersion}-{#MyArch}
Compression=lzma
SolidCompression=yes
ArchitecturesAllowed={#MyArchAllowed}
ArchitecturesInstallIn64BitMode={#MyArchAllowed}
PrivilegesRequired=admin
CloseApplications=yes
CloseApplicationsFilter=*OpenPrintCast*

[Tasks]
Name: "firewall"; Description: "Add Windows Firewall rules for AirPrint / IPP (TCP 631 & UDP 5353)"; GroupDescription: "Network Configuration:"

[Files]
Source: "dist\OpenPrintCast-{#MyArch}.exe"; DestDir: "{app}"; DestName: "{#MyAppExeName}"; Flags: ignoreversion

[Run]
Filename: "netsh.exe"; Parameters: "advfirewall firewall delete rule name=""OpenPrintCast - IPP (TCP 631)"""; Flags: runhidden waituntilterminated; Tasks: firewall
Filename: "netsh.exe"; Parameters: "advfirewall firewall delete rule name=""OpenPrintCast - mDNS (UDP 5353)"""; Flags: runhidden waituntilterminated; Tasks: firewall
Filename: "netsh.exe"; Parameters: "advfirewall firewall add rule name=""OpenPrintCast - IPP (TCP 631)"" dir=in action=allow protocol=TCP localport=631 remoteip=localsubnet profile=any"; Flags: runhidden waituntilterminated; Tasks: firewall
Filename: "netsh.exe"; Parameters: "advfirewall firewall add rule name=""OpenPrintCast - mDNS (UDP 5353)"" dir=in action=allow protocol=UDP localport=5353 remoteip=localsubnet profile=any"; Flags: runhidden waituntilterminated; Tasks: firewall
Filename: "{app}\{#MyAppExeName}"; Description: "Launch {#MyAppName}"; Flags: nowait postinstall skipifsilent runasoriginaluser

[UninstallRun]
Filename: "{sys}\taskkill.exe"; Parameters: "/F /IM {#MyAppExeName} /T"; Flags: runhidden waituntilterminated; RunOnceId: "StopApp"
Filename: "netsh.exe"; Parameters: "advfirewall firewall delete rule name=""OpenPrintCast - IPP (TCP 631)"""; Flags: runhidden waituntilterminated; RunOnceId: "RemoveFirewallIPP"
Filename: "netsh.exe"; Parameters: "advfirewall firewall delete rule name=""OpenPrintCast - mDNS (UDP 5353)"""; Flags: runhidden waituntilterminated; RunOnceId: "RemoveFirewallMDNS"
Filename: "netsh.exe"; Parameters: "advfirewall firewall delete rule name=""OpenPrintCast (IPP)"""; Flags: runhidden waituntilterminated; RunOnceId: "RemoveAppFirewallIPP"
Filename: "netsh.exe"; Parameters: "advfirewall firewall delete rule name=""OpenPrintCast (mDNS)"""; Flags: runhidden waituntilterminated; RunOnceId: "RemoveAppFirewallMDNS"

[Code]
// Stops the running app (best effort; a no-op when it is not running).
procedure StopRunningApp();
var
  ResultCode: Integer;
begin
  Exec('taskkill.exe', '/F /IM OpenPrintCast.exe /T', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  // Same AppId: Inno Setup upgrades in place, so only the running app needs
  // to be out of the way before files are replaced.
  if CurStep = ssInstall then
    StopRunningApp();
end;

// Deletes the "Start with Windows" Run entry of the app (and Task Manager's
// on/off state for it) for the user running the uninstaller.
procedure RemoveOpenAtLoginValue(ValueName: String);
begin
  RegDeleteValue(HKCU, 'Software\Microsoft\Windows\CurrentVersion\Run', ValueName);
  RegDeleteValue(HKCU, 'Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run', ValueName);
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  // Keep the user's "Start with Windows" choice on silent (scripted or managed)
  // uninstalls, and clean up only on an interactive uninstall.
  if (CurUninstallStep = usUninstall) and not UninstallSilent() then
  begin
    // mygo names the entry after the package identifier (mygo build) or, in a
    // plain "go build", after the executable name without ".exe".
    RemoveOpenAtLoginValue('OpenPrintCast');
    RemoveOpenAtLoginValue('io.github.ilyasaftr.openprintcast');
  end;
end;
