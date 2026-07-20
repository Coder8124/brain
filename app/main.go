package main

import (
	"embed"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func vaultPath() string {
	if v := os.Getenv("BRAIN_VAULT"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "brain-vault")
}

func main() {
	app := NewApp(vaultPath())

	err := wails.Run(&options.App{
		Title: "brain",
		// Panel-sized, not a window. It is a dropdown, invoked dozens of times
		// a day; it should feel like part of the menubar, not an application.
		Width:  420,
		Height: 640,
		// The Modern Dark direction: never pure black (OLED smear on scroll).
		BackgroundColour: &options.RGBA{R: 5, G: 6, B: 10, A: 255},
		Assets:           assets,
		OnStartup:        app.startup,
		Bind:             []any{app},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			About: &mac.AboutInfo{
				Title:   "brain",
				Message: "A local-first second brain.",
			},
		},
	})
	if err != nil {
		println("fatal:", err.Error())
	}
}
