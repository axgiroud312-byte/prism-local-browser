//go:build windows

package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// MigrationProbe owns one ephemeral local origin across the OLD and NEW launches
// of the same copied profile. It never navigates an existing user tab or exposes
// a general evaluate/navigation RPC. No source environment is opened by this API.
type MigrationProbe struct {
	url    string
	marker string
	close  func()
}

type MigrationObservation struct {
	Fingerprint  Observation `json:"fingerprint"`
	Cookie       bool        `json:"cookie"`
	LocalStorage bool        `json:"localStorage"`
	IndexedDB    bool        `json:"indexedDB"`
	SampledAt    string      `json:"sampledAt"`
}

func NewMigrationProbe() (*MigrationProbe, error) {
	url, closePage, err := newProbePage()
	if err != nil {
		return nil, err
	}
	return &MigrationProbe{url: url, marker: uuid.NewString(), close: closePage}, nil
}
func (p *MigrationProbe) Close() { p.close() }

// Sample seeds a synthetic canary under the old build or checks and removes it
// under the new build. This is a narrow storage-migration check, not a claim that
// every third-party site's login or all existing stored values are compatible.
func (p *MigrationProbe) Sample(ctx context.Context, process *ManagedProcess, record Record, input FingerprintInput, seedCanary bool) (_ MigrationObservation, resultErr error) {
	if process.network != nil {
		return MigrationObservation{}, problem("NETWORK_PROTECTION_UNAVAILABLE", "migration-local-probe-route", "此迁移诊断尚无已核验的代理本机测试路由，未改为直连。")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := process.pipe.beginCommand(ctx); err != nil {
		return MigrationObservation{}, err
	}
	defer func() { <-process.pipe.commandGate }()
	state := process.Snapshot()
	if !state.RootAlive || !state.ControlReady || state.ResourcesExited {
		return MigrationObservation{}, errors.New("migration process unavailable")
	}
	call := process.pipe.callLocked
	var browser struct {
		Product string `json:"product"`
	}
	if err := call(ctx, "Browser.getVersion", struct{}{}, "", &browser); err != nil {
		return MigrationObservation{}, err
	}
	var target struct {
		ID string `json:"targetId"`
	}
	if err := call(ctx, "Target.createTarget", map[string]any{"url": "about:blank"}, "", &target); err != nil {
		return MigrationObservation{}, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var result struct {
			Success bool `json:"success"`
		}
		err := call(cleanup, "Target.closeTarget", map[string]any{"targetId": target.ID}, "", &result)
		if err == nil && !result.Success {
			err = errors.New("migration probe tab close unconfirmed")
		}
		resultErr = errors.Join(resultErr, err)
	}()
	var session struct {
		ID string `json:"sessionId"`
	}
	if err := call(ctx, "Target.attachToTarget", map[string]any{"targetId": target.ID, "flatten": true}, "", &session); err != nil {
		return MigrationObservation{}, err
	}
	if err := call(ctx, "Page.navigate", map[string]any{"url": p.url}, session.ID, nil); err != nil {
		return MigrationObservation{}, err
	}
	script := `(async()=>{
 const marker=` + quoted(p.marker) + `, key='prism-migration-'+marker, seed=` + strconv.FormatBool(seedCanary) + `;
 const db=await new Promise((resolve,reject)=>{const r=indexedDB.open(key,1);r.onupgradeneeded=()=>r.result.createObjectStore('canary');r.onsuccess=()=>resolve(r.result);r.onerror=()=>reject(Error('open'));});
 try {
  if(seed){document.cookie=key+'='+marker+'; Max-Age=3600; Path=/; SameSite=Strict';localStorage.setItem(key,marker);await new Promise((resolve,reject)=>{const tx=db.transaction('canary','readwrite');tx.objectStore('canary').put(marker,'value');tx.oncomplete=resolve;tx.onerror=tx.onabort=()=>reject(Error('write'));});}
  const value=await new Promise((resolve,reject)=>{const tx=db.transaction('canary','readonly');const r=tx.objectStore('canary').get('value');r.onsuccess=()=>resolve(r.result);r.onerror=()=>reject(Error('read'));});
  const storage={cookie:document.cookie.split('; ').includes(key+'='+marker),localStorage:localStorage.getItem(key)===marker,indexedDB:value===marker};
  if(!seed){document.cookie=key+'=; Max-Age=0; Path=/';localStorage.removeItem(key);}
  const fingerprint=await ` + probeScript + `;
  return {...storage,fingerprint};
 } finally {db.close();if(!seed)await new Promise((resolve,reject)=>{const r=indexedDB.deleteDatabase(key);r.onsuccess=resolve;r.onerror=r.onblocked=()=>reject(Error('cleanup'));});}
})()`
	var result MigrationObservation
	for {
		var evaluation struct {
			Result struct {
				Value json.RawMessage `json:"value"`
			} `json:"result"`
			Exception json.RawMessage `json:"exceptionDetails"`
		}
		if err := call(ctx, "Runtime.evaluate", map[string]any{"expression": "location.href===" + quoted(p.url) + " ? " + script + " : null", "returnByValue": true, "awaitPromise": true}, session.ID, &evaluation); err != nil {
			return result, err
		}
		if len(evaluation.Exception) > 0 {
			return result, problem("KERNEL_INTEGRITY_FAILED", "migration-observation-failed", "工作副本的实际指纹或存储诊断失败，未切换原环境。")
		}
		if len(evaluation.Result.Value) > 0 && string(evaluation.Result.Value) != "null" {
			if err := json.Unmarshal(evaluation.Result.Value, &result); err != nil {
				return result, err
			}
			break
		}
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	result.Fingerprint.Seed, _ = strconv.Atoi(input.Seed)
	result.Fingerprint.BrowserVersion = strings.TrimPrefix(strings.TrimPrefix(browser.Product, "Chrome/"), "HeadlessChrome/")
	result.Fingerprint.PID = uint32(process.PID())
	result.Fingerprint.ProcessCreatedAt = process.CreatedAt()
	result.SampledAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := checkIdentity(result.Fingerprint, record.Version); err != nil {
		return result, err
	}
	if result.Fingerprint.Language != input.Language || result.Fingerprint.Timezone != input.Timezone || !strings.HasPrefix(result.Fingerprint.AcceptLanguage, input.Language) || (input.CPU != "auto" && strconv.Itoa(result.Fingerprint.CPU) != input.CPU) || !result.Cookie || !result.LocalStorage || !result.IndexedDB {
		return result, problem("KERNEL_INTEGRITY_FAILED", "migration-values-mismatch", "工作副本的参数或Cookie/LocalStorage/IndexedDB迁移读回不符，未切换原环境。")
	}
	return result, nil
}
