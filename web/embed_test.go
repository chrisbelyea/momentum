package web

import (
	"io/fs"
	"testing"
)

func TestEmbeddedAssetsPresent(t *testing.T) {
	for _, path := range []string{
		"templates/index.html",
		"templates/list.html",
		"templates/backends.html",
		"static/manifest.json",
		"static/sw.js",
		"static/icon.svg",
		"static/icon-192.png",
		"static/icon-512.png",
	} {
		if _, err := fs.Stat(Files, path); err != nil {
			t.Errorf("embedded asset %q is missing: %v", path, err)
		}
	}
}
