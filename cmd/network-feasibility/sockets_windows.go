//go:build windows

package main

import (
	"io"
	"net"
	"sync"
	"time"
)

type socketObservation struct {
	Protocol string `json:"protocol"`
	Echo     bool   `json:"echo"`
	Received int    `json:"received"`
}
type socketObserver struct {
	protocol string
	address  string
	close    func() error
	mu       sync.Mutex
	packets  map[string]int
	done     chan struct{}
}

func startSocketObservers() ([]*socketObserver, error) {
	result := []*socketObserver{}
	for _, protocol := range []string{"tcp6", "udp4", "udp6"} {
		o := &socketObserver{protocol: protocol, packets: map[string]int{}, done: make(chan struct{})}
		address := "[::1]:0"
		if protocol == "udp4" {
			address = "127.0.0.1:0"
		}
		var err error
		if protocol == "tcp6" {
			var listener net.Listener
			listener, err = net.Listen(protocol, address)
			if err == nil {
				o.address = listener.Addr().String()
				o.close = listener.Close
				go func() {
					defer close(o.done)
					for {
						c, e := listener.Accept()
						if e != nil {
							return
						}
						_ = c.SetDeadline(time.Now().Add(2 * time.Second))
						packet := make([]byte, 256)
						n, e := c.Read(packet)
						if e == nil {
							o.record(string(packet[:n]))
							_, _ = c.Write(packet[:n])
						}
						c.Close()
					}
				}()
			}
		} else {
			var conn net.PacketConn
			conn, err = net.ListenPacket(protocol, address)
			if err == nil {
				o.address = conn.LocalAddr().String()
				o.close = conn.Close
				go func() {
					defer close(o.done)
					packet := make([]byte, 256)
					for {
						n, peer, e := conn.ReadFrom(packet)
						if e != nil {
							return
						}
						o.record(string(packet[:n]))
						_, _ = conn.WriteTo(packet[:n], peer)
					}
				}()
			}
		}
		if err != nil {
			for _, existing := range result {
				existing.close()
				<-existing.done
			}
			return nil, err
		}
		result = append(result, o)
	}
	return result, nil
}
func (o *socketObserver) record(marker string) { o.mu.Lock(); o.packets[marker]++; o.mu.Unlock() }
func (o *socketObserver) count(marker string) int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.packets[marker]
}
func socketEcho(protocol, address, marker string) bool {
	c, err := net.DialTimeout(protocol, address, 2*time.Second)
	if err != nil {
		return false
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err = io.WriteString(c, marker); err != nil {
		return false
	}
	buffer := make([]byte, len(marker))
	if _, err = io.ReadFull(c, buffer); err != nil {
		return false
	}
	return string(buffer) == marker
}
