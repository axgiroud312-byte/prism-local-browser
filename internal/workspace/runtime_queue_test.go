package workspace

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

func TestRuntimeQueueFIFOStopInvalidatesWaitingItemAndRetryIsIndependent(t *testing.T) {
	var mu sync.Mutex
	order := []string{}
	attempts := map[string]int{}
	first := make(chan struct{})
	entered := make(chan struct{}, 1)
	var failing string
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(ctx context.Context, input RuntimeLaunch) (RuntimeProcess, error) {
		mu.Lock()
		order = append(order, input.EnvironmentID)
		attempts[input.EnvironmentID]++
		count, attempt := len(order), attempts[input.EnvironmentID]
		mu.Unlock()
		if count == 1 {
			entered <- struct{}{}
			select {
			case <-first:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if input.EnvironmentID == failing && attempt == 1 {
			return nil, &kernel.Problem{Code: "RESOURCE_EXHAUSTED", Message: "synthetic temporary resource exhaustion", Retryable: true}
		}
		return newSyntheticRuntimeProcess(), nil
	}})
	envs := []Environment{}
	for _, name := range []string{"queue A", "queue B cancelled", "queue C retry", "queue D"} {
		envs = append(envs, createRuntimeEnvironment(t, s, kernelID, name))
	}
	failing = envs[2].ID
	ops := []Operation{}
	for _, env := range envs {
		ops = append(ops, acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: env.ID, RequestID: id(), NetworkPolicy: "direct"}))
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("first queue item not admitted")
	}
	duplicate := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: envs[0].ID, RequestID: id(), NetworkPolicy: "direct"})
	if duplicate.ID != ops[0].ID {
		t.Fatal("duplicate received a second startup ticket")
	}
	stop := acceptRuntimeTest(t, s, "Runtime.Stop", runtimeRequest{EnvironmentID: envs[1].ID, RequestID: id()})
	if done := waitRuntimeReal(t, s, stop.ID); done.State != "completed" {
		t.Fatal("waiting item did not cancel immediately")
	}
	close(first)
	for index, op := range ops {
		done := waitRuntimeReal(t, s, op.ID)
		if index == 2 && (done.State != "failed" || done.Error.Code != "RESOURCE_EXHAUSTED") {
			t.Fatal("resource failure hidden")
		}
	}
	mu.Lock()
	got := append([]string(nil), order...)
	mu.Unlock()
	if !reflect.DeepEqual(got, []string{envs[0].ID, envs[2].ID, envs[3].ID}) {
		t.Fatal("startup order/cancellation mismatch", got)
	}
	for _, index := range []int{0, 3} {
		if view(t, s).RuntimeSessions[envs[index].ID].State != "running" {
			t.Fatal("startup slot imposed running quota")
		}
	}
	retry := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: envs[2].ID, RequestID: id(), NetworkPolicy: "direct"})
	if done := waitRuntimeReal(t, s, retry.ID); done.State != "completed" {
		t.Fatal("isolated retry failed")
	}
	mu.Lock()
	defer mu.Unlock()
	if attempts[envs[0].ID] != 1 || attempts[envs[1].ID] != 0 || attempts[envs[2].ID] != 2 || attempts[envs[3].ID] != 1 {
		t.Fatal("retry repeated successful/cancelled items")
	}
}

func TestRuntimeQueueCancellationAtReadinessCannotPublishLateSuccess(t *testing.T) {
	s, _, kernelID := fingerprintFixture(t, Options{})
	e := createRuntimeEnvironment(t, s, kernelID, "late readiness cancellation")
	_, revision, profileID, err := s.readEnvironment(e.ID)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := readProfileFrom(s.db, profileID)
	if err != nil {
		t.Fatal(err)
	}
	op := Operation{ID: id(), Kind: "runtime-start", State: "running", Stage: "verifying-and-starting", EnvironmentID: e.ID, SessionID: id(), KernelID: kernelID, Total: 1, CompletedIDs: []string{}}
	session := RuntimeSession{Mode: "native", EnvironmentID: e.ID, SessionID: op.SessionID, OperationID: op.ID, State: "starting", Revision: revision, FingerprintRevision: profile.Profile.ConfigRevision, KernelID: kernelID, UserDataRef: "environments/" + e.ID + "/user-data", NetworkPolicy: "direct", ResourceVersion: kernel.ManagedRuntimeVersion}
	if result := s.acceptRuntimeRecord("Runtime.Start", runtimeRequest{EnvironmentID: e.ID, NetworkPolicy: "direct", RequestID: id()}, op, &session); !result.OK {
		t.Fatal(result.Error)
	}
	slot := &runtimeSlot{session: session, start: op, cancel: func() {}, launchDone: make(chan struct{})}
	s.runtimeSlots[e.ID] = slot
	s.profileUses[e.ID] = true
	value[Operation](t, call(s, "Operation.Cancel", map[string]string{"operationId": op.ID}))
	process := newSyntheticRuntimeProcess()
	// Simulate a launcher whose final context check already passed; publication
	// must still honor cancellation accepted under the service lock afterwards.
	s.finishRuntimeStart(slot, process, nil, nil)
	close(slot.launchDone)
	if done := waitRuntimeReal(t, s, op.ID); done.State != "cancelled" {
		t.Fatal("late ready overwrote accepted cancellation", done.State)
	}
	if process.Alive() || process.closeCalls.Load() != 1 || view(t, s).RuntimeSessions[e.ID].State == "running" {
		t.Fatal("cancelled startup left a running browser")
	}
}
