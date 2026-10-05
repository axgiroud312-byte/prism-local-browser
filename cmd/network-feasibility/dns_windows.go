//go:build windows

package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

type dnsAddress struct {
	Sockaddr [32]byte
	User     [8]uint32
}
type dnsAddressArray struct {
	MaxCount, Count, Tag               uint32
	Family, ReservedWord               uint16
	Flags, Match, Reserved1, Reserved2 uint32
	Addresses                          [1]dnsAddress
}
type dnsRequest struct {
	Version           uint32
	Name              *uint16
	Type              uint16
	Options           uint64
	Servers           *dnsAddressArray
	Interface         uint32
	Callback, Context uintptr
}
type dnsResult struct {
	Version, Status   uint32
	Options           uint64
	Records, Reserved uintptr
}

func queryDNS(name, endpoint string) uint32 {
	host, portString, err := net.SplitHostPort(endpoint)
	if err != nil {
		return 87
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		return 87
	}
	array := dnsAddressArray{Count: 1}
	array.MaxCount = 1
	address := &array.Addresses[0]
	binary.LittleEndian.PutUint16(address.Sockaddr[:2], windows.AF_INET)
	if port != 53 {
		return 87
	}
	copy(address.Sockaddr[4:8], net.ParseIP(host).To4())
	n, _ := windows.UTF16PtrFromString(name)
	// Bypass cache, wire only, no hosts file, fully-qualified. Server list is
	// per-call: no adapter/DNS/firewall setting is changed on this machine.
	req := dnsRequest{Version: 1, Name: n, Type: 1, Options: 0x8 | 0x100 | 0x40 | 0x1000, Servers: &array}
	res := dnsResult{Version: 1}
	dll := windows.NewLazySystemDLL("dnsapi.dll")
	status, _, _ := dll.NewProc("DnsQueryEx").Call(uintptr(unsafe.Pointer(&req)), uintptr(unsafe.Pointer(&res)), 0)
	if res.Records != 0 {
		dll.NewProc("DnsRecordListFree").Call(res.Records, 1)
	}
	return uint32(status)
}

type dnsObserver struct {
	conn   *net.UDPConn
	mu     sync.Mutex
	seen   map[string]int
	owners map[string]string
	done   chan struct{}
}

func newDNSObserver() (*dnsObserver, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 53})
	if err != nil {
		return nil, err
	}
	d := &dnsObserver{conn: conn, seen: map[string]int{}, owners: map[string]string{}, done: make(chan struct{})}
	go func() {
		defer close(d.done)
		buffer := make([]byte, 4096)
		for {
			n, peer, err := conn.ReadFromUDP(buffer)
			if err != nil {
				return
			}
			packet := append([]byte(nil), buffer[:n]...)
			name, end, err := dnsQuestion(packet)
			if err != nil {
				continue
			}
			d.mu.Lock()
			d.seen[name]++
			d.owners[name] = dnsSender(peer.Port)
			d.mu.Unlock()
			response := append([]byte(nil), packet[:end]...)
			binary.BigEndian.PutUint16(response[2:4], 0x8180)
			binary.BigEndian.PutUint16(response[6:8], 1)
			binary.BigEndian.PutUint16(response[8:10], 0)
			binary.BigEndian.PutUint16(response[10:12], 0)
			response = append(response, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 0, 0, 4, 203, 0, 113, 7)
			_, _ = conn.WriteToUDP(response, peer)
		}
	}()
	return d, nil
}
func dnsQuestion(packet []byte) (string, int, error) {
	if len(packet) < 17 || binary.BigEndian.Uint16(packet[4:6]) != 1 {
		return "", 0, fmt.Errorf("invalid question")
	}
	name := ""
	i := 12
	for i < len(packet) {
		n := int(packet[i])
		i++
		if n == 0 {
			if i+4 > len(packet) {
				break
			}
			return name, i + 4, nil
		}
		if n > 63 || i+n > len(packet) {
			break
		}
		if name != "" {
			name += "."
		}
		name += string(packet[i : i+n])
		i += n
	}
	return "", 0, fmt.Errorf("invalid name")
}
func (d *dnsObserver) count(name string) int { d.mu.Lock(); defer d.mu.Unlock(); return d.seen[name] }
func (d *dnsObserver) owner(name string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.owners[name]
}
func (d *dnsObserver) close() { d.conn.Close(); <-d.done }
