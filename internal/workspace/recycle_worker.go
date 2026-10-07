package workspace

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

func recycleCounts(op Operation, plan recyclePlan) Operation {
	op = copyRecycleOperation(op)
	r := op.RecycleReport
	r.Sequence++
	r.Completed = 0
	r.Failed = 0
	r.NotExecuted = 0
	for _, item := range plan.Items {
		if item.Committed {
			r.Completed++
		} else if item.Result == "failed" || item.Result == "protected" {
			r.Failed++
		} else {
			r.NotExecuted++
		}
	}
	return op
}
func (s *Service) recycleCheckpoint(phase string) error {
	if s.options.RecycleCheckpoint != nil {
		return s.options.RecycleCheckpoint(phase)
	}
	return nil
}
func (s *Service) observeRecycle(task *recycleTask, plan recyclePlan, phase string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.recycleTask != task {
		return errors.New("recycle owner changed")
	}
	if phase == "purging" && (task.operation.CancelRequested || s.closed || s.closeRequested.Load()) {
		return context.Canceled
	}
	op := recycleCounts(task.operation, plan)
	op.State = "running"
	op.Stage = phase
	op.PersistencePending = true
	op.RecycleReport.Protected = true
	if err := s.persistRecycle(plan, op, phase); err != nil {
		return err
	}
	task.plan = copyRecyclePlan(plan)
	task.operation = op
	task.phase = phase
	return nil
}
func (s *Service) startRecycle(task *recycleTask, recovering bool) {
	if task.running {
		return
	}
	task.running = true
	ctx, cancel := context.WithCancel(context.Background())
	task.cancel = cancel
	plan := copyRecyclePlan(task.plan)
	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		defer cancel()
		index := -1
		var cause error
		if !recovering {
			index, cause = s.executeRecycle(ctx, task, &plan)
		} else {
			cause = errors.New("application interrupted recycle operation")
		}
		s.finishRecycle(task, index, cause)
	}()
}
func (s *Service) executeRecycle(ctx context.Context, task *recycleTask, plan *recyclePlan) (int, error) {
	for index := range plan.Items {
		if err := ctx.Err(); err != nil {
			return index, err
		}
		item := &plan.Items[index]
		entry := &item.Entry
		targetID := entry.Item.EnvironmentID
		s.mu.Lock()
		err := s.recycleTargetFree(targetID)
		baseline, readErr := recycleBaseline(s.db, targetID)
		s.mu.Unlock()
		if err != nil {
			return index, err
		}
		if readErr != nil || baseline != entry.Baseline {
			return index, errors.New("recycle configuration changed")
		}
		if plan.Action == "remove" {
			s.mu.Lock()
			target, err := s.backupDataTarget(targetID, entry.Reference)
			s.mu.Unlock()
			if err != nil {
				return index, err
			}
			profile, err := backup.CaptureProfile(ctx, s.root, targetID, entry.Reference, !target.DirectoryRequired, target.NeverUsed)
			if err != nil {
				return index, err
			}
			if profile.Missing == entry.Item.DataPresent {
				profile.Close()
				return index, errors.New("recycle data presence changed")
			}
			if !profile.Missing {
				identity, present, identityErr := backup.IdentifyTree(s.root, entry.Reference)
				if identityErr != nil || !present || identity != entry.Identity {
					profile.Close()
					return index, errors.New("recycle source identity changed")
				}
				entry.Files, err = profile.Inventory(ctx)
				if err == nil {
					entry.Lock, err = profile.RetainedLock(ctx)
				}
			}
			closeErr := profile.Close()
			if err != nil {
				return index, err
			}
			if closeErr != nil {
				return index, closeErr
			}
		} else if entry.Item.DataPresent {
			if err := backup.VerifyTree(ctx, s.root, recycleReference(entry.TrashID), entry.Identity, entry.Files); err != nil {
				return index, err
			}
		}
		for _, reference := range []string{entry.Reference, recycleReference(entry.TrashID)} {
			pin, err := kernel.EnsureDirectory(s.root, filepath.ToSlash(filepath.Dir(reference)))
			if err != nil {
				return index, err
			}
			pin()
		}
		item.Prepared = true
		if err := s.observeRecycle(task, *plan, "prepared"); err != nil {
			return index, err
		}
		if err := s.recycleCheckpoint("prepared"); err != nil {
			return index, err
		}
		if plan.Action == "purge" {
			item.PurgeStarted = true
			if err := s.observeRecycle(task, *plan, "purging"); err != nil {
				return index, err
			}
			if err := s.recycleCheckpoint("purging"); err != nil {
				return index, err
			}
			if err := s.deleteRecycleData(ctx, *entry); err != nil {
				return index, err
			}
			item.DataDeleted = true
			if err := s.observeRecycle(task, *plan, "data-deleted"); err != nil {
				return index, err
			}
			if err := s.recycleCheckpoint("data-deleted"); err != nil {
				return index, err
			}
		} else {
			from, to := entry.Reference, recycleReference(entry.TrashID)
			if plan.Action == "restore" {
				from, to = to, from
			}
			if _, present, err := backup.IdentifyTree(s.root, to); err != nil || present {
				return index, errors.New("recycle destination occupied")
			}
			if entry.Item.DataPresent {
				if err := backup.MoveTree(ctx, s.root, from, to, entry.Identity, entry.Files); err != nil {
					return index, err
				}
			}
		}
		if err := s.recycleCheckpoint("directory-changed"); err != nil {
			return index, err
		}
		if err := ctx.Err(); err != nil {
			return index, err
		}
		s.mu.Lock()
		err = s.commitRecycleItem(task, plan, index)
		s.mu.Unlock()
		if err != nil {
			return index, err
		}
		if err := s.recycleCheckpoint("item-committed"); err != nil {
			return index, err
		}
	}
	return -1, nil
}
func (s *Service) deleteRecycleData(ctx context.Context, entry recycleEntry) error {
	if !entry.Item.DataPresent {
		if _, exists, err := backup.IdentifyTree(s.root, recycleReference(entry.TrashID)); err != nil || exists {
			return errors.New("unexpected retained tree protected")
		}
		return nil
	}
	return backup.DeleteRetainedTree(ctx, s.root, recycleReference(entry.TrashID), entry.Identity, entry.Files, entry.Lock, true)
}
func (s *Service) recoverRecycleItem(ctx context.Context, task *recycleTask, plan *recyclePlan, index int) error {
	item := &plan.Items[index]
	entry := item.Entry
	s.mu.Lock()
	baseline, err := recycleBaseline(s.db, entry.Item.EnvironmentID)
	want := entry.Baseline
	if item.Committed {
		want = item.AfterBaseline
	}
	if err != nil || baseline != want {
		s.mu.Unlock()
		return errors.New("recycle configuration outcome differs")
	}
	wantTrash := plan.Action != "remove"
	if item.Committed {
		wantTrash = plan.Action == "remove"
	}
	membershipEntry := entry
	if item.Committed && plan.Action == "remove" {
		membershipEntry.Baseline = item.AfterBaseline
	}
	err = recycleMembership(s.db, membershipEntry, wantTrash, item.Committed && plan.Action == "remove")
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if !item.Prepared {
		return nil
	}
	from, to := entry.Reference, recycleReference(entry.TrashID)
	if plan.Action == "restore" {
		from, to = to, from
	}
	if plan.Action == "purge" {
		if item.Committed || item.DataDeleted {
			if _, exists, err := backup.IdentifyTree(s.root, to); err != nil || exists {
				return errors.New("deleted tree location is no longer absent")
			}
		} else if item.PurgeStarted {
			if err := s.deleteRecycleData(ctx, entry); err != nil {
				return err
			}
			item.DataDeleted = true
			if err := s.observeRecycle(task, *plan, "data-deleted"); err != nil {
				return err
			}
		} else {
			return s.verifyRecycleSide(ctx, entry, to, from)
		}
		if !item.Committed {
			s.mu.Lock()
			err = s.commitRecycleItem(task, plan, index)
			s.mu.Unlock()
			return err
		}
		return nil
	}
	if item.Committed {
		return s.verifyRecycleSide(ctx, entry, to, from)
	}
	if !entry.Item.DataPresent {
		return s.verifyRecycleSide(ctx, entry, from, to)
	}
	original, originalExists, err := backup.IdentifyTree(s.root, from)
	if err != nil {
		return err
	}
	moved, movedExists, err := backup.IdentifyTree(s.root, to)
	if err != nil {
		return err
	}
	if originalExists {
		if original != entry.Identity || movedExists {
			return errors.New("unknown or duplicate recycle directory")
		}
	} else {
		if !movedExists || moved != entry.Identity {
			return errors.New("original recycle tree missing")
		}
		if err := backup.MoveTree(ctx, s.root, to, from, entry.Identity, entry.Files); err != nil {
			return err
		}
	}
	return s.verifyRecycleSide(ctx, entry, from, to)
}
func (s *Service) verifyRecycleSide(ctx context.Context, entry recycleEntry, present, absent string) error {
	if _, exists, err := backup.IdentifyTree(s.root, absent); err != nil || exists {
		return errors.New("unexpected opposite recycle tree")
	}
	if !entry.Item.DataPresent {
		if _, exists, err := backup.IdentifyTree(s.root, present); err != nil || exists {
			return errors.New("expected empty recycle reference changed")
		}
		return nil
	}
	return backup.VerifyTree(ctx, s.root, present, entry.Identity, entry.Files)
}
func (s *Service) finishRecycle(task *recycleTask, failedIndex int, cause error) {
	s.mu.Lock()
	plan, _, _, readErr := s.readRecycleJournal(task.plan.ID)
	s.mu.Unlock()
	protected := readErr != nil
	if readErr != nil {
		s.mu.Lock()
		plan = copyRecyclePlan(task.plan)
		s.mu.Unlock()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if !protected {
		for index := range plan.Items {
			if err := s.recoverRecycleItem(ctx, task, &plan, index); err != nil {
				protected = true
				plan.Items[index].Result = "protected"
				plan.Items[index].Error = recycleProblem(err).Error
				break
			}
			item := &plan.Items[index]
			if item.Committed {
				item.Result = map[string]string{"remove": "recycled", "restore": "restored", "purge": "purged"}[plan.Action]
			} else if item.Prepared || index == failedIndex {
				item.Result = "failed"
				item.Error = &Error{Code: "RECYCLE_INCOMPLETE", Message: "本项未完成，已核对原配置和数据位置；没有换身份。", Retryable: true}
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.recycleTask != task {
		return
	}
	task.running = false
	task.cancel = nil
	task.plan = copyRecyclePlan(plan)
	op := recycleCounts(task.operation, plan)
	op.State = "completed"
	op.Stage = "finished"
	op.PersistencePending = false
	op.RecycleReport.Protected = false
	op.Error = nil
	if op.RecycleReport.Completed != op.Total {
		op.State = "failed"
		op.Error = &Error{Code: "RECYCLE_INCOMPLETE", Message: "操作未全部完成，已完成项按记录保留，其余未执行或已回到原状态；请查看逐项结果。", Retryable: true}
		if errors.Is(cause, context.Canceled) {
			op.State = "cancelled"
		}
	}
	if protected {
		op.State = "failed"
		op.Stage = "protected"
		op.PersistencePending = true
		op.RecycleReport.Protected = true
		op.Error = recycleProblem(errors.New("unconfirmed")).Error
		// Never overwrite a journal after a failed read/unknown COMMIT. The next
		// retry must read the original durable decision again, not this overlay.
		task.operation = op
		return
	}
	if task.startup {
		task.operation = op
		s.scheduleRecycleBootstrap(task)
		return
	}
	if err := s.persistRecycle(plan, op, "finished"); err != nil {
		final := copyRecycleOperation(op)
		task.finalPending = &final
		op.State = "failed"
		op.Stage = "storage-pending"
		op.PersistencePending = true
		op.RecycleReport.Protected = true
		op.Error = recycleProblem(err).Error
		task.operation = op
		return
	}
	task.operation = op
	task.phase = "finished"
	s.releaseRecycle(task)
}
func (s *Service) releaseRecycle(task *recycleTask) {
	if s.recycleTask != task {
		return
	}
	for _, item := range task.plan.Items {
		if item.Committed {
			delete(s.runtimeSlots, item.Entry.Item.EnvironmentID)
			delete(s.runtimePending, item.Entry.Item.EnvironmentID)
		}
	}
	s.recycleTask = nil
	s.recycleDraft = nil
	s.drafts = map[string]draft{}
	s.restorePreview = nil
}
