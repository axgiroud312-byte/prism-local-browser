package workspace

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
)

func (s *Service) initializeProxies() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version == 4 {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, statement := range []string{
			`CREATE TABLE proxy_credentials(ref TEXT PRIMARY KEY,protected BLOB NOT NULL)`,
			`CREATE TABLE proxy_request_key(singleton INTEGER PRIMARY KEY CHECK(singleton=1),ref TEXT NOT NULL,protected BLOB NOT NULL)`,
			`CREATE TABLE proxy_config(proxy_id TEXT PRIMARY KEY REFERENCES proxies(id) ON DELETE CASCADE,config_json TEXT NOT NULL,revision INTEGER NOT NULL CHECK(revision>0),check_json TEXT)`,
			`PRAGMA user_version=5`,
		} {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	for _, statement := range []string{"SELECT ref,protected FROM proxy_credentials LIMIT 0", "SELECT singleton,ref,protected FROM proxy_request_key LIMIT 0", "SELECT proxy_id,config_json,revision,check_json FROM proxy_config LIMIT 0"} {
		rows, err := s.db.Query(statement)
		if err != nil {
			return err
		}
		rows.Close()
	}
	return nil
}

func (s *Service) protectProxySecret(ref string, plain []byte) ([]byte, error) {
	if s.options.ProtectProxySecret != nil {
		return s.options.ProtectProxySecret(ref, plain)
	}
	return proxy.Protect(ref, plain)
}
func (s *Service) unprotectProxySecret(ref string, protected []byte) ([]byte, error) {
	if s.options.UnprotectProxySecret != nil {
		return s.options.UnprotectProxySecret(ref, protected)
	}
	return proxy.Unprotect(ref, protected)
}

// Unlike an unkeyed password hash, a persisted HMAC signature does not expose
// a low-entropy credential to offline guessing. Its random key is also DPAPI
// protected; ordinary request records contain neither raw import nor secrets.
func (s *Service) proxySignature(method string, input any) (string, error) {
	if len(s.proxyRequestKey) == 0 {
		var ref string
		var protected []byte
		err := s.db.QueryRow("SELECT ref,protected FROM proxy_request_key WHERE singleton=1").Scan(&ref, &protected)
		if errors.Is(err, sql.ErrNoRows) {
			key := make([]byte, 32)
			if _, err = rand.Read(key); err != nil {
				return "", err
			}
			defer proxy.Wipe(key)
			ref = id()
			protected, err = s.protectProxySecret(ref, key)
			if err != nil {
				return "", err
			}
			if _, err = s.db.Exec("INSERT INTO proxy_request_key(singleton,ref,protected) VALUES(1,?,?)", ref, protected); err != nil {
				return "", err
			}
			s.proxyRequestKey = append([]byte(nil), key...)
		} else if err != nil {
			return "", err
		} else {
			key, err := s.unprotectProxySecret(ref, protected)
			if err != nil || len(key) != 32 {
				proxy.Wipe(key)
				return "", errors.New("protected request key unavailable")
			}
			s.proxyRequestKey = key
		}
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	defer proxy.Wipe(encoded)
	digest := hmac.New(sha256.New, s.proxyRequestKey)
	digest.Write([]byte(method))
	digest.Write([]byte{0})
	digest.Write(encoded)
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func (s *Service) priorProxyRequest(requestID, signature string) (Result, bool) {
	var priorSignature, text string
	err := s.db.QueryRow("SELECT signature,result_json FROM requests WHERE id=?", requestID).Scan(&priorSignature, &text)
	if errors.Is(err, sql.ErrNoRows) {
		return Result{}, false
	}
	if err != nil {
		return storageFailure(err), true
	}
	if !hmac.Equal([]byte(priorSignature), []byte(signature)) {
		return failure("REQUEST_ID_REUSED", "同一请求标识不能用于不同代理操作或凭据。", false), true
	}
	var result Result
	if json.Unmarshal([]byte(text), &result) != nil {
		return failure("STORAGE_READ_FAILED", "代理请求记录无法读取；未重复操作。", true), true
	}
	if result.OperationID != "" {
		if operation, exists := s.proxyResults[result.OperationID]; exists {
			return success(map[string]any{"status": "accepted", "operation": operation}, operation.ID), true
		}
		var encoded string
		if err := s.db.QueryRow("SELECT result_json FROM operations WHERE id=?", result.OperationID).Scan(&encoded); err != nil {
			return storageFailure(err), true
		}
		var operation Operation
		if json.Unmarshal([]byte(encoded), &operation) != nil {
			return failure("STORAGE_READ_FAILED", "代理任务记录无法读取。", true), true
		}
		return success(map[string]any{"status": "accepted", "operation": operation}, operation.ID), true
	}
	return result, true
}

func (s *Service) commitProxyTransaction(tx *sql.Tx, requestID, signature string, result Result, action string) error {
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if _, err := tx.Exec("INSERT INTO requests(id,signature,result_json) VALUES(?,?,?)", requestID, signature, string(encoded)); err != nil {
		return err
	}
	if action != "" {
		if _, err := tx.Exec("INSERT INTO activities(id,created_at,action,target,detail) VALUES(?,?,?,?,?)", id(), timestamp(), action, "本机代理配置", "配置已事务保存，认证仅以受保护引用访问；环境设备身份未重生成。"); err != nil {
			return err
		}
	}
	if s.options.BeforeCommit != nil {
		if err := s.options.BeforeCommit(); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Service) savedProxy(proxyID string) (ProxyView, string, error) {
	var record ProxyView
	var configJSON string
	var ref, checkJSON sql.NullString
	err := s.db.QueryRow("SELECT c.config_json,c.revision,c.check_json,p.credential_ref FROM proxy_config c JOIN proxies p ON p.id=c.proxy_id WHERE c.proxy_id=?", proxyID).Scan(&configJSON, &record.Revision, &checkJSON, &ref)
	if err != nil {
		return record, "", err
	}
	if json.Unmarshal([]byte(configJSON), &record.Configuration) != nil {
		return record, "", errors.New("invalid saved proxy configuration")
	}
	normalized, err := proxy.Normalize(record.Configuration)
	if err != nil || normalized != record.Configuration || record.Revision < 1 {
		return record, "", errors.New("invalid saved proxy configuration")
	}
	record.ID, record.HasAuthentication, record.Status, record.UsedBy = proxyID, ref.Valid && ref.String != "", "unchecked", []string{}
	if checkJSON.Valid {
		var report proxy.Report
		if json.Unmarshal([]byte(checkJSON.String), &report) != nil || report.Mode != "native" || report.ProxyID != proxyID || report.Revision != record.Revision {
			return record, "", errors.New("invalid proxy check identity")
		}
		record.CheckReport = &report
		record.Status = "connected"
		if report.Error != nil {
			record.Status = "failed"
		}
	}
	rows, err := s.db.Query("SELECT id FROM environments WHERE proxy_id=? ORDER BY code", proxyID)
	if err != nil {
		return record, "", err
	}
	defer rows.Close()
	for rows.Next() {
		var environmentID string
		if err := rows.Scan(&environmentID); err != nil {
			return record, "", err
		}
		record.UsedBy = append(record.UsedBy, environmentID)
	}
	return record, ref.String, rows.Err()
}
func (s *Service) listProxies() ([]ProxyView, error) {
	rows, err := s.db.Query("SELECT proxy_id FROM proxy_config ORDER BY rowid DESC")
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, value)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	records := []ProxyView{}
	for _, value := range ids {
		record, _, err := s.savedProxy(value)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}
func (s *Service) storedProxyCredentials(ref string) (*proxy.Credentials, error) {
	if ref == "" {
		return nil, nil
	}
	var protected []byte
	if err := s.db.QueryRow("SELECT protected FROM proxy_credentials WHERE ref=?", ref).Scan(&protected); err != nil {
		return nil, err
	}
	plain, err := s.unprotectProxySecret(ref, protected)
	if err != nil {
		return nil, err
	}
	defer proxy.Wipe(plain)
	var credentials proxy.Credentials
	if json.Unmarshal(plain, &credentials) != nil || proxy.ValidateCredentials(credentials) != nil {
		return nil, errors.New("protected proxy credentials invalid")
	}
	return &credentials, nil
}
func (s *Service) insertProxyCredentials(tx *sql.Tx, credentials *proxy.Credentials) (string, error) {
	if credentials == nil {
		return "", nil
	}
	plain, err := json.Marshal(credentials)
	if err != nil {
		return "", err
	}
	defer proxy.Wipe(plain)
	ref := id()
	protected, err := s.protectProxySecret(ref, plain)
	if err != nil {
		return "", err
	}
	_, err = tx.Exec("INSERT INTO proxy_credentials(ref,protected) VALUES(?,?)", ref, protected)
	return ref, err
}
