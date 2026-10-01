package workspace

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
	"github.com/google/uuid"
)

type profileQuery interface {
	QueryRow(string, ...any) *sql.Row
}

func profileHash(profile DeviceProfile) string {
	profile.ConfigHash = ""
	encoded, _ := json.Marshal(profile)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func equalProfile(a, b DeviceProfile) bool {
	a.ConfigRevision, b.ConfigRevision = 0, 0
	a.ConfigHash, b.ConfigHash = "", ""
	return reflect.DeepEqual(a, b)
}

func profileInput(profile DeviceProfile) kernel.FingerprintInput {
	return kernel.FingerprintInput{Seed: profile.Seed, Language: profile.Language, Timezone: profile.Timezone, CPU: profile.CPU}
}

func profileMatchesConfiguration(profile DeviceProfile, config Configuration) bool {
	return profile.Seed == config.Seed && profile.KernelID == config.CoreID && profile.TemplateVersion == config.FingerprintVersion && profile.Language == config.Language && profile.Timezone == config.Timezone && profile.CPU == config.CPU && profile.Width == config.Width && profile.Height == config.Height
}

func applyProfile(config *Configuration, profile DeviceProfile) {
	config.Seed, config.CoreID, config.FingerprintVersion = profile.Seed, profile.KernelID, profile.TemplateVersion
	config.Language, config.Timezone, config.CPU = profile.Language, profile.Timezone, profile.CPU
	config.Width, config.Height = profile.Width, profile.Height
}

func dataReference(environmentID string) (string, error) {
	parsed, err := uuid.Parse(environmentID)
	if err != nil || parsed.String() != environmentID {
		return "", errors.New("invalid environment data reference")
	}
	return "environments/" + environmentID + "/user-data", nil
}

func savedKernelFrom(query profileQuery, kernelID string, requireVerified bool) (kernel.Record, error) {
	var text, status string
	if err := query.QueryRow(`SELECT e.record_json,k.status FROM kernel_evidence e JOIN kernels k ON k.id=e.kernel_id WHERE e.kernel_id=?`, kernelID).Scan(&text, &status); err != nil {
		return kernel.Record{}, err
	}
	var record kernel.Record
	if err := json.Unmarshal([]byte(text), &record); err != nil {
		return record, err
	}
	if record.ID != kernelID {
		return record, errors.New("kernel evidence ID mismatch")
	}
	if err := kernel.CheckRecord(record); err != nil {
		return record, err
	}
	if requireVerified && status != "verified" {
		return record, &kernel.Problem{Code: "KERNEL_INTEGRITY_FAILED", Reason: "not-verified", Message: "该精确内核未通过核验，不能保存新档案；不会切换版本。", Retryable: true}
	}
	return record, nil
}

func frozenProfile(config Configuration, record *kernel.Record, generator string, revision int64, legacy bool) (DeviceProfile, error) {
	profile := DeviceProfile{SchemaVersion: 1, ConfigRevision: revision, Seed: config.Seed, TemplateID: "windows-desktop-v1", TemplateVersion: config.FingerprintVersion, GeneratorVersion: generator, Platform: "windows", PlatformVersion: "15.0.0", Brand: "Chrome", KernelID: config.CoreID, Language: config.Language, AcceptLanguages: kernel.AcceptLanguages(config.Language), UILanguage: "system", Timezone: config.Timezone, CPU: config.CPU, Width: config.Width, Height: config.Height, RegionPreset: "custom", Parameters: []string{}}
	for region, pair := range map[string][2]string{"US": {"en-US", "America/New_York"}, "GB": {"en-GB", "Europe/London"}, "DE": {"de-DE", "Europe/Berlin"}, "JP": {"ja-JP", "Asia/Tokyo"}, "SG": {"en-SG", "Asia/Singapore"}, "CN": {"zh-CN", "Asia/Shanghai"}} {
		if config.Language == pair[0] && config.Timezone == pair[1] {
			profile.RegionPreset = region
		}
	}
	if record != nil {
		profile.CoreActualVersion, profile.BrandVersion, profile.CoreExecutableSHA256 = record.Version, record.Version, record.ExecutableSHA256
		profile.AdapterVersion, profile.CapabilityVersion = record.Report.AdapterVersion, record.Report.Version
		parameters, err := kernel.CompileFingerprint(*record, profileInput(profile))
		if err != nil && !legacy {
			return DeviceProfile{}, err
		}
		if err == nil {
			profile.Parameters = parameters
		}
	} else if !legacy {
		return DeviceProfile{}, &kernel.Problem{Code: "KERNEL_MISSING", Reason: "not-installed", Message: "请先选择已核验的精确内核，再生成Windows档案。", Retryable: true}
	}
	profile.ConfigHash = profileHash(profile)
	return profile, nil
}

func checkProfile(profile DeviceProfile) error {
	if profile.SchemaVersion != 1 || profile.ConfigRevision < 1 || profile.TemplateID != "windows-desktop-v1" || profile.TemplateVersion != "windows-desktop-v1" || profile.Platform != "windows" || profile.PlatformVersion != "15.0.0" || profile.Brand != "Chrome" || profile.UILanguage != "system" || profile.Parameters == nil || profile.ConfigHash != profileHash(profile) {
		return errors.New("invalid frozen profile")
	}
	if profile.GeneratorVersion != FingerprintGeneratorVersion && profile.GeneratorVersion != "native-initial-v1" {
		return errors.New("unsupported profile generator")
	}
	if !reflect.DeepEqual(profile.AcceptLanguages, kernel.AcceptLanguages(profile.Language)) {
		return errors.New("inconsistent language group")
	}
	config := Configuration{Name: "validation", CoreID: profile.KernelID, Seed: profile.Seed, Language: profile.Language, Timezone: profile.Timezone, CPU: profile.CPU, Width: profile.Width, Height: profile.Height, FingerprintVersion: profile.TemplateVersion}
	if validate(config) != "" {
		return errors.New("invalid frozen profile inputs")
	}
	return nil
}

func checkProfileEvidence(query profileQuery, profile DeviceProfile) error {
	var record *kernel.Record
	if profile.KernelID != PendingKernelID {
		stored, err := savedKernelFrom(query, profile.KernelID, false)
		if err != nil {
			return err
		}
		record = &stored
	}
	config := Configuration{CoreID: profile.KernelID, Seed: profile.Seed, Language: profile.Language, Timezone: profile.Timezone, CPU: profile.CPU, Width: profile.Width, Height: profile.Height, FingerprintVersion: profile.TemplateVersion}
	canonical, err := frozenProfile(config, record, profile.GeneratorVersion, profile.ConfigRevision, profile.GeneratorVersion == "native-initial-v1")
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(canonical, profile) {
		return errors.New("profile is not the normalized capability-compiled input")
	}
	return nil
}

func readProfileFrom(query profileQuery, profileID string) (ProfileRevision, error) {
	var revision ProfileRevision
	var text string
	var current int64
	err := query.QueryRow(`SELECT f.config_revision,r.profile_json,r.created_at,r.action,COALESCE(r.restored_from,0) FROM fingerprints f JOIN fingerprint_revisions r ON r.fingerprint_id=f.id AND r.revision=f.config_revision WHERE f.id=?`, profileID).Scan(&current, &text, &revision.CreatedAt, &revision.Action, &revision.RestoredFrom)
	if err != nil {
		return revision, err
	}
	if err = decode(json.RawMessage(text), &revision.Profile); err != nil {
		return revision, err
	}
	if revision.Profile.ConfigRevision != current {
		return revision, errors.New("profile revision reference mismatch")
	}
	if err = checkProfile(revision.Profile); err != nil {
		return revision, err
	}
	return revision, checkProfileEvidence(query, revision.Profile)
}

func (s *Service) migrateFingerprints() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range []string{
		`ALTER TABLE fingerprints ADD COLUMN config_revision INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE environments ADD COLUMN user_data_ref TEXT NOT NULL DEFAULT ''`,
		`CREATE TABLE fingerprint_revisions(fingerprint_id TEXT NOT NULL REFERENCES fingerprints(id) ON DELETE CASCADE,revision INTEGER NOT NULL CHECK(revision>0),kernel_id TEXT NOT NULL REFERENCES kernels(id),profile_json TEXT NOT NULL,created_at TEXT NOT NULL,action TEXT NOT NULL,restored_from INTEGER,PRIMARY KEY(fingerprint_id,revision))`,
		`CREATE TRIGGER immutable_fingerprint_revision BEFORE UPDATE ON fingerprint_revisions BEGIN SELECT RAISE(ABORT,'fingerprint revisions are immutable'); END`,
	} {
		if _, err = tx.Exec(statement); err != nil {
			return err
		}
	}
	rows, err := tx.Query(`SELECT e.id,e.fingerprint_id,f.config_json,f.generator_version FROM environments e JOIN fingerprints f ON f.id=e.fingerprint_id`)
	if err != nil {
		return err
	}
	type legacyRecord struct{ environmentID, fingerprintID, text, generator string }
	legacy := []legacyRecord{}
	for rows.Next() {
		var row legacyRecord
		if err = rows.Scan(&row.environmentID, &row.fingerprintID, &row.text, &row.generator); err != nil {
			rows.Close()
			return err
		}
		legacy = append(legacy, row)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, row := range legacy {
		var config Configuration
		if err = decode(json.RawMessage(row.text), &config); err != nil || validate(config) != "" {
			return errors.New("invalid legacy configuration; not migrated")
		}
		var record *kernel.Record
		if config.CoreID != PendingKernelID {
			stored, err := savedKernelFrom(tx, config.CoreID, false)
			if err != nil {
				return err
			}
			record = &stored
		}
		profile, err := frozenProfile(config, record, row.generator, 1, true)
		if err != nil {
			return err
		}
		if err = checkProfile(profile); err != nil {
			return err
		}
		ref, err := dataReference(row.environmentID)
		if err != nil {
			return err
		}
		if _, err = tx.Exec("UPDATE environments SET user_data_ref=? WHERE id=?", ref, row.environmentID); err != nil {
			return err
		}
		if err = appendProfile(tx, row.fingerprintID, profile, "migrated", 0); err != nil {
			return err
		}
	}
	if _, err = tx.Exec("PRAGMA user_version=3"); err != nil {
		return err
	}
	return tx.Commit()
}

func appendProfile(tx *sql.Tx, profileID string, profile DeviceProfile, action string, restoredFrom int64) error {
	encoded, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	var restored any
	if restoredFrom > 0 {
		restored = restoredFrom
	}
	if _, err = tx.Exec(`INSERT INTO fingerprint_revisions(fingerprint_id,revision,kernel_id,profile_json,created_at,action,restored_from) VALUES(?,?,?,?,?,?,?)`, profileID, profile.ConfigRevision, profile.KernelID, string(encoded), timestamp(), action, restored); err != nil {
		return err
	}
	result, err := tx.Exec("UPDATE fingerprints SET config_revision=?,template_version=?,generator_version=? WHERE id=?", profile.ConfigRevision, profile.TemplateVersion, profile.GeneratorVersion, profileID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return errors.New("current fingerprint reference not updated")
	}
	return nil
}

func capabilityReport(profile DeviceProfile, record *kernel.Record) FingerprintReport {
	report := FingerprintReport{KernelID: profile.KernelID, EvidenceStatus: "not-probed", Capabilities: []kernel.Capability{}}
	if record != nil {
		report.EvidenceStatus = "installed-build-evidence-not-live-profile"
		report.Capabilities = append(report.Capabilities, record.Report.Capabilities...)
	}
	report.Capabilities = append(report.Capabilities,
		kernel.Capability{Field: "screen", Status: "system", Source: "application-policy", Note: "屏幕与缩放不由窗口偏好伪装；跟随实际系统/内核，本预览未读取具体值。"},
		kernel.Capability{Field: "window", Status: "configurable", Source: "application-preference", Note: "只保存窗口偏好，不编译成屏幕指纹；实际可见窗口由后续正常启动验收。"},
	)
	return report
}

func profileChanges(before *DeviceProfile, after DeviceProfile) []ProfileChange {
	changes := []ProfileChange{}
	fields := []struct{ field, value string }{
		{"seed", after.Seed}, {"kernelId", after.KernelID}, {"coreActualVersion", after.CoreActualVersion}, {"generatorVersion", after.GeneratorVersion}, {"language", after.Language}, {"timezone", after.Timezone}, {"cpu", after.CPU}, {"window", fmt.Sprintf("%d×%d", after.Width, after.Height)},
	}
	old := map[string]string{}
	if before != nil {
		for _, change := range profileChanges(nil, *before) {
			old[change.Field] = change.After
		}
	}
	for _, item := range fields {
		previous, exists := old[item.field]
		if !exists {
			previous = "未保存"
		}
		if previous != item.value {
			changes = append(changes, ProfileChange{item.field, previous, item.value})
		}
	}
	return changes
}

// AcquireProfileUse is host-only. The future runtime must hold this lease from
// start preparation until its process tree exits. RPC cannot invent busy/ready.
func (s *Service) AcquireProfileUse(environmentID string) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("workspace closed")
	}
	if _, _, _, err := s.readEnvironment(environmentID); err != nil {
		return nil, err
	}
	if s.profileUses[environmentID] {
		return nil, errors.New("profile already in use")
	}
	s.profileUses[environmentID] = true
	var once sync.Once
	return func() { once.Do(func() { s.mu.Lock(); delete(s.profileUses, environmentID); s.mu.Unlock() }) }, nil
}

func (s *Service) fingerprintCall(request Request) Result {
	switch request.Method {
	case "Fingerprint.Generate":
		var input GenerateFingerprint
		if decode(request.Payload, &input) != nil {
			return failure("VALIDATION_FAILED", "档案生成字段无效；不接受硬件读值或任意参数。", false)
		}
		return s.generateFingerprint(input)
	case "Fingerprint.ListRevisions":
		var input struct {
			EnvironmentID string `json:"environmentId"`
		}
		if decode(request.Payload, &input) != nil {
			return failure("VALIDATION_FAILED", "请选择环境。", false)
		}
		history, err := s.profileHistory(input.EnvironmentID)
		if err != nil {
			return failure("STORAGE_READ_FAILED", "档案修订无法读取，原记录保留。", true)
		}
		return success(history, "")
	case "Fingerprint.PreviewRestore":
		var input struct {
			PreviewID string `json:"previewId"`
			Revision  int64  `json:"revision"`
		}
		if decode(request.Payload, &input) != nil || input.Revision < 1 {
			return failure("VALIDATION_FAILED", "请选择有效的历史档案修订。", false)
		}
		return s.previewProfileRestore(input.PreviewID, input.Revision)
	}
	return failure("CAPABILITY_UNSUPPORTED", "档案接口尚未接入。", false)
}

func (s *Service) generateFingerprint(input GenerateFingerprint) Result {
	d, exists := s.drafts[input.PreviewID]
	if !exists {
		return failure("PREVIEW_EXPIRED", "预览已失效，请重新打开配置。", true)
	}
	if s.profileUses[d.Preview.Environment.ID] {
		return failure("PROFILE_BUSY", "环境正被运行或维护使用，请停止后生成档案。", true)
	}
	if input.TemplateID != "windows-desktop-v1" {
		return failure("VALIDATION_FAILED", "请选择已支持的Windows桌面模板。", false)
	}
	if d.Kind == "edit" && d.BaseProfile != nil && d.BaseProfile.KernelID != PendingKernelID && input.KernelID != d.BaseProfile.KernelID {
		return failure("CAPABILITY_UNSUPPORTED", "已绑定的精确内核不能在普通编辑中切换；跨内核迁移使用独立流程。", false)
	}
	record, err := savedKernelFrom(s.db, input.KernelID, true)
	if err != nil {
		if err == sql.ErrNoRows {
			return failure("KERNEL_MISSING", "所选精确内核未安装，请先在内核页完成安装核验。", true)
		}
		return kernelFailure(err)
	}
	config := d.Preview.Environment.Configuration
	config.CoreID, config.FingerprintVersion = input.KernelID, input.TemplateID
	config.Language, config.Timezone, config.CPU = input.Overrides.Language, input.Overrides.Timezone, input.Overrides.CPU
	config.Width, config.Height = input.Overrides.Width, input.Overrides.Height
	validation := config
	validation.Name = "preview"
	if message := validate(validation); message != "" {
		return failure("VALIDATION_FAILED", message, false)
	}
	if input.Regenerate {
		config.Seed, err = s.newSeed(config.Seed)
		if err != nil {
			return storageFailure(err)
		}
	}
	revision := int64(1)
	if d.BaseProfile != nil {
		revision = d.BaseProfile.ConfigRevision + 1
	}
	profile, err := frozenProfile(config, &record, FingerprintGeneratorVersion, revision, false)
	if err != nil {
		return kernelFailure(err)
	}
	if d.BaseProfile != nil && equalProfile(*d.BaseProfile, profile) {
		profile = *d.BaseProfile
	}
	action := "edit"
	if d.Kind == "create" || d.BaseProfile == nil || d.BaseProfile.KernelID == PendingKernelID {
		action = "generate"
	}
	if input.Regenerate {
		action = "regenerate"
	}
	d.Preview.Environment.Configuration = config
	d.Preview.Fingerprint = &FingerprintPreview{Mode: "native", PreviewProfile: profile, CapabilityReport: capabilityReport(profile, &record), Changes: profileChanges(d.BaseProfile, profile), Action: action}
	s.drafts[input.PreviewID] = d
	return success(d.Preview, "")
}

func (s *Service) profileHistory(environmentID string) ([]ProfileRevision, error) {
	_, _, profileID, err := s.readEnvironment(environmentID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT profile_json,created_at,action,COALESCE(restored_from,0) FROM fingerprint_revisions WHERE fingerprint_id=? ORDER BY revision DESC`, profileID)
	if err != nil {
		return nil, err
	}
	history := []ProfileRevision{}
	for rows.Next() {
		var revision ProfileRevision
		var text string
		if err = rows.Scan(&text, &revision.CreatedAt, &revision.Action, &revision.RestoredFrom); err != nil {
			rows.Close()
			return nil, err
		}
		if err = decode(json.RawMessage(text), &revision.Profile); err != nil {
			rows.Close()
			return nil, err
		}
		if err = checkProfile(revision.Profile); err != nil {
			rows.Close()
			return nil, err
		}
		history = append(history, revision)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for _, item := range history {
		if err = checkProfileEvidence(s.db, item.Profile); err != nil {
			return nil, err
		}
	}
	return history, nil
}

func (s *Service) previewProfileRestore(previewID string, revision int64) Result {
	d, exists := s.drafts[previewID]
	if !exists || d.Kind != "edit" || d.BaseProfile == nil {
		return failure("PREVIEW_EXPIRED", "请重新打开已有环境的配置。", true)
	}
	if s.profileUses[d.Preview.Environment.ID] {
		return failure("PROFILE_BUSY", "环境正被运行或维护使用，请停止后回滚档案。", true)
	}
	history, err := s.profileHistory(d.Preview.Environment.ID)
	if err != nil {
		return failure("STORAGE_READ_FAILED", "历史档案无法读取，原档案保留。", true)
	}
	var target *DeviceProfile
	for _, item := range history {
		if item.Profile.ConfigRevision == revision {
			profile := item.Profile
			target = &profile
			break
		}
	}
	if target == nil {
		return failure("NOT_FOUND", "历史档案修订不存在。", false)
	}
	if target.KernelID != d.BaseProfile.KernelID || target.KernelID == PendingKernelID {
		return failure("CAPABILITY_UNSUPPORTED", "只允许回滚同一精确内核的已生成档案；未绑定或跨内核历史不能直接使用。", false)
	}
	record, err := savedKernelFrom(s.db, target.KernelID, true)
	if err != nil {
		return kernelFailure(err)
	}
	parameters, err := kernel.CompileFingerprint(record, profileInput(*target))
	if err != nil {
		return kernelFailure(err)
	}
	if !reflect.DeepEqual(parameters, target.Parameters) || target.CoreExecutableSHA256 != record.ExecutableSHA256 || target.CoreActualVersion != record.Version {
		return failure("KERNEL_INTEGRITY_FAILED", "历史档案与精确内核证据不一致，未回滚。", true)
	}
	target.ConfigRevision = d.BaseProfile.ConfigRevision + 1
	target.ConfigHash = profileHash(*target)
	applyProfile(&d.Preview.Environment.Configuration, *target)
	d.Preview.Fingerprint = &FingerprintPreview{Mode: "native", PreviewProfile: *target, CapabilityReport: capabilityReport(*target, &record), Changes: profileChanges(d.BaseProfile, *target), Action: "restore", RestoredFrom: revision}
	s.drafts[previewID] = d
	return success(d.Preview, "")
}

func (s *Service) mutationProfile(tx *sql.Tx, d draft, input Mutation, creating bool) (DeviceProfile, Result) {
	config := input.Configuration
	if d.BaseProfile != nil && d.BaseProfile.KernelID != PendingKernelID && config.CoreID != d.BaseProfile.KernelID {
		return DeviceProfile{}, failure("CAPABILITY_UNSUPPORTED", "普通编辑不能切换已绑定的精确内核。", false)
	}
	if config.CoreID == PendingKernelID {
		revision := int64(1)
		if d.BaseProfile != nil {
			revision = d.BaseProfile.ConfigRevision + 1
		}
		profile, err := frozenProfile(config, nil, "native-initial-v1", revision, true)
		if err != nil {
			return profile, storageFailure(err)
		}
		if d.BaseProfile != nil && equalProfile(*d.BaseProfile, profile) {
			profile = *d.BaseProfile
		}
		return profile, Result{OK: true}
	}
	if d.Preview.Fingerprint == nil || !profileMatchesConfiguration(d.Preview.Fingerprint.PreviewProfile, config) {
		return DeviceProfile{}, failure("VALIDATION_FAILED", "设备字段已改变，请先生成并查看完整档案预览再保存。", false)
	}
	profile := d.Preview.Fingerprint.PreviewProfile
	if err := checkProfile(profile); err != nil {
		return profile, failure("VALIDATION_FAILED", "设备档案预览无效，请重新生成。", false)
	}
	if input.ProfileHash != "" && input.ProfileHash != profile.ConfigHash {
		return profile, failure("VALIDATION_FAILED", "档案摘要与服务预览不一致，未保存。", false)
	}
	record, err := savedKernelFrom(tx, config.CoreID, true)
	if err != nil {
		return profile, kernelFailure(err)
	}
	parameters, err := kernel.CompileFingerprint(record, profileInput(profile))
	if err != nil {
		return profile, kernelFailure(err)
	}
	if !reflect.DeepEqual(parameters, profile.Parameters) || profile.CoreActualVersion != record.Version || profile.CoreExecutableSHA256 != record.ExecutableSHA256 || profile.AdapterVersion != record.Report.AdapterVersion || profile.CapabilityVersion != record.Report.Version {
		return profile, failure("KERNEL_INTEGRITY_FAILED", "设备档案与当前精确内核证据不一致，未保存。", true)
	}
	if creating && profile.ConfigRevision != 1 {
		return profile, failure("VALIDATION_FAILED", "新建档案修订无效。", false)
	}
	return profile, Result{OK: true}
}

func (s *Service) profileViews() (map[string]ProfileRevision, map[string]string, error) {
	rows, err := s.db.Query("SELECT id,fingerprint_id,user_data_ref FROM environments")
	if err != nil {
		return nil, nil, err
	}
	type reference struct{ environmentID, profileID, ref string }
	references := []reference{}
	for rows.Next() {
		var item reference
		if err = rows.Scan(&item.environmentID, &item.profileID, &item.ref); err != nil {
			rows.Close()
			return nil, nil, err
		}
		references = append(references, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	profiles, refs := map[string]ProfileRevision{}, map[string]string{}
	for _, item := range references {
		profile, err := readProfileFrom(s.db, item.profileID)
		if err != nil {
			return nil, nil, err
		}
		profiles[item.environmentID], refs[item.environmentID] = profile, item.ref
	}
	return profiles, refs, nil
}

func (s *Service) previewCurrentProfile(environmentID, profileID string, config Configuration) (*FingerprintPreview, string, error) {
	revision, err := readProfileFrom(s.db, profileID)
	if err != nil {
		return nil, "", err
	}
	var ref string
	if err = s.db.QueryRow("SELECT user_data_ref FROM environments WHERE id=?", environmentID).Scan(&ref); err != nil {
		return nil, "", err
	}
	var record *kernel.Record
	if config.CoreID != PendingKernelID {
		stored, err := savedKernelFrom(s.db, config.CoreID, false)
		if err != nil {
			return nil, "", err
		}
		record = &stored
	}
	return &FingerprintPreview{Mode: "native", PreviewProfile: revision.Profile, CapabilityReport: capabilityReport(revision.Profile, record), Changes: []ProfileChange{}, Action: "edit"}, ref, nil
}
