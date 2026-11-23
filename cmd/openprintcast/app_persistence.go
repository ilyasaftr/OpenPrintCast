package main

import (
	"github.com/ilyasaftr/OpenPrintCast/internal/core"
	"github.com/ilyasaftr/OpenPrintCast/internal/winspool"
)

// renderSettingsSetter is implemented by spooler backends that take
// render settings; other backends simply ignore them.
type renderSettingsSetter interface {
	SetRenderSettings(winspool.RenderSettings)
}

// supplyReadingSetter is implemented by spooler backends that can read
// supply levels from the printers; other backends ignore the setting.
type supplyReadingSetter interface {
	SetReadSupplyLevels(bool)
}

// applyReadSupplyLevels hands the Read ink and toner levels setting to the
// backend, which sends no SNMP traffic while it is off.
func applyReadSupplyLevels(backend winspool.Backend, cfg core.AppConfig) {
	if setter, ok := backend.(supplyReadingSetter); ok {
		setter.SetReadSupplyLevels(cfg.ReadSupplyLevels)
	}
}

// renderSettingsFrom maps the persisted render options to the backend's.
func renderSettingsFrom(cfg core.AppConfig) winspool.RenderSettings {
	return winspool.RenderSettings{
		MaxDPI: cfg.MaxRenderDPI, ReversePages: cfg.ReversePageOrder, AutoRotateToFit: cfg.AutoRotateToFit,
	}
}

// persistSettings writes the current render settings back to config.json.
func (a *bridgeApp) persistSettings() {
	a.cfg.ReversePageOrder = a.reversePageOrder
	a.cfg.AutoRotateToFit = a.autoRotateToFit
	a.cfg.MaxRenderDPI = a.maxRenderDPI
	if backend, ok := a.backend.(renderSettingsSetter); ok {
		backend.SetRenderSettings(renderSettingsFrom(a.cfg))
	}
	a.saveConfig("save config")
}

// notifyOnFailureChanged applies the failure-notification switch: it
// updates the announcer the server goroutine reads, then saves the config.
func (a *bridgeApp) notifyOnFailureChanged() {
	a.announcer.SetEnabled(a.cfg.NotifyOnFailure)
	a.persistSettings()
}

// readSupplyLevelsChanged applies the Read ink and toner levels switch to
// the backend at once (off stops all SNMP traffic and drops what was read),
// then saves the config.
func (a *bridgeApp) readSupplyLevelsChanged() {
	applyReadSupplyLevels(a.backend, a.cfg)
	a.persistSettings()
}

// verboseLoggingChanged applies the verbose-logging switch to the logger
// at once, then saves the config.
func (a *bridgeApp) verboseLoggingChanged() {
	a.logger().SetVerbose(a.cfg.VerboseLogging)
	a.persistSettings()
}

// persistPrinters writes the current per-printer customisation back to the
// config file so toggles and renames survive restarts. Printers are keyed
// by Windows name, so removed printers drop out and new ones appear once.
func (a *bridgeApp) persistPrinters() {
	entries := make([]core.PrinterEntry, 0, len(a.printers))
	for _, p := range a.printers {
		entries = append(entries, core.PrinterEntry{
			WindowsName:     p.WindowsName,
			AirPrintName:    p.AirPrintName,
			Location:        p.Location,
			Enabled:         p.Enabled,
			VirtualOverride: p.VirtualOverride,
		})
	}
	a.cfg.Printers = entries
	a.saveConfig("save printers")
}

// saveConfig writes the config file. A failure is logged under action and
// queued as a toast, since the page says changes apply immediately and the
// user would otherwise lose them unknowingly at the next start.
func (a *bridgeApp) saveConfig(action string) {
	if !a.trySaveConfig(action) {
		a.notices.PostError(saveFailedNotice)
	}
}

// trySaveConfig writes the config file and reports whether that worked. A
// failure is only logged, for writes the user did not ask for.
func (a *bridgeApp) trySaveConfig(action string) bool {
	if err := a.store.Save(a.cfg); err != nil {
		componentLogger(componentConfig).Error("save failed", "action", action, "err", err)
		return false
	}
	return true
}
