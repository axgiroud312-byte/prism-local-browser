# 首版候选集中验收报告

日期：2026-10-05。分支`goal/v1-remaining-integration`。覆盖正在处理的T05–T21 / Issues #6–#22；**正式验收仍4/21，本报告不自动关闭任何票。** 本机代码、实际运行、候选产物和正式交付分别记录。

## 结论与当前产物

后台回归、本机真实固定档案、正式代理保护链、三种存储、备份恢复及硬中断均有下列证据。独立远端出口、人工完整页面、干净Windows及远程CI尚缺，**未达到正式交付条件**。候选仅用于合成数据检查，不用于真实账号/凭据。

候选目标为`0.3.0-preview.4` / `v1-candidate`，沿用NSIS/同用户升级格式，不捆绑Chromium。构建/安装目前待记录；实际SHA、sourceCommit/sourceTree、版本、依赖、签名和字节数以`build/releases/v1-candidate/`的release清单为准，不能把版本字符串当作构建成功。

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

除第一行之外表中日志均在`output/goal/V1-final/`。没有运行`npm run check`或Playwright/UI自动点击，**不声称完整check通过**。人工替代项见下文。网页build保留原有大chunk和Lucide `use client`警告，均不是静默当失败修掉的测试；候选production build需另记。

未提交旧修复已调查保留：shutdown仅防御不存在cancel；网络/Cookie旧夹具改为真正provider缺失或受控host seam；adapter只读快照用值相等而非旧对象引用。不删除测试、不放宽生产schema或网络保护。

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

上述硬中断确实Kill自己创建的helper，不是panic/error注入；回收/迁移属于合成服务/目录状态验证，**迁移四切点不是又一次真实浏览器升级**。测试只用合成根/归档，不操作真实profile或Windows服务。

## T05–T21逐票矩阵

共同命令：G=`npm run test:desktop`（internal结果）＋上述根包补编/vet；F=`npm run check:background`＋受影响路径定向修订。共同待验：对应人工native流程、候选/干净Windows、远程规则。旧票中的“全部未执行/迁移代理未接入”是早期开发记录，不覆盖本报告的实跑事实。

| 需求 / Issue | 实现位置（`internal/workspace/`，除注明） | 命令/实际结果与证据 | 未覆盖条件 / 关闭条件 | 可关闭 |
| --- | --- | --- | --- | --- |
| FP-001/002 T05 #6 | `fingerprints.go`、kernel/fingerprint | G/F；本轮RealSavedFingerprintRevisions正常读回 | 新抽屉人工生成/取消/重生成/历史回滚 | 否 |
| ENV-003 DATA-001 T06 #7 | `runtime.go`、kernel/runtime | G/F；full-runtime真实A/B三存储/停重开 | native单个/批量启动停止及安装版闭环 | 否 |
| ENV-003 T07 #8 | `runtime_supervisor.go`、`runtime_persistence.go` | G；full-runtime根故障、旧session拒绝；T11管理器硬退出 | 实际普通停止失败→ForceStop及应用完整会话恢复、人工故障提示 | 否 |
| PRX-001 T08 #9 | `proxies.go`、`proxy_storage.go` | G/F；真实Windows DPAPI，读回/幂等/失败保旧凭据 | 实际外部阶段/出口及人工导入replace/clear/引用保护 | 否 |
| PRX-001 T09 #10 | `runtime_network.go`、proxy/bridge | G；正式protected启动/重开、HTTP/CONNECT/TLS库测试 | 两真实认证代理与换代理保数据的外部闭环 | 否 |
| PRX-001 T10 #11 | proxy/socks5、`socks5_test.go` | G：RFC认证/域名与IPv4/6字节、错误分类、只拨上游 | 真实SOCKS5浏览器、独立DNS/IPv6/UDP观察 | 否 |
| PRX-001 ENV-003 T11 #12 | kernel/network_store、`runtime_network_resources.go` | G；正式资源/故障/端口接管/管理器Kill/A-B独立 | 下文独立远端全路径；跨登录/重启及人工资源重试 | 否 |
| CK-001 DATA-001 T12 #13 | `cookie_worker.go`、NativeCookieImport | G/F；双protected浏览器导入/写后读回/去重；入口修订6项通过 | 分区/到期/冲突/明确清空/取消全部真实矩阵及人工流程 | 否 |
| ENV-001/002 T13 #14 | `batch_worker.go`、`environment_query.go` | G/F；本轮production目录clone空/源合成登录文件不动，31项真实目录/分页/重复请求通过 | 超大实际规模/真实资源不足及人工映射/跨页选择；百万条只虚拟预览 | 否 |
| ENV-003 T14 #15 | `runtime_queue.go` | G/F：FIFO取消/失败独立重试/迟到ready；双protected保持运行 | 实际资源不足/更大队列、人工取消/单失败重试 | 否 |
| BKP-001 T15 #16 | `backup_worker.go`、`backup_snapshot.go` | G/F：真实合成文件/SQLite/all11非页8、发布/取消/原DPAPI密文；full-runtime选定A真实三存储包 | 多运行环境正常停后完整备份/实际空间与权限失败 | 否 |
| BKP-001 CORE-001 T16 #17 | `restore_preview.go`、`restore_configuration.go` | G/F：DB/WAL/SHM不变/恶意包/预算；本轮同精确build异ID候选、DPAPI可用及拒解后重输提示通过 | 候选实际选择器/同build映射文件校验闭环；跨SID不可解密仅模拟故障 | 否 |
| BKP-001 DATA-001 T17 #18 | `restore_worker.go`、`restore_commit.go` | G/F：事务/多目标/取消/回滚；full-runtime选定A真实恢复重开 | 多真实环境完整恢复，实际磁盘/权限失败、人工明确覆盖 | 否 |
| DATA-001 T18 #19 | `restore_recovery.go`、`restore_consistency.go` | G；full-runtime五主切点真实读回＋本轮占用/回滚再中断 | 实际空间/权限条件、安装版中断提示与重试 | 否 |
| DATA-001 ENV-001 T19 #20 | `recycle_worker.go`、`recycle_recovery.go` | G/F：实际Windows移动/删除/ABA；full-runtime三存储回收找回；本轮10 Kill切点 | 安装页永久删除/失败恢复及真实资源故障；不删除真实数据 | 否 |
| CORE-001 FP-001 T20 #21 | `migration_trial.go`、`migration_commit.go` | G/F；真实148→150 protected切换/完整回退；本轮4 Kill切点 | 人工默认/副本/切换/回退、独立外部与目标网站兼容 | 否 |
| UX-001 DOC-001 T21 #22 | `diagnostics_host.go`、desktopbase、NSIS | G/F：脱敏、只读、不flush、实际JSON/失败核实；候选构建/安装待记录 | 系统保存器人工取消/核实、干净Windows、全产品链、全部blocking条件 | 否 |

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

独立出口用上表资源单独验收；人工界面顺利不替代网络/进程/目录证据。禁止自动点击仍生效；不要求逐步询问“是否继续”。

## 远程收尾与未关闭原因

`gh auth status --hostname github.com`明确token失效。用户在本机执行`gh auth login -h github.com`后再核对身份；本轮不伪称推送/PR/Issue关闭。待同步评论和PR材料应包含此矩阵、实际检查及剩余边界，符合仓库模板；blocking/所有验收未满足前保持OPEN。

本地提交、候选SHA和最终安装回执在构建完成后补入本报告；公开仅脱敏合成观测，不附profile、备份原包或真实私有路径。
