package desktopclose

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func await(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal("desktop close coordination did not respond")
	}
}

func TestCloseRequestsWaitForOneConfirmedCleanup(t *testing.T) {
	started, release, quit := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	c := New(func(context.Context) error {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return nil
	}, func(context.Context, error) Decision { t.Error("successful cleanup showed failure"); return KeepOpen }, func(context.Context) { close(quit) }, time.Second)
	if !c.BeforeClose(context.Background()) {
		t.Fatal("window closed before cleanup started")
	}
	await(t, started)
	if !c.Closing() || !c.BeforeClose(context.Background()) || calls.Load() != 1 {
		t.Fatal("another close request abandoned or duplicated the original cleanup")
	}
	select {
	case <-quit:
		t.Fatal("window exited while original resources were retained")
	default:
	}
	close(release)
	await(t, quit)
	if c.BeforeClose(context.Background()) || c.Finalize() != nil || calls.Load() != 1 {
		t.Fatal("confirmed cleanup was not reused by final close")
	}
}

func TestFailureKeepsWindowAndRetryUsesOriginalClose(t *testing.T) {
	feedback, allowFeedback, quit := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	c := New(func(context.Context) error {
		if calls.Add(1) == 1 {
			return errors.New("synthetic retained resource")
		}
		return nil
	}, func(context.Context, error) Decision {
		close(feedback)
		<-allowFeedback
		return Retry
	}, func(context.Context) { close(quit) }, time.Second)
	c.BeforeClose(context.Background())
	await(t, feedback)
	if !c.BeforeClose(context.Background()) || calls.Load() != 1 {
		t.Fatal("close during failure feedback created a second attempt")
	}
	select {
	case <-quit:
		t.Fatal("failed cleanup silently exited")
	default:
	}
	close(allowFeedback)
	await(t, quit)
	if calls.Load() != 2 || c.Finalize() != nil {
		t.Fatal("explicit retry did not confirm the original cleanup")
	}
}

func TestKeepOpenRejectsBusinessAndLaterCloseRetries(t *testing.T) {
	feedback, quit := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	c := New(func(context.Context) error {
		if calls.Add(1) == 1 {
			return errors.New("synthetic occupied scratch")
		}
		return nil
	}, func(context.Context, error) Decision { close(feedback); return KeepOpen }, func(context.Context) { close(quit) }, time.Second)
	c.BeforeClose(context.Background())
	await(t, feedback)
	// Synchronize with the end of the feedback handler before the next close.
	deadline := time.Now().Add(time.Second)
	for {
		c.mu.Lock()
		active := c.active
		c.mu.Unlock()
		if !active {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("retained window never finished failure feedback")
		}
		time.Sleep(time.Millisecond)
	}
	if !c.Closing() || c.AcknowledgedExit() {
		t.Fatal("keeping the window reopened business or acknowledged an exit")
	}
	c.BeforeClose(context.Background())
	await(t, quit)
	if calls.Load() != 2 {
		t.Fatal("later window close did not retry retained cleanup")
	}
}

func TestAcknowledgedIncompleteExitRetainsError(t *testing.T) {
	failure, quit := errors.New("synthetic protected durable outcome"), make(chan struct{})
	var calls atomic.Int32
	c := New(func(context.Context) error { calls.Add(1); return failure }, func(context.Context, error) Decision { return ExitWithError }, func(context.Context) { close(quit) }, time.Second)
	c.BeforeClose(context.Background())
	await(t, quit)
	if !c.AcknowledgedExit() || c.BeforeClose(context.Background()) || !errors.Is(c.Finalize(), failure) || calls.Load() != 1 {
		t.Fatal("acknowledged incomplete exit became a successful cleanup")
	}
}

func TestFallbackExitChecksBoundedCleanup(t *testing.T) {
	c := New(func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }, func(context.Context, error) Decision { t.Error("fallback invoked desktop UI"); return KeepOpen }, func(context.Context) { t.Error("fallback quit invoked") }, time.Millisecond)
	if !errors.Is(c.Finalize(), context.DeadlineExceeded) {
		t.Fatal("fallback exit hid unconfirmed cleanup")
	}
}
