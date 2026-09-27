//go:build ui

package webui

import (
	"embed"
	"io/fs"
)

// embeddedAssets is populated by the production/container build before compiling
// with -tags ui.
//
//go:embed dist/*
var embeddedAssets embed.FS

var assets fs.FS = embeddedAssets

const embedded = true
