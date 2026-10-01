package workspace

import (
	"database/sql"
	"encoding/json"
	"errors"
)

func (s *Service) batchResultPage(plan batchPlan, operationID string, offset int64, pageSize int) (BatchPage, error) {
	if operationID == "" || operationID == plan.OperationID {
		return s.batchPage(plan, offset, pageSize)
	}
	var text string
	if err := s.db.QueryRow(`SELECT result_json FROM operations WHERE id=?`, operationID).Scan(&text); err != nil {
		return BatchPage{}, err
	}
	var operation Operation
	if json.Unmarshal([]byte(text), &operation) != nil || operation.BatchReport == nil || operation.BatchReport.PlanID != plan.ID || operation.Kind != "batch-"+plan.Kind || operation.BatchReport.Total != plan.Total {
		return BatchPage{}, errors.New("batch history does not match plan")
	}
	page := BatchPage{BatchReport: *operation.BatchReport, Offset: offset, PageSize: pageSize, ExpiresAt: plan.ExpiresAt, OperationID: operationID, CurrentOperationID: plan.OperationID, History: true, Items: []BatchItem{}}
	for index := offset; index < plan.Total && index-offset < int64(pageSize); index++ {
		// Own attempt observations plus earlier completed rows. A prior failure
		// that this attempt never executed must NOT become its failure result.
		err := s.db.QueryRow(`SELECT v.item_json FROM batch_item_events v JOIN operations o ON o.id=v.operation_id WHERE v.plan_id=? AND v.item_index=? AND (v.operation_id=? OR (v.state='completed' AND o.rowid<=(SELECT rowid FROM operations WHERE id=?))) ORDER BY o.rowid DESC LIMIT 1`, plan.ID, index, operationID, operationID).Scan(&text)
		var item BatchItem
		if err == nil {
			if decode(json.RawMessage(text), &item) != nil || item.Index != index {
				return page, errors.New("invalid historical batch item")
			}
		} else if err == sql.ErrNoRows {
			stored, readErr := s.readBatchItem(plan, index)
			if readErr != nil {
				return page, readErr
			}
			stored.Identity, stored.Error, stored.State = nil, nil, "not-executed"
			item = projectBatchItem(plan, stored)
		} else {
			return page, err
		}
		page.Items = append(page.Items, item)
	}
	return page, nil
}
