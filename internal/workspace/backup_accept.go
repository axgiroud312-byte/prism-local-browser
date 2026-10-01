package workspace

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
)

func (s *Service) selectBackupDestination() Result {
	if s.options.ChooseBackupDestination == nil {
		return failure("CAPABILITY_UNSUPPORTED", "本机保存对话框尚不可用，未生成任何备份。", false)
	}
	path, err := s.options.ChooseBackupDestination()
	if err != nil {
		return failure("BACKUP_OUTPUT_UNAVAILABLE", "无法选择输出文件；原数据未修改，请重新选择。", true)
	}
	if path == "" {
		return success(map[string]string{"status": "cancelled"}, "")
	}
	path, _, err = backup.OutputPaths(s.root, path, id())
	if err != nil {
		return failure("PATH_OUTSIDE_ROOT", "备份需选择工作区外的本机新 .prismbackup 文件；不覆盖配置、浏览数据或已有文件。", false)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.closeRequested.Load() {
		return failure("NATIVE_UNAVAILABLE", "工作区正在退出，未保存选择。", true)
	}
	for key, value := range s.backupDestinations {
		if time.Now().After(value.expires) {
			delete(s.backupDestinations, key)
		}
	}
	token := id()
	s.backupDestinations[token] = backupDestination{path: path, expires: time.Now().Add(30 * time.Minute)}
	return success(map[string]string{"status": "selected", "destinationToken": token, "name": filepath.Base(path)}, "")
}

func (s *Service) acceptBackup(input BackupExportRequest) Result {
	if strings.TrimSpace(input.RequestID) == "" || len(input.RequestID) > 128 || input.DestinationToken == "" || input.Scope != "all" && input.Scope != "selected" || input.Scope == "all" && len(input.EnvironmentIDs) != 0 || input.Scope == "selected" && len(input.EnvironmentIDs) == 0 {
		return failure("VALIDATION_FAILED", "备份只接受明确范围、桌面输出标识与请求标识，不接受路径或原型快照。", false)
	}
	encoded, _ := json.Marshal(input)
	digest := sha256.Sum256(encoded)
	signature := hex.EncodeToString(digest[:])
	var priorSignature, text string
	err := s.db.QueryRow("SELECT signature,result_json FROM requests WHERE id=?", input.RequestID).Scan(&priorSignature, &text)
	if err == nil {
		if signature != priorSignature {
			return failure("REQUEST_ID_REUSED", "原备份请求不能更换目标、范围或输出；未重复导出。", false)
		}
		var result Result
		if json.Unmarshal([]byte(text), &result) != nil || result.OperationID == "" {
			return failure("STORAGE_READ_FAILED", "备份受理记录无法核对；未重复执行。", true)
		}
		if task := s.backupTasks[result.OperationID]; task != nil {
			if task.acceptancePending {
				if err = s.confirmBackupAcceptance(task); err != nil {
					result := storageFailure(err)
					result.OperationID = task.operation.ID
					return result
				}
			}
			observed := copyBackupOperation(task.operation)
			if pending := s.pendingBackupOperation(task.operation.ID); pending != nil {
				observed = *pending
			}
			return success(map[string]any{"status": "accepted", "operation": observed}, task.operation.ID)
		}
		var operation Operation
		if s.db.QueryRow("SELECT result_json FROM operations WHERE id=?", result.OperationID).Scan(&text) != nil || json.Unmarshal([]byte(text), &operation) != nil || operation.Kind != "backup-export" {
			return failure("STORAGE_READ_FAILED", "原备份操作无法读取；未重复执行。", true)
		}
		return success(map[string]any{"status": "accepted", "operation": operation}, operation.ID)
	}
	if err != sql.ErrNoRows {
		return storageFailure(err)
	}
	for _, task := range s.backupTasks {
		if task.acceptancePending && task.requestID == input.RequestID {
			result := storageFailure(errors.New("backup acceptance unconfirmed"))
			result.OperationID = task.operation.ID
			return result
		}
	}
	destination, exists := s.backupDestinations[input.DestinationToken]
	if !exists || time.Now().After(destination.expires) {
		return failure("PREVIEW_EXPIRED", "备份输出选择已失效，请重新选择新文件。", true)
	}
	ids := append([]string(nil), input.EnvironmentIDs...)
	if input.Scope == "all" {
		rows, err := s.db.Query("SELECT id FROM environments ORDER BY code")
		if err != nil {
			return storageFailure(err)
		}
		for rows.Next() {
			var value string
			if err = rows.Scan(&value); err != nil {
				rows.Close()
				return storageFailure(err)
			}
			ids = append(ids, value)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return storageFailure(err)
		}
	}
	targets := []backupTarget{}
	seen := map[string]bool{}
	for _, environmentID := range ids {
		if seen[environmentID] {
			continue
		}
		seen[environmentID] = true
		_, _, _, err := s.readEnvironment(environmentID)
		if err != nil {
			return failure("NOT_FOUND", "明确所选环境无法核对，未备份其他环境。", false)
		}
		busy, readErr := s.backupBatchBusy(environmentID)
		if readErr != nil || busy {
			return failure("PROFILE_BUSY", "相关批次已受理、排队或受理结果待核实；先结束该任务，备份不抢占其目标。", true)
		}
		if s.backupUses[environmentID] != nil || s.batchUses[environmentID] != nil || s.cookieTasks[environmentID] != nil || s.profileUses[environmentID] && !s.runtimeOwnsProfileUse(environmentID) {
			return failure("PROFILE_BUSY", "范围中有维护、批次或Cookie任务，未抢占它的租约；结束后重试。", true)
		}
		slot := s.runtimeSlots[environmentID]
		if slot != nil && (slot.session.NeedsReconcile || slot.session.PersistencePending) {
			return failure("SESSION_IDENTITY_UNCONFIRMED", "范围中有待核对或观测待保存会话；先确认实际进程树及目录已释放，再备份。", true)
		}
		if s.runtimeOwnsProfileUse(environmentID) && !input.StopRunning {
			return failure("PROFILE_BUSY", "相关环境仍运行；需明确允许正常关闭，未强制结束进程。", true)
		}
		ref, _ := dataReference(environmentID)
		target, err := s.backupDataTarget(environmentID, ref)
		if err != nil {
			return storageFailure(err)
		}
		targets = append(targets, target)
	}
	operationID := id()
	final, temp, err := backup.OutputPaths(s.root, destination.path, operationID)
	if err != nil {
		return failure("PATH_OUTSIDE_ROOT", "备份输出位置不安全，未读取浏览数据。", false)
	}
	operation := Operation{ID: operationID, Kind: "backup-export", State: "accepted", Stage: "accepted", Total: len(targets), CompletedIDs: []string{}, BackupReport: backupReport(input.Scope, filepath.Base(final), len(targets))}
	operation.BackupReport.RequestID = input.RequestID
	task := &backupTask{operation: operation, targets: targets, destination: final, temporary: temp, createdAt: timestamp(), requestID: input.RequestID, signature: signature}
	tx, err := s.db.Begin()
	if err != nil {
		return storageFailure(err)
	}
	defer tx.Rollback()
	opJSON, _ := json.Marshal(operation)
	targetsJSON, _ := json.Marshal(targets)
	result := success(map[string]any{"status": "accepted", "operation": operation}, operation.ID)
	resultJSON, _ := json.Marshal(result)
	if _, err = tx.Exec("INSERT INTO operations(id,result_json) VALUES(?,?)", operation.ID, string(opJSON)); err == nil {
		_, err = tx.Exec("INSERT INTO backup_exports(operation_id,destination,temporary,targets_json,created_at,phase) VALUES(?,?,?,?,?,?)", operation.ID, final, temp, string(targetsJSON), task.createdAt, "accepted")
	}
	if err == nil {
		_, err = tx.Exec("INSERT INTO requests(id,signature,result_json) VALUES(?,?,?)", input.RequestID, signature, string(resultJSON))
	}
	if err == nil && s.options.BeforeCommit != nil {
		err = s.options.BeforeCommit()
	}
	if err != nil {
		return storageFailure(err)
	}
	commitErr := tx.Commit()
	s.backupTasks[operation.ID] = task
	for _, target := range targets {
		s.backupUses[target.ID] = task
		s.profileUses[target.ID] = true
	}
	if commitErr != nil {
		task.acceptancePending = true
		if err = s.confirmBackupAcceptance(task); err != nil {
			result := storageFailure(err)
			if s.backupTasks[operation.ID] != nil {
				result.OperationID = operation.ID
			}
			return result
		}
	} else {
		s.startBackup(task)
	}
	delete(s.backupDestinations, input.DestinationToken)
	observed := copyBackupOperation(task.operation)
	if pending := s.pendingBackupOperation(operation.ID); pending != nil {
		observed = *pending
	}
	return success(map[string]any{"status": "accepted", "operation": observed}, operation.ID)
}

func (s *Service) confirmBackupAcceptance(task *backupTask) error {
	var signature, text, opText, destination, temporary, targetsText, phase, created string
	err := s.db.QueryRow(`SELECT r.signature,r.result_json,o.result_json,b.destination,b.temporary,b.targets_json,b.phase,b.created_at FROM requests r JOIN operations o ON o.id=json_extract(r.result_json,'$.operationId') JOIN backup_exports b ON b.operation_id=o.id WHERE r.id=?`, task.requestID).Scan(&signature, &text, &opText, &destination, &temporary, &targetsText, &phase, &created)
	if err == sql.ErrNoRows {
		var exists bool
		if readErr := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM requests WHERE id=?)", task.requestID).Scan(&exists); readErr != nil {
			return readErr
		}
		if !exists {
			s.releaseBackupUses(task)
			delete(s.backupTasks, task.operation.ID)
		}
		return errors.New("backup acceptance did not commit or journal is unconfirmed")
	}
	if err != nil {
		return err
	}
	var result Result
	if signature != task.signature || json.Unmarshal([]byte(text), &result) != nil || !result.OK || result.OperationID != task.operation.ID {
		return errors.New("backup acceptance identity unconfirmed")
	}
	var stored Operation
	var targets []backupTarget
	if decode(json.RawMessage(opText), &stored) != nil || !reflect.DeepEqual(stored, task.operation) || decode(json.RawMessage(targetsText), &targets) != nil || !reflect.DeepEqual(targets, task.targets) || destination != task.destination || temporary != task.temporary || created != task.createdAt || phase != "accepted" {
		return errors.New("backup acceptance journal unconfirmed")
	}
	acceptedJSON, err := json.Marshal(result.Data)
	if err != nil {
		return err
	}
	var accepted struct {
		Status    string    `json:"status"`
		Operation Operation `json:"operation"`
	}
	if decode(acceptedJSON, &accepted) != nil || accepted.Status != "accepted" || !reflect.DeepEqual(accepted.Operation, stored) {
		return errors.New("backup request operation mismatch")
	}
	task.acceptancePending = false
	s.startBackup(task)
	return nil
}

func (s *Service) backupBatchBusy(environmentID string) (bool, error) {
	pending := []string{}
	for planID := range s.batchTasks {
		pending = append(pending, planID)
	}
	for _, acceptance := range s.batchAcceptances {
		pending = append(pending, acceptance.planID)
	}
	encoded, _ := json.Marshal(pending)
	var busy bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM batch_items i JOIN batch_plans p ON p.id=i.plan_id WHERE p.kind IN ('clone','assign') AND i.state<>'completed' AND json_extract(i.snapshot_json,'$.sourceId')=? AND (p.state IN ('accepted','running') OR p.id IN (SELECT value FROM json_each(?))))`, environmentID, string(encoded)).Scan(&busy)
	return busy, err
}
func (s *Service) startBackup(task *backupTask) {
	if task.started {
		return
	}
	task.started = true
	if s.closed || s.closeRequested.Load() {
		operation := copyBackupOperation(task.operation)
		operation.State, operation.Stage = "failed", "interrupted"
		operation.BackupReport.Sequence++
		operation.Error = &Error{Code: "APPLICATION_INTERRUPTED", Message: "已核实原受理但应用正在退出；未自动复制或发布备份。", Retryable: true}
		task.final, task.finalPending = operation, true
		_ = s.flushOneBackupFinal(task)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	task.cancel = cancel
	s.workers.Add(1)
	input := backupExecution{OperationID: task.operation.ID, Scope: task.operation.BackupReport.Scope, CreatedAt: task.createdAt, Destination: task.destination, Targets: append([]backupTarget(nil), task.targets...)}
	go s.runBackup(ctx, task, input)
}
func (s *Service) cancelBackupOperation(task *backupTask) Result {
	if task.acceptancePending || task.finalPending {
		return success(*s.pendingBackupOperation(task.operation.ID), task.operation.ID)
	}
	if task.publishing || task.recovery {
		return failure("BACKUP_PUBLICATION_IN_PROGRESS", "已进入已核对文件发布阶段，不能用迟到取消回退；请读取发布结果。", true)
	}
	operation := copyBackupOperation(task.operation)
	operation.CancelRequested = true
	operation.BackupReport.Sequence++
	task.operation = operation
	if task.cancel != nil {
		task.cancel()
	}
	if err := s.persistBackup(task, operation, "running"); err != nil {
		result := storageFailure(err)
		result.OperationID = operation.ID
		return result
	}
	return success(operation, operation.ID)
}
