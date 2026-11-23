package main

import (
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

// folderSettingsChanged applies a change to the Print to Folder settings:
// it saves them, re-publishes the destination and refreshes the tray tip.
func (a *bridgeApp) folderSettingsChanged() {
	a.persistSettings()
	a.publishPrinters()
	a.syncTray()
}

// chooseFolder lets the user pick the folder for PDFs. The native dialog
// blocks, so it runs off the UI thread and the result is applied back on it.
func (a *bridgeApp) chooseFolder() {
	parent := a.win
	current := a.cfg.FolderPath
	go func() {
		path, err := pickFolder(parent, current)
		if err != nil {
			mygo.RunOnMain(a.folderDialogFailed)
			return
		}
		if path == "" {
			return
		}
		mygo.RunOnMain(func() {
			a.cfg.FolderPath = path
			a.folderSettingsChanged()
			a.invalidateWindow()
		})
	}()
}

// folderDialogFailed tells the user the folder picker could not open.
func (a *bridgeApp) folderDialogFailed() {
	a.notices.PostError(folderDialogNotice)
	a.invalidateWindow()
}

// pickFolder shows the folder dialog and returns the chosen folder. The
// path is empty when the user cancelled or the choice is not an existing
// absolute directory (which is reported to the user); err is set when the
// dialog itself failed.
func pickFolder(parent *mygo.Window, current string) (path string, err error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: parent, Title: "Choose a folder for PDFs", DefaultPath: current, Directory: true,
	})
	if err != nil {
		componentLogger(componentApp).Error("choose folder dialog failed", "err", err)
		return "", err
	}
	if len(paths) == 0 {
		return "", nil
	}
	path = paths[0]
	info, statErr := os.Stat(path)
	if statErr != nil || !info.IsDir() || !filepath.IsAbs(path) {
		mygo.Dialog.Error("Invalid folder", "Choose an existing folder with an absolute path.")
		return "", nil
	}
	return path, nil
}
