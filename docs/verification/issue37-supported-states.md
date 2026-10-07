[继续补核](issue37-continuation.md) · [原终验阶段](issue37.md) · [完整结构化清单](issue37-supported-states.json)

# #32–#37 支持入口与状态交叉清单

2026-10-06，独立只读源码清单的公开合成。**7路由、35所有者、274前端清单项；历史快照113 COVERED / 60 PARTIAL / 101 MISSING，不是274项验收通过。**

本清单读取时App SHA为`24c1abb35ae592eb9fa36cc61e747bb573e7babbfd0f1e86f6f7aea818f2158a`，没有证明当时Git HEAD/干净状态，不认证随后修复。P/ID绑定旧709源码的346图；E/ID绑定1c1f2d4修复前24图。当时E待视觉复核，随后实际结论22 MAPPED/2 FAIL，旧缺口不在本页静默改成通过。现有修复、当前证据及结果见继续补核，不使用旧图给新源码改标签。

全部条件窗口/弹窗归当前35组owner，回收和迁移是条件窗口，不新造路由。每项需求、源码定位（读取时行号）、实际入口、参考映射、确切证明键及轻量行为断言见JSON。源码入口不等于已经运行；同一frame不证明不同子内容/数量/模式/滚动/权限。缺截图不等于产品故障，缺完整原帮助是参考家族缺口，不是16张坏屏。

## 清单

| ID | 责任票 / owner | 模式 | 读取时快照覆盖 | 确切证明键 | 支持语义与边界 |
| --- | --- | --- | --- | --- | --- |
| S01 | #33 / shell | demo | COVERED | P/environment-list | demo shell/navigation/topbar geometry; full navigation action remains an assertion |
| S02 | #33 / shell | native | PARTIAL | P/native-proxy-list | native shell pictured on proxies, not proof of all route transitions/help/bell |
| S03 | #37 / workspace | native | MISSING | — | WORKSPACE_LOADING disables reread and has no diagnostic entry; distinct affordance |
| S04 | #37 / workspace | native | COVERED | P/workspace-native-fault | fault alertdialog, reread and diagnostics; bad bridge/read/schema codes share this container, not separate windows |
| S05 | #37 / workspace | demo | MISSING | — | damagedRecord has export-original and reset-demo buttons; not native reread UI |
| S06 | #37 / workspace | demo | MISSING | — | read/write conflict without damagedRecord→reload; no automatic overwrite |
| S07 | #37 / workspace | native | PARTIAL | P/native-restore-protected, P/native-recycle-protected, P/native-migration-progress | three separate restore/migration/recycle maintenance banners; related task views do not certify each banner and return target |
| S08 | #37 / workspace | both | MISSING | — | suspended draft notice→continue or abandon; resume usable native kernel without clearing draft |
| S09 | #33 / shell | both | PARTIAL | P/fingerprint-randomized, P/environment-open-failure | success/error toast shares App owner; only supplied strings/states bound, not every message or native notification |
| E01 | #33 / env | demo | COVERED | P/environment-list | ordinary list, top |
| E02 | #33 / env | demo | COVERED | P/environment-empty | empty workspace with first-create entry |
| E03 | #33 / env | demo | COVERED | P/environment-no-results | filtered zero rows/reset retains saved records |
| E04 | #33 / env | demo | COVERED | P/environment-selected | two selected, current-page bulk toolbar |
| E05 | #33 / env | demo | COVERED | P/environment-list-bottom | 10-row body bottom; not second page proof |
| E06 | #33 / env | both | PARTIAL | P/environment-list-bottom | page two with off-page selected IDs and partial header checkbox; no exact pair |
| E07 | #33 / env | native | MISSING | — | native normal/empty/no-results/query busy/query read error are service-page variants; demo PNG cannot certify labels and disabled controls |
| E08 | #33 / env | demo | COVERED | P/environment-row-menu | demo row menu five supported actions |
| E09 | #33 / env | native | MISSING | — | native row adds assign and full backup; template enters batch clone, not demo drawer |
| E10 | #33 / env | demo | COVERED | P/environment-create-menu | demo create dropdown |
| E11 | #33 / env | native | MISSING | — | native create dropdown additionally exposes batch history |
| E12 | #33 / env | demo | COVERED | P/environment-advanced-search, P/environment-group-popup | two named shared filter popovers at recorded initial input; group popup reference is in its exact proof |
| E13 | #33 / env | both | PARTIAL | P/environment-group-popup, P/environment-advanced-search | same JSX filter surfaces, but group-search-empty/reset/applied compound filter and native service scope lack exact pairs |
| E14 | #33 / env | both | MISSING | — | 更多操作 dropdown differs by mode and selection; native FIFO retry, batch clone/assign/history/full selected backup |
| E15 | #33 / runtime | demo | COVERED | P/environment-running | simulated running row, close entry |
| E16 | #33 / runtime | demo | COVERED | P/environment-open-failure | open failed outcome and retry |
| E17 | #33 / runtime | demo | MISSING | — | starting/stopping storage write failure→模拟结果待保存 and retry open/close |
| E18 | #33 / runtime | both | PARTIAL | P/environment-open-failure | mixed pending/accepted/success/error/skipped per-item outcomes, retry and collapse; one failure is not mixed-bulk proof |
| E19 | #33 / runtime | native | MISSING | — | starting queued FIFO bar/cancel queue, starting active/cancel start and stopping disabled; phases in same table, not extra windows |
| E20 | #33 / runtime | native | PARTIAL | P/native-busy-running, P/native-busy-persistence, P/native-busy-reconcile, P/native-busy-resources | normal running/resource cleanup/reconcile/persistence row affordances; those exact proofs are edit-drawer captures, not each list variant |
| E21 | #33 / runtime | native | MISSING | — | missing saved proxy renders refusal not direct; missing/unavailable exact core is 未就绪 |
| E22 | #33 / runtime | native | COVERED | P/native-runtime-details | busy-resources exact popup, only its actual snapshot |
| E23 | #33 / runtime | native | PARTIAL | P/native-runtime-details | direct-policy/error-only popup; NativeRuntimeNetwork intentionally absent for direct; not proven by proxy popup |
| E24 | #33 / runtime | native | MISSING | — | current same-channel precheck in progress/pass/fail/write-pending labels; expanded phases contain channel/target/observed IP |
| E25 | #33 / runtime | native | MISSING | — | historical report and reconcile disclaimer do not claim live bridge ownership |
| E26 | #33 / runtime | native | MISSING | — | NETWORK_PROTECTION_UNAVAILABLE versus containment stopped/exit-unconfirmed alerts; preserved proxy policy, no separate modal |
| G01 | #33 / groups | demo | COVERED | P/group-list | derived labels/counts, not independent group entities |
| G02 | #33 / groups | native | MISSING | — | 当前页环境数 and rename only loaded members require distinct native capture |
| G03 | #33 / groups | both | MISSING | — | group search empty/reset and view-group return filter; no fake create/delete |
| G04 | #33 / group-window | demo | COVERED | P/group-add, P/group-edit | same assignment modal reached from selected list and existing group; references differ explicitly |
| G05 | #33 / group-window | native | PARTIAL | P/group-add, P/group-edit | byte-identical modal definition for equal props; native exact loaded IDs, busy and mixed saved outcomes are not demo behavior evidence |
| G06 | #33 / group-window | demo | COVERED | P/environment-group-save-error | save write error keeps entered label and IDs |
| F01 | #34 / form | demo | COVERED | P/environment-create | create basic top |
| F02 | #34 / form | demo | COVERED | P/environment-create-proxy | proxy section, direct versus saved node semantics |
| F03 | #34 / form | demo | COVERED | P/environment-create-preferences | URLs/window/restore-tabs section |
| F04 | #34 / form | demo | COVERED | P/environment-create-fingerprint | supported fingerprint section |
| F05 | #34 / form | demo | COVERED | P/environment-create-bottom | long body bottom with fixed primary footer |
| F06 | #34 / form | demo | COVERED | P/environment-edit, P/environment-clone | two distinct snapshots: edit retains identity; template new drawer creates identity, not native batch clone |
| F07 | #34 / form | native | PARTIAL | P/native-environment-create-pending, P/native-profile-history | idle native create top/proxy/preferences/fingerprint/bottom and idle edit top require exact mode/content pairs; locked create/history are not those states |
| F08 | #34 / form | both | MISSING | — | create no usable core→cannot generate/save; edit saved-core unavailable has different message; native kernel-pending metadata edit is separate allowed-save branch |
| F09 | #34 / form | demo | COVERED | P/kernel-selector | service usable-core dropdown geometry |
| F10 | #34 / form | native | PARTIAL | P/kernel-selector | same select API but native version labels and saved-build lock differ; demo dropdown does not prove native |
| F11 | #34 / profile | both | MISSING | — | automatic preview busy, stale device input and preview failure/retry are same drawer branches, not new windows |
| F12 | #34 / profile | demo | COVERED | P/fingerprint-randomized | explicit regenerate notification; draft only |
| F13 | #34 / profile | native | PARTIAL | P/fingerprint-randomized | identical App toast text does not certify native generated preview/capability source |
| F14 | #34 / profile | native | COVERED | P/native-profile-history | technical/history opened at bottom; only supplied revisions and capabilities |
| F15 | #34 / profile | demo | MISSING | — | demo technical details/history has distinct demo capability source labels |
| F16 | #34 / profile | both | PARTIAL | P/native-profile-history | rollback preview/restoredFrom/change list/new revision and incompatible-history disabled buttons are not pictured by history list alone |
| F17 | #34 / form | native | COVERED | P/native-busy-running, P/native-busy-resources, P/native-busy-persistence, P/native-busy-reconcile | four exact metadata-only locked edit snapshots; does not prove all downstream runtime actions |
| F18 | #34 / form | demo | COVERED | P/environment-create-save-error, P/environment-edit-save-error | two exact known-save-failure snapshots; edit uses environment-edit reference |
| F19 | #34 / form | native | MISSING | — | known create/edit/profile revision refusal retains mutable draft; history-read failure sets form error |
| F20 | #34 / form | demo | COVERED | P/environment-quantity-invalid | invalid integer quantity no submit |
| F21 | #34 / form | native | PARTIAL | P/environment-quantity-invalid | same validation but native count>1 routes plan and source capture absent for invalid native quantity |
| F22 | #34 / form | demo | COVERED | P/batch-create | multi-count create drawer, not execution progress/cancel proof |
| F23 | #34 / form | native | COVERED | P/native-environment-create-pending | in-flight admission held; close/save inputs locked |
| F24 | #34 / form | native | COVERED | P/native-environment-create-unknown | unknown no-id original create request frozen; explicit核实 |
| F25 | #34 / form | native | COVERED | P/native-environment-create-accepted | accepted operation then held Operation.Read; original ID only |
| F26 | #34 / form | both | MISSING | — | create terminal no-completed-IDs returns editable draft; partial completion retains created IDs; persistence pending cannot prematurely close as success |
| F27 | #33 / runtime | demo | COVERED | P/environment-created-open-failure | created ID retained; open failed with saved proxy refusal |
| F28 | #33 / runtime | native | PARTIAL | P/environment-created-open-failure | native created-then-open-failure/retry exact ID must be separately exercised, not borrowed from demo |
| F29 | #34 / env-confirm | demo | COVERED | P/environment-dirty-confirm | dirty alertdialog over same draft |
| F30 | #34 / env-confirm | native | PARTIAL | P/environment-dirty-confirm | byte-identical dirty child message; native cancellation/preview ownership not proven by demo |
| F31 | #34 / env-confirm | native | COVERED | P/native-force-confirm | exact environment/session force warning opened only; zero ForceStop in bound capture |
| F32 | #34 / env-confirm | native | PARTIAL | P/native-force-confirm, E/native-activity-session-force-confirm | valid control-lost force authority and stale/blocked/reconcile revalidation are delegated; no fixfile/test review here |
| I01 | #35 / draft-import | demo | COVERED | P/environment-create-proxy-manager, P/demo-environment-proxy-preview | unique importer atop retained draft; preview uses proxy-import-preview mapping |
| I02 | #35 / draft-import | native | COVERED | P/native-environment-proxy-preview | native parent import preview; no alternate import owner |
| I03 | #35 / draft-import | demo | COVERED | P/demo-environment-proxy-failure-return, P/demo-environment-proxy-success-return | known fail/success named snapshots returning retained draft |
| I04 | #35 / draft-import | native | COVERED | P/native-environment-proxy-failure-return, P/native-environment-proxy-success-return, P/native-environment-proxy-unknown-return | known fail/success/unknown original-return snapshots; unknown uses proxy-import-error ref |
| I05 | #35 / draft-import | both | PARTIAL | P/native-proxy-import-return | page importer hide/reopen is related, not both actual-App draft cancel/Escape/reopen focus and identical inputs proof |
| C01 | #34 / demo-cookie | demo | COVERED | P/cookie-populated | parsed two-row demo Cookie snapshot, not create-time Cookie field |
| C02 | #34 / demo-cookie | demo | MISSING | — | demo empty text, parse invalid rows, selected file returning raw text and save write failure; no native file subwindow or explicit File.text rejection alert is invented |
| C03 | #34 / cookie | native | COVERED | P/native-cookie-text | masked raw input before parse |
| C04 | #34 / cookie | native | COVERED | P/cookie-import | 520px read-UTF8 file window, explicitly not ZIP import |
| C05 | #34 / cookie | native | COVERED | P/native-cookie-file-selected, P/native-cookie-file-error | file read returns same text window; invalid UTF8 remains file window with error |
| C06 | #34 / cookie | native | COVERED | P/native-cookie-preview | safe parsed metadata, selected valid rows; only supplied fixture data |
| C07 | #34 / cookie | native | PARTIAL | P/native-cookie-preview | input-key conflicts/expired/partition attributes, parser error/observation failure and dense body bottom require explicit fixture branch coverage |
| C08 | #34 / cookie | native | MISSING | — | no controlled writer: explicit direct startup consent versus saved proxy refusal; starting/unknown session cannot write |
| C09 | #34 / cookie | native | MISSING | — | replace-all destructive checkbox inline in same window, not separate alertdialog; retry must never re-clear |
| C10 | #34 / cookie | native | COVERED | P/native-cookie-pending | accepted active result at body bottom/cancel remainder |
| C11 | #34 / cookie | native | COVERED | P/native-cookie-partial, P/native-cookie-unknown | partial and item-unknown reports, distinct from admission unknown |
| C12 | #34 / cookie | native | COVERED | P/native-cookie-unconfirmed | admission unknown freezes original rows/policy/request; replay with a new range or clear policy is not allowed |
| C13 | #34 / cookie | native | MISSING | — | durable all-verified/already-matched result, cancelled-remainder and persistence-pending differ from partial/unknown; original effects preserved |
| C14 | #34 / cookie | native | PARTIAL | P/native-cookie-partial | only retry selected unverified keys/merge, close then reopen safe report; an available retry button is not retry result proof |
| B01 | #34 / batch | native | COVERED | P/native-batch-create-plan, P/native-batch-clone | three-item create plan and two-source clone plan are explicit separate snapshots |
| B02 | #34 / batch | native | COVERED | P/native-batch-proxy-assignment | assignment editor requires per-ID explicit mapping |
| B03 | #34 / batch | native | MISSING | — | assignment preview direct/shared acknowledgements and unset mapping error; same body has changed controls |
| B04 | #34 / batch | native | COVERED | P/native-batch-current, P/native-batch-history | current plan and old attempt frozen observations; not all progress branches |
| B05 | #34 / batch | native | COVERED | P/batch-progress-fixture | 400px result frame for supplied current batch state only; display container mapping |
| B06 | #34 / batch | native | PARTIAL | P/batch-progress-fixture | old-result title and active/persist-pending/final mixed result footers are not all proven by same frame |
| B07 | #34 / batch | native | COVERED | P/native-batch-page-error | second page read fails; stored task itself not reported failed |
| B08 | #34 / batch | native | MISSING | — | successful page two, dense table bottom, ID lookup empty/error and old/current handoff |
| B09 | #34 / batch | native | MISSING | — | accepted active/cancelRequested, persistence pending and missing final detail after confirmed counts preserve phase/authority; no new modal |
| B10 | #34 / batch | native | PARTIAL | P/native-batch-history | known/unknown commit refusal and continue-not-completed only; must observe real synthetic calls/new attempt without duplicating completed IDs |
| B11 | #34 / form | demo | PARTIAL | P/batch-create | demo App batch status bar/progress/cancel/mixed saved results not certified by multi-count drawer |
| R01 | #34 / recycle | native | COVERED | P/recycle-list | page-shaped manager, not a separately supported hash route |
| R02 | #34 / recycle | native | MISSING | — | unread/read-error/empty list branches and explicit retry |
| R03 | #34 / recycle | native | MISSING | — | dense bottom/page two/selected IDs/original identity details |
| R04 | #34 / recycle | native | COVERED | P/native-environment-remove | exact selected remove preflight and consent; no directory mutation in capture |
| R05 | #34 / recycle | native | COVERED | P/recycle-delete-confirm, P/recycle-restore | purge warning and 440px restore original-group window separate exact proofs; original restore ref is recycle-restore |
| R06 | #34 / recycle | native | MISSING | — | multi-page target confirmation/body bottom and restore identity details expanded; all total IDs require consent, not current page |
| R07 | #34 / recycle | native | COVERED | P/native-recycle-protected | protected failure result and maintenance fact |
| R08 | #34 / recycle | native | MISSING | — | admission unknown/pending original request in confirmation; active/cancelled/failed-unprotected/completed results and per-item details differ |
| R09 | #34 / recycle | native | PARTIAL | P/native-recycle-protected | retry exact protected task and read/cancel historical operation cannot infer success from preseeded result |
| R10 | #34 / env-confirm | demo | COVERED | P/environment-delete-confirm | demo remove record confirmation; not native recycle |
| R11 | #34 / env-confirm | demo | MISSING | — | demo running refusal and save-write failure stay same remove window |
| P01 | #35 / proxy-list | demo | COVERED | P/proxy-list, P/proxy-no-results | ordinary and nonmatch lists; not dense/selected variants |
| P02 | #35 / proxy-list | native | COVERED | P/native-proxy-list | native list with native status/credential labels |
| P03 | #35 / proxy-list | both | MISSING | — | empty versus filtered native empty, status filter, dense body bottom/page2 and off-page selected checks scope |
| P04 | #35 / proxy-list | both | MISSING | — | add-method popup and more popup; both managers have refresh/clear, native additionally exposes assignment |
| P05 | #35 / proxy-list | native | MISSING | — | independent check accepted/stage/cancel-requested/write-pending row buttons and admission unknown verification; not runtime launch authority |
| P06 | #35 / proxy-import | both | COVERED | P/proxy-add, P/native-proxy-add | single add initial dialog exists in BOTH exact pairs, no authentication preview yet |
| P07 | #35 / proxy-import | both | MISSING | — | single add authenticated preview, error/duplicate/selection, SOCKS byte length constraints; native/demo parse labels differ |
| P08 | #35 / proxy-import | both | COVERED | P/proxy-import-text, P/native-proxy-import-text | empty text initial window: two exact modes, not mode inheritance |
| P09 | #35 / proxy-import | both | COVERED | P/proxy-import-preview, P/native-proxy-import-preview | two-row safe preview in each mode |
| P10 | #35 / proxy-import | both | COVERED | P/proxy-import-error, P/native-proxy-import-error | format-error preview in both exact modes |
| P11 | #35 / proxy-import | demo | COVERED | P/proxy-import-duplicates | duplicate and existing addresses explicitly shown |
| P12 | #35 / proxy-import | native | PARTIAL | P/proxy-import-duplicates | native duplicates/groups and authenticated redaction require actual native parse; demo duplicate snapshot is related |
| P13 | #35 / proxy-import | both | COVERED | P/proxy-import-file, P/native-proxy-import-file | file chooser app frame in each mode, not system chooser screenshot |
| P14 | #35 / proxy-import | demo | COVERED | P/proxy-import-file-selected, P/proxy-import-file-error | selected filename and invalid UTF8 error exact pairs |
| P15 | #35 / proxy-import | native | PARTIAL | P/proxy-import-file-selected, P/proxy-import-file-error | shared loadFile does not prove native locked/error/selection labels; file cancel can simply preserve input without new modal |
| P16 | #35 / proxy-import | both | MISSING | — | raw reveal toggle, dense capped-height preview bottom, partial valid-row save leaving remainder and fresh-preview invalidation |
| P17 | #35 / proxy-import | native | COVERED | P/native-proxy-save-failure, P/native-proxy-save-unknown | known commit error versus frozen original unknown admission |
| P18 | #35 / proxy-import | native | COVERED | P/native-proxy-import-return | page importer hide/reopen retains preview and input; not every environment draft return |
| P19 | #35 / proxy-edit | both | COVERED | P/proxy-edit, P/native-proxy-edit | keep-credential edit baseline has exact pair in both modes |
| P20 | #35 / proxy-edit | native | MISSING | — | replace adds two masked fields and increases height614→726; bottom at1280 required, SOCKS lengths and HTTPS warning differ |
| P21 | #35 / proxy-edit | both | MISSING | — | clear versus keep no-auth label, demo replace and simulated-failure checkbox; protocol warning conditional branches |
| P22 | #35 / proxy-edit | native | MISSING | — | save known refusal, unknown locked original config/retry/hide/reopen; native action owner persists beyond visible editor |
| P23 | #35 / proxy-usage | both | COVERED | P/proxy-bound, P/native-proxy-bound | bound-usage initial list in each mode |
| P24 | #35 / proxy-usage | native | MISSING | — | off-page ID shows未在当前页, usedCount may exceed loaded IDs; selected reassignment routes exact IDs to native batch |
| P25 | #35 / proxy-usage | both | MISSING | — | empty/no-results/dense bottom/page2 usage; selected count and final-page controls |
| P26 | #35 / proxy-delete | demo | COVERED | P/proxy-delete-confirm | demo unreferenced delete confirm; group-delete only geometry mapping |
| P27 | #35 / proxy-delete | native | MISSING | — | native delete confirm, referenced protection, refusal/unknown original delete verification; not same native credential behavior as demo |
| P28 | #35 / proxy-report | demo | MISSING | — | 620px demo report has unchecked/failure/success simulation text, no native stage table; checking is a LIST label, not a fourth report branch |
| P29 | #35 / proxy-report | native | MISSING | — | empty/unchecked report: no safe current observation, no inferred exit/auth success |
| P30 | #35 / proxy-report | native | MISSING | — | in-progress exact-revision task report with running steps and 未完成 |
| P31 | #35 / proxy-report | native | MISSING | — | failed check shows safe error, failed/unsupported stages and target policy |
| P32 | #35 / proxy-report | native | MISSING | — | task persistencePending shows结果待保存 not durable terminal; phase retained in same report container |
| P33 | #35 / proxy-report | native | MISSING | — | current exact-revision complete report IP/time/stages/resolution; independent check grants no startup/permanent authority |
| P34 | #35 / proxy-report | native | MISSING | — | old proxyId/revision report filtered out entirely; same modal no stale IP/table, unlike runtime's historical report |
| P35 | #35 / proxy-report | native | MISSING | — | long safe stage explanations/report bottom; cannot inherit short empty620px container screenshot |
| K01 | #35 / kernel | demo | COVERED | P/demo-kernel-list | metadata list, no install controls; explicitly not real installed kernels |
| K02 | #35 / kernel | native | COVERED | P/native-kernel-list | service IDs/hashes/default and real-report projection, not current environment sample |
| K03 | #35 / kernel | both | MISSING | — | no registered core vs filtered no-match, unavailable records, dense bottom/page2; mode-specific controls |
| K04 | #35 / kernel | native | PARTIAL | P/native-kernel-list | set default/manual-selection-pending, unknown original-default replay and referenced/default delete disabled; ordinary list insufficient for each action state |
| K05 | #35 / kernel-details | demo | COVERED | P/demo-kernel-details | demo capability drawer top only |
| K06 | #35 / kernel-details | native | COVERED | P/native-kernel-details, P/native-kernel-details-bottom | native capability drawer top and bottom, precise build evidence labels |
| K07 | #35 / kernel-details | demo | PARTIAL | P/demo-kernel-details | demo long detail bottom remains uncaptured; top not proof final controls reachable with dense content |
| K08 | #35 / kernel-prepare | native | COVERED | P/native-kernel-prepare, P/native-kernel-prepare-local | official420px and trusted-local530px forms are separate exact snapshots |
| K09 | #35 / kernel-prepare | native | MISSING | — | local archive selected/trust-reset, invalid version/SHA, chooser refused/unknown messages; cancelled chooser leaves same form, not new dialog |
| K10 | #35 / kernel-prepare | native | MISSING | — | unknown install freezes source/version/SHA/token/trust and shows原请求核实; no task progress if no known operation |
| K11 | #35 / kernel-task | native | COVERED | P/native-kernel-progress | recorded in-flight task container; does not cover all label/permission branches |
| K12 | #35 / kernel-task | native | COVERED | P/native-kernel-persistence-pending | completed observation with persistencePending remains nonterminal and cannot cancel/retry as ordinary terminal |
| K13 | #35 / kernel-task | native | COVERED | P/native-kernel-failed | failed result and retry/prepare entry |
| K14 | #35 / kernel-task | native | MISSING | — | cancel requested/cancel result/unknown cancel and interrupted result preserve exact task; cancelled/interrupted aren't failed-completed equivalence |
| K15 | #35 / kernel-task | native | MISSING | — | durable completed, verify/delete task results and acquiring/extracting/probing/verifying/publishing labels in same container |
| K16 | #35 / kernel-task | native | MISSING | — | accepted refresh error must retain task/admission and continue read, NOT unknown no-task or resubmit |
| K17 | #35 / kernel-task | native | MISSING | — | Operation.Read error/late previous receipt and retry-same-build transition; phase facts, not extra windows |
| K18 | #35 / kernel-task | native | COVERED | P/native-kernel-history | recent task table baseline |
| K19 | #35 / kernel-task | native | MISSING | — | history empty/dense bottom and expanded reverify observations, exact history-task selection |
| K20 | #35 / kernel | native | COVERED | P/native-kernel-delete-confirm | unreferenced exact-build delete confirm |
| K21 | #35 / kernel | native | MISSING | — | native delete/verify refusal or unknown original request; protected referenced/default button must stay disabled |
| M01 | #35 / migration | native | COVERED | P/native-migration-select | 1040px selection initial, explicit one environment |
| M02 | #35 / migration | native | MISSING | — | lookup page2/search-empty/read-error/late result and no alternative verified build; selecting target resets diff |
| M03 | #35 / migration | native | COVERED | P/native-migration-diff | diff and saved exact-target network policy; no Prepare call |
| M04 | #35 / migration | native | MISSING | — | expanded before/after capabilities/actual parameter lists and dense diff bottom/fixed footer |
| M05 | #35 / migration | native | MISSING | — | exact saved policy read failure/revision change/malformed preview keep trial blocked; no implicit direct |
| M06 | #35 / migration | native | COVERED | P/native-migration-confirm | trial confirmation unchecked for supplied policy ONLY; explicit note says do not submit Prepare |
| M07 | #35 / migration | native | PARTIAL | P/native-migration-confirm | direct versus proxy trial consent and checked+Prepare transition need exact variants and synthetic calls; one warning isn't both |
| M08 | #35 / migration | native | MISSING | — | Prepare unknown with/no operationId keeps原迁移请求; same confirmation/selection/result depending reply, no new copy |
| M09 | #35 / migration | native | COVERED | P/native-migration-progress | preseeded accepted task, read/cancel only; not evidence Prepare/backup/copy/trial occurred |
| M10 | #35 / migration | native | MISSING | — | backup-ready/copy-ready/trial-starting phases in same task window; exact backupVerified/archive facts required |
| M11 | #35 / migration | native | MISSING | — | trial-running shows actual synthetic before/after samples and正常停止试用副本; stop allowed even persistence pending with after&&!trialExited |
| M12 | #35 / migration | native | MISSING | — | StopTrial outcome/unknown and ready after confirmed whole-tree exit; ready enables明确切换 |
| M13 | #35 / migration | native | MISSING | — | separate400px switch confirmation with explicit checkbox; not covered by trial confirmation |
| M14 | #35 / migration | native | MISSING | — | prepared/committed/completed reports with switched configuration/backup/trial facts; no automatic default or seed change |
| M15 | #35 / migration | native | MISSING | — | protected/storage-pending/acceptance-pending keep maintenance;失败 or read error cannot unlock or claim switch complete |
| M16 | #35 / migration | native | COVERED | P/native-migration-cancelled | cancel result for preseeded task only; not every post-Prepare rollback path |
| M17 | #35 / migration | native | MISSING | — | Recover exact original pending/protected task; known failure retains original state and fresh diff retry; no generic rerun-Prepare button |
| M18 | #35 / migration | native | MISSING | — | history details/old-task selection and verified-backup gate for升级前完整恢复; no old-kernel launch on upgraded data |
| M19 | #35 / migration-restore | native | MISSING | — | upgrade-before restore preview is INLINE in migration selection with counts/kernel lines, NOT NativeRestoreManager three-tab preflight |
| M20 | #35 / migration-restore | native | PARTIAL | P/native-restore-confirm | same NativeRestoreExecution confirm child for equal props; migration stack/preview consumption and required acks need direct proof |
| M21 | #35 / migration-restore | native | PARTIAL | P/native-restore-progress, P/native-restore-protected, P/native-restore-rollback, P/native-restore-success | restore result/history shares component but migration context/ownership differs; kernel-scoped complete/protected/retry remains unpictured |
| M22 | #35 / migration-restore | native | MISSING | — | upgrade rollback preview error/expired/discard/close and consume exact preview; closing selection must not discard accepted restore authority |
| D01 | #36 / demo-backup | demo | COVERED | P/demo-backup-list | demo JSON snapshot list |
| D02 | #36 / demo-backup | demo | COVERED | P/demo-backup-dense-top, P/demo-backup-dense-bottom, P/demo-backup-dense-page-two | three exact dense positions/page2 |
| D03 | #36 / demo-backup | demo | MISSING | — | empty/no matching snapshot list and search-reset |
| D04 | #36 / demo-backup | demo | COVERED | P/demo-backup-create, P/demo-backup-save-error | create500px form and failed write retained original workspace |
| D05 | #36 / demo-backup | demo | COVERED | P/demo-backup-import, P/demo-backup-import-error | initial import and invalid format error; not native package |
| D06 | #36 / demo-backup | demo | MISSING | — | file selected/cancel/read-in-flight/oversize and retry valid; filename/error preservation |
| D07 | #36 / demo-backup | demo | PARTIAL | P/demo-backup-import | upload header hover/foreground actualApp fix tests delegated; no active stylesheet review |
| D08 | #36 / demo-backup | demo | PARTIAL | P/demo-backup-list | download compatible snapshot bytes/filename excludes proxy passwords but preserves synthetic Cookie; visible export button not download proof |
| D09 | #36 / demo-restore | demo | COVERED | P/demo-restore-preview | small read-only restore preview |
| D10 | #36 / demo-restore | demo | PARTIAL | E/demo-restore-dense-bottom | 12-row dense preflight table/body bottom; reuse extra pair, review outstanding |
| D11 | #36 / demo-restore | demo | COVERED | P/demo-restore-confirm, P/demo-restore-save-error | nested confirm and retained failed-write error; no standalone success-result window in demo |
| D12 | #36 / demo-restore | demo | MISSING | — | running/starting/stopping demo environments block restore; zero-item compatible snapshot still replaces demo config |
| D13 | #36 / demo-restore | demo | PARTIAL | P/demo-restore-save-error | successful retry closes to list and keeps compatible identity/history; Escape backs confirm only, not entire preflight |
| N01 | #36 / backup | native | COVERED | P/native-backup-list, P/native-backup-task-list | published and task tabs separate exact snapshots |
| N02 | #36 / backup | native | COVERED | P/native-backup-dense, P/native-backup-dense-bottom, P/native-backup-dense-page-two | published dense top/bottom/page2 exact pairs |
| N03 | #36 / backup | native | MISSING | — | task empty/nonmatch/dense bottom/page2 and published empty labels differ |
| N04 | #36 / backup | native | COVERED | P/native-backup-create, P/native-backup-selected-create | all persisted environments versus frozen selectedIDs forms; selected proof enters environments then backups |
| N05 | #36 / backup | native | MISSING | — | destination chooser cancelled before admission/refused, stop checkbox missing and no selected IDs disabled; distinct from task cancelled |
| N06 | #36 / backup | native | COVERED | P/native-backup-progress, P/native-backup-published | accepted progress versus published hash/filename result |
| N07 | #36 / backup | native | COVERED | P/native-backup-unknown, P/native-backup-unknown-no-id, P/native-backup-original-request-return | original unknown with/without taskID and route-return exact snapshots |
| N08 | #36 / backup | native | COVERED | P/native-backup-cancelled, P/native-backup-failed | cancelled export task and failed report, not source/destination chooser cancel |
| N09 | #36 / backup | native | MISSING | — | publication-pending/storage-pending, cancelRequested/cancel unavailable publishing, interrupted and incomplete completed-stage report cannot claim published success |
| N10 | #36 / backup | native | MISSING | — | known admission refusal retains editable output/scope; exact-ID read error and replay/recover preserve original task, no new export |
| Q01 | #36 / restore | native | COVERED | P/native-backup-import | native import initial read-only window; complete package not demo JSON |
| Q02 | #36 / restore | native | PARTIAL | P/native-backup-import | native upload header hover/foreground delegates active fix tests |
| Q03 | #36 / restore | native | MISSING | — | source chooser selected/cancelled/error, streaming preflight busy/failure and retry remain import; zero ApplyRestore |
| Q04 | #36 / restore | native | COVERED | P/native-restore-preview, P/native-restore-conflict, P/native-restore-conflict-bottom | small preflight and conflict/missing-core blocked counts with bottom; only supplied impact pages |
| Q05 | #36 / restore | native | COVERED | P/native-restore-preflight-kernels, P/native-restore-preflight-credentials | two exact section tabs: precise builds and credential projection |
| Q06 | #36 / restore | native | PARTIAL | E/native-restore-dense-top, E/native-restore-dense-bottom | 25-row impact first page top and bottom; exact existing extras, not new take required |
| Q07 | #36 / restore | native | PARTIAL | E/native-restore-dense-page-two, E/native-restore-dense-page-error | page2 one row and read error; extras assert offsets0/25 and no restoration; recovery check is action evidence not new PNG verdict |
| Q08 | #36 / restore | native | PARTIAL | P/native-restore-preflight-kernels, P/native-restore-preflight-credentials | pending/missing/unavailable/same-digest mapped kernels; credentials none/current-user/reentry and dense tab bottoms beyond supplied rows |
| Q09 | #36 / restore | native | MISSING | — | discard unknown freezes confirm but allows retry cancel/close only; late page/preview cleanup must preserve newer source |
| Q10 | #36 / restore | native | MISSING | — | read-only empty impact page, conflict resolved/repreflight and source-change invalidation; no seed conflict bypass |
| Q11 | #36 / restore-execution | native | COVERED | P/native-restore-confirm | required-stop/overwrite/credential confirmation snapshot for supplied preview |
| Q12 | #36 / restore-execution | native | PARTIAL | P/native-restore-confirm | each required checkbox independently gates submit; no-overwrite/no-reentry has only stop checkbox, different content |
| Q13 | #36 / restore-execution | native | COVERED | P/native-restore-progress, P/native-restore-unknown | accepted progress versus original admission unknown reports; exact IDs/hashes retained |
| Q14 | #36 / restore-execution | native | COVERED | P/native-restore-rollback, P/native-restore-protected, P/native-restore-success | rolled-back, protected and count-gated full success exact snapshots; none is native directory evidence |
| Q15 | #36 / restore-execution | native | MISSING | — | cancelRequested/cancelled/not-accepted refusal, storage-pending vs finalized-unconfirmed and recovering/workspace-recovery phases preserve task authority |
| Q16 | #36 / restore-execution | native | MISSING | — | recoveredAfterRestart + interruptedStage adds text; protected-failed repair instructions and exact RecoverRestore retry cannot inherit ordinary result |
| Q17 | #36 / restore-execution | native | COVERED | P/native-restore-history | recent restore history exact snapshot |
| Q18 | #36 / restore-execution | native | MISSING | — | history empty/dense bottom/old-task read while current pending cannot consume newer request or alter session |
| A01 | #36 / activity | demo | COVERED | P/activity-list, P/activity-empty, P/activity-page-two | demo ordinary/empty/page2 exact snapshots |
| A02 | #36 / activity | native | PARTIAL | P/native-activity-session-details | native list header has diagnostic controls; detail screenshot does not prove ordinary/empty/page2/filter/native dense list |
| A03 | #36 / activity | both | MISSING | — | result filters success/error/info plus keyword reset; dense final body rows not every normal/page2 screenshot |
| A04 | #36 / activity | demo | PARTIAL | P/activity-list | export activity JSON filename/content is action proof, not visible-button proof |
| A05 | #36 / activity-detail | demo | COVERED | P/activity-details | record object/result/time/detail baseline |
| A06 | #36 / activity-detail | both | PARTIAL | P/activity-details | errorCode/nextAction and long detail bottom, native no-session disclaimer; not inferred from short demo detail |
| A07 | #36 / activity-detail | native | COVERED | P/native-activity-session-details | exact current session detail from local fixture; no force/reconcile execution certified |
| A08 | #36 / activity-detail | native | PARTIAL | E/native-activity-session-old | old session no control buttons, exact extra pair; zero dispatch |
| A09 | #36 / activity-detail | native | PARTIAL | E/native-activity-session-reconcile, E/native-activity-session-pending | needsReconcile and held exact Reconcile request disables action; pending extra holds supported method before capture |
| A10 | #36 / activity-detail | native | PARTIAL | E/native-activity-session-force-confirm | force confirmation above native detail, exact session IDs; actualRoute activity although planned entry route environments |
| A11 | #36 / activity-detail | native | MISSING | — | blockedIds disable session buttons; missing/mismatched identity or canForce false/needsReconcile cannot force, no new visual container |
| A12 | #36 / activity-detail | native | PARTIAL | P/native-activity-session-details | control-lost owned force dispatch+fresh confirmation tests are isolated implementer's work; inventory does not evaluate active fixes |
| H01 | #36 / guide | demo | PARTIAL | P/guide-user | user document top implementation present; old verdict MISSING refers original help, not broken runtime |
| H02 | #36 / guide | demo | PARTIAL | P/guide-user-bottom | user document bottom implementation present |
| H03 | #36 / guide | demo | PARTIAL | P/guide-prd | PRD top; current factual corrections remain MAIN's docs work |
| H04 | #36 / guide | demo | PARTIAL | P/guide-prd-bottom | PRD body bottom implementation present |
| H05 | #36 / guide | demo | PARTIAL | P/guide-development | development document top implementation present |
| H06 | #36 / guide | demo | PARTIAL | P/guide-development-bottom | development document bottom implementation present |
| H07 | #36 / guide | demo | PARTIAL | P/guide-kernel | kernel contract top implementation present |
| H08 | #36 / guide | demo | PARTIAL | P/guide-kernel-bottom | kernel contract bottom implementation present |
| H09 | #36 / guide | native | MISSING | — | native user tab top has本机使用指南与排错 banner; cannot inherit demo banner |
| H10 | #36 / guide | native | MISSING | — | native user tab bottom |
| H11 | #36 / guide | native | MISSING | — | native PRD tab top |
| H12 | #36 / guide | native | MISSING | — | native PRD tab bottom |
| H13 | #36 / guide | native | MISSING | — | native development tab top |
| H14 | #36 / guide | native | MISSING | — | native development tab bottom |
| H15 | #36 / guide | native | MISSING | — | native kernel contract top |
| H16 | #36 / guide | native | MISSING | — | native kernel contract bottom |
| H17 | #36 / guide | both | PARTIAL | P/guide-user, P/guide-prd, P/guide-development, P/guide-kernel | four downloads exact USER_GUIDE.md/PRD.md/DEVELOPMENT.md/KERNEL.md bytes; guide screenshots do not prove download artifacts |
| H18 | #36 / guide | both | PARTIAL | P/guide-user | four Markdown internal-doc switches + sidebar doc buttons, five function entries; body resets to top, external links docHref safe |
| H19 | #36 / guide | both | MISSING | — | lazy Markdown fallback 正在载入文档 exists; stable capture depends chunk hold/acquisition not evaluated; no fabricated permanent loading page |
| X01 | #36 / diagnostic | native | COVERED | P/native-diagnostics-preview, P/native-diagnostics-bottom | regular white-list preview top and bottom |
| X02 | #36 / diagnostic | native | COVERED | P/native-diagnostics-minimal | workspace unavailable minimal report top |
| X03 | #36 / diagnostic | native | PARTIAL | E/native-diagnostics-minimal-bottom | minimal report bottom exact extra pair; safe fields only, not raw pre |
| X04 | #36 / diagnostic | native | COVERED | P/workspace-native-fault-diagnostic, P/workspace-cookie-fault-diagnostic | workspace-fault diagnostic top, with and without retained Cookie input; highest owner distinct from ordinary activity preview |
| X05 | #36 / diagnostic | native | PARTIAL | E/workspace-fault-diagnostic-bottom, E/workspace-cookie-fault-diagnostic-bottom | two workspace stack bottom variants, exact extras; lower Cookie remains masked/inert |
| X06 | #36 / diagnostic | native | MISSING | — | preview generating/error and dense100-tasks/100-sessions/20-kernels fields body bottom; missing optional fields stay unavailable, not fake data |
| X07 | #36 / diagnostic | native | COVERED | P/native-diagnostics-saved, P/native-diagnostics-cancel, P/native-diagnostics-unknown | saved hash, cancelled save, original unknown result exact snapshots |
| X08 | #36 / diagnostic | native | COVERED | P/native-diagnostics-end-confirm | nested EndVerification warning; does not delete old file |
| X09 | #36 / diagnostic | native | MISSING | — | busy save/retry failure/end receipt 保留旧文件可能已发布; exact report/request retained without new chooser/write |
| X10 | #36 / diagnostic | native | PARTIAL | P/workspace-native-fault-diagnostic | route return and recover blocker/source owner while diagnostic pending require action assertions; frame source identity alone not cross-stack behavior proof |

## 使用边界

- 这是从当前前端分支衍生的有界状态/可见权限类别，不枚举全部后台组合、不增桌面计数。
- 原173状态、修复前12状态及后续补图都不是全部产品分母；每项需其实际行为与声明参考范围。
- 普通安全阶段/错误码在相同owner里报告，不机械制造新屏；不同确认、子字段、任务权限及滚底须分别核对。
- 来源/图片哈希、旧结果和失败历史保留；未达标票继续OPEN，PR保持草稿、不自动合并。
