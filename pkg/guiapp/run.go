package guiapp

import (
	"io/fs"
	"net/http"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"poseavpn/ui/common"
)

// RunOptions describes one desktop app flavour.
type RunOptions struct {
	Mode      string // ModeSingle or ModeMulti
	Title     string
	UniqueID  string // single-instance lock id
	Width     int
	Height    int
	MinWidth  int
	MinHeight int
	Frontend  fs.FS // app specific files (index.html, app.js, app.css)
}

// Run starts the Wails window. Files missing from Frontend fall back to the
// shared assets (style.css, common.js).
func Run(o RunOptions) error {
	app := New(o.Mode)

	shared := http.FileServer(http.FS(common.Assets))

	return wails.Run(&options.App{
		Title:     o.Title,
		Width:     o.Width,
		Height:    o.Height,
		MinWidth:  o.MinWidth,
		MinHeight: o.MinHeight,
		AssetServer: &assetserver.Options{
			Assets:  o.Frontend,
			Handler: shared,
		},
		BackgroundColour: &options.RGBA{R: 10, G: 14, B: 26, A: 255},
		OnStartup:        app.Startup,
		OnShutdown:       app.Shutdown,
		Bind:             []interface{}{app},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: o.UniqueID,
			// Launching the app again (e.g. from the tray) brings the open window forward.
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				if app.ctx != nil {
					runtime.WindowUnminimise(app.ctx)
					runtime.Show(app.ctx)
				}
			},
		},
		Windows: &windows.Options{
			Theme:                windows.Dark,
			WebviewIsTransparent: false,
			DisableWindowIcon:    false,
		},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarDefault(),
			Appearance:           mac.NSAppearanceNameDarkAqua,
			WebviewIsTransparent: false,
			About: &mac.AboutInfo{
				Title:   o.Title,
				Message: "PoseaVPN v1.3.0 - Active Directory & Multi-Tunnel VPN Client",
			},
		},
	})
}
