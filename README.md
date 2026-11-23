<p align="center">
  <img src="assets/icon-256.png" alt="OpenPrintCast logo" width="128" height="128">
</p>

<h1 align="center">OpenPrintCast</h1>

<p align="center">
  Print from iPhone, iPad, Mac and Android to any printer on your Windows PC.
</p>

<p align="center">
  <img alt="License: GPL-3.0" src="https://img.shields.io/badge/license-GPL--3.0-blue">
  <img alt="Platform: Windows 10/11, amd64 and arm64" src="https://img.shields.io/badge/Windows-10%20%7C%2011%20%C2%B7%20amd64%20%7C%20arm64-0078d4">
  <img alt="Written in Go" src="https://img.shields.io/badge/Go-1.27-00add8">
</p>

OpenPrintCast is a small tray app that shares the printers on a Windows PC as
AirPrint and IPP Everywhere printers. iPhone, iPad, Mac and Android find them automatically;
there is nothing to install on the device.

## Features

- **Your existing printers.** Paper size, color, duplex, page ranges and copies,
  when the driver supports them.
- **Automatic discovery** over Bonjour on iOS, iPadOS, macOS and Android.
- **Print to Folder.** Save what you print as a PDF in a folder you choose.
- **Clear status.** Printer status, ink and toner levels, job history, cancel
  and failure details.
- **Reachability check.** Spots a blocked port or a Public network and fixes the
  firewall in one click.
- **Native and lightweight.** A single Windows executable written in Go, with a
  small memory footprint.

## Install

Download from [Releases](../../releases) and pick `amd64` (Intel, AMD) or
`arm64` (ARM, for example Snapdragon).

| File | Description |
| --- | --- |
| `OpenPrintCast-Setup-<version>-<arch>.exe` | Installer for all users, with an optional firewall rule. |
| `OpenPrintCast-<arch>.exe` | Standalone program, runs from any folder. |

## Use

1. Start OpenPrintCast. It runs in the tray; closing the window hides it.
2. On the **Printers** tab, switch on the printers to share.
3. On your device, pick the printer in the print dialog (on a Mac, add it in
   **Printers & Scanners**).

**Print to Folder:** choose a folder and switch it on. It appears on devices as
`Print to Folder (<computer name>)`, and each document is saved as a PDF named
after the job. Only page ranges apply.

## Supported documents

PDF, JPEG, PNG, Apple URF and PWG Raster, optionally `gzip` or `deflate`
compressed. Devices convert other formats before sending. IPP Everywhere is
declared for printers that report supply levels, checked by the CUPS `ipptool`
suites in CI (not PWG-certified).

## Troubleshooting

- **A device sees the printer but can't print.** Windows treats the network as
  Public, or the firewall blocks TCP 631. Use the banner on the Printers tab:
  **Open Network Settings** to switch to Private, or **Allow in Windows
  Firewall** (one UAC prompt, local subnet only). **Copy Diagnostics** gives you
  text to share when reporting a problem.
- **"iPhone, iPad and Mac can't print here".** The driver doesn't report paper
  sizes, orientation or resolution. Update it, then choose **Scan Again**.
- **Files.** Settings: `%APPDATA%\OpenPrintCast\config.json`. Logs and job
  history: `%LOCALAPPDATA%\OpenPrintCast` (tray menu > **Open Log File**).

## Privacy and security

Jobs are accepted over unencrypted IPP without authentication, so use a trusted
network. Supply levels are read from the printer's own address over read-only
SNMP. The update check sends one HTTPS request to this repository's releases
and never installs anything. SNMP and automatic update checks can both be turned
off in Settings.

## Build from source

```sh
go run ./cmd/openprintcast   # run
go test ./...                # test
```

CI checks amd64 and arm64. Pushing a `v*` tag builds a program and an installer
(`installer.iss`, Inno Setup) for each.

## Built with

- [MyGo](https://github.com/egoist/mygo) — the native UI, system tray and
  startup.
- [goipp](https://github.com/OpenPrinting/goipp) — the IPP wire protocol.
- [miekg/dns](https://github.com/miekg/dns) — DNS-SD and mDNS messages.
- [gosnmp](https://github.com/gosnmp/gosnmp) — read-only SNMP for supply
  levels.
- [go-pdfium](https://github.com/klippa-app/go-pdfium) and
  [wazero](https://github.com/tetratelabs/wazero) — PDFium running as
  WebAssembly for PDF work.

## License

[GPL-3.0](LICENSE)
