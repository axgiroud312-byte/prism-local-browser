//go:build windows

package kernel

import (
	"encoding/binary"
	"net"
	"os"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var tcpOwners = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetExtendedTcpTable")
var processInJob = windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")

// A QUERY-only, non-inheritable duplicate. Its lifetime is bounded by the
// same channel/process, never handed to a separate long-lived helper.
type proxyJobGuard struct {
	mu  sync.Mutex
	job windows.Handle
}

func newProxyJobGuard(job windows.Handle) (*proxyJobGuard, error) {
	if err := tcpOwners.Find(); err != nil {
		return nil, err
	}
	if err := processInJob.Find(); err != nil {
		return nil, err
	}
	var query windows.Handle
	if err := windows.DuplicateHandle(windows.CurrentProcess(), job, windows.CurrentProcess(), &query, 4 /* JOB_OBJECT_QUERY */, false, 0); err != nil {
		return nil, err
	}
	return &proxyJobGuard{job: query}, nil
}
func (g *proxyJobGuard) close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.job != 0 {
		_ = windows.CloseHandle(g.job)
		g.job = 0
	}
}
func (g *proxyJobGuard) allow(conn net.Conn) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.job == 0 {
		return false
	}
	pid, ok := tcpClientOwner(conn)
	if !ok {
		return false
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(process)
	var created, exited, kernelTime, userTime windows.Filetime
	if windows.GetProcessTimes(process, &created, &exited, &kernelTime, &userTime) != nil || created.Nanoseconds() <= 0 {
		return false
	}
	state, err := windows.WaitForSingleObject(process, 0)
	if err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		return false
	}
	var member int32
	result, _, _ := processInJob.Call(uintptr(process), uintptr(g.job), uintptr(unsafe.Pointer(&member)))
	if result == 0 || member == 0 {
		return false
	}
	// Recheck the exact connection, not an executable name or cached PID. The
	// open handle fixes this process object while both fresh snapshots are read.
	again, ok := tcpClientOwner(conn)
	if !ok || again != pid {
		return false
	}
	state, err = windows.WaitForSingleObject(process, 0)
	return err == nil && state == uint32(windows.WAIT_TIMEOUT)
}

func AuthorizeProxyProbe(conn net.Conn) bool {
	pid, ok := tcpClientOwner(conn)
	if !ok || pid != uint32(os.Getpid()) {
		return false
	}
	again, ok := tcpClientOwner(conn)
	return ok && again == pid
}

func tcpClientOwner(conn net.Conn) (uint32, bool) {
	local, a := conn.LocalAddr().(*net.TCPAddr)
	remote, b := conn.RemoteAddr().(*net.TCPAddr)
	if !a || !b || !local.IP.Equal(net.IPv4(127, 0, 0, 1)) || !remote.IP.Equal(net.IPv4(127, 0, 0, 1)) || local.Port < 1 || remote.Port < 1 {
		return 0, false
	}
	if tcpOwners.Find() != nil {
		return 0, false
	}
	var size uint32
	code, _, _ := tcpOwners.Call(0, uintptr(unsafe.Pointer(&size)), 0, windows.AF_INET, 5 /* TCP_TABLE_OWNER_PID_ALL */, 0)
	if code != uintptr(windows.ERROR_INSUFFICIENT_BUFFER) && code != 0 {
		return 0, false
	}
	for attempt := 0; attempt < 4; attempt++ {
		if size < 4 || size > 4<<20 {
			return 0, false
		}
		buffer := make([]byte, size)
		code, _, _ = tcpOwners.Call(uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)), 0, windows.AF_INET, 5, 0)
		runtime.KeepAlive(buffer)
		if code == uintptr(windows.ERROR_INSUFFICIENT_BUFFER) {
			continue
		}
		if code != 0 || size > uint32(len(buffer)) || size < 4 {
			return 0, false
		}
		return ownerFromTCPTable(buffer[:size], remote, local)
	}
	return 0, false
}

func ownerFromTCPTable(buffer []byte, client, server *net.TCPAddr) (uint32, bool) {
	if len(buffer) < 4 {
		return 0, false
	}
	count := binary.LittleEndian.Uint32(buffer[:4])
	if uint64(count) > uint64((len(buffer)-4)/24) {
		return 0, false
	}
	var owner uint32
	matches := 0
	for index := uint32(0); index < count; index++ {
		row := buffer[4+int(index)*24 : 4+int(index+1)*24]
		if binary.LittleEndian.Uint32(row[:4]) != 5 /* ESTABLISHED */ {
			continue
		}
		// IP memory and the low 16 port bits are network byte order. PID/count
		// are Windows little-endian DWORDs. Match the CLIENT's reversed tuple.
		if !net.IP(row[4:8]).Equal(client.IP) || int(binary.BigEndian.Uint16(row[8:10])) != client.Port || !net.IP(row[12:16]).Equal(server.IP) || int(binary.BigEndian.Uint16(row[16:18])) != server.Port {
			continue
		}
		owner = binary.LittleEndian.Uint32(row[20:24])
		matches++
	}
	return owner, matches == 1 && owner != 0
}
