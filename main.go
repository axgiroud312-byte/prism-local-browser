//go:build windows

package main

import (
	"context"
	"embed"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/axgiroud312-byte/prism-local-browser/internal/desktopbase"
	"github.com/axgiroud312-byte/prism-local-browser/internal/workspace"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:dist
var frontend embed.FS

// Numeric PE version is 0.3.0.0; preview revision is recorded separately by the build.
var applicationVersion = "0.3.0-preview.1"

type DesktopApp struct{ service *workspace.Service }

func (app *DesktopApp) Call(request workspace.Request) workspace.Result {
	if app.service == nil {
		return workspace.Result{Mode: "native", Error: &workspace.Error{Code: "STORAGE_READ_FAILED", Message: "本地数据库无法打开，原数据未重置。请检查磁盘权限或数据库版本后重新打开。", Retryable: true}}
	}
	return app.service.Call(request)
}
func main() {
	if desktopbase.BindingGeneration {
		assets, _ := fs.Sub(frontend, "dist")
		_ = wails.Run(&options.App{AssetServer: &assetserver.Options{Assets: assets}, Bind: []interface{}{&DesktopApp{}}})
		return
	}
	if err := desktopbase.CheckRuntime(); err != nil {
		desktopbase.ShowError(err.Error())
		return
	}
	root := os.Getenv("PRISM_WORKSPACE_ROOT")
	if root == "" {
		var err error
		root, err = desktopbase.DefaultRoot()
		if err != nil {
			desktopbase.ShowError("无法读取 Windows 本机应用数据目录，未打开或修改工作区。")
			return
		}
	}
	lock, err := desktopbase.Acquire(root)
	if err != nil {
		desktopbase.ShowError(err.Error())
		return
	}
	defer lock.Close()
	if err := os.MkdirAll(filepath.Join(root, "workbench-webview"), 0700); err != nil {
		desktopbase.ShowError("工作台目录无法创建，未打开或修改配置。请检查磁盘权限。")
		return
	}
	release, err := desktopbase.PinDirectories(filepath.Join(root, "workbench-webview"))
	if err != nil {
		desktopbase.ShowError("工作区目录不能安全锁定，未打开配置。请检查目录链接或权限。")
		return
	}
	defer release()
	if err := desktopbase.ValidateTree(root); err != nil {
		desktopbase.ShowError("工作区包含目录链接或无法检查，未打开数据库。原数据未修改。")
		return
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
	err = wails.Run(&options.App{
		Title: "棱镜浏览器 · 开发预览 " + applicationVersion, Width: 1440, Height: 1000, MinWidth: 720, MinHeight: 600,
		AssetServer: &assetserver.Options{Assets: assets}, Bind: []interface{}{app},
		OnShutdown: func(context.Context) {
			if service != nil {
				service.Close()
			}
		},
		Windows:            &windows.Options{WebviewUserDataPath: filepath.Join(root, "workbench-webview"), WindowClassName: "PrismBrowserWorkspace"},
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: "cadb5081-585a-4e92-89c7-40c8ace49dc1"},
	})
	if err != nil {
		desktopbase.ShowError("桌面工作台未能打开。请检查 WebView2 与磁盘权限；已有档案未重置。")
	}
}
