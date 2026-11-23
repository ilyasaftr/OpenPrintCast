// Package assets embeds the application icon files that Go code needs at
// run time. The PNG and ICO files are rendered from the design in icon.svg.
package assets

import _ "embed"

// TrayIcon is the 32x32 PNG for the notification area icon: the simplified
// design, which stays legible when Windows scales it down to 16 px.
//
//go:embed icon-32.png
var TrayIcon []byte

// MarkSVG is the full icon design, for drawing the app mark in the window
// at any scale.
//
//go:embed icon.svg
var MarkSVG []byte
