package workspace

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
)

func batchName(prefix string, index int64) string {
	return prefix + " " + strconv.FormatInt(index+1, 10)
}
func batchProxy(query profileQuery, proxyID string) (string, int64, error) {
	if proxyID == "" {
		return "明确直连（不绑定代理）", 0, nil
	}
	var text string
	var revision int64
	if err := query.QueryRow(`SELECT config_json,revision FROM proxy_config WHERE proxy_id=?`, proxyID).Scan(&text, &revision); err != nil {
		return "", 0, err
	}
	var config proxy.Configuration
	if decode(json.RawMessage(text), &config) != nil {
		return "", 0, fmt.Errorf("invalid proxy")
	}
	normalized, err := proxy.Normalize(config)
	if err != nil || config != normalized || revision < 1 {
		return "", 0, fmt.Errorf("invalid proxy")
	}
	return config.Name, revision, nil
}
func (s *Service) batchCall(request Request) Result {
	switch request.Method {
	case "Batch.Preview":
		var input BatchPreviewRequest
		if decode(request.Payload, &input) != nil {
			return failure("VALIDATION_FAILED", "批次预览只接受配置、明确源ID或逐项代理映射，不接受身份、路径或登录数据。", false)
		}
		return s.previewBatch(input)
	case "Batch.ReadPage":
		var input struct {
			PlanID      string `json:"planId"`
			OperationID string `json:"operationId,omitempty"`
			Offset      int64  `json:"offset"`
			PageSize    int    `json:"pageSize"`
		}
		if decode(request.Payload, &input) != nil || input.Offset < 0 || input.Offset > maxSafeInteger || input.PageSize < 1 || input.PageSize > 100 {
			return failure("VALIDATION_FAILED", "批次分页参数无效；每页1–100只是传输边界，不是总数配额。", false)
		}
		plan, err := readBatchPlan(s.db, input.PlanID)
		if err != nil {
			return failure("NOT_FOUND", "批次计划无法读取。", true)
		}
		page, err := s.batchResultPage(plan, input.OperationID, input.Offset, input.PageSize)
		if err != nil {
			return storageFailure(err)
		}
		return success(page, page.OperationID)
	case "Batch.Commit":
		var input struct {
			PlanID    string `json:"planId"`
			RequestID string `json:"requestId"`
		}
		if decode(request.Payload, &input) != nil {
			return failure("VALIDATION_FAILED", "批次提交字段无效。", false)
		}
		return s.acceptBatch(request.Method, input.PlanID, "", input.RequestID)
	case "Batch.Retry":
		var input struct {
			OperationID string `json:"operationId"`
			RequestID   string `json:"requestId"`
		}
		if decode(request.Payload, &input) != nil {
			return failure("VALIDATION_FAILED", "批次继续字段无效。", false)
		}
		return s.acceptBatch(request.Method, "", input.OperationID, input.RequestID)
	default:
		return failure("CAPABILITY_UNSUPPORTED", "批次方法未支持，未改环境。", false)
	}
}

func (s *Service) previewBatch(input BatchPreviewRequest) Result {
	if input.Kind != "create" && input.Kind != "clone" && input.Kind != "assign" || input.Kind != "create" && input.Create != nil || input.Kind != "clone" && len(input.SourceIDs) != 0 || input.Kind != "assign" && len(input.Mappings) != 0 {
		return failure("VALIDATION_FAILED", "批次类型与输入不一致。", false)
	}
	plan := batchPlan{ID: id(), Kind: input.Kind, State: "preview", ExpiresAt: time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339Nano)}
	snapshots := []batchSnapshot{}
	if input.Kind == "create" {
		if input.Create == nil || input.Create.Count < 1 || int64(input.Create.Count) > maxSafeInteger {
			return failure("VALIDATION_FAILED", "数量必须是可精确表示的正整数，没有环境总数产品配额。", false)
		}
		mutation := *input.Create
		d, exists := s.drafts[mutation.PreviewID]
		if !exists || d.Kind != "create" {
			return failure("PREVIEW_EXPIRED", "创建草稿已失效，请重新打开。", true)
		}
		mutation.Configuration.Name = strings.TrimSpace(mutation.Configuration.Name)
		if message := validate(mutation.Configuration); message != "" {
			return failure("VALIDATION_FAILED", message, false)
		}
		if mutation.Configuration.Seed != d.Preview.Environment.Seed {
			return failure("VALIDATION_FAILED", "首个身份需来自原生草稿；其他条目会分配独立新身份。", false)
		}
		tx, err := s.db.Begin()
		if err != nil {
			return storageFailure(err)
		}
		profile, result := s.mutationProfile(tx, d, mutation, true)
		_ = tx.Rollback()
		if !result.OK {
			return result
		}
		name, revision, err := batchProxy(s.db, mutation.Configuration.ProxyID)
		if err != nil {
			return failure("PROXY_UNAVAILABLE", "所选代理引用无效，未改为直连。", true)
		}
		plan.Total = int64(mutation.Count)
		plan.Template = batchSnapshot{Configuration: mutation.Configuration, Profile: profile, ProxyName: name, ProxyRevision: revision}
	} else {
		ids := input.SourceIDs
		if input.Kind == "assign" {
			for _, mapping := range input.Mappings {
				ids = append(ids, mapping.EnvironmentID)
			}
		}
		if len(ids) == 0 {
			return failure("VALIDATION_FAILED", "请选择明确的环境ID，筛选不能扩大范围。", false)
		}
		seen, names := map[string]bool{}, map[string]bool{}
		for index, environmentID := range ids {
			if seen[environmentID] {
				return failure("VALIDATION_FAILED", "同一批次不能重复同一环境ID。", false)
			}
			seen[environmentID] = true
			environment, revision, profileID, err := s.readEnvironment(environmentID)
			if err != nil {
				return failure("NOT_FOUND", "所选环境或完整档案不存在，请重新读取。", true)
			}
			profile, err := readProfileFrom(s.db, profileID)
			if err != nil {
				return failure("STORAGE_READ_FAILED", "源固定档案无法读取，未复制登录数据。", true)
			}
			snapshot := batchSnapshot{Configuration: environment.Configuration, Profile: profile.Profile, SourceID: environmentID, ExpectedRevision: revision}
			if input.Kind == "assign" {
				snapshot.Configuration.ProxyID = input.Mappings[index].ProxyID
			} else {
				base := environment.Name + " 副本"
				snapshot.Configuration.Name = base
				for suffix := int64(2); ; suffix++ {
					var exists int
					if err = s.db.QueryRow("SELECT COUNT(*) FROM environments WHERE name=?", snapshot.Configuration.Name).Scan(&exists); err != nil {
						return storageFailure(err)
					}
					if exists == 0 && !names[snapshot.Configuration.Name] {
						break
					}
					snapshot.Configuration.Name = base + " " + strconv.FormatInt(suffix, 10)
				}
				names[snapshot.Configuration.Name] = true
				snapshot.Configuration.Note = ""
			}
			name, proxyRevision, err := batchProxy(s.db, snapshot.Configuration.ProxyID)
			if err != nil {
				return failure("PROXY_UNAVAILABLE", "映射中的代理引用无效，未复用其他节点或直连。", true)
			}
			snapshot.ProxyName, snapshot.ProxyRevision = name, proxyRevision
			snapshots = append(snapshots, snapshot)
		}
		plan.Total = int64(len(snapshots))
	}
	proxyAssignments := map[string]int64{}
	if plan.Kind == "create" {
		if plan.Template.Configuration.ProxyID != "" {
			proxyAssignments[plan.Template.Configuration.ProxyID] = plan.Total
		}
	} else {
		for _, snapshot := range snapshots {
			if snapshot.Configuration.ProxyID != "" {
				proxyAssignments[snapshot.Configuration.ProxyID]++
			}
		}
	}
	plan.Direct = plan.Total
	for _, count := range proxyAssignments {
		plan.Direct -= count
	}
	for proxyID, count := range proxyAssignments {
		var used int64
		if err := s.db.QueryRow("SELECT COUNT(*) FROM environments WHERE proxy_id=?", proxyID).Scan(&used); err != nil {
			return storageFailure(err)
		}
		if used > 0 || count > 1 {
			plan.Shared += count
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return storageFailure(err)
	}
	defer tx.Rollback()
	encoded, _ := json.Marshal(plan.Template)
	if _, err = tx.Exec(`INSERT INTO batch_plans(id,kind,total,template_json,expires_at,state,shared,direct) VALUES(?,?,?,?,?,'preview',?,?)`, plan.ID, plan.Kind, plan.Total, string(encoded), plan.ExpiresAt, plan.Shared, plan.Direct); err != nil {
		return storageFailure(err)
	}
	for index, snapshot := range snapshots {
		encoded, _ = json.Marshal(snapshot)
		if _, err = tx.Exec(`INSERT INTO batch_items(plan_id,item_index,snapshot_json,state) VALUES(?,?,?,'not-executed')`, plan.ID, index, string(encoded)); err != nil {
			return storageFailure(err)
		}
	}
	if s.options.BeforeCommit != nil {
		if err = s.options.BeforeCommit(); err != nil {
			return storageFailure(err)
		}
	}
	if err = tx.Commit(); err != nil {
		return storageFailure(err)
	}
	page, err := s.batchPage(plan, 0, batchPageSize)
	if err != nil {
		return storageFailure(err)
	}
	return success(page, "")
}

func (s *Service) batchPage(plan batchPlan, offset int64, pageSize int) (BatchPage, error) {
	page := BatchPage{BatchReport: batchReport(plan), Offset: offset, PageSize: pageSize, ExpiresAt: plan.ExpiresAt, OperationID: plan.OperationID, CurrentOperationID: plan.OperationID, Items: []BatchItem{}}
	for index := offset; index < plan.Total && index-offset < int64(pageSize); index++ {
		stored, err := s.readBatchItem(plan, index)
		if err != nil {
			return page, err
		}
		item := projectBatchItem(plan, stored)
		if plan.State == "preview" && stored.Snapshot.SourceID != "" && (s.profileUses[stored.Snapshot.SourceID] || s.runtimeOwnsProfileUse(stored.Snapshot.SourceID)) {
			item.Error = &Error{Code: "PROFILE_BUSY", Message: "当前环境正运行或维护；执行时仍忙会逐项失败，不会停止或改其代理。", Retryable: true}
		}
		page.Items = append(page.Items, item)
	}
	return page, nil
}

func projectBatchItem(plan batchPlan, stored batchStoredItem) BatchItem {
	snapshot := stored.Snapshot
	item := BatchItem{Index: stored.Index, Name: snapshot.Configuration.Name, ProxyID: snapshot.Configuration.ProxyID, ProxyName: snapshot.ProxyName, NewIdentity: plan.Kind != "assign", State: stored.State, Error: stored.Error}
	if item.State == "prepared" {
		item.State = "not-executed"
	}
	if plan.Kind == "clone" {
		item.SourceID, item.SourceRevision = snapshot.SourceID, snapshot.ExpectedRevision
	}
	if plan.Kind == "assign" {
		item.EnvironmentID, item.ExpectedRevision = snapshot.SourceID, snapshot.ExpectedRevision
	}
	if stored.Identity != nil {
		item.EnvironmentID = stored.Identity.EnvironmentID
	}
	return item
}
