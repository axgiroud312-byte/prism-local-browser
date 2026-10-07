package workspace

import (
	"encoding/json"
	"fmt"
	"strings"
)

type EnvironmentQuery struct {
	Page     int64  `json:"page"`
	PageSize int    `json:"pageSize"`
	Search   string `json:"search"`
	Group    string `json:"group"`
	Status   string `json:"status"`
}
type EnvironmentPage struct {
	Page          int64    `json:"page"`
	PageSize      int      `json:"pageSize"`
	Total         int64    `json:"total"`
	FilteredTotal int64    `json:"filteredTotal"`
	Groups        []string `json:"groups"`
	RunningCount  int64    `json:"runningCount"`
	ErrorCount    int64    `json:"errorCount"`
}

func (s *Service) environmentIDs(query EnvironmentQuery) ([]string, EnvironmentPage, error) {
	page := EnvironmentPage{Page: query.Page, PageSize: query.PageSize, Groups: []string{}}
	if query.Page < 1 || query.Page > maxSafeInteger || query.PageSize < 1 || query.PageSize > 100 || query.Status != "" && query.Status != "all" && !strings.Contains("|ready|starting|running|stopping|error|", "|"+query.Status+"|") {
		return nil, page, fmt.Errorf("invalid environment page")
	}
	// In-memory pending observations override stale stored session states in
	// filters/counts too, not only after a row has already entered the page.
	cte := "WITH observed(environment_id,record_json) AS (SELECT NULL,NULL WHERE 0) "
	args := []any{}
	if len(s.runtimePending) != 0 {
		values := []string{}
		for environmentID, pending := range s.runtimePending {
			encoded, _ := json.Marshal(pending.Slot.session)
			values = append(values, "(?,?)")
			args = append(args, environmentID, string(encoded))
		}
		cte = "WITH observed(environment_id,record_json) AS (VALUES " + strings.Join(values, ",") + ") "
	}
	from := ` FROM (SELECT * FROM environments WHERE NOT EXISTS(SELECT 1 FROM environment_trash WHERE environment_id=environments.id)) e JOIN fingerprints f ON f.id=e.fingerprint_id LEFT JOIN runtime_sessions r ON r.environment_id=e.id LEFT JOIN observed v ON v.environment_id=e.id `
	status := `COALESCE(json_extract(v.record_json,'$.state'),json_extract(r.record_json,'$.state'),'ready')`
	if err := s.db.QueryRow(cte+"SELECT COUNT(*),COALESCE(SUM("+status+"='running'),0),COALESCE(SUM("+status+"='error'),0)"+from, args...).Scan(&page.Total, &page.RunningCount, &page.ErrorCount); err != nil {
		return nil, page, err
	}
	where := ` WHERE (?='' OR instr(lower(e.name||' '||printf('%03d',e.code)||' '||COALESCE(json_extract(f.config_json,'$.note'),'')),lower(?))>0) AND (?='' OR json_extract(f.config_json,'$.group')=?) AND (?='' OR ` + status + `=?)`
	filterStatus := query.Status
	if filterStatus == "all" {
		filterStatus = ""
	}
	filteredArgs := append(append([]any{}, args...), query.Search, query.Search, query.Group, query.Group, filterStatus, filterStatus)
	if err := s.db.QueryRow(cte+"SELECT COUNT(*)"+from+where, filteredArgs...).Scan(&page.FilteredTotal); err != nil {
		return nil, page, err
	}
	lastPage := (page.FilteredTotal + int64(query.PageSize) - 1) / int64(query.PageSize)
	if lastPage < 1 {
		lastPage = 1
	}
	if page.Page > lastPage {
		page.Page = lastPage
	}
	rows, err := s.db.Query(cte+"SELECT e.id"+from+where+" ORDER BY e.code DESC LIMIT ? OFFSET ?", append(filteredArgs, query.PageSize, (page.Page-1)*int64(query.PageSize))...)
	if err != nil {
		return nil, page, err
	}
	ids := []string{}
	for rows.Next() {
		var environmentID string
		if err = rows.Scan(&environmentID); err != nil {
			rows.Close()
			return nil, page, err
		}
		ids = append(ids, environmentID)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, page, err
	}
	rows, err = s.db.Query(`SELECT DISTINCT json_extract(f.config_json,'$.group') FROM fingerprints f JOIN environments e ON e.fingerprint_id=f.id WHERE json_extract(f.config_json,'$.group')<>'' AND NOT EXISTS(SELECT 1 FROM environment_trash WHERE environment_id=e.id) ORDER BY 1`)
	if err != nil {
		return nil, page, err
	}
	for rows.Next() {
		var group string
		if err = rows.Scan(&group); err != nil {
			rows.Close()
			return nil, page, err
		}
		page.Groups = append(page.Groups, group)
	}
	err = rows.Err()
	rows.Close()
	return ids, page, err
}
