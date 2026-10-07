//go:build windows

package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
)

func recycleFixture(t *testing.T, s *Service, root, name string) (Environment, string) {
	t.Helper()
	e, _ := create(t, s, name)
	ref, _ := dataReference(e.ID)
	dir := filepath.Join(root, filepath.FromSlash(ref))
	if err := os.MkdirAll(filepath.Join(dir, "empty"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Cookies"), []byte("SYNTHETIC_RECYCLE_"+e.ID), 0600); err != nil {
		t.Fatal(err)
	}
	return e, dir
}
func previewRecycleFixture(t *testing.T, s *Service, action string, ids ...string) RecyclePage {
	t.Helper()
	return value[RecyclePage](t, call(s, "Recycle.Preview", map[string]any{"action": action, "ids": ids}))
}
func acceptRecycleFixture(t *testing.T, s *Service, page RecyclePage) (RecycleRequest, Operation) {
	t.Helper()
	request := RecycleRequest{PreviewID: page.Preview.PreviewID, Confirm: true, RequestID: id()}
	op := value[struct {
		Operation Operation `json:"operation"`
	}](t, call(s, "Recycle.Commit", request)).Operation
	return request, op
}
func runRecycleFixture(t *testing.T, s *Service, action string, ids ...string) Operation {
	t.Helper()
	_, op := acceptRecycleFixture(t, s, previewRecycleFixture(t, s, action, ids...))
	return waitBackupFixture(t, s, op.ID)
}
func recycleListFixture(t *testing.T, s *Service) RecyclePage {
	t.Helper()
	return value[RecyclePage](t, call(s, "Recycle.ReadPage", recyclePageRequest{PageSize: 100}))
}
func waitRecycleProtectedFixture(t *testing.T, s *Service) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		ready := s.recycleTask != nil && !s.recycleTask.running
		s.mu.Unlock()
		if ready {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("recycle did not retain a stopped protected task")
}

func TestRecycleReopenRestorePreservesIdentityHistoryAndData(t *testing.T) {
	s, root := fixture(t, Options{})
	e, dir := recycleFixture(t, s, root, "合成回收身份")
	second := preview(t, s, "edit", e.ID)
	second.Environment.CPU = "8"
	value[any](t, call(s, "Environment.Update", Mutation{PreviewID: second.PreviewID, Configuration: second.Environment.Configuration, ExpectedRevision: second.ExpectedRevision, RequestID: id()}))
	e, _, profileID, readErr := s.readStoredEnvironment(e.ID)
	if readErr != nil {
		t.Fatal(readErr)
	}
	history := value[[]ProfileRevision](t, call(s, "Fingerprint.ListRevisions", map[string]string{"environmentId": e.ID}))
	if len(history) != 2 {
		t.Fatal("fixture lacks multiple profile revisions")
	}
	before := preview(t, s, "edit", e.ID)
	profiles, refs, err := s.profileViews([]string{e.ID})
	if err != nil {
		t.Fatal(err)
	}
	oldBytes, err := os.ReadFile(filepath.Join(dir, "Cookies"))
	if err != nil {
		t.Fatal(err)
	}
	request, accepted := acceptRecycleFixture(t, s, previewRecycleFixture(t, s, "remove", e.ID))
	if op := waitBackupFixture(t, s, accepted.ID); op.State != "completed" {
		t.Fatal(op)
	}
	if len(view(t, s).State.Environments) != 0 {
		t.Fatal("recycled environment remained active")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("live tree remained", err)
	}
	duplicate := value[struct {
		Operation Operation `json:"operation"`
	}](t, call(s, "Recycle.Commit", request))
	if duplicate.Operation.ID != accepted.ID {
		t.Fatal("request replayed")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	list := recycleListFixture(t, reopened)
	if list.Total != 1 || list.Items[0].EnvironmentID != e.ID {
		t.Fatal(list)
	}
	if op := runRecycleFixture(t, reopened, "restore", list.Items[0].ID); op.State != "completed" {
		t.Fatal(op)
	}
	restored := preview(t, reopened, "edit", e.ID)
	_, _, afterProfileID, readErr := reopened.readStoredEnvironment(e.ID)
	if readErr != nil {
		t.Fatal(readErr)
	}
	afterHistory := value[[]ProfileRevision](t, call(reopened, "Fingerprint.ListRevisions", map[string]string{"environmentId": e.ID}))
	if profileID != afterProfileID || !reflect.DeepEqual(history, afterHistory) {
		t.Fatal("original fingerprint ID or historical revisions changed")
	}
	afterProfiles, afterRefs, err := reopened.profileViews([]string{e.ID})
	if err != nil {
		t.Fatal(err)
	}
	if restored.Environment.Configuration != e.Configuration || restored.Environment.Code != e.Code || restored.ExpectedRevision != before.ExpectedRevision+2 || !reflect.DeepEqual(profiles, afterProfiles) || !reflect.DeepEqual(refs, afterRefs) {
		t.Fatal("restore replaced original identity/history/configuration")
	}
	got, err := os.ReadFile(filepath.Join(dir, "Cookies"))
	if err != nil || string(got) != string(oldBytes) {
		t.Fatal("original data not restored", err)
	}
}

func TestRecycleHistoryReadPageKeepsFrozenTargetsAfterRestoreAndReopen(t *testing.T) {
	s, root := fixture(t, Options{})
	ids := make([]string, 27)
	for index := range ids {
		e, _ := create(t, s, fmt.Sprintf("合成回收历史 %02d", index+1))
		ids[index] = e.ID
	}
	op := runRecycleFixture(t, s, "remove", ids...)
	if op.State != "completed" {
		t.Fatal("history fixture was not completely recycled", op.Error)
	}
	readHistory := func(service *Service, offset int) RecyclePage {
		return value[RecyclePage](t, call(service, "Recycle.ReadPage", recyclePageRequest{OperationID: op.ID, Offset: offset, PageSize: 25}))
	}
	first, last := readHistory(s, 0), readHistory(s, 25)
	if first.Total != 27 || len(first.Items) != 25 || len(last.Items) != 2 || first.Operation.ID != op.ID || last.Operation.RecycleReport.Completed != 27 {
		t.Fatal("ReadPage dropped frozen targets or returned the wrong operation")
	}
	all := append(append([]RecycleItem{}, first.Items...), last.Items...)
	for index, item := range all {
		if item.EnvironmentID != ids[index] || item.State != "recycled" {
			t.Fatal("history order or original outcome changed", index, item)
		}
	}
	live := recycleListFixture(t, s)
	trashIDs := map[string]string{}
	for _, item := range live.Items {
		trashIDs[item.EnvironmentID] = item.ID
	}
	// Remove history freezes original environment IDs; restore consumes the
	// live list's trash IDs. Do not mislabel those two different contracts.
	restored := runRecycleFixture(t, s, "restore", trashIDs[ids[0]], trashIDs[ids[26]])
	if restored.State != "completed" || recycleListFixture(t, s).Total != 25 {
		t.Fatal("restore fixture did not change the live recycle list")
	}
	if !reflect.DeepEqual(readHistory(s, 0), first) || !reflect.DeepEqual(readHistory(s, 25), last) {
		t.Fatal("later restore rewrote original frozen history")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !reflect.DeepEqual(readHistory(reopened, 0), first) || !reflect.DeepEqual(readHistory(reopened, 25), last) {
		t.Fatal("reopen did not preserve exact ReadPage history")
	}
	wantError(t, call(reopened, "Recycle.ReadPage", recyclePageRequest{OperationID: op.ID, Offset: 28, PageSize: 25}), "VALIDATION_FAILED")
	wantError(t, call(reopened, "Recycle.ReadPage", recyclePageRequest{OperationID: op.ID, PreviewID: id(), PageSize: 25}), "VALIDATION_FAILED")
}

func TestRecycleRejectsBusyMissingDataAndOccupiedRestore(t *testing.T) {
	s, root := fixture(t, Options{})
	e, dir := recycleFixture(t, s, root, "合成冲突")
	s.mu.Lock()
	s.profileUses[e.ID] = true
	s.mu.Unlock()
	wantError(t, call(s, "Recycle.Preview", map[string]any{"action": "remove", "ids": []string{e.ID}}), "PROFILE_BUSY")
	s.mu.Lock()
	delete(s.profileUses, e.ID)
	s.mu.Unlock()
	if op := runRecycleFixture(t, s, "remove", e.ID); op.State != "completed" {
		t.Fatal(op)
	}
	item := recycleListFixture(t, s).Items[0]
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	wantError(t, call(s, "Recycle.Preview", map[string]any{"action": "restore", "ids": []string{item.ID}}), "RECYCLE_CONFLICT")
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	runRecycleFixture(t, s, "restore", item.ID)
	if _, err := s.db.Exec("UPDATE environment_data_state SET state='runtime-claimed' WHERE environment_id=?", e.ID); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(dir, dir+"-retained"); err != nil {
		t.Fatal(err)
	}
	wantError(t, call(s, "Recycle.Preview", map[string]any{"action": "remove", "ids": []string{e.ID}}), "RECYCLE_INCOMPLETE")
}

func TestRecyclePurgeDeletesOnlyConfirmedItemAndPreservesBackup(t *testing.T) {
	s, root := fixture(t, Options{})
	a, aDir := recycleFixture(t, s, root, "合成永久删除A")
	b, bDir := recycleFixture(t, s, root, "合成保留B")
	path := restorePackageFixture(t, s, []string{a.ID})
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if op := runRecycleFixture(t, s, "remove", a.ID); op.State != "completed" {
		t.Fatal(op)
	}
	list := recycleListFixture(t, s)
	if !list.Items[0].BackupRecorded {
		t.Fatal("backup impact absent")
	}
	wantError(t, call(s, "Recycle.Preview", map[string]any{"action": "purge", "ids": []string{b.ID}}), "NOT_FOUND")
	if op := runRecycleFixture(t, s, "purge", list.Items[0].ID); op.State != "completed" {
		t.Fatal(op)
	}
	if recycleListFixture(t, s).Total != 0 {
		t.Fatal("purged membership remained")
	}
	if _, _, _, err := s.readStoredEnvironment(a.ID); err == nil {
		t.Fatal("purged configuration remained")
	}
	if _, err := os.Stat(aDir); !os.IsNotExist(err) {
		t.Fatal("live purged tree returned")
	}
	if data, err := os.ReadFile(filepath.Join(bDir, "Cookies")); err != nil || !strings.HasSuffix(string(data), b.ID) {
		t.Fatal("unselected environment affected", err)
	}
	if after, err := os.ReadFile(path); err != nil || string(after) != string(before) {
		t.Fatal("historical backup affected", err)
	}
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM kernels").Scan(&count); err != nil || count != 1 {
		t.Fatal("shared kernel reference deleted", err)
	}
}

func TestRecyclePartialMoveRollsBackOnlyFailedItem(t *testing.T) {
	var changes atomic.Int32
	s, root := fixture(t, Options{RecycleCheckpoint: func(phase string) error {
		if phase == "directory-changed" && changes.Add(1) == 2 {
			return errors.New("SYNTHETIC_MOVE_FAILURE")
		}
		return nil
	}})
	a, _ := recycleFixture(t, s, root, "合成部分A")
	b, bDir := recycleFixture(t, s, root, "合成部分B")
	op := runRecycleFixture(t, s, "remove", a.ID, b.ID)
	if op.State != "failed" || op.RecycleReport.Completed != 1 || op.RecycleReport.Failed != 1 || op.RecycleReport.Protected {
		t.Fatal(op)
	}
	if _, _, _, err := s.readEnvironment(b.ID); err != nil {
		t.Fatal("failed item config lost", err)
	}
	if data, err := os.ReadFile(filepath.Join(bDir, "Cookies")); err != nil || !strings.HasSuffix(string(data), b.ID) {
		t.Fatal("failed item data lost", err)
	}
	if list := recycleListFixture(t, s); list.Total != 1 || list.Items[0].EnvironmentID != a.ID {
		t.Fatal("wrong item retained", list)
	}
}

func TestRecycleCancelBeforePurgeAuthorizationRetainsEntireItem(t *testing.T) {
	paused, resume := make(chan struct{}), make(chan struct{})
	var enabled atomic.Bool
	var once sync.Once
	s, root := fixture(t, Options{RecycleCheckpoint: func(phase string) error {
		if enabled.Load() && phase == "prepared" {
			once.Do(func() { close(paused); <-resume })
		}
		return nil
	}})
	e, _ := recycleFixture(t, s, root, "合成删除取消")
	runRecycleFixture(t, s, "remove", e.ID)
	item := recycleListFixture(t, s).Items[0]
	enabled.Store(true)
	_, op := acceptRecycleFixture(t, s, previewRecycleFixture(t, s, "purge", item.ID))
	select {
	case <-paused:
	case <-time.After(10 * time.Second):
		close(resume)
		t.Fatal("purge did not prepare")
	}
	result := call(s, "Operation.Cancel", map[string]string{"operationId": op.ID})
	close(resume)
	value[Operation](t, result)
	final := waitBackupFixture(t, s, op.ID)
	if final.State != "cancelled" || final.RecycleReport.Completed != 0 {
		t.Fatal("cancelled before authorization still purged", final)
	}
	if op := runRecycleFixture(t, s, "restore", item.ID); op.State != "completed" {
		t.Fatal("cancel lost retained data", op)
	}
}

func TestRecycleFinalStorageFailureReopensWithOriginalDirectoryObject(t *testing.T) {
	var blocked atomic.Bool
	var changes atomic.Int32
	s, root := fixture(t, Options{BeforeCommit: func() error {
		if blocked.Load() {
			return errors.New("SYNTHETIC_STORAGE_FAILURE")
		}
		return nil
	}, RecycleCheckpoint: func(phase string) error {
		if phase == "directory-changed" {
			changes.Add(1)
		}
		if phase == "item-committed" {
			blocked.Store(true)
		}
		return nil
	}})
	e, _ := recycleFixture(t, s, root, "合成收尾重开")
	_, op := acceptRecycleFixture(t, s, previewRecycleFixture(t, s, "remove", e.ID))
	waitRecycleProtectedFixture(t, s)
	s.mu.Lock()
	retained := s.recycleTask.plan.Items[0].Entry
	s.mu.Unlock()
	original, exists, err := backup.IdentifyTree(root, recycleReference(retained.TrashID))
	if err != nil || !exists {
		t.Fatal(err)
	}
	if err := s.Close(); err == nil {
		t.Fatal("shutdown claimed pending result persisted")
	}
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if final := waitBackupFixture(t, reopened, op.ID); final.State != "completed" || final.RecycleReport.Protected {
		t.Fatal(final)
	}
	after, exists, err := backup.IdentifyTree(root, recycleReference(retained.TrashID))
	if changes.Load() != 1 || recycleListFixture(t, reopened).Total != 1 || err != nil || !exists || after != original {
		t.Fatal("restart did not preserve original retained directory object", err)
	}
}

func TestRecycleCancellationWriteFailureStillStopsForwardPurge(t *testing.T) {
	paused, resume := make(chan struct{}), make(chan struct{})
	var enabled, blocked atomic.Bool
	var once sync.Once
	s, root := fixture(t, Options{BeforeCommit: func() error {
		if blocked.Load() {
			return errors.New("SYNTHETIC_CANCEL_STORAGE_FAILURE")
		}
		return nil
	}, RecycleCheckpoint: func(phase string) error {
		if enabled.Load() && phase == "prepared" {
			once.Do(func() { close(paused); <-resume })
		}
		return nil
	}})
	e, _ := recycleFixture(t, s, root, "合成取消写失败")
	runRecycleFixture(t, s, "remove", e.ID)
	item := recycleListFixture(t, s).Items[0]
	enabled.Store(true)
	_, op := acceptRecycleFixture(t, s, previewRecycleFixture(t, s, "purge", item.ID))
	select {
	case <-paused:
	case <-time.After(10 * time.Second):
		close(resume)
		t.Fatal("prepare timeout")
	}
	blocked.Store(true)
	result := call(s, "Operation.Cancel", map[string]string{"operationId": op.ID})
	blocked.Store(false)
	close(resume)
	if result.OK {
		t.Fatal("failed cancellation write reported persisted")
	}
	final := waitBackupFixture(t, s, op.ID)
	if final.State != "cancelled" || final.RecycleReport.Completed != 0 || !final.CancelRequested {
		t.Fatal("write failure lost cancellation intent", final)
	}
	if op := runRecycleFixture(t, s, "restore", item.ID); op.State != "completed" {
		t.Fatal("cancel failure destroyed retained item", op)
	}
}

func TestRecycleStartupDoesNotClaimUnloadedRuntimeStopped(t *testing.T) {
	s, _ := fixture(t, Options{})
	e, _ := create(t, s, "合成未加载会话")
	s.mu.Lock()
	s.recycleTask = &recycleTask{startup: true}
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.recycleTask = nil; s.mu.Unlock() }()
	wantError(t, call(s, "Runtime.Stop", runtimeRequest{EnvironmentID: e.ID, RequestID: id()}), "RECYCLE_INCOMPLETE")
	wantError(t, call(s, "Runtime.Inspect", map[string]any{"ids": []string{e.ID}}), "RECYCLE_INCOMPLETE")
}

func TestRecycleJournalReadFailureNeverOverwritesDecision(t *testing.T) {
	var service *Service
	var renamed atomic.Bool
	s, root := fixture(t, Options{RecycleCheckpoint: func(phase string) error {
		if phase == "directory-changed" && !renamed.Swap(true) {
			service.mu.Lock()
			defer service.mu.Unlock()
			_, err := service.db.Exec("ALTER TABLE recycle_jobs RENAME TO synthetic_recycle_hold")
			return err
		}
		return nil
	}})
	service = s
	e, dir := recycleFixture(t, s, root, "合成日志未知")
	_, op := acceptRecycleFixture(t, s, previewRecycleFixture(t, s, "remove", e.ID))
	waitRecycleProtectedFixture(t, s)
	s.mu.Lock()
	var phase string
	err := s.db.QueryRow("SELECT phase FROM synthetic_recycle_hold WHERE operation_id=?", op.ID).Scan(&phase)
	if err == nil {
		_, err = s.db.Exec("ALTER TABLE synthetic_recycle_hold RENAME TO recycle_jobs")
	}
	s.mu.Unlock()
	if err != nil || phase != "prepared" {
		t.Fatal("unknown journal replaced", phase, err)
	}
	value[Operation](t, call(s, "Recycle.Recover", map[string]string{"operationId": op.ID}))
	final := waitBackupFixture(t, s, op.ID)
	if final.State != "failed" || final.RecycleReport.Protected {
		t.Fatal(final)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "Cookies")); err != nil || !strings.HasSuffix(string(data), e.ID) {
		t.Fatal("recovery failed original data", err)
	}
}

func TestRecycleRejectsOldEditAndBatchAfterRoundTrip(t *testing.T) {
	s, _ := fixture(t, Options{})
	e, _ := create(t, s, "合成旧草稿")
	edit := preview(t, s, "edit", e.ID)
	batch := value[BatchPage](t, call(s, "Batch.Preview", BatchPreviewRequest{Kind: "assign", Mappings: []BatchMapping{{EnvironmentID: e.ID, ProxyID: ""}}}))
	runRecycleFixture(t, s, "remove", e.ID)
	runRecycleFixture(t, s, "restore", recycleListFixture(t, s).Items[0].ID)
	wantError(t, call(s, "Environment.Update", Mutation{PreviewID: edit.PreviewID, EnvironmentID: e.ID, Configuration: edit.Environment.Configuration, ExpectedRevision: edit.ExpectedRevision, RequestID: id()}), "PREVIEW_EXPIRED")
	op := waitBatchFixture(t, s, acceptBatchFixture(t, s, batch, id()).ID)
	if op.State == "completed" {
		t.Fatal("old batch accepted revision ABA")
	}
}

func TestRecycleCorruptIncompleteJournalBlocksOpen(t *testing.T) {
	for _, corruption := range []string{"hash", "missing-decision", "receipt"} {
		t.Run(corruption, func(t *testing.T) {
			s, root := fixture(t, Options{})
			e, _ := create(t, s, "合成日志校验")
			op := runRecycleFixture(t, s, "remove", e.ID)
			var raw string
			if err := s.db.QueryRow("SELECT plan_json FROM recycle_jobs WHERE operation_id=?", op.ID).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			if _, err := s.db.Exec("UPDATE recycle_jobs SET phase='item-committed' WHERE operation_id=?", op.ID); err != nil {
				t.Fatal(err)
			}
			switch corruption {
			case "hash":
				_, _ = s.db.Exec("UPDATE recycle_jobs SET plan_sha256=? WHERE operation_id=?", strings.Repeat("0", 64), op.ID)
			case "missing-decision":
				var plan map[string]any
				json.Unmarshal([]byte(raw), &plan)
				delete(plan["items"].([]any)[0].(map[string]any), "committed")
				encoded, _ := json.Marshal(plan)
				_, _ = s.db.Exec("UPDATE recycle_jobs SET plan_json=?,plan_sha256=? WHERE operation_id=?", string(encoded), recyclePlanHash(encoded), op.ID)
			case "receipt":
				_, _ = s.db.Exec("UPDATE requests SET signature=? WHERE id=?", strings.Repeat("0", 64), op.RecycleReport.RequestID)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(root, Options{})
			if err == nil {
				reopened.Close()
				t.Fatal("corrupt deletion journal opened")
			}
		})
	}
}

func TestRecycleAllBackupExcludesTrashAndRemainsSchemaSeven(t *testing.T) {
	s, root := fixture(t, Options{})
	a, _ := recycleFixture(t, s, root, "合成排除回收")
	b, _ := create(t, s, "合成全量保留")
	runRecycleFixture(t, s, "remove", a.ID)
	path := filepath.Join(t.TempDir(), "all.prismbackup")
	token := backupDestinationFixture(t, s, path)
	op := waitBackupFixture(t, s, acceptBackupFixture(t, s, BackupExportRequest{Scope: "all", EnvironmentIDs: []string{}, DestinationToken: token, StopRunning: true, RequestID: id()}).ID)
	if op.State != "completed" {
		t.Fatal(op)
	}
	manifest, _ := readPackageFixture(t, path)
	if manifest.WorkspaceSchema != 7 || len(manifest.Environments) != 1 || manifest.Environments[0].ID != b.ID {
		t.Fatal("all export included trash or changed v1 format", manifest)
	}
	p := value[RestorePreview](t, previewPackageFixture(t, s, path))
	if !p.CanRestore {
		t.Fatal("schema7 export lost compatibility")
	}
}
