package main

import (
	"context"
	"os"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
	bridgenet "github.com/ilyasaftr/OpenPrintCast/internal/net"
	"github.com/ilyasaftr/OpenPrintCast/internal/winspool"
)

// version is the bridge version reported at startup. Release builds stamp
// the tag in via -ldflags "-X main.version=..."; dev builds say "dev".
var version = "dev"

// bridgeApp is the UI-owned model: printer selection, job snapshots, the
// paused flag, render settings, and the window/tray handles. The IPP
// server thread never touches it directly; both sides meet in SharedState.
type bridgeApp struct {
	printers          []core.PrinterInfo
	tab               int
	settingsSection   string
	paused            bool
	printerQuery      string
	printerSearch     ui.Handle
	editingPrinter    string
	printerDialogOpen bool
	draftName         string
	draftLocation     string
	draftVirtual      bool

	reversePageOrder bool
	autoRotateToFit  bool
	maxRenderDPI     int

	jobsList ui.ListState

	// clock reads the time the Jobs table dates its rows against (nil:
	// the system clock); tests set it to a fixed moment.
	clock func() time.Time

	// failures counts jobs that failed while the Jobs page was not shown,
	// for the Jobs tab's badge (nil: no badge).
	failures unseenFailures

	shared   *bridgenet.SharedState
	canceler jobCanceler
	store    configStore
	cfg      core.AppConfig
	backend  winspool.Backend

	// printerStatus caches live printer conditions for the view; the
	// monitor polls off the UI thread, fed by printerNames. nil means no
	// status is known (the view then says "Status unknown").
	printerStatus *statusMonitor
	printerNames  nameSnapshot

	// printerCompat caches whether each shared printer's driver reports
	// what IPP/2.0 needs; its monitor checks off the UI thread, fed by
	// printerList. nil means nothing is known (the view then shows no
	// warning).
	printerCompat *compatibilityMonitor
	printerList   printerSnapshot

	// scans tracks Scan Again (created on first use); scanner and runScan
	// replace the spooler scan and its goroutine in tests (nil: the
	// production ones); emptyScanSeen remembers an empty scan answer.
	scans         *scanState
	scanner       printerScanner
	runScan       scanRunner
	emptyScanSeen bool

	// net is the reachability check and the firewall fix (created on first
	// use; see network).
	net *networkModel

	// notices queues messages for the UI thread to show as toasts; errors
	// found outside the view (a failed save) reach the user through it.
	notices noticeQueue

	// closeHint explains, once per run, that closing the window keeps the
	// app in the tray (its zero value gives no hint).
	closeHint trayHint

	// host and ip describe how other devices reach this PC; ip is cached
	// because finding it opens a socket (see lanIP).
	host string
	ip   string
	ipAt time.Time

	// logs controls the process logger and names the file the tray's Open
	// Log File item opens; openPath opens it. nil means no logger and the
	// system shell.
	logs     logControl
	openPath pathOpener

	// startupEntry is the sign-in item behind the Open at login switch
	// (nil: always off) and openAtLogin the state that switch shows.
	startupEntry startupItem
	openAtLogin  bool

	// announcer notifies about failed jobs from the server goroutine;
	// the settings switch only flips its enabled flag.
	announcer *jobFailureAnnouncer

	// notifyHealth remembers that the system refuses notifications; the
	// notifiers write it and Settings reads it (nil: notifications work).
	notifyHealth *notificationHealth

	// update checks for new versions (nil: update checking is not set up),
	// openURL opens the download page (nil: the system shell) and
	// revealUpdates asks the Settings page to show its Updates section
	// once, as the tray's Check for Updates does.
	update        updateService
	openURL       urlOpener
	revealUpdates bool

	// jobHistory records finished jobs across restarts; the Keep job
	// history switch and Clear Finished act on it (nil: no history).
	jobHistory jobHistory

	// raiseWindow replaces show when a tab is brought up, in tests (nil:
	// show).
	raiseWindow func()

	win      *mygo.Window
	tray     *mygo.Tray
	quitting bool
	cancel   context.CancelFunc
}

// newBridgeApp loads the config, seeds printers from the spooler, and
// builds the shared handoff for the server thread. logs is the running
// logger, which takes on the persisted verbosity.
func newBridgeApp(logs logControl) *bridgeApp {
	store := newFileConfigStore()
	cfg := store.Load()
	applyLoggingConfig(logs, cfg)

	host := bridgenet.HostLabel()
	backend := winspool.NewSystemBackend()
	backend.SetRenderSettings(renderSettingsFrom(cfg))
	applyReadSupplyLevels(backend, cfg)
	printers := discoverPrinters(backend, host, cfg.Printers)

	shared := bridgenet.NewSharedState(printers, &core.JobLog{})
	shared.SetSpooler(backend)
	history := attachJobHistory(shared.Jobs, jobHistoryPath(), cfg.KeepJobHistory)
	app := &bridgeApp{
		printers:         printers,
		tab:              tabFromName(cfg.LastTab),
		settingsSection:  sectionIDOrDefault(cfg.LastSettingsSection),
		paused:           shared.IsPaused(),
		reversePageOrder: cfg.ReversePageOrder,
		autoRotateToFit:  cfg.AutoRotateToFit,
		maxRenderDPI:     cfg.MaxRenderDPI,
		shared:           shared,
		canceler:         shared,
		store:            store,
		cfg:              cfg,
		backend:          backend,
		host:             host,
		logs:             logs,
		openPath:         shellOpenPath,
		startupEntry:     mygoStartupItem{},
		jobHistory:       history,
	}
	app.notifyHealth = &notificationHealth{}
	app.closeHint = trayHint{notifier: systemNotifier{onClick: app.show, health: app.notifyHealth}}
	app.update = newUpdateService(app, resolveUpdateSetup(version, updateFeedURL, os.Getenv))
	app.announcer = newJobFailureAnnouncer(systemNotifier{onClick: app.showJobs, health: app.notifyHealth}, shared, shared, cfg.NotifyOnFailure)
	shared.Jobs.Observe(app.announcer.OnJob)
	tally := &failureTally{changed: app.invalidateFromBackground}
	app.failures = tally
	shared.Jobs.Observe(tally.OnJob)
	app.printerStatus = newStatusMonitor(backend, app.invalidateFromBackground)
	app.printerCompat = newCompatibilityMonitor(sharedCompatibility{shared}, nil, app.invalidateFromBackground)
	app.setupNetworkCheck()
	app.persistPrinters()
	app.publishPrinters()
	componentLogger(componentApp).Info("dashboard ready", "printers", app.sharedPrinterCount())
	return app
}
