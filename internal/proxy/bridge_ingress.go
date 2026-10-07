package proxy

import (
	"context"
	"errors"
	"net"
)

// BridgeIngress is a host-owned pair: a listener created with the session's
// socket identity and a probe dialer for that exact listener. Supplying this
// pair does not establish or certify the browser's OS network boundary.
// OpenBridge takes ownership of Listener only on success; the caller retains
// it on error. The container/token and directory grants belong to the session
// owner, not the Bridge, and must outlive every browser and bridge socket.
// ProbeDial must honor context cancellation and return only a connection to
// Listener; it must never retry using a different token or ordinary socket.
type BridgeIngress struct {
	Listener  net.Listener
	ProbeDial func(context.Context) (net.Conn, error)
}

func openBridgeIngress(supplied *BridgeIngress) (net.Listener, func(context.Context) (net.Conn, error), string, error) {
	if supplied == nil {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return nil, nil, "", err
		}
		return listener, nil, "http://" + listener.Addr().String(), nil
	}
	// Copy host launch material once. Later changes to the options container
	// cannot redirect an already checked bridge or remove its identity dialer.
	ingress := *supplied
	if ingress.Listener == nil || ingress.ProbeDial == nil {
		return nil, nil, "", errors.New("incomplete host ingress")
	}
	address, ok := ingress.Listener.Addr().(*net.TCPAddr)
	if !ok || address == nil || address.Port < 1 || address.Port > 65535 || address.Zone != "" || !address.IP.Equal(net.IPv4(127, 0, 0, 1)) {
		return nil, nil, "", errors.New("host ingress is not the private IPv4 loopback endpoint")
	}
	return ingress.Listener, ingress.ProbeDial, "http://" + address.String(), nil
}
