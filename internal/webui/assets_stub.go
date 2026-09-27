//go:build !ui

package webui

import "io/fs"

var assets fs.FS

const embedded = false
