//go:build windows

package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/desktopclose"
	"github.com/axgiroud312-byte/prism-local-browser/internal/workspace"
)

func TestDesktopShutdownRejectsCallsBeforeServiceCleanupStarts(t *testing.T) {
	release, quit := make(chan struct{}), make(chan struct{})
	shutdown := desktopclose.New(func(context.Context) error { <-release; return nil }, func(context.Context, error) desktopclose.Decision { return desktopclose.KeepOpen }, func(context.Context) { close(quit) }, time.Second)
	app := &DesktopApp{shutdown: shutdown}
	if !shutdown.BeforeClose(context.Background()) {
		t.Fatal("window did not retain the original cleanup")
	}
	for _, method := range []string{"Workspace.Read", "Environment.Create", "Diagnostics.Export"} {
		result := app.Call(workspace.Request{Mode: "native", Method: method})
		if result.OK || result.Error == nil || result.Error.Code != "NATIVE_UNAVAILABLE" {
			t.Fatal("desktop accepted business while cleanup was starting", method, result)
		}
	}
	close(release)
	select {
	case <-quit:
	case <-time.After(time.Second):
		t.Fatal("confirmed cleanup did not release the window")
	}
}

func TestDesktopShutdownFeedbackDoesNotExposeRawErrors(t *testing.T) {
	private := `SYNTHETIC-PRIVATE-C:\profile\Cookie=secret`
	message := shutdownMessage(errors.New(private))
	if strings.Contains(message, private) || strings.Contains(message, "Cookie=") || message == "" {
		t.Fatal("shutdown feedback exposed the underlying storage or profile error")
	}
	if message := shutdownMessage(&workspace.ShutdownError{Code: "EXIT_CLOSE_FAILED"}); !strings.Contains(message, "不能在此继续保存") || !strings.Contains(message, "没有报告清理成功") {
		t.Fatal("irreversible storage closure was advertised as retryable or successful")
	}
}
