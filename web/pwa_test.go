package web

import (
	"encoding/json"
	"image/png"
	"io/fs"
	"strings"
	"testing"
)

type manifestIcon struct {
	Src     string `json:"src"`
	Sizes   string `json:"sizes"`
	Type    string `json:"type"`
	Purpose string `json:"purpose"`
}

type manifest struct {
	Name       string         `json:"name"`
	ShortName  string         `json:"short_name"`
	StartURL   string         `json:"start_url"`
	Scope      string         `json:"scope"`
	Display    string         `json:"display"`
	ThemeColor string         `json:"theme_color"`
	Icons      []manifestIcon `json:"icons"`
}

func TestManifestHasInstallableBrandedIcons(t *testing.T) {
	raw, err := fs.ReadFile(Files, "static/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var got manifest
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}
	if got.Name != "Momentum" || got.ShortName != "Momentum" {
		t.Fatalf("unexpected app identity: %#v", got)
	}
	if got.StartURL != "/" || got.Scope != "/" || got.Display != "standalone" {
		t.Fatalf("manifest does not describe the application scope: %#v", got)
	}
	if len(got.Icons) < 2 {
		t.Fatalf("expected 192px and 512px icons, got %d", len(got.Icons))
	}
	seen := map[string]bool{}
	for _, icon := range got.Icons {
		seen[icon.Sizes] = true
		if icon.Type != "image/png" || !strings.HasPrefix(icon.Src, "/static/") {
			t.Errorf("icon is not a local PNG asset: %#v", icon)
		}
	}
	for _, size := range []string{"192x192", "512x512"} {
		if !seen[size] {
			t.Errorf("manifest is missing %s icon", size)
		}
	}
}

func TestEmbeddedPNGIconsHaveManifestDimensions(t *testing.T) {
	for _, asset := range []struct {
		path string
		want int
	}{
		{"static/icon-192.png", 192},
		{"static/icon-512.png", 512},
	} {
		file, err := Files.Open(asset.path)
		if err != nil {
			t.Fatal(err)
		}
		config, err := png.DecodeConfig(file)
		_ = file.Close()
		if err != nil {
			t.Fatalf("decode %s: %v", asset.path, err)
		}
		if config.Width != asset.want || config.Height != asset.want {
			t.Errorf("%s is %dx%d, want %dx%d", asset.path, config.Width, config.Height, asset.want, asset.want)
		}
	}
}

func TestServiceWorkerOnlyCachesStaticGETs(t *testing.T) {
	raw, err := fs.ReadFile(Files, "static/sw.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"request.method !== \"GET\"",
		"url.origin !== self.location.origin",
		"!url.pathname.startsWith(\"/static/\")",
		"url.pathname === \"/static/sw.js\"",
		"self.addEventListener(\"fetch\"",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("service worker lost static-only guard %q", required)
		}
	}
	// A navigation fallback or broad API cache would make cookie-authenticated
	// data persist offline. Keep this invariant obvious and mechanically tested.
	if strings.Contains(source, "caches.match(event.request)") || strings.Contains(source, "url.pathname.startsWith(\"/api") {
		t.Fatal("service worker must not cache navigations or API responses")
	}
}

func TestTemplatesRegisterRootServiceWorker(t *testing.T) {
	for _, path := range []string{"templates/index.html", "templates/list.html", "templates/backends.html"} {
		raw, err := fs.ReadFile(Files, path)
		if err != nil {
			t.Fatal(err)
		}
		source := string(raw)
		if !strings.Contains(source, "navigator.serviceWorker.register('/sw.js'") || !strings.Contains(source, "scope: '/'") {
			t.Errorf("%s does not register the root-scoped service worker", path)
		}
	}
}
