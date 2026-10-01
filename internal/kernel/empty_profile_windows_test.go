//go:build windows

package kernel

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func syntheticEmptyOwner() EmptyProfileOwner {
	environment := uuid.NewString()
	return EmptyProfileOwner{EnvironmentID: environment, PlanID: uuid.NewString(), Index: 0, DataReference: "environments/" + environment + "/user-data"}
}

func TestEmptyProfileCreatesOwnedEmptyDirectoryAndCanResumeSameJournal(t *testing.T) {
	root := t.TempDir()
	owner := syntheticEmptyOwner()
	lease, err := PrepareEmptyProfile(root, owner)
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.CheckEmpty(); err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(root, filepath.FromSlash(owner.DataReference))
	entries, err := os.ReadDir(data)
	if err != nil || len(entries) != 0 {
		t.Fatal("new profile contains browsing data or an in-data lock")
	}
	marker, err := os.ReadFile(filepath.Join(filepath.Dir(data), ".prism-batch.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved EmptyProfileOwner
	if json.Unmarshal(marker, &saved) != nil || saved != owner {
		t.Fatal("durable owner marker does not match journal")
	}
	lease, err = PrepareEmptyProfile(root, owner)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if err = lease.CheckEmpty(); err != nil {
		t.Fatal(err)
	}
}

func TestEmptyProfileNeverClaimsUnmarkedForeignOrNonemptyDirectory(t *testing.T) {
	for _, kind := range []string{"unmarked", "foreign-owner", "nonempty"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			owner := syntheticEmptyOwner()
			data := filepath.Join(root, filepath.FromSlash(owner.DataReference))
			environmentRoot := filepath.Dir(data)
			if kind == "unmarked" {
				if err := os.MkdirAll(data, 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				lease, err := PrepareEmptyProfile(root, owner)
				if err != nil {
					t.Fatal(err)
				}
				lease.Close()
			}
			if kind == "foreign-owner" {
				owner.PlanID = uuid.NewString()
			}
			if kind == "nonempty" {
				if err := os.WriteFile(filepath.Join(data, "synthetic-login-marker"), []byte("synthetic-only"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			lease, err := PrepareEmptyProfile(root, owner)
			if lease != nil {
				lease.Close()
			}
			var problem *Problem
			if !errors.As(err, &problem) || problem.Code != "DATA_DIR_NOT_EMPTY" {
				t.Fatal("existing unknown/nonempty directory was adopted")
			}
			if _, err := os.Stat(environmentRoot); err != nil {
				t.Fatal("rejection deleted the original directory")
			}
		})
	}
}

func TestEmptyProfileRejectsClientPathOverridesAndKeepsOpaqueErrors(t *testing.T) {
	root := t.TempDir()
	owner := syntheticEmptyOwner()
	owner.DataReference = "environments/../outside/user-data"
	lease, err := PrepareEmptyProfile(root, owner)
	if lease != nil {
		lease.Close()
	}
	if err == nil {
		t.Fatal("noncanonical reference was repaired instead of rejected")
	}
	owner = syntheticEmptyOwner()
	owner.EnvironmentID = "not-a-managed-uuid"
	if lease, err := PrepareEmptyProfile(root, owner); err == nil || lease != nil {
		t.Fatal("unmanaged identity accepted")
	}
}
