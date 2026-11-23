# OpenPrintCast — native UI app

The `openprintcast` command is the mygo native-UI desktop shell: main
window with Printers / Jobs / Settings pages plus a system-tray icon (menu:
Open, Pause/Resume, Printers with a sharing checkbox each, Check for
Updates, Open Log File, Quit). It drives the `core`, `net`, `winspool` and
`updatecheck` packages under `internal/`. It is a per-user tray app, not a
Windows service.

## Prerequisites

`go.mod` pins the mygo release and lists the CLI as a tool (`tool
github.com/egoist/mygo/cmd/mygo`), so `go tool mygo` works without a separate
install.

`mygo.json` in this directory is the project configuration (`main: "."`).

## Develop and build

```sh
# from this directory
go tool mygo dev    # live-reload development app
go tool mygo build  # packaged builds, one directory per platform
```

`mygo build` writes one directory per platform (see `out` in the
[configuration docs](https://github.com/egoist/mygo/blob/main/docs/configuration.md)).
Releases use plain `go build -ldflags "-H windowsgui ..."` instead (see
`.github/workflows/release.yml`).

## Notes

- Builds are cgo-free: `CGO_ENABLED=0 go build ./...` from the repository
  root.
- Linux needs a display; headless CI can only compile-check.
- Only one instance runs: a second launch exits and brings the first
  window forward (`RequestSingleInstanceLock`).
- Settings persist to `os.UserConfigDir()/OpenPrintCast/config.json`
  (`%APPDATA%` on Windows), saved atomically with a `config.json.bak`
  backup that is used to recover a damaged file. A legacy `config.json`
  beside the executable is imported once. Closing the window hides it to
  the tray; Quit comes from the tray menu.
- Settings > Startup "Start with Windows" calls `mygo.App.SetOpenAtLogin`, a
  per-user `Run` entry on Windows; the app then starts hidden in the tray.
  The installer's uninstall step removes that entry (it knows the value
  names mygo uses: the package identifier or the executable name).
- Logs are structured (`log/slog`) in rolling files
  `%LOCALAPPDATA%\OpenPrintCast\openprintcast-YYYYMMDD.log` (`-2`,
  `-3` after 5 MiB; the 5 newest kept). Settings > Diagnostics "Verbose
  logging" lowers the level; the tray's Open Log File opens the current file.
