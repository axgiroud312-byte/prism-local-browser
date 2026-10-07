//go:build windows

package main

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/desktopbase"
	"github.com/axgiroud312-byte/prism-local-browser/internal/desktopclose"
	"github.com/axgiroud312-byte/prism-local-browser/internal/workspace"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:dist
var frontend embed.FS

// Numeric PE version is 0.3.0.0; preview revision is recorded separately by the build.
var applicationVersion = "0.3.0-preview.1"
var applicationChannel = "development-preview"

type DesktopApp struct {
	service      *workspace.Service
	startupError *workspace.Error
	diagnostics  *workspace.DiagnosticHost
	shutdown     *desktopclose.Coordinator
}

func (app *DesktopApp) Call(request workspace.Request) workspace.Result {
	if app.shutdown != nil && app.shutdown.Closing() {
		return workspace.Result{Mode: "native", Error: &workspace.Error{Code: "NATIVE_UNAVAILABLE", Message: "工作区正在退出清理，未接受新操作。请在退出提示中再次清理；完成后重新打开应用。", Retryable: true}}
	}
	if app.diagnostics != nil && (request.Method == "Diagnostics.Preview" || request.Method == "Diagnostics.Export" || request.Method == "Diagnostics.EndVerification") {
		return app.diagnostics.Call(request)
	}
	if app.service == nil {
		if app.startupError != nil {
			return workspace.Result{Mode: "native", Error: app.startupError}
		}
		return workspace.Result{Mode: "native", Error: &workspace.Error{Code: "STORAGE_READ_FAILED", Message: "本地数据库无法打开，原数据未重置。请检查磁盘权限或数据库版本后重新打开。", Retryable: true}}
	}
	return app.service.Call(request)
}
func main() {
	if code := runDesktop(); code != 0 {
		os.Exit(code)
	}
}

func runDesktop() (exitCode int) {
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
	var desktopContext context.Context
	service, openErr := workspace.Open(root, workspace.Options{AppVersion: applicationVersion, ChooseArchive: func() (string, error) {
		if desktopContext == nil {
			return "", errors.New("desktop not ready")
		}
		return wailsruntime.OpenFileDialog(desktopContext, wailsruntime.OpenDialogOptions{Title: "选择可信 fingerprint-chromium ZIP", Filters: []wailsruntime.FileFilter{{DisplayName: "Windows内核ZIP", Pattern: "*.zip"}}})
	}, ChooseBackupSource: func() (string, error) {
		if desktopContext == nil {
			return "", errors.New("desktop not ready")
		}
		return wailsruntime.OpenFileDialog(desktopContext, wailsruntime.OpenDialogOptions{Title: "选择完整本机备份（先只读预检）", Filters: []wailsruntime.FileFilter{{DisplayName: "完整本机备份", Pattern: "*.prismbackup"}}})
	}, ChooseBackupDestination: func() (string, error) {
		if desktopContext == nil {
			return "", errors.New("desktop not ready")
		}
		return wailsruntime.SaveFileDialog(desktopContext, wailsruntime.SaveDialogOptions{Title: "导出完整本机备份（请选择新文件，不覆盖）", DefaultFilename: "prism-local-backup.prismbackup", Filters: []wailsruntime.FileFilter{{DisplayName: "完整本机备份", Pattern: "*.prismbackup"}}})
	}})
	shutdown := desktopclose.New(func(ctx context.Context) error {
		if service == nil {
			return nil
		}
		return service.CloseContext(ctx)
	}, shutdownFeedback, wailsruntime.Quit, 20*time.Second)
	defer func() {
		if err := shutdown.Finalize(); err != nil {
			exitCode = 1
			if !shutdown.AcknowledgedExit() {
				desktopbase.ShowError(shutdownMessage(err) + "\n\n本次退出没有报告为清理成功。请保留原工作区，处理占用或存储问题后重新打开核对。")
			}
		}
	}()
	assets, err := fs.Sub(frontend, "dist")
	if err != nil {
		return
	}
	app := &DesktopApp{service: service, shutdown: shutdown}
	var safeOpenError *workspace.Error
	if errors.As(openErr, &safeOpenError) {
		app.startupError = safeOpenError
	}
	if openErr != nil && app.startupError == nil {
		app.startupError = &workspace.Error{Code: "STORAGE_READ_FAILED", Message: "本地数据库无法打开，原数据未重置。", Retryable: true}
	}
	app.diagnostics = workspace.NewDiagnosticHost(root, applicationVersion, service, app.startupError, func() (string, error) {
		if desktopContext == nil {
			return "", errors.New("desktop not ready")
		}
		return wailsruntime.SaveFileDialog(desktopContext, wailsruntime.SaveDialogOptions{Title: "保存脱敏诊断（请选择新的 JSON 文件）", DefaultFilename: "prism-diagnostics.json", Filters: []wailsruntime.FileFilter{{DisplayName: "脱敏诊断 JSON", Pattern: "*.json"}}})
	})
	label := "开发预览"
	if applicationChannel == "v1-candidate" {
		label = "首版候选"
	}
	err = wails.Run(&options.App{
		Title: "棱镜浏览器 · " + label + " " + applicationVersion, Width: 1440, Height: 1000, MinWidth: 720, MinHeight: 600,
		AssetServer: &assetserver.Options{Assets: assets}, Bind: []interface{}{app},
		OnStartup:          func(ctx context.Context) { desktopContext = ctx },
		OnBeforeClose:      shutdown.BeforeClose,
		Windows:            &windows.Options{WebviewUserDataPath: filepath.Join(root, "workbench-webview"), WindowClassName: "PrismBrowserWorkspace"},
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: "cadb5081-585a-4e92-89c7-40c8ace49dc1"},
	})
	if err != nil {
		exitCode = 1
		desktopbase.ShowError("桌面工作台未能打开。请检查 WebView2 与磁盘权限；已有档案未重置。")
	}
	return
}

func shutdownMessage(err error) string {
	var failure *workspace.ShutdownError
	if errors.As(err, &failure) {
		switch failure.Code {
		case "EXIT_CLEANUP_PENDING":
			return "本程序的浏览器或后台任务尚未确认结束，正在继续清理。资源归属仍保留，尚不能退出。"
		case "EXIT_SCRATCH_PENDING":
			return "原恢复预检暂存仍被占用或无法清理。请释放该文件占用或修复权限，再清理原暂存；未释放原来源。"
		case "EXIT_STORAGE_PENDING":
			return "原操作受理或资源清理结果尚未核实、保存。请处理磁盘空间或读写权限后再次清理；原请求、结果和数据库仍保留供核实与重试。"
		case "EXIT_RECOVERY_REQUIRED":
			return "本程序的进程与后台任务已退出，但原恢复、回收或迁移任务仍受保护，需要保留原数据与日志并重新打开核对。任务没有报告为成功。"
		case "EXIT_CLOSE_FAILED":
			return "本程序的进程与后台任务已退出，但存储关闭返回错误。本会话已停止，不能在此继续保存或重新执行关闭；请保留原数据并退出后重新打开核对。首次关闭错误仍保留，没有报告清理成功。"
		}
	}
	return "退出清理尚未确认完成。请保留原数据，处理磁盘或资源占用后再次尝试。"
}

func shutdownFeedback(ctx context.Context, err error) desktopclose.Decision {
	var failure *workspace.ShutdownError
	if errors.As(err, &failure) && failure.CanExit {
		choice, dialogErr := wailsruntime.MessageDialog(ctx, wailsruntime.MessageDialogOptions{
			Type: wailsruntime.QuestionDialog, Title: "保留日志退出",
			Message:       shutdownMessage(err) + "\n\n是否保留原数据与日志并退出，以便重新打开核对？\n这次退出会记录为未完成清理，不会报告原任务成功。选择“否”继续保留窗口。",
			DefaultButton: "No",
		})
		if dialogErr == nil && choice == "Yes" {
			return desktopclose.ExitWithError
		}
		return desktopclose.KeepOpen
	}
	choice, dialogErr := wailsruntime.MessageDialog(ctx, wailsruntime.MessageDialogOptions{
		Type: wailsruntime.QuestionDialog, Title: "退出清理尚未完成",
		Message:       shutdownMessage(err) + "\n\n是否再次清理并退出？\n选择“是”重试原清理；选择“否”暂留窗口。暂留期间不接受新操作，处理问题后可再次点击关闭。",
		DefaultButton: "No",
	})
	if dialogErr != nil {
		desktopbase.ShowError(shutdownMessage(err) + "\n\n提示窗口未能显示；本程序继续保留，处理问题后请再次点击关闭。")
		return desktopclose.KeepOpen
	}
	if choice == "Yes" {
		return desktopclose.Retry
	}
	return desktopclose.KeepOpen
}
