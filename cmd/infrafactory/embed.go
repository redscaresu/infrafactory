//go:build !noui

package main

import (
	"embed"
	"io/fs"
)

//go:embed all:ui/build
var uiBuild embed.FS

var uiAssets fs.FS = uiBuild
