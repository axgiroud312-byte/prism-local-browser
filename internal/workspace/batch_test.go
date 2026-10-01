package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

// A host-only filesystem seam, never evidence that real profile files exist.
type syntheticBatchDirectory struct{ closed atomic.Bool }

func (lease *syntheticBatchDirectory) Close() error { lease.closed.Store(true); return nil }
func (lease *syntheticBatchDirectory) CheckEmpty() error {
	if lease.closed.Load() {
		return errors.New("synthetic lease closed")
	}
	return nil
}
func syntheticBatchPrepare(BatchDirectoryInput) (BatchDirectoryLease, error) {
	return &syntheticBatchDirectory{}, nil
}
func createBatchPreviewFixture(t *testing.T, s *Service, name string, count int) BatchPage {
	t.Helper()
	draft := preview(t, s, "create", "")
	draft.Environment.Name = name
	return value[BatchPage](t, call(s, "Batch.Preview", BatchPreviewRequest{Kind: "create", Create: &Mutation{PreviewID: draft.PreviewID, Configuration: draft.Environment.Configuration, Count: count, RequestID: id()}}))
}
func acceptBatchFixture(t *testing.T, s *Service, page BatchPage, requestID string) Operation {
	t.Helper()
	return value[struct {
		Operation Operation `json:"operation"`
	}](t, call(s, "Batch.Commit", map[string]string{"planId": page.PlanID, "requestId": requestID})).Operation
}
func waitBatchFixture(t *testing.T, s *Service, operationID string) Operation {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		operation := value[Operation](t, call(s, "Operation.Read", map[string]string{"operationId": operationID}))
		if operation.State == "completed" || operation.State == "failed" || operation.State == "cancelled" {
			return operation
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("synthetic batch did not reach terminal observation")
	return Operation{}
}
func readBatchPageFixture(t *testing.T, s *Service, planID string, offset int64) BatchPage {
	t.Helper()
	return value[BatchPage](t, call(s, "Batch.ReadPage", map[string]any{"planId": planID, "offset": offset, "pageSize": 25}))
}

func TestBatchHugePreviewIsVirtualAndDoesNotCreateOrLaunch(t *testing.T) {
	var preparations atomic.Int32
	s, _ := fixture(t, Options{PrepareBatchDirectory: func(input BatchDirectoryInput) (BatchDirectoryLease, error) {
		preparations.Add(1)
		return syntheticBatchPrepare(input)
	}})
	page := createBatchPreviewFixture(t, s, "合成大计划", 1000003)
	last := readBatchPageFixture(t, s, page.PlanID, 1000000)
	var rows int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM batch_items WHERE plan_id=?", page.PlanID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if page.Total != 1000003 || len(page.Items) != 25 || page.NotExecutedCount != page.Total || len(last.Items) != 3 || last.Items[2].Name != "合成大计划 1000003" || rows != 0 || preparations.Load() != 0 || view(t, s).EnvironmentPage.Total != 0 {
		t.Fatal("preview expanded a million identities, imposed a quota or executed side effects")
	}
}

func TestBatchCreateCommitsIndividualNewIdentitiesAndOnlyPagedViews(t *testing.T) {
	s, _ := fixture(t, Options{PrepareBatchDirectory: syntheticBatchPrepare})
	page := createBatchPreviewFixture(t, s, "合成批次", 31)
	requestID := id()
	operation := acceptBatchFixture(t, s, page, requestID)
	completed := waitBatchFixture(t, s, operation.ID)
	if completed.State != "completed" || completed.BatchReport.CompletedCount != 31 || len(completed.CompletedIDs) != 0 {
		t.Fatal("batch operation accumulated all IDs or counted acceptance as completion")
	}
	prior := acceptBatchFixture(t, s, page, requestID)
	if prior.ID != operation.ID {
		t.Fatal("requestId replay accepted another batch")
	}
	first := value[View](t, call(s, "Workspace.Read", map[string]any{"environmentQuery": EnvironmentQuery{Page: 1, PageSize: 8, Status: "all"}}))
	second := value[View](t, call(s, "Workspace.Read", map[string]any{"environmentQuery": EnvironmentQuery{Page: 2, PageSize: 8, Status: "all"}}))
	if first.EnvironmentPage.Total != 31 || len(first.State.Environments) != 8 || len(first.Fingerprints) != 8 || len(first.DataReferences) != 8 || first.State.Environments[0].ID == second.State.Environments[0].ID {
		t.Fatal("native paging still returned all profiles or repeated page")
	}
	seen := map[string]bool{}
	for offset := int64(0); offset < 31; offset += 25 {
		for _, item := range readBatchPageFixture(t, s, page.PlanID, offset).Items {
			if item.State != "completed" || item.EnvironmentID == "" || seen[item.EnvironmentID] {
				t.Fatal("completed identities were duplicated or lost")
			}
			seen[item.EnvironmentID] = true
		}
	}
}

func TestBatchDirectoryResourceFailureResumesPreparedIdentityWithoutRecreatingCompleted(t *testing.T) {
	var blocked atomic.Bool
	blocked.Store(true)
	preparedIDs := map[int64]string{}
	s, _ := fixture(t, Options{PrepareBatchDirectory: func(input BatchDirectoryInput) (BatchDirectoryLease, error) {
		if prior := preparedIDs[input.Index]; prior != "" && prior != input.EnvironmentID {
			return nil, errors.New("synthetic prepared identity changed")
		}
		preparedIDs[input.Index] = input.EnvironmentID
		if input.Index == 2 && blocked.Load() {
			return nil, &kernel.Problem{Code: "DISK_FULL", Reason: "synthetic-only", Message: "合成磁盘不足，不是实际文件系统证据。", Retryable: true}
		}
		return syntheticBatchPrepare(input)
	}})
	page := createBatchPreviewFixture(t, s, "合成资源恢复", 4)
	failed := waitBatchFixture(t, s, acceptBatchFixture(t, s, page, id()).ID)
	if failed.State != "failed" || failed.Error.Code != "DISK_FULL" || failed.BatchReport.CompletedCount != 2 || failed.BatchReport.NotExecutedCount != 2 {
		t.Fatal("resource failure discarded completed rows or marked unexecuted items complete")
	}
	before := readBatchPageFixture(t, s, page.PlanID, 0)
	blocked.Store(false)
	continued := value[struct {
		Operation Operation `json:"operation"`
	}](t, call(s, "Batch.Retry", map[string]string{"operationId": failed.ID, "requestId": id()})).Operation
	completed := waitBatchFixture(t, s, continued.ID)
	after := readBatchPageFixture(t, s, page.PlanID, 0)
	if completed.State != "completed" || completed.BatchReport.CompletedCount != 4 || before.Items[0].EnvironmentID != after.Items[0].EnvironmentID || before.Items[1].EnvironmentID != after.Items[1].EnvironmentID || before.Items[2].EnvironmentID != after.Items[2].EnvironmentID {
		t.Fatal("explicit continuation changed completed or prepared identities")
	}
	wantError(t, call(s, "Batch.Retry", map[string]string{"operationId": failed.ID, "requestId": id()}), "REVISION_CONFLICT")
}

func TestBatchCancellationKeepsCommittedItemsAndCancelsOnlyItsQueue(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var wait atomic.Bool
	wait.Store(true)
	s, _ := fixture(t, Options{PrepareBatchDirectory: func(input BatchDirectoryInput) (BatchDirectoryLease, error) {
		if input.Index == 1 && wait.Load() {
			close(entered)
			<-release
		}
		return syntheticBatchPrepare(input)
	}})
	page := createBatchPreviewFixture(t, s, "合成取消", 3)
	operation := acceptBatchFixture(t, s, page, id())
	<-entered
	value[Operation](t, call(s, "Operation.Cancel", map[string]string{"operationId": operation.ID}))
	close(release)
	final := waitBatchFixture(t, s, operation.ID)
	if final.State != "cancelled" || final.BatchReport.CompletedCount != 1 || final.BatchReport.NotExecutedCount != 2 {
		t.Fatal("cancellation reverted committed items or sent remaining rows")
	}
	wait.Store(false)
	other := createBatchPreviewFixture(t, s, "合成另一批次", 1)
	if waitBatchFixture(t, s, acceptBatchFixture(t, s, other, id()).ID).State != "completed" {
		t.Fatal("cancelling one operation cancelled another batch")
	}
}

func TestBatchCloneKeepsExactTemplateButNewIDSeedAndNoLoginData(t *testing.T) {
	s, _, kernelID := fingerprintFixture(t, Options{PrepareBatchDirectory: syntheticBatchPrepare})
	source := createRuntimeEnvironment(t, s, kernelID, "合成配置源")
	before := view(t, s).Fingerprints[source.ID].Profile
	page := value[BatchPage](t, call(s, "Batch.Preview", BatchPreviewRequest{Kind: "clone", SourceIDs: []string{source.ID}}))
	final := waitBatchFixture(t, s, acceptBatchFixture(t, s, page, id()).ID)
	item := readBatchPageFixture(t, s, page.PlanID, 0).Items[0]
	if final.State != "completed" || !item.NewIdentity || item.EnvironmentID == source.ID {
		t.Fatal("clone did not create a separate identity")
	}
	cloned := view(t, s).Fingerprints[item.EnvironmentID].Profile
	if cloned.Seed == before.Seed || cloned.KernelID != before.KernelID || cloned.GeneratorVersion != before.GeneratorVersion || cloned.CoreExecutableSHA256 != before.CoreExecutableSHA256 || reflect.DeepEqual(cloned.Parameters, before.Parameters) {
		t.Fatal("clone changed the exact kernel/template or kept old seed parameters")
	}
}

func TestBatchAssignmentPreservesProfileAndReportsBusyRevisionConflictsPerItem(t *testing.T) {
	s, _ := fixture(t, Options{PrepareBatchDirectory: syntheticBatchPrepare})
	a, _ := create(t, s, "合成映射A")
	b, _ := create(t, s, "合成映射B")
	record := importProxyFixture(t, s, "localhost:8080")
	before := view(t, s).Fingerprints[a.ID].Profile
	page := value[BatchPage](t, call(s, "Batch.Preview", BatchPreviewRequest{Kind: "assign", Mappings: []BatchMapping{{EnvironmentID: a.ID, ProxyID: record.ID}, {EnvironmentID: b.ID, ProxyID: record.ID}}}))
	release, err := s.AcquireProfileUse(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	final := waitBatchFixture(t, s, acceptBatchFixture(t, s, page, id()).ID)
	release()
	items := readBatchPageFixture(t, s, page.PlanID, 0).Items
	if final.State != "failed" || final.BatchReport.CompletedCount != 1 || final.BatchReport.FailedCount != 1 || items[1].Error.Code != "PROFILE_BUSY" || !reflect.DeepEqual(before, view(t, s).Fingerprints[a.ID].Profile) {
		t.Fatal("assignment changed identity or ignored busy protection")
	}
	edit := preview(t, s, "edit", b.ID)
	edit.Environment.Note = "合成新修订"
	value[map[string]any](t, call(s, "Environment.Update", Mutation{PreviewID: edit.PreviewID, Configuration: edit.Environment.Configuration, ExpectedRevision: edit.ExpectedRevision, RequestID: id()}))
	continued := value[struct {
		Operation Operation `json:"operation"`
	}](t, call(s, "Batch.Retry", map[string]string{"operationId": final.ID, "requestId": id()})).Operation
	final = waitBatchFixture(t, s, continued.ID)
	items = readBatchPageFixture(t, s, page.PlanID, 0).Items
	if final.BatchReport.CompletedCount != 1 || items[1].Error.Code != "REVISION_CONFLICT" || view(t, s).State.Environments[0].ProxyID != "" {
		t.Fatal("old plan overwrote a new revision or replayed a completed assignment")
	}
}

func TestBatchFinalStorageFailureOnlyRetriesJournalSaveAndWipesExternalWork(t *testing.T) {
	var broken atomic.Bool
	var preparations atomic.Int32
	s, _ := fixture(t, Options{BeforeCommit: func() error {
		if broken.Load() {
			return errors.New("synthetic journal unavailable")
		}
		return nil
	}, PrepareBatchDirectory: func(input BatchDirectoryInput) (BatchDirectoryLease, error) {
		preparations.Add(1)
		return syntheticBatchPrepare(input)
	}})
	page := createBatchPreviewFixture(t, s, "合成保存恢复", 1)
	operation := acceptBatchFixture(t, s, page, id())
	complete := waitBatchFixture(t, s, operation.ID)
	if complete.State != "completed" {
		t.Fatal("synthetic batch not complete")
	}
	// Inject only terminal persistence failure after the item transaction.
	s.mu.Lock()
	plan, err := readBatchPlan(s.db, page.PlanID)
	if err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	task := &batchTask{planID: plan.ID, operationID: plan.OperationID, last: complete, finalState: "completed"}
	s.batchTasks[plan.ID] = task
	broken.Store(true)
	_ = s.flushOneBatchFinal(task)
	s.mu.Unlock()
	pending := value[Operation](t, call(s, "Operation.Read", map[string]string{"operationId": operation.ID}))
	if !pending.PersistencePending || pending.BatchReport.CompletedCount != 1 {
		t.Fatal("final pending result lost committed count")
	}
	wantError(t, call(s, "Batch.Retry", map[string]string{"operationId": operation.ID, "requestId": id()}), "BATCH_BUSY")
	broken.Store(false)
	saved := value[Operation](t, call(s, "Operation.Read", map[string]string{"operationId": operation.ID}))
	if saved.State != "completed" || saved.PersistencePending || preparations.Load() != 1 {
		t.Fatal("saving terminal result replayed directory work")
	}
}

func TestBatchReopenInterruptsWithoutAutoReplayAndKeepsSafeJournal(t *testing.T) {
	var preparations atomic.Int32
	options := Options{PrepareBatchDirectory: func(input BatchDirectoryInput) (BatchDirectoryLease, error) {
		preparations.Add(1)
		return syntheticBatchPrepare(input)
	}}
	s, root := fixture(t, options)
	page := createBatchPreviewFixture(t, s, "合成重开", 2)
	final := waitBatchFixture(t, s, acceptBatchFixture(t, s, page, id()).ID)
	if _, err := s.db.Exec("UPDATE batch_plans SET state='running' WHERE id=?", page.PlanID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE operations SET result_json=json_set(result_json,'$.state','running') WHERE id=?", final.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root, options)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	interrupted := value[Operation](t, call(reopened, "Operation.Read", map[string]string{"operationId": final.ID}))
	if interrupted.State != "failed" || interrupted.Error.Code != "APPLICATION_INTERRUPTED" || interrupted.BatchReport.CompletedCount != 2 || preparations.Load() != 2 {
		t.Fatal("reopen replayed directory work or forgot committed items")
	}
	rows, err := reopened.db.Query("SELECT snapshot_json,identity_json FROM batch_items WHERE plan_id=?", page.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var snapshot, identity string
		if err := rows.Scan(&snapshot, &identity); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(snapshot+identity, "cookies") || strings.Contains(snapshot+identity, "password") {
			t.Fatal("batch journal contains login material")
		}
	}
}

func TestBatchRejectsUnknownFieldsDuplicateTargetsAndIdReuse(t *testing.T) {
	s, _ := fixture(t, Options{PrepareBatchDirectory: syntheticBatchPrepare})
	environment, _ := create(t, s, "合成批次边界")
	wantError(t, call(s, "Batch.Preview", map[string]any{"kind": "clone", "sourceIds": []string{environment.ID}, "userDataRef": "foreign/profile", "cookies": []any{}}), "VALIDATION_FAILED")
	wantError(t, call(s, "Batch.Preview", BatchPreviewRequest{Kind: "clone", SourceIDs: []string{environment.ID, environment.ID}}), "VALIDATION_FAILED")
	page := createBatchPreviewFixture(t, s, "合成去重", 1)
	requestID := id()
	waitBatchFixture(t, s, acceptBatchFixture(t, s, page, requestID).ID)
	other := createBatchPreviewFixture(t, s, "合成去重另计划", 1)
	wantError(t, call(s, "Batch.Commit", map[string]string{"planId": other.PlanID, "requestId": requestID}), "REQUEST_ID_REUSED")
	encoded, _ := json.Marshal(page)
	if strings.Contains(string(encoded), "password") || strings.Contains(string(encoded), "parameters") {
		t.Fatal("safe batch page includes private execution data")
	}
}

func stripBatchSchema(t *testing.T, s *Service) {
	t.Helper()
	stripMaintenanceSchema(t, s)
	for _, statement := range []string{"DROP TABLE restore_jobs", "DROP TABLE backup_exports", "DROP TABLE environment_data_state", "DROP TABLE batch_item_events", "DROP TABLE batch_items", "DROP TABLE batch_plans", "DROP INDEX environment_proxy_usage", "DROP INDEX environment_kernel_usage", "DROP INDEX fingerprint_seed_history"} {
		if _, err := s.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
}
func TestSchemaFiveToSixAddsOnlyBatchJournalAndKeepsOriginalIdentity(t *testing.T) {
	s, root := fixture(t, Options{})
	environment, _ := create(t, s, "合成v5迁移")
	before := view(t, s).Fingerprints[environment.ID]
	stripBatchSchema(t, s)
	if _, err := s.db.Exec("PRAGMA user_version=5"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var version int
	if err = reopened.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 10 || !reflect.DeepEqual(before, view(t, reopened).Fingerprints[environment.ID]) {
		t.Fatal("schema migration changed saved identity")
	}
}

func TestBatchLeaseOwnsSourceUntilItsPreparationFinishes(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	s, _ := fixture(t, Options{PrepareBatchDirectory: func(input BatchDirectoryInput) (BatchDirectoryLease, error) {
		close(entered)
		<-release
		return syntheticBatchPrepare(input)
	}})
	source, _ := create(t, s, "合成独立租约源")
	sessionID := id()
	s.mu.Lock()
	s.runtimeSlots[source.ID] = &runtimeSlot{session: RuntimeSession{Mode: "native", EnvironmentID: source.ID, SessionID: sessionID, State: "ready"}}
	s.mu.Unlock()
	page := value[BatchPage](t, call(s, "Batch.Preview", BatchPreviewRequest{Kind: "clone", SourceIDs: []string{source.ID}}))
	operation := acceptBatchFixture(t, s, page, id())
	<-entered
	wantError(t, call(s, "Runtime.Reconcile", runtimeRequest{EnvironmentID: source.ID, SessionID: sessionID, RequestID: id()}), "PROFILE_BUSY")
	s.mu.Lock()
	s.releaseProfileUse(source.ID)
	stillOwned := s.profileUses[source.ID] && s.batchUses[source.ID] != nil
	s.mu.Unlock()
	close(release)
	if !stillOwned {
		t.Fatal("an old runtime release discarded the independent batch owner")
	}
	if final := waitBatchFixture(t, s, operation.ID); final.State != "completed" {
		t.Fatal("the original owned preparation did not finish")
	}
}

func TestBatchReservedFirstSeedCannotBeConsumedByOrdinaryCreate(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	s, _ := fixture(t, Options{PrepareBatchDirectory: func(input BatchDirectoryInput) (BatchDirectoryLease, error) {
		if input.Index == 0 {
			close(entered)
			<-release
		}
		return syntheticBatchPrepare(input)
	}})
	draft := preview(t, s, "create", "")
	draft.Environment.Name = "合成预约冲突"
	mutation := Mutation{PreviewID: draft.PreviewID, Configuration: draft.Environment.Configuration, Count: 2, RequestID: id()}
	page := value[BatchPage](t, call(s, "Batch.Preview", BatchPreviewRequest{Kind: "create", Create: &mutation}))
	operation := acceptBatchFixture(t, s, page, id())
	<-entered
	mutation.Count, mutation.RequestID = 1, id()
	result := call(s, "Environment.Create", mutation)
	close(release)
	wantError(t, result, "SEED_CONFLICT")
	if final := waitBatchFixture(t, s, operation.ID); final.State != "completed" || final.BatchReport.CompletedCount != 2 {
		t.Fatal("ordinary create stole the durably reserved seed")
	}
}

func TestBatchConfirmsDurableUnknownAcceptanceAndStartsOnlyOriginalWorker(t *testing.T) {
	var preparations atomic.Int32
	s, _ := fixture(t, Options{PrepareBatchDirectory: func(input BatchDirectoryInput) (BatchDirectoryLease, error) {
		preparations.Add(1)
		return syntheticBatchPrepare(input)
	}})
	page := createBatchPreviewFixture(t, s, "合成受理结果未知", 1)
	requestID := id()
	encoded, _ := json.Marshal(struct{ Method, PlanID, OperationID string }{"Batch.Commit", page.PlanID, ""})
	digest := sha256.Sum256(encoded)
	signature := hex.EncodeToString(digest[:])
	s.mu.Lock()
	plan, err := readBatchPlan(s.db, page.PlanID)
	if err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	plan.OperationID, plan.State, plan.Sequence = id(), "accepted", 1
	operation := batchOperation(plan)
	result := success(map[string]any{"status": "accepted", "operation": operation}, operation.ID)
	tx, err := s.db.Begin()
	if err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	opJSON, _ := json.Marshal(operation)
	resultJSON, _ := json.Marshal(result)
	if _, err = tx.Exec("INSERT INTO operations(id,result_json) VALUES(?,?)", operation.ID, string(opJSON)); err == nil {
		err = persistBatchPlan(tx, plan, operation)
	}
	if err == nil {
		_, err = tx.Exec("INSERT INTO requests(id,signature,result_json) VALUES(?,?,?)", requestID, signature, string(resultJSON))
	}
	if err == nil {
		err = tx.Commit()
	} else {
		tx.Rollback()
	}
	if err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	// A durable COMMIT whose acknowledgement was lost: no worker exists yet.
	observed := s.deferBatchAcceptance(requestID, signature, result, errors.New("synthetic commit acknowledgement lost"))
	s.mu.Unlock()
	if !observed.OK || observed.OperationID != operation.ID {
		t.Fatal("durable acceptance was not resolved from its original request")
	}
	final := waitBatchFixture(t, s, operation.ID)
	repeated := acceptBatchFixture(t, s, page, requestID)
	if final.State != "completed" || repeated.ID != operation.ID || preparations.Load() != 1 {
		t.Fatal("unknown acceptance stayed idle or spawned another worker")
	}
}

func TestBatchRetrySkipsCompletedRowsWithoutPerSuccessTransactions(t *testing.T) {
	var blocked atomic.Bool
	blocked.Store(true)
	var commits atomic.Int32
	s, _ := fixture(t, Options{BeforeCommit: func() error { commits.Add(1); return nil }, PrepareBatchDirectory: func(input BatchDirectoryInput) (BatchDirectoryLease, error) {
		if input.Index == 30 && blocked.Load() {
			return nil, &kernel.Problem{Code: "DISK_FULL", Message: "合成资源限制，仅用例。", Retryable: true}
		}
		return syntheticBatchPrepare(input)
	}})
	page := createBatchPreviewFixture(t, s, "合成跳过完成索引", 31)
	failed := waitBatchFixture(t, s, acceptBatchFixture(t, s, page, id()).ID)
	if failed.BatchReport.CompletedCount != 30 {
		t.Fatal("synthetic completed frontier incorrect")
	}
	commits.Store(0)
	blocked.Store(false)
	continued := value[struct {
		Operation Operation `json:"operation"`
	}](t, call(s, "Batch.Retry", map[string]string{"operationId": failed.ID, "requestId": id()})).Operation
	final := waitBatchFixture(t, s, continued.ID)
	if final.State != "completed" || final.BatchReport.AttemptCompletedCount != 1 || commits.Load() > 4 {
		t.Fatal("continuation rewrote every old success instead of jumping to unfinished work")
	}
}

func TestBatchOldAttemptPagesPreserveFailureAndDoNotExposeLaterPreparedIdentity(t *testing.T) {
	s, _ := fixture(t, Options{PrepareBatchDirectory: syntheticBatchPrepare})
	a, _ := create(t, s, "合成历史A")
	b, _ := create(t, s, "合成历史B")
	page := value[BatchPage](t, call(s, "Batch.Preview", BatchPreviewRequest{Kind: "clone", SourceIDs: []string{a.ID, b.ID}}))
	release, err := s.AcquireProfileUse(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	first := waitBatchFixture(t, s, acceptBatchFixture(t, s, page, id()).ID)
	release()
	before := readBatchPageFixture(t, s, page.PlanID, 0)
	continued := value[struct {
		Operation Operation `json:"operation"`
	}](t, call(s, "Batch.Retry", map[string]string{"operationId": first.ID, "requestId": id()})).Operation
	final := waitBatchFixture(t, s, continued.ID)
	history := value[BatchPage](t, call(s, "Batch.ReadPage", map[string]any{"planId": page.PlanID, "operationId": first.ID, "offset": 0, "pageSize": 25}))
	if final.State != "completed" || !history.History || history.OperationID != first.ID || history.CurrentOperationID != continued.ID || history.CompletedCount != 1 || history.FailedCount != 1 || !reflect.DeepEqual(history.Items, before.Items) || history.Items[1].EnvironmentID != "" || history.Items[1].Error.Code != "PROFILE_BUSY" {
		t.Fatal("old attempt was overwritten by current results or later identity")
	}
	for _, activity := range view(t, s).State.Activities {
		if activity.ID == first.ID && (activity.Result != "error" || activity.ErrorCode != "BATCH_PARTIAL_FAILED") {
			t.Fatal("failed batch activity was reported as success")
		}
	}
}

func TestNativeStartUsesExpectedRevisionEvenOutsideCookiePurpose(t *testing.T) {
	s, _ := fixture(t, Options{})
	environment, _ := create(t, s, "合成跨页启动修订")
	wantError(t, call(s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct", ExpectedRevision: 2}), "REVISION_CONFLICT")
}
