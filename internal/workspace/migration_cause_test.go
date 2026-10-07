//go:build windows

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

func TestMigrationRecoveryCausePreservesClassificationWithoutRawMessages(t *testing.T) {
	s, _, e, newID := migrationFixture(t)
	original := view(t, s).Fingerprints[e.ID]
	task := stagedMigrationFixture(t, s, e, newID)
	private := `SYNTHETIC_PRIVATE_EXCEPTION C:\private-profile\Cookies document.cookie=secret`
	problem := &kernel.Problem{Code: "KERNEL_INTEGRITY_FAILED", Reason: "migration-observation-failed", Message: private, Retryable: false}
	s.finishMigrationRecovery(context.Background(), task, fmt.Errorf("%s: %w", private, problem))
	result := value[Operation](t, call(s, "Operation.Read", map[string]string{"operationId": task.plan.ID}))
	want := map[string]any{"code": "KERNEL_INTEGRITY_FAILED", "reason": "migration-observation-failed", "retryable": false}
	if result.Error == nil || result.Error.Code != "MIGRATION_INCOMPLETE" || !reflect.DeepEqual(result.Error.Details["cause"], want) {
		t.Fatal("safe underlying classification was not retained", result.Error)
	}
	encoded, err := json.Marshal(result)
	if err != nil || strings.Contains(string(encoded), "SYNTHETIC_PRIVATE_EXCEPTION") || strings.Contains(string(encoded), "private-profile") || strings.Contains(string(encoded), "document.cookie") {
		t.Fatal("raw failure text entered the operation result")
	}
	plan, persisted, phase, err := s.readMigrationJournal(task.plan.ID)
	if err != nil || phase != "finished" || plan.Committed || result.State != "failed" || result.Stage != "original-retained" || result.PersistencePending || result.MigrationReport.Committed || result.MigrationReport.Protected || s.migrationTask != nil {
		t.Fatal("diagnostics changed the original-retained or protected contract", result, err)
	}
	if !reflect.DeepEqual(persisted.Error, result.Error) || !reflect.DeepEqual(view(t, s).Fingerprints[e.ID], original) {
		t.Fatal("classification was not durable or original identity changed")
	}
}

func TestMigrationRecoveryCauseUnknownAbsentAndResourceProtection(t *testing.T) {
	private := `SYNTHETIC_PRIVATE_EXCEPTION C:\private-profile\Cookies document.cookie=secret`
	for _, scenario := range []string{"unknown", "nil", "startup", "invalid-tags", "resources-unconfirmed"} {
		t.Run(scenario, func(t *testing.T) {
			s, _, e, newID := migrationFixture(t)
			original := view(t, s).Fingerprints[e.ID]
			task := stagedMigrationFixture(t, s, e, newID)
			var cause error = errors.New(private)
			var process *syntheticRuntimeProcess
			switch scenario {
			case "nil":
				cause = nil
			case "startup":
				// Startup compensation uses cancellation as control flow, not as
				// evidence of why the earlier process actually failed.
				task.startup, task.bootstrapReady = true, true
				cause = context.Canceled
			case "invalid-tags":
				cause = &kernel.Problem{Code: private, Reason: private, Message: private, Retryable: false}
			case "resources-unconfirmed":
				process = newSyntheticRuntimeProcess()
				task.process = process
			}
			s.finishMigrationRecovery(context.Background(), task, cause)
			result := value[Operation](t, call(s, "Operation.Read", map[string]string{"operationId": task.plan.ID}))
			encoded, err := json.Marshal(result)
			if err != nil || strings.Contains(string(encoded), "SYNTHETIC_PRIVATE_EXCEPTION") || strings.Contains(string(encoded), "private-profile") || strings.Contains(string(encoded), "document.cookie") {
				t.Fatal("raw or invalid classification text entered the result")
			}
			if scenario == "resources-unconfirmed" {
				if s.migrationTask != task || result.Stage != "protected" || !result.PersistencePending || !result.MigrationReport.Protected || result.MigrationReport.Committed {
					t.Fatal("adding diagnostics released an unconfirmed resource owner", result)
				}
				if err := process.Close(); err != nil {
					t.Fatal(err)
				}
				s.finishMigrationRecovery(context.Background(), task, nil)
			} else {
				if result.MigrationReport.Committed || result.MigrationReport.Protected || result.PersistencePending || s.migrationTask != nil {
					t.Fatal("diagnostics changed uncommitted recovery state", result)
				}
				if scenario == "nil" || scenario == "startup" {
					if result.Error == nil || result.Error.Details != nil {
						t.Fatal("recovery invented a cause without failure evidence", result.Error)
					}
				} else {
					classification, ok := result.Error.Details["cause"].(map[string]any)
					if !ok || classification["code"] != "MIGRATION_CAUSE_UNKNOWN" || classification["reason"] != "unclassified" {
						t.Fatal("unknown cause did not retain a safe generic classification", result.Error)
					}
				}
			}
			if !reflect.DeepEqual(view(t, s).Fingerprints[e.ID], original) {
				t.Fatal("original identity changed while recording a diagnostic cause")
			}
		})
	}
}
