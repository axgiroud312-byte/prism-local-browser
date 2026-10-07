//go:build windows

package kernel

import (
	"encoding/binary"
	"net"
	"testing"
)

func TestTCPClientOwnerMatchesReversedTupleAndNetworkByteOrder(t *testing.T) {
	client := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 49152}
	server := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 18443}
	buffer := make([]byte, 4+2*24)
	binary.LittleEndian.PutUint32(buffer, 2)
	put := func(offset int, local, remote *net.TCPAddr, pid uint32) {
		row := buffer[offset : offset+24]
		binary.LittleEndian.PutUint32(row, 5)
		copy(row[4:8], local.IP.To4())
		binary.BigEndian.PutUint16(row[8:10], uint16(local.Port))
		copy(row[12:16], remote.IP.To4())
		binary.BigEndian.PutUint16(row[16:18], uint16(remote.Port))
		binary.LittleEndian.PutUint32(row[20:24], pid)
	}
	put(4, server, client, 8001)
	put(28, client, server, 8002)
	if pid, ok := ownerFromTCPTable(buffer, client, server); !ok || pid != 8002 {
		t.Fatal("server-side host PID was accepted as the browser caller")
	}
	put(4, client, server, 8003)
	if _, ok := ownerFromTCPTable(buffer, client, server); ok {
		t.Fatal("ambiguous caller snapshot was authorized")
	}
}
func TestTCPClientOwnerRejectsTruncatedNonEstablishedAndZeroPIDRows(t *testing.T) {
	client := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 49152}
	server := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 18443}
	buffer := make([]byte, 28)
	binary.LittleEndian.PutUint32(buffer, 2)
	if _, ok := ownerFromTCPTable(buffer, client, server); ok {
		t.Fatal("truncated TCP table was accepted")
	}
	binary.LittleEndian.PutUint32(buffer, 1)
	row := buffer[4:]
	copy(row[4:8], client.IP.To4())
	binary.BigEndian.PutUint16(row[8:10], uint16(client.Port))
	copy(row[12:16], server.IP.To4())
	binary.BigEndian.PutUint16(row[16:18], uint16(server.Port))
	binary.LittleEndian.PutUint32(row[20:24], 8002)
	if _, ok := ownerFromTCPTable(buffer, client, server); ok {
		t.Fatal("non-established socket was authorized")
	}
	binary.LittleEndian.PutUint32(row, 5)
	binary.LittleEndian.PutUint32(row[20:24], 0)
	if _, ok := ownerFromTCPTable(buffer, client, server); ok {
		t.Fatal("zero caller PID was accepted")
	}
}
