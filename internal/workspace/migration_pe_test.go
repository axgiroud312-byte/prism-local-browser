//go:build windows

package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

// Resource-only amd64 PE fixture: no code section or executable entry point.
// This lets the REAL Windows version reader and file pins run in directory/
// transaction tests without launching a fake browser or bypassing production
// version checks. Capability evidence still comes from a labelled host seam.
func migrationVersionPE(version string) ([]byte, error) {
	parts := strings.Split(version, ".")
	if len(parts) != 4 {
		return nil, errors.New("synthetic version requires four parts")
	}
	v := [4]uint32{}
	for i, part := range parts {
		n, err := strconv.ParseUint(part, 10, 16)
		if err != nil {
			return nil, err
		}
		v[i] = uint32(n)
	}
	image := make([]byte, 0x400)
	u16 := func(offset int, value uint16) { binary.LittleEndian.PutUint16(image[offset:], value) }
	u32 := func(offset int, value uint32) { binary.LittleEndian.PutUint32(image[offset:], value) }
	u64 := func(offset int, value uint64) { binary.LittleEndian.PutUint64(image[offset:], value) }
	copy(image, "MZ")
	u32(0x3c, 0x80)
	copy(image[0x80:], "PE\x00\x00")
	u16(0x84, 0x8664)
	u16(0x86, 1)
	u16(0x94, 240)
	u16(0x96, 0x22)
	o := 0x98 // IMAGE_OPTIONAL_HEADER64
	u16(o, 0x20b)
	image[o+2] = 14
	u32(o+8, 0x200)
	u64(o+24, 0x140000000)
	u32(o+32, 0x1000)
	u32(o+36, 0x200)
	u16(o+40, 6)
	u16(o+48, 6)
	u32(o+56, 0x2000)
	u32(o+60, 0x200)
	u16(o+68, 3)
	u64(o+72, 0x100000)
	u64(o+80, 0x1000)
	u64(o+88, 0x100000)
	u64(o+96, 0x1000)
	u32(o+108, 16)
	u32(o+128, 0x1000)
	u32(o+132, 0x200)
	section := o + 240
	copy(image[section:], ".rsrc")
	u32(section+8, 0x200)
	u32(section+12, 0x1000)
	u32(section+16, 0x200)
	u32(section+20, 0x200)
	u32(section+36, 0x40000040)
	r := 0x200
	// Resource tree: RT_VERSION (16) -> resource 1 -> language 0x0409 -> data.
	u16(r+14, 1)
	u32(r+16, 16)
	u32(r+20, 0x80000018)
	u16(r+24+14, 1)
	u32(r+40, 1)
	u32(r+44, 0x80000030)
	u16(r+48+14, 1)
	u32(r+64, 0x409)
	u32(r+68, 72)
	u32(r+72, 0x1000+88)
	u32(r+76, 92)
	b := r + 88
	u16(b, 92)
	u16(b+2, 52)
	for i, c := range utf16.Encode([]rune("VS_VERSION_INFO\x00")) {
		u16(b+6+i*2, c)
	}
	f := b + 40 // DWORD-aligned VS_FIXEDFILEINFO, no string children needed.
	u32(f, 0xfeef04bd)
	u32(f+4, 0x10000)
	u32(f+8, v[0]<<16|v[1])
	u32(f+12, v[2]<<16|v[3])
	u32(f+16, v[0]<<16|v[1])
	u32(f+20, v[2]<<16|v[3])
	u32(f+24, 0x3f)
	u32(f+32, 0x40004)
	u32(f+36, 1)
	return image, nil
}

func migrationKernelPrepare(ctx context.Context, root string, input kernel.InstallInput, path string, probe kernel.ProbeFunc, progress kernel.ProgressFunc) (*kernel.Prepared, error) {
	p, err := syntheticKernelPrepare(ctx, root, input, path, probe, progress)
	if err != nil {
		return nil, err
	}
	image, err := migrationVersionPE(input.Version)
	if err != nil {
		return nil, err
	}
	executable := filepath.Join(p.Directory, "chrome.exe")
	if err = os.WriteFile(executable, image, 0600); err != nil {
		return nil, err
	}
	if actual, err := kernel.FileVersion(executable); err != nil || actual != input.Version {
		return nil, errors.New("synthetic PE resource did not round-trip through Windows version reader")
	}
	digest := sha256.Sum256(image)
	hash := hex.EncodeToString(digest[:])
	p.Record.ExecutableSHA256 = hash
	p.Record.Files = map[string]string{"chrome.exe": hash}
	return p, nil
}
