//go:build windows

package main

import (
	"context"
	"embed"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/axgiroud312-byte/prism-local-browser/internal/workspace"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:dist
var frontend embed.FS

type DesktopApp struct{ service *workspace.Service }

func (app *DesktopApp) Call(request workspace.Request) workspace.Result {
	if app.service == nil {
		return workspace.Result{Mode: "native", Error: &workspace.Error{Code: "STORAGE_READ_FAILED", Message: "本地数据库无法打开，原数据未重置。请检查磁盘权限或数据库版本后重新打开。", Retryable: true}}
	}
	return app.service.Call(request)
}
func main() {
	root := os.Getenv("PRISM_WORKSPACE_ROOT")
	if root == "" {
		local, err := os.UserCacheDir()
		if err != nil {
			return
		}
		root = filepath.Join(local, "PrismBrowser")
	}
	service, _ := workspace.Open(root, workspace.Options{})
	if service != nil {
		defer service.Close()
	}
	assets, err := fs.Sub(frontend, "dist")
	if err != nil {
		return
	}
	app := &DesktopApp{service: service}
	_ = wails.Run(&options.App{
		Title: "棱镜浏览器 · 本机工作区", Width: 1440, Height: 1000, MinWidth: 720, MinHeight: 600,
		AssetServer: &assetserver.Options{Assets: assets}, Bind: []interface{}{app},
		OnShutdown: func(context.Context) {
			if service != nil {
				service.Close()
			}
		},
		Windows:            &windows.Options{WebviewUserDataPath: filepath.Join(root, "workbench-webview"), WindowClassName: "PrismBrowserWorkspace"},
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: "cadb5081-585a-4e92-89c7-40c8ace49dc1"},
	})
}
