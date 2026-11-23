// Package main is the OpenPrintCast desktop app: a native-UI (mygo)
// window plus a system-tray icon driving the shared IPP/mDNS bridge.
//
// Startup order matters: the single-instance lock first, then file logging
// (applog.Init), then printer seeding from the winspool backend, then the
// SharedState handoff to the IPP server goroutine, and finally the mygo
// event loop. The UI owns the printer selection, the paused flag, and the
// render settings; the server thread only reads SharedState, so the two
// stay decoupled.
package main
