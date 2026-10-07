//go:build windows

package kernel

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
)

type unverifiedManagedNetwork struct{ touched bool }

func (n *unverifiedManagedNetwork) Endpoint() string {
	n.touched = true
	return "http://127.0.0.1:18443"
}
func (n *unverifiedManagedNetwork) BindBrowser(func(net.Conn) bool) error {
	n.touched = true
	return nil
}
func (*unverifiedManagedNetwork) Close() error             { return nil }
func (*unverifiedManagedNetwork) Fault() *proxy.CheckError { return nil }
func (*unverifiedManagedNetwork) Failed() <-chan struct{}  { return nil }

func TestMissingSystemEgressBoundaryIsExplicitAndCannotBeRetriedIntoReady(t *testing.T) {
	var failure *Problem
	if err := RequireProxyNetworkBoundary(); !errors.As(err, &failure) || failure.Code != "NETWORK_PROTECTION_UNAVAILABLE" || failure.Retryable {
		t.Fatal("unimplemented isolation became available or a retryable connection problem")
	}
}
func TestRealManagedLauncherRejectsUnverifiedNetworkBeforeAnyNativeResource(t *testing.T) {
	network := &unverifiedManagedNetwork{}
	process, err := LaunchManagedProfile(context.Background(), t.TempDir(), Record{}, ManagedProfile{Network: network})
	var failure *Problem
	if process != nil || !errors.As(err, &failure) || failure.Code != "NETWORK_PROTECTION_UNAVAILABLE" || network.touched {
		t.Fatal("real launcher consumed private endpoints or created native resources before protection gate")
	}
}
