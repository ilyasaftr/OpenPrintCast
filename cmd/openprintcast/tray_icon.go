package main

import "github.com/ilyasaftr/OpenPrintCast/assets"

// trayIcon returns the PNG for the tray: 32x32 pixels for a 16x16 point
// icon, as mygo's TrayOptions.Icon asks.
func trayIcon() []byte { return assets.TrayIcon }
