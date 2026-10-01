package workspace

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
	"modernc.org/sqlite"
)

// Options is host-injected, never accepted through Wails. Hooks are test seams;
// the desktop sets only ChooseArchive and always installs/probes real bytes.
type Options struct {
	BeforeCommit  func() error
	ChooseArchive func() (string, error)
	// Test seam only; the desktop always uses kernel.Prepare and a real probe.
	PrepareKernel func(context.Context, string, kernel.InstallInput, string, kernel.ProbeFunc, kernel.ProgressFunc) (*kernel.Prepared, error)
	VerifyKernel  func(context.Context, string, kernel.Record, string) (kernel.Report, error)
	// Test seam only. The desktop always launches the verified real process.
	LaunchRuntime func(context.Context, RuntimeLaunch) (RuntimeProcess, error)
}
type draft struct {
	Kind        string
	Preview     Preview
	BaseProfile *DeviceProfile
}
type Service struct {
	mu           sync.Mutex
	db           *sql.DB
	drafts       map[string]draft
	options      Options
	root         string
	archives     map[string]string
	kernelTask   *kernelTask
	workers      sync.WaitGroup
	closed       bool
	profileUses  map[string]bool
	runtimeSlots map[string]*runtimeSlot
	startGate    chan struct{}
	closeDone    chan struct{}
	closeError   error
}

func failure(code, message string, retryable bool) Result {
	return Result{Mode: "native", Error: &Error{Code: code, Message: message, Retryable: retryable}}
}
func success(data any, operationID string) Result {
	return Result{OK: true, Mode: "native", Data: data, OperationID: operationID}
}
func storageFailure(err error) Result {
	var sqliteError *sqlite.Error
	if errors.As(err, &sqliteError) && sqliteError.Code()&255 == 13 {
		return failure("DISK_FULL", "磁盘空间不足，本次修改未保存，原档案仍在。请释放空间后重试。", true)
	}
	return failure("STORAGE_WRITE_FAILED", "本地数据库写入失败，本次修改未保存，原档案仍在。请修复磁盘或权限后重试。", true)
}
func id() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("secure random unavailable")
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func timestamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func Open(root string, options Options) (*Service, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(absolute, 0700); err != nil {
		return nil, err
	}
	dbPath := filepath.ToSlash(filepath.Join(absolute, "app.db"))
	if !strings.HasPrefix(dbPath, "/") {
		dbPath = "/" + dbPath
	}
	uri := url.URL{Scheme: "file", Path: dbPath}
	query := uri.Query()
	// These pragmas apply to every connection, including one replaced after an I/O failure.
	for _, pragma := range []string{"foreign_keys(1)", "busy_timeout(5000)", "synchronous(FULL)"} {
		query.Add("_pragma", pragma)
	}
	uri.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Service{db: db, root: absolute, drafts: map[string]draft{}, archives: map[string]string{}, profileUses: map[string]bool{}, runtimeSlots: map[string]*runtimeSlot{}, startGate: make(chan struct{}, 1), options: options}
	if err = s.initialize(); err != nil {
		db.Close()
		return nil, err
	}
	if err = s.recoverKernelOperations(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}
func (s *Service) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return s.CloseContext(ctx)
}

// Cancelling a waiter never abandons cleanup or claims the database is closed.
// Every caller waits on the same completion; a timed-out close can be retried.
func (s *Service) CloseContext(ctx context.Context) error {
	s.mu.Lock()
	if s.closeDone == nil {
		s.closeDone = make(chan struct{})
		s.closed = true
		if s.kernelTask != nil {
			s.kernelTask.cancel()
		}
		processes := []RuntimeProcess{}
		for _, slot := range s.runtimeSlots {
			slot.cancel()
			if slot.process != nil {
				processes = append(processes, slot.process)
			}
		}
		go s.closeResources(processes, s.closeDone)
	}
	finished := s.closeDone
	s.mu.Unlock()
	select {
	case <-finished:
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.closeError
	case <-ctx.Done():
		return errors.New("controlled sessions have not all exited; cleanup continues and its resources are retained")
	}
}

func (s *Service) closeResources(processes []RuntimeProcess, finished chan struct{}) {
	var shutdown sync.WaitGroup
	for _, process := range processes {
		shutdown.Add(1)
		go func(process RuntimeProcess) {
			defer shutdown.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			_ = process.Stop(ctx)
			cancel()
			// App shutdown releases only our owned Job tree, never a PID/name search.
			_ = process.Close()
		}(process)
	}
	shutdown.Wait()
	s.workers.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeError = s.db.Close()
	close(finished)
}
func (s *Service) initialize() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > 3 {
		return errors.New("unsupported workspace version")
	}
	if version == 3 {
		return s.checkSchema()
	}
	if version == 2 {
		if err := s.checkKernelSchema(); err != nil {
			return err
		}
		return s.migrateFingerprints()
	}
	if version == 1 {
		if err := s.checkBaseSchema(); err != nil {
			return err
		}
		if err := s.migrateKernels(); err != nil {
			return err
		}
		return s.migrateFingerprints()
	}
	var tables int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'").Scan(&tables); err != nil {
		return err
	}
	if tables != 0 {
		return errors.New("unversioned database is not a native workspace")
	}
	if _, err := s.db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statements := []string{
		`CREATE TABLE IF NOT EXISTS kernels(id TEXT PRIMARY KEY,version TEXT NOT NULL,source TEXT NOT NULL,status TEXT NOT NULL CHECK(status IN ('missing','verified')))`,
		`CREATE TABLE IF NOT EXISTS proxies(id TEXT PRIMARY KEY,credential_ref TEXT)`,
		`CREATE TABLE IF NOT EXISTS fingerprints(id TEXT PRIMARY KEY,seed INTEGER NOT NULL UNIQUE CHECK(seed BETWEEN 1 AND 2147483647),kernel_id TEXT NOT NULL REFERENCES kernels(id),template_version TEXT NOT NULL,generator_version TEXT NOT NULL,config_json TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS environments(id TEXT PRIMARY KEY,code INTEGER NOT NULL UNIQUE,name TEXT NOT NULL UNIQUE,kernel_id TEXT NOT NULL REFERENCES kernels(id),proxy_id TEXT REFERENCES proxies(id),fingerprint_id TEXT NOT NULL UNIQUE REFERENCES fingerprints(id),revision INTEGER NOT NULL CHECK(revision>0),created_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS operations(id TEXT PRIMARY KEY,result_json TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS requests(id TEXT PRIMARY KEY,signature TEXT NOT NULL,result_json TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS activities(id TEXT PRIMARY KEY,created_at TEXT NOT NULL,action TEXT NOT NULL,target TEXT NOT NULL,detail TEXT NOT NULL)`,
		`INSERT OR IGNORE INTO kernels(id,version,source,status) VALUES('kernel-pending','未安装','fingerprint-chromium','missing')`,
		`PRAGMA user_version=1`,
	}
	for _, statement := range statements {
		if _, err = tx.Exec(statement); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if err = s.migrateKernels(); err != nil {
		return err
	}
	return s.migrateFingerprints()
}
func (s *Service) checkSchema() error {
	if err := s.checkKernelSchema(); err != nil {
		return err
	}
	for _, statement := range []string{
		"SELECT config_revision FROM fingerprints LIMIT 0",
		"SELECT user_data_ref FROM environments LIMIT 0",
		"SELECT fingerprint_id,revision,kernel_id,profile_json,created_at,action,restored_from FROM fingerprint_revisions LIMIT 0",
	} {
		rows, err := s.db.Query(statement)
		if err != nil {
			return err
		}
		rows.Close()
	}
	return nil
}
func (s *Service) checkKernelSchema() error {
	if err := s.checkBaseSchema(); err != nil {
		return err
	}
	rows, err := s.db.Query("SELECT kernel_id,record_json FROM kernel_evidence LIMIT 0")
	if err != nil {
		return err
	}
	return rows.Close()
}
func (s *Service) migrateKernels() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range []string{
		`CREATE TABLE kernel_evidence(kernel_id TEXT PRIMARY KEY REFERENCES kernels(id),record_json TEXT NOT NULL)`,
		`CREATE TRIGGER immutable_kernel_evidence BEFORE UPDATE ON kernel_evidence BEGIN SELECT RAISE(ABORT,'kernel evidence is immutable'); END`,
		`PRAGMA user_version=2`,
	} {
		if _, err = tx.Exec(statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Service) checkBaseSchema() error {
	for _, statement := range []string{
		"SELECT id,version,source,status FROM kernels LIMIT 0",
		"SELECT id,credential_ref FROM proxies LIMIT 0",
		"SELECT id,seed,kernel_id,template_version,generator_version,config_json FROM fingerprints LIMIT 0",
		"SELECT id,code,name,kernel_id,proxy_id,fingerprint_id,revision,created_at FROM environments LIMIT 0",
		"SELECT id,result_json FROM operations LIMIT 0",
		"SELECT id,signature,result_json FROM requests LIMIT 0",
		"SELECT id,created_at,action,target,detail FROM activities LIMIT 0",
	} {
		rows, err := s.db.Query(statement)
		if err != nil {
			return err
		}
		rows.Close()
	}
	rows, err := s.db.Query("PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("workspace contains invalid references")
	}
	return rows.Err()
}
func decode(payload json.RawMessage, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("trailing input")
	}
	return nil
}
func (s *Service) Call(request Request) Result {
	if request.Mode != "native" {
		return failure("CAPABILITY_UNSUPPORTED", "演示输入不能写入真实工作区；请使用桌面原生流程。", false)
	}
	if request.Method == "Kernel.SelectArchive" {
		if decode(request.Payload, &struct{}{}) != nil {
			return failure("VALIDATION_FAILED", "归档必须由桌面文件选择器选择，不接受客户端路径。", false)
		}
		return s.selectKernelArchive()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return failure("NATIVE_UNAVAILABLE", "工作区已关闭，请重新打开应用。", true)
	}
	switch request.Method {
	case "Runtime.Start", "Runtime.Stop", "Runtime.Inspect":
		return s.runtimeCall(request)
	case "Fingerprint.Generate", "Fingerprint.ListRevisions", "Fingerprint.PreviewRestore":
		return s.fingerprintCall(request)
	case "Kernel.Install", "Kernel.Verify", "Kernel.Delete", "Kernel.List":
		return s.kernelCall(request)
	case "Workspace.Read":
		view, err := s.view()
		if err != nil {
			return failure("STORAGE_READ_FAILED", "本地档案读取失败。原数据保留，请检查数据库版本或磁盘。", true)
		}
		return success(view, "")
	case "Environment.Preview":
		var input struct {
			Kind     string `json:"kind"`
			SourceID string `json:"sourceId,omitempty"`
		}
		if decode(request.Payload, &input) != nil || (input.Kind != "create" && input.Kind != "edit") {
			return failure("VALIDATION_FAILED", "环境预览请求无效。", false)
		}
		return s.preview(input.Kind, input.SourceID)
	case "Preview.Discard", "Preview.Regenerate":
		var input struct {
			PreviewID string `json:"previewId"`
		}
		if decode(request.Payload, &input) != nil {
			return failure("VALIDATION_FAILED", "预览请求无效。", false)
		}
		d, exists := s.drafts[input.PreviewID]
		if request.Method == "Preview.Discard" {
			delete(s.drafts, input.PreviewID)
			return success(map[string]string{"status": "discarded"}, "")
		}
		if !exists {
			return failure("PREVIEW_EXPIRED", "预览已失效，请重新打开配置。", true)
		}
		if s.profileUses[d.Preview.Environment.ID] {
			return failure("PROFILE_BUSY", "环境正被运行或维护使用，请停止后重新生成。", true)
		}
		if d.Preview.Environment.CoreID != PendingKernelID {
			c := d.Preview.Environment.Configuration
			return s.generateFingerprint(GenerateFingerprint{PreviewID: input.PreviewID, KernelID: c.CoreID, TemplateID: c.FingerprintVersion, Overrides: FingerprintOverrides{Language: c.Language, Timezone: c.Timezone, CPU: c.CPU, Width: c.Width, Height: c.Height}, Regenerate: true})
		}
		seed, err := s.newSeed(d.Preview.Environment.Seed)
		if err != nil {
			return storageFailure(err)
		}
		d.Preview.Environment.Seed = seed
		if d.Preview.Fingerprint != nil {
			revision := int64(1)
			if d.BaseProfile != nil {
				revision = d.BaseProfile.ConfigRevision + 1
			}
			profile, err := frozenProfile(d.Preview.Environment.Configuration, nil, "native-initial-v1", revision, true)
			if err != nil {
				return storageFailure(err)
			}
			d.Preview.Fingerprint = &FingerprintPreview{Mode: "native", PreviewProfile: profile, CapabilityReport: capabilityReport(profile, nil), Changes: profileChanges(d.BaseProfile, profile), Action: "regenerate"}
		}
		s.drafts[input.PreviewID] = d
		return success(d.Preview, "")
	case "Environment.Create", "Environment.Update", "Fingerprint.CommitRevision":
		var input Mutation
		if decode(request.Payload, &input) != nil {
			return failure("VALIDATION_FAILED", "提交字段无效；不能导入原型快照、身份或浏览数据。", false)
		}
		if request.Method == "Fingerprint.CommitRevision" && (input.EnvironmentID == "" || input.ProfileHash == "") {
			return failure("VALIDATION_FAILED", "提交完整档案需要环境标识和服务预览摘要。", false)
		}
		return s.mutate(request.Method, input)
	case "Operation.Read", "Operation.Cancel":
		var input struct {
			OperationID string `json:"operationId"`
		}
		if decode(request.Payload, &input) != nil {
			return failure("VALIDATION_FAILED", "任务请求无效。", false)
		}
		var text string
		if err := s.db.QueryRow("SELECT result_json FROM operations WHERE id=?", input.OperationID).Scan(&text); err != nil {
			return failure("NOT_FOUND", "任务不存在；没有取消其他任务。", false)
		}
		var operation Operation
		if json.Unmarshal([]byte(text), &operation) != nil {
			return failure("STORAGE_READ_FAILED", "任务记录损坏，请重新打开工作区。", true)
		}
		if request.Method == "Operation.Cancel" && s.kernelTask != nil && s.kernelTask.operation.ID == operation.ID {
			s.kernelTask.cancel()
			operation.CancelRequested = true
			s.kernelTask.operation.CancelRequested = true
			if err := s.storeOperation(operation); err != nil {
				return storageFailure(err)
			}
		}
		if request.Method == "Operation.Cancel" {
			return s.cancelRuntimeOperation(operation)
		}
		return success(operation, operation.ID)
	default:
		return failure("CAPABILITY_UNSUPPORTED", "此功能尚未接入真实桌面服务，未修改本地数据。", false)
	}
}
func (s *Service) newSeed(exclude string) (string, error) {
	for {
		value, err := rand.Int(rand.Reader, big.NewInt(2147483647))
		if err != nil {
			return "", err
		}
		candidate := value.Int64() + 1
		if strconv.FormatInt(candidate, 10) == exclude {
			continue
		}
		var count int
		if err = s.db.QueryRow("SELECT (SELECT COUNT(*) FROM fingerprints WHERE seed=?)+(SELECT COUNT(*) FROM fingerprint_revisions WHERE json_extract(profile_json,'$.seed')=?)", candidate, strconv.FormatInt(candidate, 10)).Scan(&count); err != nil {
			return "", err
		}
		if count == 0 {
			return strconv.FormatInt(candidate, 10), nil
		}
	}
}
func (s *Service) readEnvironment(environmentID string) (Environment, int64, string, error) {
	var e Environment
	var configJSON, profileID, name, kernelID, profileKernelID, proxyID, seed, dataRef string
	var revision int64
	var code int
	err := s.db.QueryRow(`SELECT e.id,e.code,e.created_at,e.revision,e.fingerprint_id,f.config_json,e.name,e.kernel_id,f.kernel_id,COALESCE(e.proxy_id,''),CAST(f.seed AS TEXT),e.user_data_ref FROM environments e JOIN fingerprints f ON f.id=e.fingerprint_id WHERE e.id=?`, environmentID).Scan(&e.ID, &code, &e.CreatedAt, &revision, &profileID, &configJSON, &name, &kernelID, &profileKernelID, &proxyID, &seed, &dataRef)
	if err != nil {
		return e, 0, "", err
	}
	if err = decode(json.RawMessage(configJSON), &e.Configuration); err != nil {
		return e, 0, "", err
	}
	if validate(e.Configuration) != "" || e.Name != name || e.CoreID != kernelID || profileKernelID != kernelID || e.ProxyID != proxyID || e.Seed != seed {
		return e, 0, "", errors.New("inconsistent saved configuration")
	}
	profile, err := readProfileFrom(s.db, profileID)
	if err != nil {
		return e, 0, "", err
	}
	ref, err := dataReference(e.ID)
	if err != nil || dataRef != ref || !profileMatchesConfiguration(profile.Profile, e.Configuration) {
		return e, 0, "", errors.New("inconsistent current profile or data reference")
	}
	e.Code = fmt.Sprintf("%03d", code)
	e.Status = "ready"
	e.Cookies = []any{}
	return e, revision, profileID, nil
}
func (s *Service) preview(kind, sourceID string) Result {
	var source Environment
	var revision int64
	if sourceID != "" {
		var err error
		source, revision, _, err = s.readEnvironment(sourceID)
		if err != nil {
			return failure("NOT_FOUND", "环境不存在，请重新读取本地工作区。", true)
		}
	}
	if kind == "edit" && sourceID == "" {
		return failure("VALIDATION_FAILED", "请选择要编辑的环境。", false)
	}
	e := source
	if kind == "create" {
		seed, err := s.newSeed("")
		if err != nil {
			return storageFailure(err)
		}
		config := Configuration{Name: "", Group: "日常运营", CoreID: PendingKernelID, Seed: seed, Language: "en-US", Timezone: "America/New_York", CPU: "auto", Width: 1280, Height: 800, RestoreTabs: true, FingerprintVersion: "windows-desktop-v1"}
		if sourceID != "" {
			config = source.Configuration
			config.Name = source.Name + " 副本"
			config.Seed = seed
			config.Note = ""
		}
		e = Environment{Configuration: config, ID: id(), Status: "ready", Cookies: []any{}, CreatedAt: timestamp()}
		revision = 0
	}
	p := Preview{PreviewID: id(), Environment: e, ExpectedRevision: revision}
	d := draft{Kind: kind, Preview: p}
	if kind == "edit" {
		if s.profileUses[e.ID] && !s.runtimeOwnsProfileUse(e.ID) {
			return failure("PROFILE_BUSY", "请先停止该环境，再编辑关键配置。", true)
		}
		_, _, profileID, err := s.readEnvironment(e.ID)
		if err != nil {
			return failure("STORAGE_READ_FAILED", "本地档案无法读取。", true)
		}
		p.Fingerprint, p.UserDataRef, err = s.previewCurrentProfile(e.ID, profileID, e.Configuration)
		if err != nil {
			return failure("STORAGE_READ_FAILED", "本地完整档案无法读取。", true)
		}
		base := p.Fingerprint.PreviewProfile
		d.BaseProfile = &base
	}
	d.Preview = p
	s.drafts[p.PreviewID] = d
	return success(p, "")
}
func validate(config Configuration) string {
	seed, err := strconv.ParseInt(config.Seed, 10, 64)
	if strings.TrimSpace(config.Name) == "" {
		return "请输入环境名称。"
	}
	if err != nil || seed < 1 || seed > 2147483647 || strconv.FormatInt(seed, 10) != config.Seed {
		return "种子应为 1 至 2147483647 的整数。"
	}
	if config.Width < 400 || config.Width > 7680 || config.Height < 400 || config.Height > 7680 {
		return "窗口宽高应为 400 至 7680 的整数。"
	}
	if !kernel.ValidTimezone(config.Timezone) {
		return "请选择有效的 IANA 时区。"
	}
	if !map[string]bool{"auto": true, "4": true, "8": true, "12": true, "16": true}[config.CPU] {
		return "请选择支持的 CPU 偏好。"
	}
	if config.FingerprintVersion != "windows-desktop-v1" {
		return "设备模板版本不受支持。"
	}
	if !map[string]bool{"en-US": true, "en-GB": true, "de-DE": true, "ja-JP": true, "en-SG": true, "zh-CN": true}[config.Language] {
		return "请选择支持的语言。"
	}
	for _, line := range strings.Split(config.URLs, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		u, err := url.Parse(strings.TrimSpace(line))
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
			return "启动网址仅支持无凭据的完整 http 或 https URL。"
		}
	}
	return ""
}
func (s *Service) mutate(method string, input Mutation) Result {
	if strings.TrimSpace(input.RequestID) == "" {
		return failure("VALIDATION_FAILED", "缺少请求标识。", false)
	}
	encoded, _ := json.Marshal(input)
	digest := sha256.Sum256(append([]byte(method), encoded...))
	signature := hex.EncodeToString(digest[:])
	var priorSignature, priorJSON string
	err := s.db.QueryRow("SELECT signature,result_json FROM requests WHERE id=?", input.RequestID).Scan(&priorSignature, &priorJSON)
	if err == nil {
		if signature != priorSignature {
			return failure("REQUEST_ID_REUSED", "同一请求标识不能用于不同操作。", false)
		}
		var prior Result
		if json.Unmarshal([]byte(priorJSON), &prior) != nil {
			return failure("STORAGE_READ_FAILED", "请求记录损坏，请检查工作区。", true)
		}
		return prior
	}
	if err != sql.ErrNoRows {
		return storageFailure(err)
	}
	d, exists := s.drafts[input.PreviewID]
	if !exists {
		return failure("PREVIEW_EXPIRED", "预览已失效，请重新打开配置。", true)
	}
	creating := method == "Environment.Create"
	if (creating && d.Kind != "create") || (!creating && d.Kind != "edit") {
		return failure("VALIDATION_FAILED", "预览与提交类型不一致。", false)
	}
	if !creating && s.profileUses[d.Preview.Environment.ID] {
		current, _, _, err := s.readEnvironment(d.Preview.Environment.ID)
		if err != nil {
			return failure("STORAGE_READ_FAILED", "当前档案无法读取，未保存。", true)
		}
		safe := current.Configuration
		safe.Name, safe.Group, safe.Note = input.Configuration.Name, input.Configuration.Group, input.Configuration.Note
		if !s.runtimeOwnsProfileUse(current.ID) || safe != input.Configuration {
			return failure("PROFILE_BUSY", "环境正在运行或维护；只能保存名称、分组和备注，关键配置未修改。", true)
		}
	}
	if method == "Fingerprint.CommitRevision" && input.EnvironmentID != d.Preview.Environment.ID {
		return failure("VALIDATION_FAILED", "预览不属于所选环境。", false)
	}
	if method == "Fingerprint.CommitRevision" && (d.Preview.Fingerprint == nil || d.Preview.Fingerprint.PreviewProfile.KernelID == PendingKernelID || input.ProfileHash != d.Preview.Fingerprint.PreviewProfile.ConfigHash) {
		return failure("VALIDATION_FAILED", "请生成完整的精确内核档案，并按服务预览摘要提交。", false)
	}
	if creating && input.Count != 1 {
		return failure("CAPABILITY_UNSUPPORTED", "持久批量任务尚未接入，请暂用单个创建；这不是产品数量配额。", false)
	}
	config := input.Configuration
	config.Name = strings.TrimSpace(config.Name)
	if message := validate(config); message != "" {
		return failure("VALIDATION_FAILED", message, false)
	}
	if config.Seed != d.Preview.Environment.Seed {
		return failure("VALIDATION_FAILED", "种子与原生预览不一致；改变身份请显式重新生成后保存。", false)
	}
	if !creating && input.ExpectedRevision != d.Preview.ExpectedRevision {
		return failure("REVISION_CONFLICT", "提交修订与原始预览不一致，请重新打开最新记录。", true)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return storageFailure(err)
	}
	defer tx.Rollback()
	var count int
	var kernelStatus string
	if err = tx.QueryRow("SELECT status FROM kernels WHERE id=?", config.CoreID).Scan(&kernelStatus); err != nil {
		if err == sql.ErrNoRows {
			return failure("VALIDATION_FAILED", "指定内核引用不存在，未保存。", false)
		}
		return storageFailure(err)
	}
	if config.CoreID != PendingKernelID && kernelStatus != "verified" {
		return failure("KERNEL_INTEGRITY_FAILED", "指定内核未通过核验，未保存；不会改用其他内核。", true)
	}
	profile, prepared := s.mutationProfile(tx, d, input, creating)
	if !prepared.OK {
		return prepared
	}
	if config.ProxyID != "" {
		if err = tx.QueryRow("SELECT COUNT(*) FROM proxies WHERE id=?", config.ProxyID).Scan(&count); err != nil {
			return storageFailure(err)
		}
		if count != 1 {
			return failure("VALIDATION_FAILED", "代理引用不存在，未保存且不会静默直连。", false)
		}
	}
	e := d.Preview.Environment
	profileID := ""
	revision := int64(1)
	code := 0
	if creating {
		e.ID = id()
		profileID = id()
		e.CreatedAt = timestamp()
		if err = tx.QueryRow("SELECT COALESCE(MAX(code),0)+1 FROM environments").Scan(&code); err != nil {
			return storageFailure(err)
		}
	} else {
		if err = tx.QueryRow("SELECT fingerprint_id,revision,code FROM environments WHERE id=?", e.ID).Scan(&profileID, &revision, &code); err != nil {
			return failure("NOT_FOUND", "环境已不存在，请重新读取。", true)
		}
		if input.ExpectedRevision != revision {
			return failure("REVISION_CONFLICT", "档案已经被修改，请重新打开最新记录后重试。", true)
		}
		revision++
	}
	if !creating {
		current, err := readProfileFrom(tx, profileID)
		if err != nil {
			return failure("STORAGE_READ_FAILED", "当前完整档案无法读取，未保存。", true)
		}
		if d.BaseProfile == nil || current.Profile.ConfigHash != d.BaseProfile.ConfigHash {
			return failure("REVISION_CONFLICT", "设备档案已改变，请重新打开最新记录。", true)
		}
		if profile.ConfigHash != current.Profile.ConfigHash && profile.ConfigRevision != current.Profile.ConfigRevision+1 {
			return failure("REVISION_CONFLICT", "档案修订序号不连续，未保存。", true)
		}
		if s.profileUses[e.ID] && profile.ConfigHash != current.Profile.ConfigHash {
			return failure("PROFILE_BUSY", "环境仍在使用当前设备档案，旧的生成或回滚预览不能追加新修订。请停止后重新预览。", true)
		}
	}
	if err = tx.QueryRow("SELECT COUNT(*) FROM environments WHERE name=? AND id<>?", config.Name, e.ID).Scan(&count); err != nil {
		return storageFailure(err)
	}
	if count > 0 {
		return failure("VALIDATION_FAILED", "已有同名环境，请换一个名称。", false)
	}
	if err = tx.QueryRow("SELECT COUNT(*) FROM fingerprints WHERE seed=? AND id<>?", config.Seed, profileID).Scan(&count); err != nil {
		return storageFailure(err)
	}
	if count > 0 {
		return failure("VALIDATION_FAILED", "种子重复，请显式生成新档案。", false)
	}
	configJSON, _ := json.Marshal(config)
	if creating {
		_, err = tx.Exec(`INSERT INTO fingerprints(id,seed,kernel_id,template_version,generator_version,config_json) VALUES(?,?,?,'windows-desktop-v1','native-initial-v1',?)`, profileID, config.Seed, config.CoreID, string(configJSON))
		if err != nil {
			return storageFailure(err)
		}
		ref, referenceErr := dataReference(e.ID)
		if referenceErr != nil {
			return storageFailure(referenceErr)
		}
		_, err = tx.Exec(`INSERT INTO environments(id,code,name,kernel_id,proxy_id,fingerprint_id,revision,created_at,user_data_ref) VALUES(?,?,?,?,?,?,?,?,?)`, e.ID, code, config.Name, config.CoreID, nullable(config.ProxyID), profileID, revision, e.CreatedAt, ref)
	} else {
		_, err = tx.Exec(`UPDATE fingerprints SET seed=?,kernel_id=?,config_json=? WHERE id=?`, config.Seed, config.CoreID, string(configJSON), profileID)
		if err != nil {
			return storageFailure(err)
		}
		var updated sql.Result
		updated, err = tx.Exec(`UPDATE environments SET name=?,kernel_id=?,proxy_id=?,revision=? WHERE id=? AND revision=?`, config.Name, config.CoreID, nullable(config.ProxyID), revision, e.ID, input.ExpectedRevision)
		if err == nil {
			count, affectedErr := updated.RowsAffected()
			if affectedErr != nil {
				return storageFailure(affectedErr)
			}
			if count != 1 {
				return failure("REVISION_CONFLICT", "环境已被修改，未覆盖较新的配置。", true)
			}
		}
	}
	if err != nil {
		return storageFailure(err)
	}
	if creating || d.BaseProfile == nil || profile.ConfigHash != d.BaseProfile.ConfigHash {
		action, restoredFrom := "legacy-edit", int64(0)
		if d.Preview.Fingerprint != nil && config.CoreID != PendingKernelID {
			action, restoredFrom = d.Preview.Fingerprint.Action, d.Preview.Fingerprint.RestoredFrom
		}
		if creating && config.CoreID == PendingKernelID {
			action = "pending-create"
		}
		if err = appendProfile(tx, profileID, profile, action, restoredFrom); err != nil {
			return storageFailure(err)
		}
	}
	e.Configuration = config
	e.Code = fmt.Sprintf("%03d", code)
	e.Status = "ready"
	if slot := s.runtimeSlots[e.ID]; slot != nil {
		e.Status, e.LastOpened = slot.session.State, slot.session.StartedAt
		if slot.session.Error != nil {
			e.Error = slot.session.Error.Message
		}
	}
	e.Cookies = []any{}
	operation := Operation{ID: id(), Kind: map[bool]string{true: "create", false: "edit"}[creating], State: "completed", Total: 1, CompletedIDs: []string{e.ID}}
	operationJSON, _ := json.Marshal(operation)
	if _, err = tx.Exec("INSERT INTO operations(id,result_json) VALUES(?,?)", operation.ID, string(operationJSON)); err != nil {
		return storageFailure(err)
	}
	result := success(map[string]any{"status": "completed", "environment": map[string]any{"record": e, "revision": revision}}, operation.ID)
	if creating {
		result = success(map[string]any{"status": "accepted", "operation": operation}, operation.ID)
	}
	if method == "Fingerprint.CommitRevision" {
		result = success(map[string]any{"status": "completed", "environment": map[string]any{"record": e, "revision": revision}, "newRevision": revision, "fingerprintRevision": profile.ConfigRevision}, operation.ID)
	}
	resultJSON, _ := json.Marshal(result)
	if _, err = tx.Exec("INSERT INTO requests(id,signature,result_json) VALUES(?,?,?)", input.RequestID, signature, string(resultJSON)); err != nil {
		return storageFailure(err)
	}
	if _, err = tx.Exec("INSERT INTO activities(id,created_at,action,target,detail) VALUES(?,?,?,?,?)", id(), timestamp(), map[bool]string{true: "创建环境", false: "修改环境"}[creating], config.Name, "本地 SQLite 事务已提交；档案固定，内核未安装时不能启动。"); err != nil {
		return storageFailure(err)
	}
	if s.options.BeforeCommit != nil {
		if err = s.options.BeforeCommit(); err != nil {
			return storageFailure(err)
		}
	}
	if err = tx.Commit(); err != nil {
		return storageFailure(err)
	}
	delete(s.drafts, input.PreviewID)
	return result
}
func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func (s *Service) view() (View, error) {
	state := State{SchemaVersion: 1, Environments: []Environment{}, Proxies: []any{}, Kernels: []Kernel{}, Backups: []any{}, Activities: []Activity{}}
	rows, err := s.db.Query("SELECT id FROM environments ORDER BY code DESC")
	if err != nil {
		return View{}, err
	}
	ids := []string{}
	for rows.Next() {
		var value string
		if err = rows.Scan(&value); err != nil {
			rows.Close()
			return View{}, err
		}
		ids = append(ids, value)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return View{}, err
	}
	for _, value := range ids {
		e, _, _, err := s.readEnvironment(value)
		if err != nil {
			return View{}, err
		}
		state.Environments = append(state.Environments, e)
	}
	rows, err = s.db.Query("SELECT id,version,source,status FROM kernels ORDER BY id")
	if err != nil {
		return View{}, err
	}
	for rows.Next() {
		var k Kernel
		var status string
		if err = rows.Scan(&k.ID, &k.Version, &k.Source, &status); err != nil {
			rows.Close()
			return View{}, err
		}
		k.Available = status == "verified"
		k.Note = "未安装真实 fingerprint-chromium，不能启动。"
		if k.ID != PendingKernelID {
			if k.Available {
				k.Note = "精确构建已安装并通过隔离诊断；正常环境启停仍待T06。"
			} else {
				k.Note = "精确构建缺失或核验失败；不会自动切换版本。"
			}
		}
		state.Kernels = append(state.Kernels, k)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return View{}, err
	}
	rows, err = s.db.Query("SELECT id,created_at,action,target,detail FROM activities ORDER BY rowid DESC")
	if err != nil {
		return View{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var a Activity
		if err = rows.Scan(&a.ID, &a.Time, &a.Action, &a.Target, &a.Detail); err != nil {
			return View{}, err
		}
		a.Result = "success"
		state.Activities = append(state.Activities, a)
	}
	if err = rows.Err(); err != nil {
		return View{}, err
	}
	installed, err := s.listKernels()
	if err != nil {
		return View{}, err
	}
	operations, err := s.listKernelOperations()
	if err != nil {
		return View{}, err
	}
	profiles, references, err := s.profileViews()
	if err != nil {
		return View{}, err
	}
	sessions := map[string]RuntimeSession{}
	for environmentID, slot := range s.runtimeSlots {
		sessions[environmentID] = slot.session
	}
	for index := range state.Environments {
		if session, ok := sessions[state.Environments[index].ID]; ok {
			state.Environments[index].Status = session.State
			state.Environments[index].LastOpened = session.StartedAt
			if session.Error != nil {
				state.Environments[index].Error = session.Error.Message
			}
		}
	}
	return View{Mode: "native", State: state, KernelRecords: installed, KernelOperations: operations, Fingerprints: profiles, DataReferences: references, RuntimeSessions: sessions}, nil
}
