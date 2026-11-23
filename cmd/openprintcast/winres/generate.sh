#!/bin/sh
# Regenerates rsrc_windows_{amd64,arm64}.syso (exe icon and version strings)
# from winres.json and assets/icon.ico. go-winres is run pinned and is not a
# module dependency.
cd "$(dirname "$0")/.." && go run github.com/tc-hib/go-winres@v0.3.3 make --arch amd64,arm64
