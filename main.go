package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	demo := false
	for _, arg := range os.Args[1:] {
		if arg == "--demo" {
			demo = true
		}
	}
	app := NewApp(demo)
	err := wails.Run(&options.App{
		Title: "IDP Dashboard", Width: 1440, Height: 1000, MinWidth: 480, MinHeight: 600,
		BackgroundColour: &options.RGBA{R: 12, G: 17, B: 25, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets}, Bind: []interface{}{app},
		OnStartup: app.startup, OnShutdown: app.shutdown, OnDomReady: app.restoreWindow,
		OnBeforeClose: app.beforeClose,
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: fmt.Sprintf("idp-dashboard-%t", demo), OnSecondInstanceLaunch: func(options.SecondInstanceData) {
			if app.ctx != nil {
				runtime.WindowUnminimise(app.ctx)
				runtime.WindowShow(app.ctx)
				app.Refresh()
			}
		}},
		Windows: &windows.Options{OnResume: func() { app.Refresh() }},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "IDP Dashboard could not start:", err)
	}
}
