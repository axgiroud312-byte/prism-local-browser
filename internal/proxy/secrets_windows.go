//go:build windows

package proxy

import (
	"errors"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

func dataBlob(value []byte) windows.DataBlob {
	if len(value) == 0 {
		return windows.DataBlob{}
	}
	return windows.DataBlob{Size: uint32(len(value)), Data: &value[0]}
}
func Wipe(value []byte) { clear(value); runtime.KeepAlive(value) }

// User scope only: no CRYPTPROTECT_LOCAL_MACHINE and no plaintext fallback.
// Entropy binds each ciphertext to its private, server-generated reference.
func Protect(reference string, plain []byte) ([]byte, error) {
	return transformSecret(reference, plain, false)
}
func Unprotect(reference string, protected []byte) ([]byte, error) {
	return transformSecret(reference, protected, true)
}
func transformSecret(reference string, input []byte, decrypt bool) ([]byte, error) {
	if reference == "" || len(input) == 0 || len(input) > 64<<10 {
		return nil, errors.New("invalid protected input")
	}
	entropy := []byte("prism-local-proxy-dpapi-v1:" + reference)
	in, salt, out := dataBlob(input), dataBlob(entropy), windows.DataBlob{}
	var err error
	if decrypt {
		err = windows.CryptUnprotectData(&in, nil, &salt, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	} else {
		err = windows.CryptProtectData(&in, nil, &salt, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	}
	runtime.KeepAlive(input)
	runtime.KeepAlive(entropy)
	if err != nil {
		return nil, errors.New("Windows user-protected credentials unavailable")
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	if out.Data == nil || out.Size == 0 || out.Size > 64<<10 {
		return nil, errors.New("invalid Windows protected output")
	}
	bytes := unsafe.Slice(out.Data, out.Size)
	defer Wipe(bytes)
	return append([]byte(nil), bytes...), nil
}
