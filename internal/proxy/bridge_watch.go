package proxy

import "time"

// Resource scheduling, not a saved/running environment quota.
var bridgeWatchGate = make(chan struct{}, 4)

// Per-bridge lifecycle supervision is not an OS boundary against Chromium's
// other paths. A latched fault signals the exact Job owner, never revives an
// old endpoint, changes a saved proxy policy, or affects another bridge.
func (b *Bridge) failNetwork(failure *CheckError) {
	b.mu.Lock()
	if b.closing || b.browserGuard == nil || b.fatal != nil {
		b.mu.Unlock()
		return
	}
	copy := *failure
	b.fatal = &copy
	b.faultOnce.Do(func() { close(b.failed) })
	b.mu.Unlock()
	// Do not wait for workers while executing one of those same workers.
	go b.Close()
}

func (b *Bridge) watchNetwork() {
	defer b.monitors.Done()
	timer := time.NewTicker(30 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-b.ctx.Done():
			return
		case <-timer.C:
			select {
			case bridgeWatchGate <- struct{}{}:
			case <-b.ctx.Done():
				return
			}
			report := b.Preflight(b.ctx, nil)
			<-bridgeWatchGate
			if b.ctx.Err() != nil {
				return
			}
			if report.Error != nil {
				b.failNetwork(report.Error)
				return
			}
		}
	}
}
