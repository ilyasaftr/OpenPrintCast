package main

import (
	"errors"
	"reflect"
	"testing"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

func TestOpenLogOpensThePathCurrentAtClickTime(t *testing.T) {
	logs := &fakeLogControl{path: "/logs/2024-01-01.log"}
	opener := &openRecorder{}
	app := &bridgeApp{logs: logs, openPath: opener.open}

	app.openLog()
	logs.setPath("/logs/2024-01-02.log")
	app.openLog()

	want := []string{"/logs/2024-01-01.log", "/logs/2024-01-02.log"}
	if !reflect.DeepEqual(opener.paths, want) {
		t.Fatalf("opened %v, want %v", opener.paths, want)
	}
}

func TestOpenLogToleratesOpenerErrorAndMissingLog(t *testing.T) {
	opener := &openRecorder{err: errors.New("no handler")}
	app := &bridgeApp{logs: &fakeLogControl{path: "/logs/x.log"}, openPath: opener.open}
	app.openLog()
	if len(opener.paths) != 1 {
		t.Fatalf("opened %v, want one attempt", opener.paths)
	}

	none := &openRecorder{}
	(&bridgeApp{openPath: none.open}).openLog()
	if len(none.paths) != 0 {
		t.Fatalf("opened %v although there is no log file", none.paths)
	}
}

func TestTrayOpenLogItemReachesOpenLog(t *testing.T) {
	logs := &fakeLogControl{path: "/logs/a.log"}
	opener := &openRecorder{}
	app := &bridgeApp{logs: logs, openPath: opener.open}
	click(itemByLabel(t, trayItems(trayModel{}, app), trayOpenLogLabel))
	if !reflect.DeepEqual(opener.paths, []string{"/logs/a.log"}) {
		t.Fatalf("opened %v", opener.paths)
	}
}

func TestApplyLoggingConfigFollowsPersistedVerbosity(t *testing.T) {
	for _, verbose := range []bool{true, false} {
		logs := &fakeLogControl{}
		cfg := core.DefaultConfig()
		cfg.VerboseLogging = verbose
		applyLoggingConfig(logs, cfg)
		if !reflect.DeepEqual(logs.calls(), []bool{verbose}) {
			t.Fatalf("verbose=%t: SetVerbose calls %v", verbose, logs.calls())
		}
	}
}

func TestLoggerDefaultIsInert(t *testing.T) {
	app := &bridgeApp{}
	app.logger().SetVerbose(true)
	if app.logger().CurrentPath() != "" {
		t.Fatal("default logger should have no file")
	}
}
