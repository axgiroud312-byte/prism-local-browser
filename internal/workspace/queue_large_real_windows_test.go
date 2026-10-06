//go:build windows

package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRealLocalAcceptanceQueue12FIFOActualDirectoryFailureAndIndependentRetry(t *testing.T) {
	f := openLocalAcceptanceFixture(t)
	const cancelledIndex, failingIndex = 5, 8
	environments := make([]Environment, 12)
	for index := range environments {
		environments[index] = f.environment(t, fmt.Sprintf("queue-%02d", index), false)
	}
	failingDirectory := filepath.Join(f.s.root, "environments", environments[failingIndex].ID, "user-data")
	if err := os.MkdirAll(failingDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	denial := denyOwnedACL(t, failingDirectory, ownedDirectoryAddFile)
	assertOwnedAccessDenied(t, failingDirectory, ownedDirectoryAddFile)
	barrier := &localAcceptanceCheckBarrier{entered: make(chan struct{}, 1), release: make(chan struct{})}
	f.checkBarrier.Store(barrier)
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(barrier.release) }) }
	t.Cleanup(release)
	operations := make([]Operation, len(environments))
	for index, environment := range environments {
		operations[index] = acceptRuntimeTest(t, f.s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, Purpose: "cookie-import", ExpectedRevision: preview(t, f.s, "edit", environment.ID).ExpectedRevision, NetworkPolicy: "proxy", RequestID: id()})
	}
	select {
	case <-barrier.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("real first upstream check did not reach the controlled queue barrier")
	}
	f.s.mu.Lock()
	queued := len(f.s.startQueue)
	f.s.mu.Unlock()
	if queued != 12 {
		t.Fatal("the twelve real start requests were not all queued before release")
	}
	stopping := acceptRuntimeTest(t, f.s, "Runtime.Stop", runtimeRequest{EnvironmentID: environments[cancelledIndex].ID, RequestID: id()})
	if done := waitLocalAcceptanceDurable(t, f.s, stopping.ID); done.State != "completed" {
		t.Fatal("waiting actual queue item cancellation did not complete")
	}
	release()
	previous := time.Time{}
	baseline := map[string]RuntimeSession{}
	var failureCode string
	for index, operation := range operations {
		done := waitLocalAcceptanceDurable(t, f.s, operation.ID)
		session := localAcceptanceSession(t, f.s, environments[index].ID)
		switch index {
		case cancelledIndex:
			if done.State != "cancelled" || session.PID != 0 {
				t.Fatal("cancelled waiting item launched an actual browser")
			}
		case failingIndex:
			if done.State != "failed" || done.Error == nil || session.PID != 0 || session.ResourcesPending {
				t.Fatal("actual directory permission failure was hidden or leaked its protected resources")
			}
			failureCode = done.Error.Code
		default:
			if done.State != "completed" || session.State != "running" || session.ProxyReport == nil || session.ProxyReport.Error != nil {
				t.Fatalf("real queue item %d: operation=%s/%s error=%v, session=%s error=%v fault=%v", index, done.State, done.Stage, done.Error, session.State, session.Error, session.NetworkFault)
			}
			created, err := time.Parse(time.RFC3339Nano, session.ProcessCreatedAt)
			if err != nil || !previous.IsZero() && !created.After(previous) {
				t.Fatal("actual Chromium root creation order was not FIFO")
			}
			previous = created
			assertRealRootAlive(t, session)
			baseline[environments[index].ID] = session
		}
	}
	if len(baseline) != 10 {
		t.Fatal("queue successes were limited by a running-instance quota")
	}
	if err := denial.restore(); err != nil {
		t.Fatal(err)
	}
	f.start(t, environments[failingIndex], true)
	for environmentID, before := range baseline {
		after := localAcceptanceSession(t, f.s, environmentID)
		if after.SessionID != before.SessionID || after.RootPID != before.RootPID || after.ProcessCreatedAt != before.ProcessCreatedAt {
			t.Fatal("retrying the one actual failure restarted a successful queue item")
		}
		assertRealRootAlive(t, after)
	}
	if localAcceptanceSession(t, f.s, environments[cancelledIndex].ID).PID != 0 {
		t.Fatal("single failure retry also restarted the cancelled queue item")
	}
	for index, environment := range environments {
		if index != cancelledIndex {
			f.stop(t, environment)
		}
	}
	writeLocalAcceptanceEvidence(t, "queue-12-real", map[string]any{"queuedBeforeFirstActualCheckReleased": 12, "actualProtectedRunningSuccessesBeforeRetry": 10, "cancelledWaiterNeverLaunched": true, "actualWin32DirectoryAccessDenialFailedOnlyOneItem": true, "failureCode": failureCode, "failedItemReleasedAllResources": true, "actualRootCreationOrderFIFO": true, "singleFailureRetryStartedOnlyThatEnvironment": true, "successesRetainedExactSessionsAndProcessesDuringRetry": true, "runningAfterIndependentRetry": 11, "allOwnedTreesNormallyStopped": true, "originalDACLRestoredAndCompared": true, "resourceBoundary": "actual directory access failure; not OS memory/process exhaustion"})
}
