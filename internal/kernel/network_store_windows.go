//go:build windows

package kernel

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"sync"

	"github.com/axgiroud312-byte/prism-local-browser/internal/desktopbase"
	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
	"golang.org/x/sys/windows"
)

// NetworkStore is workspace-private control state, not part of app.db exports.
// Its exclusive file handle prevents a second host recovering live resources.
type NetworkStore struct {
	root            string
	journal         *NetworkJournal
	owner, database windows.Handle
	release         func()
}

func OpenNetworkStore(root string) (*NetworkStore, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	release, err := EnsureDirectory(absolute, "network-resources")
	if err != nil {
		return nil, err
	}
	store := &NetworkStore{root: absolute, release: release}
	fail := func(err error) (*NetworkStore, error) { store.Close(); return nil, err }
	open := func(name string, share uint32) (windows.Handle, error) {
		path := filepath.Join(absolute, "network-resources", name)
		if err := desktopbase.ValidatePath(path); err != nil {
			return 0, err
		}
		wide, _ := windows.UTF16PtrFromString(path)
		h, err := windows.CreateFile(wide, windows.GENERIC_READ|windows.GENERIC_WRITE, share, nil, windows.OPEN_ALWAYS, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if err != nil {
			return 0, err
		}
		var info windows.ByHandleFileInformation
		if err = windows.GetFileInformationByHandle(h, &info); err != nil || info.NumberOfLinks != 1 || info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 {
			windows.CloseHandle(h)
			return 0, errors.New("unsafe network journal file")
		}
		return h, nil
	}
	if store.owner, err = open("owner.lock", 0); err != nil {
		return fail(err)
	}
	if store.database, err = open("resources.db", windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE); err != nil {
		return fail(err)
	}
	for _, suffix := range []string{"-journal", "-wal", "-shm"} {
		if err = desktopbase.ValidatePath(filepath.Join(absolute, "network-resources", "resources.db"+suffix)); err != nil {
			return fail(err)
		}
	}
	store.journal, err = OpenNetworkJournal(filepath.Join(absolute, "network-resources", "resources.db"))
	if err != nil {
		return fail(err)
	}
	return store, nil
}

func (s *NetworkStore) Close() error {
	var err error
	if s.journal != nil {
		err = s.journal.Close()
		s.journal = nil
	}
	if s.database != 0 {
		err = errors.Join(err, windows.CloseHandle(s.database))
		s.database = 0
	}
	if s.owner != 0 {
		err = errors.Join(err, windows.CloseHandle(s.owner))
		s.owner = 0
	}
	if s.release != nil {
		s.release()
		s.release = nil
	}
	return err
}

func (s *NetworkStore) Pending(ctx context.Context) (map[string]NetworkSessionIntent, error) {
	result := map[string]NetworkSessionIntent{}
	after := ""
	for {
		page, err := s.journal.PendingSessions(ctx, after)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			return result, nil
		}
		for _, intent := range page {
			if _, exists := result[intent.EnvironmentID]; exists {
				return nil, ErrNetworkJournalConflict
			}
			result[intent.EnvironmentID] = intent
			after = intent.SessionID
		}
	}
}

func (s *NetworkStore) Recover(ctx context.Context, intent NetworkSessionIntent) error {
	saved, err := s.journal.Session(ctx, intent.SessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	} // intent commit failed before any OS action
	if err != nil {
		return err
	}
	if saved != intent {
		return ErrNetworkJournalConflict
	}
	if !validNetworkSession(intent) || intent.DataReference != "environments/"+intent.EnvironmentID+"/user-data" || intent.JobName != `Global\PrismManagedSession-`+intent.SessionID {
		return ErrNetworkJournalConflict
	}
	return s.journal.Cleanup(ctx, intent.SessionID, func(context.Context) error {
		exited, err := managedJobResourcesExited(intent.SessionID)
		if err != nil || !exited {
			return errors.New("network session process tree not confirmed empty")
		}
		return nil
	}, func(ctx context.Context, resource NetworkResourceRecord) error { return s.undo(ctx, intent, resource) })
}

func (s *NetworkStore) undo(ctx context.Context, intent NetworkSessionIntent, resource NetworkResourceRecord) (resultErr error) {
	defer func() {
		if resultErr != nil {
			resultErr = fmt.Errorf("recover-%s: %w", resource.Kind, resultErr)
		}
	}()
	switch resource.Kind {
	case "job", "bridge":
		// The application lock excludes an old owner; exact Job empty was
		// verified above. Never reconnect or bind an old socket address.
		if resource.Kind == "job" && (resource.ObjectIdentity != intent.JobName || resource.Locator != intent.JobName) {
			return ErrNetworkJournalConflict
		}
		if resource.Kind == "bridge" && (resource.ObjectIdentity != intent.ChannelID || resource.Locator != intent.ChannelID) {
			return ErrNetworkJournalConflict
		}
		return nil
	case "container":
		if intent.Mode != "" {
			return ErrNetworkJournalConflict
		}
		if resource.ObjectIdentity != intent.PackageSID || resource.Locator != intent.ContainerName {
			return ErrNetworkJournalConflict
		}
		return deleteNetworkContainer(intent)
	case "file-acl":
		if intent.Mode != "" {
			return ErrNetworkJournalConflict
		}
		var delta networkACLDelta
		if json.Unmarshal([]byte(resource.Delta), &delta) != nil || delta.SID != intent.PackageSID || !delta.Tree {
			return ErrNetworkJournalConflict
		}
		relative, err := filepath.Rel(s.root, resource.Locator)
		if err != nil || relative == "." || strings.HasPrefix(relative, "..") || filepath.IsAbs(relative) {
			return ErrNetworkJournalConflict
		}
		// Only this environment's data tree or an installed kernel tree.
		normalized := filepath.ToSlash(relative)
		if normalized != intent.DataReference && !strings.HasPrefix(normalized, "kernels/") {
			return ErrNetworkJournalConflict
		}
		return applyNetworkTree(ctx, resource.Locator, resource.ObjectIdentity, delta, false)
	case "window-station-acl":
		if intent.Mode != "" {
			return ErrNetworkJournalConflict
		}
		if resource.Delta != intent.PackageSID || resource.ObjectIdentity != intent.LogonSID+":"+resource.Locator {
			return ErrNetworkJournalConflict
		}
		if intent.BootID != "" {
			boot, err := networkBootIdentity()
			if err != nil {
				return err
			}
			if boot != intent.BootID {
				return nil
			}
		}
		logon, name, err := networkLogonIdentity()
		if err != nil {
			return err
		}
		if logon != intent.LogonSID {
			ended, err := networkLogonEnded(intent.LogonSID)
			if err != nil {
				return err
			}
			if ended {
				return nil
			} // old transient USER objects died with that logon
			return errors.New("original window station logon still exists")
		}
		if resource.Locator != name || resource.ObjectIdentity != logon+":"+name {
			return errors.New("original window station logon unavailable")
		}
		h, err := openNetworkStation(name, false)
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			return nil
		}
		if err != nil {
			return err
		}
		defer func() { resultErr = errors.Join(resultErr, closeNetworkStation(h)) }()
		unlock, err := lockNetworkACL(resource.ObjectIdentity)
		if err != nil {
			return err
		}
		defer unlock()
		sid, err := windows.StringToSid(intent.PackageSID)
		if err != nil {
			return err
		}
		return changeNetworkACL(h, windows.SE_WINDOW_OBJECT, sid, windows.GENERIC_READ|8, 0, false)
	}
	return ErrNetworkJournalConflict
}

type ProtectedProxy struct {
	store        *NetworkStore
	intent       NetworkSessionIntent
	sid          *windows.SID
	token        windows.Token
	station      windows.Handle
	bridge       *proxy.Bridge
	listener     net.Listener
	lock         *managedProfileLock
	releaseFiles func()
	record       Record
	mu           sync.Mutex
	closed       bool
	preflighted  bool
	failed       chan struct{}
}

// OpenProtectedProxy returns its owner even on partial failure. The caller must
// retain and close it; a failed cleanup is still a busy environment, including
// failures before a browser PID exists.
func (s *NetworkStore) OpenProtectedProxy(ctx context.Context, record Record, profile ManagedProfile, channelID string, config proxy.Configuration, credentials *proxy.Credentials, check proxy.CheckOptions) (p *ProtectedProxy, err error) {
	return s.openProtectedProxy(ctx, record, profile, channelID, config, credentials, proxy.BridgeOptions{TargetURL: check.TargetURL, RootCAs: check.RootCAs})
}

func (s *NetworkStore) openProtectedProxy(ctx context.Context, record Record, profile ManagedProfile, channelID string, config proxy.Configuration, credentials *proxy.Credentials, bridgeOptions proxy.BridgeOptions) (p *ProtectedProxy, err error) {
	stage := "kernel-evidence"
	defer func() {
		if err != nil {
			err = fmt.Errorf("protected-%s: %w", stage, err)
		}
	}()
	if err = CheckRecord(record); err != nil {
		return nil, err
	}
	intent := NetworkSessionIntent{Mode: proxyBridgeSessionMode, SessionID: profile.SessionID, EnvironmentID: profile.EnvironmentID, ChannelID: channelID, DataReference: profile.UserDataRef, JobName: `Global\PrismManagedSession-` + profile.SessionID}
	p = &ProtectedProxy{store: s, intent: intent, record: record, failed: make(chan struct{})}
	stage = "session-intent"
	if err = s.journal.PrepareSession(ctx, intent); err != nil {
		return p, err
	}
	stage = "profile-lock"
	if p.lock, err = lockManagedProfile(s.root, profile.EnvironmentID, profile.UserDataRef); err != nil {
		return p, err
	}
	directory, err := RecordDirectory(s.root, record)
	if err != nil {
		return p, err
	}
	stage = "kernel-files"
	if p.releaseFiles, err = PinFiles(directory, record.Files); err != nil {
		return p, err
	}
	if err = VerifyFiles(directory, record.Files); err != nil {
		return p, err
	}
	actualVersion, versionErr := FileVersion(filepath.Join(directory, filepath.FromSlash(record.ExecutableRelativePath)))
	if versionErr != nil || actualVersion != record.Version {
		return p, problem("KERNEL_INTEGRITY_FAILED", "version-mismatch", "固定内核文件版本不匹配，尚未建立代理通道或启动浏览器。")
	}
	resource := NetworkResourceIntent{ResourceID: "bridge", Kind: "bridge", ObjectIdentity: channelID, Locator: channelID}
	stage = "identity-bridge"
	if err = s.journal.ApplyResource(ctx, intent.SessionID, resource, func(context.Context) error {
		bridgeOptions.ChannelID, bridgeOptions.AuthorizeProbe, bridgeOptions.Ingress = channelID, AuthorizeProxyProbe, nil
		p.bridge, err = proxy.OpenBridge(config, credentials, bridgeOptions)
		return err
	}); err != nil {
		return p, err
	}
	return p, nil
}

func (p *ProtectedProxy) ID() string { return p.intent.ChannelID }
func (p *ProtectedProxy) Endpoint() string {
	if p.bridge == nil {
		return ""
	}
	return p.bridge.Endpoint()
}
func (p *ProtectedProxy) BindBrowser(allow func(net.Conn) bool) error {
	if p.bridge == nil {
		return RequireProxyNetworkBoundary()
	}
	return p.bridge.BindBrowser(allow)
}
func (p *ProtectedProxy) Failed() <-chan struct{} {
	if p.bridge == nil {
		return p.failed
	}
	return p.bridge.Failed()
}
func (p *ProtectedProxy) Fault() *proxy.CheckError {
	if p.bridge == nil {
		return nil
	}
	return p.bridge.Fault()
}
func (p *ProtectedProxy) Preflight(ctx context.Context, progress func(proxy.Step)) proxy.Report {
	report := p.bridge.Preflight(ctx, progress)
	p.mu.Lock()
	p.preflighted = report.Error == nil && report.Mode == "native" && report.ChannelID == p.intent.ChannelID && report.ExitIP != ""
	p.mu.Unlock()
	return report
}

func (p *ProtectedProxy) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.preflighted = false
	if p.bridge != nil {
		if err := p.bridge.Close(); err != nil {
			return err
		}
	}
	if p.listener != nil {
		if err := p.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			return err
		}
		p.listener = nil
	}
	// Bridge.Close has joined all private probe dials before token disposal.
	if p.token != 0 {
		if err := p.token.Close(); err != nil {
			return err
		}
		p.token = 0
	}
	if err := p.store.Recover(context.Background(), p.intent); err != nil {
		return err
	}
	if p.token != 0 {
		if err := p.token.Close(); err != nil {
			return err
		}
		p.token = 0
	}
	if p.station != 0 {
		if err := closeNetworkStation(p.station); err != nil {
			return err
		}
		p.station = 0
	}
	if p.releaseFiles != nil {
		p.releaseFiles()
		p.releaseFiles = nil
	}
	if p.lock != nil {
		p.lock.release()
		p.lock = nil
	}
	p.closed = true
	return nil
}

// Fail-closed material is concrete and session-bound, never a boolean supplied
// by RPC or an arbitrary ManagedNetwork implementation.
func (p *ProtectedProxy) validate(root string, record Record, profile ManagedProfile) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || !p.preflighted || p.bridge == nil || p.lock == nil || p.intent.Mode != proxyBridgeSessionMode || p.intent.SessionID != profile.SessionID || p.intent.EnvironmentID != profile.EnvironmentID || p.intent.DataReference != profile.UserDataRef || p.record.ID != record.ID || p.record.ExecutableSHA256 != record.ExecutableSHA256 || p.store.root != root {
		return RequireProxyNetworkBoundary()
	}
	return nil
}
