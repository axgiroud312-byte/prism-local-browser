package workspace

import (
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

// These run the real Batch RPC/SQLite worker with a controlled directory seam.
// Faults and waits are automation evidence, not an actual Wails desktop run.
func TestBatchCloseoutOrdinaryCreateRejectsMultiItemWithoutPartialCommit(t *testing.T) {
	s, _ := fixture(t, Options{PrepareBatchDirectory: syntheticBatchPrepare})
	draft := preview(t, s, "create", "")
	configuration := draft.Environment.Configuration
	configuration.Name = "Closeout unsupported single Create count"
	wantError(t, call(s, "Environment.Create", Mutation{PreviewID: draft.PreviewID, Configuration: configuration, Count: 2, RequestID: id()}), "CAPABILITY_UNSUPPORTED")
	if len(view(t, s).State.Environments) != 0 {
		t.Fatal("single Create unexpectedly committed part of an unsupported multi-item request")
	}
}

func TestBatchCloseoutMixedFailureReportsExactRangeAndRetryNeverDuplicatesCommittedItems(t *testing.T) {
	var blocked atomic.Bool
	blocked.Store(true)
	var mu sync.Mutex
	var attempted []int64
	s, _ := fixture(t, Options{PrepareBatchDirectory: func(input BatchDirectoryInput) (BatchDirectoryLease, error) {
		mu.Lock()
		attempted = append(attempted, input.Index)
		mu.Unlock()
		if blocked.Load() {
			if input.Index == 1 {
				return nil, &kernel.Problem{Code: "PROFILE_DIRECTORY_UNSAFE", Message: "synthetic per-item directory refusal", Retryable: true}
			}
			if input.Index == 3 {
				return nil, &kernel.Problem{Code: "DISK_FULL", Message: "synthetic resource pause", Retryable: true}
			}
		}
		return syntheticBatchPrepare(input)
	}})
	page := createBatchPreviewFixture(t, s, "Closeout exact five", 5)
	requestID := id()
	first := acceptBatchFixture(t, s, page, requestID)
	failed := waitBatchFixture(t, s, first.ID)
	before := readBatchPageFixture(t, s, page.PlanID, 0)
	if failed.State != "failed" || failed.Error.Code != "DISK_FULL" || before.CompletedCount != 2 || before.FailedCount != 1 || before.NotExecutedCount != 2 {
		t.Fatal("mixed resource pause did not retain exact committed/failed/unexecuted counts")
	}
	wantStates := []string{"completed", "failed", "completed", "not-executed", "not-executed"}
	for index, item := range before.Items {
		if item.Index != int64(index) || item.State != wantStates[index] || item.Name != batchName("Closeout exact five", int64(index)) {
			t.Fatal("exact frozen scope was lost", index, item)
		}
	}
	if before.Items[1].Error == nil || before.Items[1].Error.Code != "PROFILE_DIRECTORY_UNSAFE" || before.Items[4].EnvironmentID != "" {
		t.Fatal("per-item failure or never-started suffix was misrepresented")
	}
	beforeView := view(t, s)
	identities := map[string]Environment{}
	for _, environment := range beforeView.State.Environments {
		identities[environment.ID] = environment
	}
	if len(identities) != 2 {
		t.Fatal("an uncommitted item became an environment")
	}
	mu.Lock()
	if !reflect.DeepEqual(attempted, []int64{0, 1, 2, 3}) {
		t.Fatal("worker executed beyond the resource pause", attempted)
	}
	mu.Unlock()
	blocked.Store(false)
	retryRequest := id()
	continued := value[struct {
		Operation Operation `json:"operation"`
	}](t, call(s, "Batch.Retry", map[string]string{"operationId": first.ID, "requestId": retryRequest})).Operation
	completed := waitBatchFixture(t, s, continued.ID)
	after := readBatchPageFixture(t, s, page.PlanID, 0)
	if completed.State != "completed" || after.CompletedCount != 5 || after.FailedCount != 0 || after.NotExecutedCount != 0 || after.AttemptCompletedCount != 3 {
		t.Fatal("continuation did not complete exactly the three unfinished items")
	}
	mu.Lock()
	if !reflect.DeepEqual(attempted, []int64{0, 1, 2, 3, 1, 3, 4}) {
		t.Fatal("retry repeated committed indexes or omitted unfinished ones", attempted)
	}
	mu.Unlock()
	seenIDs, seenSeeds := map[string]bool{}, map[string]bool{}
	afterView := view(t, s)
	for index, item := range after.Items {
		if item.State != "completed" || item.EnvironmentID == "" || seenIDs[item.EnvironmentID] || (before.Items[index].EnvironmentID != "" && before.Items[index].EnvironmentID != item.EnvironmentID) {
			t.Fatal("retry duplicated or changed a committed/prepared identity", item)
		}
		environment, _, _, err := s.readEnvironment(item.EnvironmentID)
		if err != nil || seenSeeds[environment.Seed] {
			t.Fatal("created identity/seed is missing or duplicated", err)
		}
		if original, exists := identities[environment.ID]; exists && (!reflect.DeepEqual(original, environment) || beforeView.DataReferences[environment.ID] != afterView.DataReferences[environment.ID]) {
			t.Fatal("continuation changed an already-created environment or data reference")
		}
		seenIDs[environment.ID], seenSeeds[environment.Seed] = true, true
	}
	if len(afterView.State.Environments) != 5 || len(seenIDs) != 5 {
		t.Fatal("retry created duplicate environments")
	}
	history := value[BatchPage](t, call(s, "Batch.ReadPage", map[string]any{"planId": page.PlanID, "operationId": first.ID, "offset": 0, "pageSize": 25}))
	for index, item := range history.Items {
		if item.State != wantStates[index] || item.Name != before.Items[index].Name {
			t.Fatal("old attempt was overwritten with later successes", index)
		}
		if index >= 3 && item.EnvironmentID != "" {
			t.Fatal("old unexecuted row leaked a later prepared identity")
		}
	}
	replayed := value[struct {
		Operation Operation `json:"operation"`
	}](t, call(s, "Batch.Retry", map[string]string{"operationId": first.ID, "requestId": retryRequest})).Operation
	if replayed.ID != continued.ID || acceptBatchFixture(t, s, page, requestID).ID != first.ID || len(view(t, s).State.Environments) != 5 {
		t.Fatal("original commit/retry request replay duplicated the plan")
	}
}

func TestBatchCloseoutCancelListsExactUnexecutedSuffixAndContinuesOriginalPreparedID(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var block atomic.Bool
	block.Store(true)
	var mu sync.Mutex
	var attempted []int64
	s, _ := fixture(t, Options{PrepareBatchDirectory: func(input BatchDirectoryInput) (BatchDirectoryLease, error) {
		mu.Lock()
		attempted = append(attempted, input.Index)
		mu.Unlock()
		if input.Index == 1 && block.Load() {
			close(entered)
			<-release
		}
		return syntheticBatchPrepare(input)
	}})
	page := createBatchPreviewFixture(t, s, "Closeout cancel four", 4)
	first := acceptBatchFixture(t, s, page, id())
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("controlled second preparation was not reached")
	}
	value[Operation](t, call(s, "Operation.Cancel", map[string]string{"operationId": first.ID}))
	close(release)
	cancelled := waitBatchFixture(t, s, first.ID)
	before := readBatchPageFixture(t, s, page.PlanID, 0)
	if cancelled.State != "cancelled" || before.CompletedCount != 1 || before.FailedCount != 0 || before.NotExecutedCount != 3 {
		t.Fatal("cancel reported attempted directory work as a committed environment")
	}
	for index, item := range before.Items {
		want := "not-executed"
		if index == 0 {
			want = "completed"
		}
		if item.Index != int64(index) || item.State != want || (index > 1 && item.EnvironmentID != "") {
			t.Fatal("cancel omitted or misreported an exact remaining item", item)
		}
	}
	if before.Items[1].EnvironmentID == "" || len(view(t, s).State.Environments) != 1 {
		t.Fatal("cancel lost prepared identity or created an uncommitted environment")
	}
	block.Store(false)
	continued := value[struct {
		Operation Operation `json:"operation"`
	}](t, call(s, "Batch.Retry", map[string]string{"operationId": first.ID, "requestId": id()})).Operation
	final := waitBatchFixture(t, s, continued.ID)
	after := readBatchPageFixture(t, s, page.PlanID, 0)
	if final.State != "completed" || after.CompletedCount != 4 || after.AttemptCompletedCount != 3 || after.NotExecutedCount != 0 || before.Items[0].EnvironmentID != after.Items[0].EnvironmentID || before.Items[1].EnvironmentID != after.Items[1].EnvironmentID || len(view(t, s).State.Environments) != 4 {
		t.Fatal("cancel continuation repeated committed work or changed the prepared identity")
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(attempted, []int64{0, 1, 1, 2, 3}) {
		t.Fatal("cancel/retry executed an unexpected index range", attempted)
	}
}
