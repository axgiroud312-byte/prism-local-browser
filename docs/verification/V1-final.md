# 首版候选集中验收报告

本机验收日期：2026-10-05；远程接续及缺口补验：2026-10-06。分支`goal/v1-remaining-integration`。覆盖T05–T21 / Issues #6–#22；**正式验收仍4/21，本报告不自动关闭任何票。** 本机代码、实际运行、候选产物和正式交付分别记录。

## 结论与当前产物

**10月6日安全补验全部通过：**[12项统一实跑与准确边界](V1-local-acceptance.md)、[脱敏源码/观测回执](V1-local-acceptance.json)。实际普通停止超时后的ForceStop、Cookie边界、257项真实目录、12项真实队列、两运行环境完整备份恢复和实际ACL/SQLite容量恢复通过。实际目录拒绝暴露生产typed-nil清理崩溃，wrapper已修且无seam回归/续跑通过，保护未降低。

**当前主候选为`0.3.0-preview.6`**，目录`output/delivery/0.3.0-preview.6-v1-candidate/`。14,763,567字节，SHA-256：

```text
9f8c60df8bb6e6369c14a13d4b5e0c5a037404b98bf2b3445a9d765a9355b7a9
```

干净源码`67c98dbe2a106976eb42e9d37a14cbde2b10f015`、tree=`7e9432df114c089aee54c0827f560102f63e730d`，Wails官方trimpath，未签名/无内核、9个payload文件、26 Go模块/runtime和108前端包许可。`.5`只作同源码升级夹具，不复用旧版本或覆盖旧包。源码`prism-source-67c98db.zip`，SHA=`b182dbf6e5c6930e3a34ec68ac543b7f4b4d9c867e9b85c1807de1a842c437b5`；源码ZIP文档保留构建当时状态，交付`docs/`为后续记录。

`npm run build:installer -- -PreviewRevision 5/6 -Candidate`分别通过；`npm run verify:installer -- --no-clicks --candidate --first-revision=5 --second-revision=6`实际安装/6次只读native加载与exit0/升级保身份/拒降级25/保留卸载/重装/只删自有合成根通过，五个产品位置均不存在。主程序与维护helper的ASCII/UTF16 Windows编译用户路径扫描未发现匹配。[当前脱敏回执](V1-candidate-acceptance.json)，原证据在`output/goal/V1-local-acceptance/`。这是本机空产品根，不是新Windows用户/完整人工流程。

候选来源67c98db的[远程CI 37406703053](https://github.com/axgiroud312-byte/prism-local-browser/actions/runs/37406703053)三个job已通过，点击分支跳过；该次尚无产品安装步骤。[本轮远程回执](V1-local-remote.json)含10个补验评论，仍不闭票。默认CI新增一次性Windows runner无点击安装，首轮[37407497714](https://github.com/axgiroud312-byte/prism-local-browser/actions/runs/37407497714)在安装前被源码dirty门禁拒绝；两Node/Go/build通过，后续内核探测跳过，不把首轮整体改记通过。

全新Windows检出已复现只有go.mod由CRLF变LF、规范化diff为空、锁文件内容相同；Wails正常go mod tidy改写换行导致状态不干净。用户批准只修CI/构建管理，`.gitattributes`仅固定go.mod/go.sum为LF，没有取消tidy、忽略脏源码、重写manifest或放宽门禁。修正后又全新检出，两次production预览构建/manifest sourceDirty=false且工作树完全干净；在自有检出实际修改go.mod内容，原安装脚本仍exit1在安装前拒绝，恢复本测试改动后再次干净。[源码门禁实测回执](V1-ci-source-guard.json)。旧go.sum物理CRLF与新LF的SHA不同但规范化依赖逐字相同，分别记录不改写旧哈希。远程安装复验另记。产品代码和本机`.6`来源不变，runner development-preview.1/.2不冒充candidate.6或同一哈希。**下列`.4`仅为修复前历史产物**。

**远程无点击安装复验已通过：**[37409611901](https://github.com/axgiroud312-byte/prism-local-browser/actions/runs/37409611901)，eec3333三个job SUCCESS。一次性Windows Server 2025已有WebView2，实际default known folders/空产品根、development-preview.1/.2安装→6次只读native加载及正常exit0→升级保身份→拒降级25→保留卸载/重装→仅删自有合成根；5次要求合成记录的读取均满足。[下载核对的安装回执](V1-ci-installation.json)、[版本/哈希/步骤及范围](V1-local-remote.json)。实际checkout是PR merge `fa5ade1`，与PR head eec3333分别记录；两manifest sourceDirty=false、安装包实际摘要匹配。两Node各131后台/类型/build/51文档、Go全包/vet、真实148诊断探测23.67秒和档案保存/重生成/回滚41.12秒通过，四个点击分支均SKIP。初轮FAIL仍保留。

这是一次性Server测试机上的开发预览安装，不是精确交付`.6`在Windows Home新用户/VM、缺WebView2或人工选择器/卸载默认框验收；`freshWindowsUser=false`不改写为true。没有因已有兼容WebView2就声称缺依赖流程通过，未加入产品网络或完整网站验收声明。

### 修复前候选的历史来源与证据

最终分享前发现并修复构建隐私缺口：首轮46649c0主程序带编译机路径，Wails改用官方`-trimpath`，不改业务逻辑。**最终4b38dc8包已重新构建且无点击复验通过**；主程序buildinfo确认trimpath=true，主程序/维护helper路径扫描未发现Windows编译用户路径。旧包仅在本机忽略的历史证据目录，不对外分发。

后台回归、本机真实固定档案、正式代理保护链、三种存储、备份恢复及硬中断均有下列证据。GitHub登录/推送/草稿PR/17票同步及修订远程CI已完成；首轮FAIL保留，6cf1768三job通过，见[远程回执](V1-remote-sync.json)和[PR #27](https://github.com/axgiroud312-byte/prism-local-browser/pull/27)。独立远端、特殊真实场景、人工完整页面和干净Windows仍缺，**未达到正式交付条件**。候选仅用于合成数据检查，不用于真实账号/凭据。

**候选已构建并完成本机无点击安装闭环。** 主交付目录`output/delivery/0.3.0-preview.4-v1-candidate/`，主包`prism-browser-0.3.0-preview.4-windows-amd64-setup.exe`，14,763,334字节；SHA-256：

```text
0f1e29c50b64838c0ff0926127b6bf50b2375a2f955d208642f0aa72693fe324
```

版本`0.3.0-preview.4`，channel=`v1-candidate`，`NotSigned`，`sourceDirty=false`。构建来源`4b38dc8a94107325cbbca5050703a6bd80ca9e52`，tree=`f3c2f4798b6ad717d329519036d3199511dbc652`。Node24.14.0/npm11.9.0/Go1.27.1/Wails2.16.0/NSIS3.13，锁文件摘要及9个payload文件摘要在对应release清单；许可包含26个实际Go模块/Go runtime、108个前端包（含Vite helper）及NSIS。

同提交源码`prism-source-4b38dc8.zip`，SHA=`edc335a8026171a9f9dc4cb59f55ae97944b30c4a21ad2530df9e08b7781efb7`。源码快照保留构建时的文档状态；本报告和交付目录`docs/`是后续准确验收记录，不改变二进制来源。

NSIS bootstrap为i386是既有体系行为，安装限定x64，主程序和维护helper均实际amd64；installer PE版本0.3.0.4，主程序PE固定资源仍0.3.0.0，窗口/运行应用版本明确0.3.0-preview.4。没有把这些不同版本字段混为一谈，不捆绑Chromium。`.3`只作当时升级夹具；[历史`.4`回执](V1-candidate.4-acceptance.json)与下面10月5日实跑对应，不是当前`.6`证明。

## 集中检查与失败处理

| 命令 / 实际范围 | 实际结果 | 原始证据（仓库相对路径） |
| --- | --- | --- |
| `npm run check:background` | 129/129测试、类型、网页build、49文档通过；**无test:ui** | `output/goal/V1-final/frontend-background.log` |
| Cookie入口/对象引用修订的定向Node测试 | 6/6通过；正常running+resourcesPending可以写入，代理启动不需直连确认，未知资源禁止新启动 | `frontend-related-retest.log` |
| `npm run typecheck`（修订后） | 通过；最初新夹具TS2352已修为完整RuntimeSession，未弱化类型 | `typecheck-final.log` |
| `npm run test:desktop`，Go `-p=1 -v` | 六个internal包通过，336个顶层测试PASS、18个显式opt-in/helper SKIP；根包因并行Vite清空dist编译失败，整轮**FAIL**不改记成功 | `go-background.log` |
| `go test -p 1 -count=1 -v . ./internal/desktopbase` | 串行补编根包和安装底座通过，不重做其他已通过包 | `go-build-install-retest.log` |
| `go vet -p 1 ./...` | shell完成回执exit0；空日志本身不是通过证据 | `go-vet.log`及本会话工具回执 |
| 三项新增服务缺口及shutdown回归 | 实际空目录clone/31项分页去重、异kernel ID同精确build候选、nil取消正常退出通过；DPAPI首轮误用不存在的测试RPC失败，修成现有DiscardRestore后定向通过 | `service-gap-tests.log`、`dpapi-preview-retest.log` |
| PowerShell解析、Node语法、前端许可汇集 | 通过；108包包含107生产包和Vite注入helper许可；未新增依赖 | 本会话exit0回执，实际随包许可文件 |
| 候选首次构建与NSIS修订 | Wails production、26 Go模块/108前端通知通过；NSIS单元素条件数组被PowerShell5展开为String后原传参返回usage，整轮FAIL。改显式string[]后实际PS5/NSIS SAFEPPO exit0；候选重建另记 | `build-candidate-3-failed.log`、`nsis-powershell5-retest.log` |
| `npm run build:installer -- -PreviewRevision 3/4 -Candidate`（分别执行） | 最终两版同一干净4b38dc8源码构建成功，Wails production/trimpath/NSIS/许可/真实哈希/签名齐全，无内核 | `build-candidate-3.log`、`build-candidate-4.log`及对应release清单 |
| `npm run verify:installer -- --no-clicks --candidate --first-revision=3 --second-revision=4` | 历史`.4`实际安装/升级/拒降级/保留卸载/重装/仅删自有合成根通过；6进程exit0 | `installer-no-clicks.log`、`install-no-clicks/installer-verification.json`、[历史回执](V1-candidate.4-acceptance.json) |
| 安装后独立只读收尾核对 | 默认数据/程序/注册与两快捷方式均不存在，六自有工作台PID均不存在；没有删除真实数据 | `install-postconditions.json` |
| 二进制路径隐私与buildinfo | 主程序`-trimpath=true`，两payload exe未发现Windows编译用户路径；首轮含路径包撤出交付，只留本机历史 | `binary-path-privacy.json`、`binary-go-buildinfo.log` |
| 最新文档与格式 | 50份文档、本地链接/12需求/6路由/4嵌入文档通过；暂存diff格式通过 | `docs-final.log`和本会话回执 |
| 2026-10-06 GitHub默认无点击CI（6cf1768） | 三job通过；Node22.12.0/24各131/131后台、Go全部包/vet、preview.1/.2构建、真实148探测14.82秒及保存档案22.31秒通过。没有产品安装/自动点击，不算完整check或干净Windows产品验收 | [实际运行](https://github.com/axgiroud312-byte/prism-local-browser/actions/runs/37401271261)、[回执](V1-remote-sync.json)、`github-ci-acl-retest.log` |

除第一行之外表中日志均在`output/goal/V1-final/`。没有运行`npm run check`或Playwright/UI自动点击，**不声称完整check通过**。人工替代项见下文。网页build保留原有大chunk和Lucide `use client`警告，候选production实际结果已另列。首轮46649c0包与安装记录留在本机`pre-trimpath-artifacts/`，本表最终构建/安装日志和回执都对应4b38dc8，不混用哈希。

接续时的四个未提交修复已调查保留并提交：shutdown仅防御不存在cancel；网络/Cookie旧夹具改为真正provider缺失或受控host seam；adapter只读快照用值相等而非旧对象引用。不删除测试、不放宽生产schema或网络保护。

## 真实与硬中断证据的准确覆盖

### 已有实跑，不机械重跑

- [正式内核与应用启动](T11-production.md)：零网络能力AppContainer、差量ACL、同身份专属桥、同通道前检、准确Job全树，148两轮正常退出、Cookie/LocalStorage/IndexedDB保留及journal清理。
- [正式故障恢复](T11-recovery.md)：A listener丢失/普通服务占原端口，普通caller对照到达而浏览器不到达；停止A不影响B；上游故障闭锁准确Job；管理器prepared/running两个实际Kill切点重开清理。
- [双代理Cookie/FIFO](T11-production-application-observations.json)：实际指定会话导入/读回、空值/session/HttpOnly/persistent/Secure及A/B独立、原请求去重。不是分区/清空/到期/取消全部真实矩阵。
- [148→150代理迁移](T20-protected-observations.json)：固定seed和未选环境不变；副本共用保护，切换失败回滚→再试成功→新构建读旧数据→写新值→升级前完整备份恢复→服务重开。`private-pipe-canary`不冒充实际HTTP头/外部出口。

`output/goal/T11/production-start/full-runtime-evidence.json`对应`TestRealIndependentBrowserSessions`（历史86.36秒时长来自交接记录）：真实148、A/B独立身份/目录/进程、三存储隔离和停止重开；杀A根后B不受影响、A重试保数据、旧session请求被拒；A回收→服务重开→找回；选定A完整备份→改三存储/备注→恢复→重开、B不变；恢复prepared/old-retained/new-switched/before-db-commit/db-committed五个实际host Kill切点，包暂不可用仍收敛完整侧并真实读回。

**该组合始终显式direct。** `forceStopActuallyUsed=false`、`applicationCrashRestart=not-run`，不覆盖正式代理/批次/Cookie导入API/诊断/永久删除/回收十切点/迁移四切点/UI/安装；不能只凭五个环境变量扩大覆盖。

### 本轮只补真实缺口

| 专项命令（沿用项目Go环境、`-p 1 -count=1 -v`） | 实际结果 | 证据 |
| --- | --- | --- |
| `PRISM_RESTORE_CRASH_VERIFY=1`，`-run '^TestRestoreHardInterrupt(OccupiedRollbackStaysProtectedUntilExplicitRetry\|AgainDuringRollback)$'` | 2父用例通过：占用仍保护、显式重试收敛；回滚new-retained/old-restored再中断2切点通过 | `restore-hard-interruptions.log` |
| `PRISM_RECYCLE_CRASH_VERIFY=1`，`-run '^TestRecycleHardInterruptionPreservesConfirmedItemDecision$'` | remove/restore各3、purge4，共10切点通过 | `recycle-hard-interruptions.log` |
| `PRISM_MIGRATION_CRASH_VERIFY=1`，`-run '^TestMigrationHardInterruptFindsMatchingCompleteConfigurationAndDirectory$'` | prepared/old-retained/new-installed/configuration-committed四切点通过 | `migration-hard-interruptions.log`、`migration-hard-evidence.json` |
| 指定已有148归档，`-run '^TestRealSavedFingerprintRevisions$'` | 15.12秒通过：保存revision1→重生成2→回滚3，实际版本/CPU8/de-DE/Europe-Berlin读回，正常退出 | `saved-profile-real.log`、`saved-profile-evidence.json` |

上述硬中断确实Kill自己创建的helper，不是panic/error注入；回收/迁移属于合成服务/目录状态验证，**迁移四切点不是又一次真实浏览器升级**。T05实际读回使用诊断临时profile，不冒称正常环境启动/网站登录。测试只用合成根/归档，不操作真实profile或Windows服务。

## T05–T21逐票矩阵

共同命令：G=`npm run test:desktop`（internal结果）＋上述根包补编/vet；F=`npm run check:background`＋受影响路径定向修订。共同待验：对应人工native流程、安装版业务闭环/干净Windows、远程规则。旧票中的“全部未执行/迁移代理未接入”是早期开发记录，不覆盖本报告的实跑事实。

| 需求 / Issue | 实现位置（`internal/workspace/`，除注明） | 命令/实际结果与证据 | 未覆盖条件 / 关闭条件 | 可关闭 |
| --- | --- | --- | --- | --- |
| FP-001/002 T05 #6 | `fingerprints.go`、kernel/fingerprint | G/F；本轮RealSavedFingerprintRevisions正常读回 | 新抽屉人工生成/取消/重生成/历史回滚 | 否 |
| ENV-003 DATA-001 T06 #7 | `runtime.go`、kernel/runtime | G/F；full-runtime真实A/B三存储/停重开 | native单个/批量启动停止及安装版闭环 | 否 |
| ENV-003 T07 #8 | `runtime_supervisor.go`、`runtime_persistence.go` | G；full-runtime根故障；10月6日实际普通停止超时→原Job ForceStop/保数据重开，B不变 | 应用完整会话恢复、人工故障提示及共同条件 | 否 |
| PRX-001 T08 #9 | `proxies.go`、`proxy_storage.go` | G/F；真实Windows DPAPI，读回/幂等/失败保旧凭据 | 实际外部阶段/出口及人工导入replace/clear/引用保护 | 否 |
| PRX-001 T09 #10 | `runtime_network.go`、proxy/bridge | G；正式protected启动/重开、HTTP/CONNECT/TLS库测试 | 两真实认证代理与换代理保数据的外部闭环 | 否 |
| PRX-001 T10 #11 | proxy/socks5、`socks5_test.go` | G：RFC认证/域名与IPv4/6字节、错误分类、只拨上游 | 真实SOCKS5浏览器、独立DNS/IPv6/UDP观察 | 否 |
| PRX-001 ENV-003 T11 #12 | kernel/network_store、`runtime_network_resources.go` | G；正式资源/故障/端口接管/管理器Kill/A-B独立 | 下文独立远端全路径；跨登录/重启及人工资源重试 | 否 |
| CK-001 DATA-001 T12 #13 | `cookie_worker.go`、NativeCookieImport | G/F；双protected导入/读回/去重；10月6日三个分区键、过期拒绝、冲突、明确清空及首条真实写后取消通过 | 人工流程；任意断管线未知写窗口未由调度屏障代替 | 否 |
| ENV-001/002 T13 #14 | `batch_worker.go`、`environment_query.go` | G/F；clone/31项；10月6日257项真实目录、129项ACL失败保身份续跑/分页/去重，typed-nil清理修复 | 百万实体/真正OS空间资源不足及人工映射/跨页选择 | 否 |
| ENV-003 T14 #15 | `runtime_queue.go` | G/F；10月6日12项真实FIFO、取消1/目录ACL失败1，10运行→仅失败项重试为11运行，正常停止 | OS内存/进程资源耗尽、人工取消/单失败重试及独立远端 | 否 |
| BKP-001 T15 #16 | `backup_worker.go`、`backup_snapshot.go` | G/F；all11/原DPAPI密文；10月6日两运行环境由备份正常停/完整包、源读与输出写实际ACL恢复通过 | 真正NTFS空间不足、人工选择器及共同条件 | 否 |
| BKP-001 CORE-001 T16 #17 | `restore_preview.go`、`restore_configuration.go` | G/F；精确build/DPAPI；10月6日实际备份读拒绝及DB/WAL/SHM/浏览字节不变通过 | 实际选择器/同build映射闭环；跨SID仍仅模拟故障 | 否 |
| BKP-001 DATA-001 T17 #18 | `restore_worker.go`、`restore_commit.go` | G/F；10月6日两真实环境三存储/配置完整恢复重开、C不变；ACL及实际SQLite容量失败回滚通过 | 真正NTFS空间不足、人工明确覆盖及共同条件 | 否 |
| DATA-001 T18 #19 | `restore_recovery.go`、`restore_consistency.go` | G；五切点/回滚再中断；10月6日实际ACL阻断后重开保保护、删原包修权限同任务恢复精确旧侧；SQLITE_FULL=13 | 真正NTFS空间不足、安装版中断提示与人工重试 | 否 |
| DATA-001 ENV-001 T19 #20 | `recycle_worker.go`、`recycle_recovery.go` | G/F；三存储/10 Kill切点；10月6日DELETE双路径拒绝/重开/修复原任务，只删确认项，B/备份不变 | 安装页永久删除/失败恢复及其他OS资源故障；不删真实数据 | 否 |
| CORE-001 FP-001 T20 #21 | `migration_trial.go`、`migration_commit.go` | G/F；真实148→150 protected切换/完整回退；本轮4 Kill切点 | 人工默认/副本/切换/回退、独立外部与目标网站兼容 | 否 |
| UX-001 DOC-001 T21 #22 | `diagnostics_host.go`、desktopbase、NSIS | G/F：脱敏/只读/实际JSON；新干净67c98db候选`.5/.6`构建及本机无点击安装/升级/保留卸载/重装通过；runner无点击安装单列 | 系统保存器人工取消/核实、精确交付包干净Windows/缺WebView2、全产品链及blocking | 否 |

## 独立网络出口矩阵与最小资源

没有发现专用PRISM远端配置或`.env`；开发`HTTP_PROXY`等变量不代表获授权的验收代理。已一次性向用户说明所需资源，凭据应在本机受保护配置/密钥库中提供，**不贴聊天、不入仓库**。

| 路径 | 当前可确认 | 仍需独立远端证据 |
| --- | --- | --- |
| HTTP / HTTPS | Bridge固定上游/CONNECT、TLS库测试；protected本机HTTPS访问 | 实际HTTP/HTTPS认证成功/失败、目标日志只见代理出口 |
| WSS / HTTP Upgrade | CONNECT是加密WS候选通道；明文Upgrade不支持并拒绝 | WSS握手/消息双向、断桥/上游后无旁路，不能只凭CONNECT推导通过 |
| DNS | SOCKS5目标域名远端编码；代理host解析与目标解析分开 | 唯一随机域名、权威DNS与代理日志，正常caller对照；本机受控DNS不替代 |
| IPv6 | 字节/解析库测试和历史受控socket实验 | 双栈远端HTTP/6、直接IPv6企图被阻断；有可达普通对照 |
| WebRTC | 参数/容器策略存在，不是实际媒体证据 | 自有STUN/TURN/候选记录，正常对照到达、浏览器不直接UDP出网 |
| UDP / QUIC | SOCKS5 UDP ASSOCIATE/BIND拒绝；无自动直连fallback | 自有UDP/HTTP3观察端与接收日志，普通对照到达；浏览器支持或阻断逐项实证 |
| 上游/桥/管理器故障、旧端口接管 | 真实本机故障与资源恢复通过 | 同远端观察对照，不将本机空计数当独立无出口证明 |

最小资源：可控断线/认证的HTTP、HTTPS、SOCKS5测试代理；独立HTTPS/WSS远端观察器；权威DNS及IPv4/IPv6/UDP/STUN日志端（可同一自有主机）。只告诉代理本机安全配置入口和测试主机授权范围。公共IP回显网站无法替代故障/UDP/DNS观察端。

首版支持Windows隔离服务正常；BFE/MpsSvc/Dnscache自身损坏列后续加固。**没有重做被拒绝的停服实验**，也没有把拒绝实验改为成功。

## 最短人工补验（只用测试用户与合成数据）

1. 新Windows测试用户/干净VM：核对候选哈希，手工安装/启动；记录窗口版本，卸载选择页默认不勾删数据；升级后保原ID/seed，再保留卸载/重装核对。缺WebView2的机器另验证阻断提示，不改日常机器。
2. 安装已有精确148归档；代理安全配置好后创建A/B，生成预览/取消一次再保存；A普通编辑、重生成/回滚各一次核对seed和数据引用。排队启动两项，取消一等待项、仅重试失败项，正常关闭/重开。
3. A Cookie导入合成空值/session/Secure条目，预览→空白protected启动→明确写入→逐项读回；补明确清空、取消/到期/分区边界，B不变。代理导入编辑replace/clear/旧修订/引用保护及跨页批次映射集中一次检查。
4. A/B正常停后完整备份，改合成三存储，预检→明确恢复→重开；A回收/找回再核对，复制环境必须新seed/空目录，只对自有复制项永久删除。检查中断/占用重试和未知结果不会另建任务。
5. 选定停止A做148→150工作副本试用、正常停止、切换及原备份退回；不宣称所有目标网站兼容。活动页诊断先取消保存再新文件保存/核实，确认报告不含身份/路径/凭据/Cookie。

当前候选无点击**复验必须使用构建来源67c98db的干净Git checkout**及`.5/.6`参数，不能用后续报告提交或无Git源码ZIP冒充同源码。该历史source不含新LF属性；在新隔离clone使用`git clone -c core.autocrlf=false <仓库URL> <新目录>`后选择67c98db，避免Windows换行转换造成非内容dirty；不修改当前仓库配置、不把后续`.gitattributes`写入旧source后冒称干净。只做来源检出，不回退/重置当前工作区。历史`.3/.4`绑定4b38dc8。脚本需要局部`.tools/go/bin/go.exe`与Node开发工具，不是产品安装依赖。普通用户人工清单只需WebView2和精确内核。CI Go junction只指向setup-go固定工具，不改共享缓存；默认无点击，旧点击分支仍显式opt-in。

独立出口用上表资源单独验收；人工界面顺利不替代网络/进程/目录证据。禁止自动点击仍生效；不要求逐步询问“是否继续”。

## 远程收尾与未关闭原因

10月5日认证失效，10月6日用户恢复后`gh auth status --hostname github.com`已核对正确身份。38个本地提交普通push成功，创建并关联[草稿PR #27](https://github.com/axgiroud312-byte/prism-local-browser/pull/27)，不强制覆盖、不合并或关闭任务。#6–#22共17条实际结果/缺口评论已发布，再读确认全部OPEN，原blocking保留；URL见[同步回执](V1-remote-sync.json)。

[首轮远程CI](https://github.com/axgiroud312-byte/prism-local-browser/actions/runs/37400229514)对应b8adc31：两Node后台检查通过，desktop在`TestNetworkStoreRecoversPartialTreeGrantIncludingNewFiles`失败。测试只请求DACL却比较完整SDDL，日志ACE条目相同，owner/group及AI呈现不同；当时未直接视为误报或删安全测试，先请用户确认按实际owner/group、ACE、继承保护核实的方案。安装构建/真实内核步骤未执行，artifact上传无文件是前序未产包的后果；首轮整轮FAIL保留。

**修复已获用户明确确认，并完成本机定向及Windows CI复验。** 仅测试修改：GetNamedSecurityInfo显式请求owner/group/DACL，比较实际所有者/组SID、全部ACE原字节和顺序、每条继承标记、DACL继承保护；不比较AUTO_INHERITED完成元数据或未请求的描述字段。新增12子例区分允许/拒绝、权限扩大、SID/顺序/继承与保护改变，并拒绝部分或NULL DACL，不能通过删除安全回归绕过。原实际目录恢复/新文件授权撤销和根替换占用回归继续执行。`go test -p 1 -count=1 -v ./internal/kernel -run '^Test(NetworkStore|NetworkACLPermissionSnapshot)'`4个顶层/12子例、kernel vet本机通过，只读复核无可信P1/P2。生产ACL/provider未改；6cf1768在Windows CI的原真实目录回归和全包测试通过，owner/group、完整ACE和保护一致，不仅凭本机判断。

[修订远程CI 37401271261](https://github.com/axgiroud312-byte/prism-local-browser/actions/runs/37401271261)对应6cf1768：Node22.12.0/24各131/131、类型/build/50文档通过；Go所有包/vet、Windows preview.1/.2构建、真实148探测14.82秒与档案保存/重生成/回滚22.31秒通过，三个job均SUCCESS。上传的远程产物是本workflow的development-preview.1/.2，不是本机来源4b38dc8的最终candidate.4；不混淆版本或哈希。同步记录的后续文档提交不改业务代码，新head实际检查以PR当前checks为准。

上述历史CI未执行产品安装。10月6日新增的无点击安装已在eec3333复验真实通过，准确Server/开发预览范围见开头和[回执](V1-local-remote.json)；不能把旧后台/构建推导为安装通过，不能把runner开发预览当精确交付`.6`在Home新用户/VM上的人工验收。

本轮提交（另含最终验收记录提交）：`493da0b`修复原检查/退出与Cookie UI；`86d7b4e`候选/许可/无点击验证与逐票文档；`46649c0`修复PS5/NSIS参数；`5791be1`首轮验收记录；`4b38dc8`Wails trimpath隐私修订（最终包来源）。前轮`bc56c90`/`7b1f8bb`/`07e4049`正式保护、恢复与共用队列成果均保留。最终记录提交只更新非嵌入报告/进度，不改变交付二进制来源，无需重新构建。

公开仅脱敏合成观测；交付目录含SHA256SUMS、release、安装/恢复指南、许可、同提交源码ZIP和脱敏日志索引，不附profile、备份原包或真实私有路径。无点击安装用本机原本空产品根，不是干净用户/机器；静默卸载只证明默认保留逻辑，**向导复选框默认状态未观察**。
