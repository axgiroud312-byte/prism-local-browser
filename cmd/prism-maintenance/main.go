//go:build windows

// Narrow installer helper: no arbitrary removal path, no account/registry changes.
package main

import (
	"os"
	"path/filepath"

	"github.com/axgiroud312-byte/prism-local-browser/internal/desktopbase"
)

func main() {
	if len(os.Args) != 2 {
		os.Exit(2)
	}
	switch os.Args[1] {
	case "--check-runtime":
		if desktopbase.CheckRuntime() != nil {
			os.Exit(20)
		}
	case "--validate-install":
		root, err := desktopbase.InstallationRoot()
		if err != nil || desktopbase.ValidateTree(root) != nil {
			os.Exit(21)
		}
	case "--validate-data":
		root, err := desktopbase.DefaultRoot()
		if err != nil || desktopbase.ValidateTree(root) != nil || desktopbase.ValidatePath(root+".maintenance.lock") != nil {
			os.Exit(21)
		}
	case "--remove-data":
		if desktopbase.RemoveDefaultWorkspace() != nil {
			os.Exit(23)
		}
	case "--publish-program":
		executable, err := os.Executable()
		if err != nil || desktopbase.PublishProgram(filepath.Dir(executable)) != nil {
			os.Exit(26)
		}
	case "--clean-program":
		if desktopbase.CleanProgram() != nil {
			os.Exit(27)
		}
	case "--finish-uninstall":
		if desktopbase.FinishUninstall() != nil {
			os.Exit(27)
		}
	default:
		os.Exit(2)
	}
}
