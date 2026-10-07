// Package desktopclose keeps the desktop window open until owned cleanup is
// confirmed. It does not own processes, storage, or recovery decisions.
package desktopclose

import (
	"context"
	"sync"
	"time"
)

type Decision int

const (
	KeepOpen Decision = iota
	Retry
	ExitWithError
)

type Coordinator struct {
	mu        sync.Mutex
	close     func(context.Context) error
	feedback  func(context.Context, error) Decision
	quit      func(context.Context)
	timeout   time.Duration
	requested bool
	active    bool
	complete  bool
	exitError error
}

func New(close func(context.Context) error, feedback func(context.Context, error) Decision, quit func(context.Context), timeout time.Duration) *Coordinator {
	return &Coordinator{close: close, feedback: feedback, quit: quit, timeout: timeout}
}

// BeforeClose returns immediately so the Windows message loop stays responsive.
// Repeated close requests share one cleanup attempt and one feedback dialog.
func (c *Coordinator) BeforeClose(ctx context.Context) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.complete || c.exitError != nil {
		return false
	}
	c.requested = true
	if !c.active {
		c.active = true
		go c.attempt(ctx)
	}
	return true
}

func (c *Coordinator) Closing() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.requested
}

func (c *Coordinator) AcknowledgedExit() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.exitError != nil
}

func (c *Coordinator) attempt(desktopContext context.Context) {
	for {
		// Cancellation of a Wails caller must not abandon the service's cleanup.
		ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
		err := c.close(ctx)
		cancel()
		if err == nil {
			c.mu.Lock()
			c.complete, c.active = true, false
			c.mu.Unlock()
			c.quit(desktopContext)
			return
		}
		switch c.feedback(desktopContext, err) {
		case Retry:
			continue
		case ExitWithError:
			c.mu.Lock()
			c.exitError, c.active = err, false
			c.mu.Unlock()
			c.quit(desktopContext)
			return
		default:
			c.mu.Lock()
			c.active = false
			c.mu.Unlock()
			return
		}
	}
}

// Finalize checks fallback exits, including failure before the window appears.
// An explicitly acknowledged incomplete exit must retain its non-success result.
func (c *Coordinator) Finalize() error {
	c.mu.Lock()
	complete, exitError := c.complete, c.exitError
	c.mu.Unlock()
	if complete || exitError != nil {
		return exitError
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	return c.close(ctx)
}
