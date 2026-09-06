// Package web contains the web UI assets served by Momentum.
//
// Keeping the assets in an embedded filesystem means the server does not
// depend on the process' working directory (or on a separately installed
// web/ directory) at runtime.
package web

import (
	"embed"
	"io/fs"
)

// Files contains the templates and static assets shipped with the server.
//
// Do not rename or move these directories without updating the embed patterns
// and the asset drift checks used by CI.
//
//go:embed templates/*.html static/*
var Files embed.FS

// StaticFiles returns the embedded filesystem rooted at web/static.
func StaticFiles() fs.FS {
	static, err := fs.Sub(Files, "static")
	if err != nil {
		// The path is part of this package's compile-time contract. A failure
		// here indicates a broken build, not a runtime configuration problem.
		panic(err)
	}
	return static
}
