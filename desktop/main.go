// Command checkfleet-desktop is a thin Wails GUI over the checkfleet engine.
// It reuses internal/engine, internal/registry and internal/output directly —
// the CLI stays the source of truth, the GUI is just another frontend (CF-15).
package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

// version is injected at build time with -ldflags "-X main.version=vX.Y.Z",
// matching the CLI release (CF-15: same version across binaries).
var version = "dev"

func main() {
	app := NewApp(version)

	err := wails.Run(&options.App{
		Title:            "checkfleet",
		Width:            1180,
		Height:           780,
		MinWidth:         820,
		MinHeight:        560,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 11, G: 17, B: 32, A: 1}, // slate-950, matches the UI
		OnStartup:        app.startup,
		Bind:             []any{app},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(),
			About: &mac.AboutInfo{
				Title:   "checkfleet",
				Message: "A fleet of domain-aware infrastructure checks.\nMIT — github.com/Allan-Nava/checkfleet",
			},
		},
	})
	if err != nil {
		println("checkfleet-desktop:", err.Error())
	}
}
