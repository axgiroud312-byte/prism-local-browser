//go:build windows

package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"

	"golang.org/x/sys/windows"
)

// Lock metadata is excluded from backups. A local purge must still authorize
// its exact object; only identity, length and digest enter the private journal.
type RetainedLock struct {
	Identity TreeIdentity `json:"identity"`
	Size     int64        `json:"size"`
	SHA256   string       `json:"sha256"`
}

func (p *Profile) RetainedLock(ctx context.Context) (*RetainedLock, error) {
	if p.lock == nil {
		return nil, nil
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(p.lock.Fd()), &info); err != nil {
		return nil, err
	}
	if _, err := p.lock.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	h := sha256.New()
	size, err := io.Copy(h, contextReader{ctx, p.lock})
	if err != nil {
		return nil, err
	}
	return &RetainedLock{Identity: treeIdentity(info), Size: size, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}
