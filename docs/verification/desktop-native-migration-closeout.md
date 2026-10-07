# 桌面收口：真实双版本迁移服务补证

日期：2026-10-07。用户取消严格 1:1 要求；本轮不扩展视觉参考、截图或图库审计。继续 `codex/issue32-37-ui-integration`，本次实际检查源码为 `be5b1768c96ffc6f448a695f6e430dc8c828402b` 及当时工作区；没有切换、重置、合并、另行提交或推送。

本次使用现有 [`TestRealMigrationTwoBuildsTrialSwitchFailureAndCompleteRollback`](../../internal/workspace/migration_real_test.go) 精确执行一项。它调用当前生产 Service、真实 `kernel.Prepare`、真实 `kernel.LaunchManagedProfile` 及生产 AppContainer/Job/独立代理桥拥有链。没有替换 Prepare、Launcher、网络隔离或运行结果。测试开真实正常浏览器窗口，但没有通过 Wails GUI 点击；它是本机真实服务、内核和数据证据，不能计作新桌面程序的迁移页面验收。

隔离根为本轮自行创建的 `output/mg-a79574/w`。只用两个已有本地归档重新安装到此根；未读取、接管或修改整合者正在运行的 Wails 工作区、其他程序或用户 profile。所有环境、Cookie、站点、上游代理和 HTTPS 观察端均为合成测试资源。

## 判定

| 范围 | 判定 | 本轮实际依据与边界 |
| --- | --- | --- |
| 两个不同可信构建的实际安装和探测 | 实际通过 | 148.0.7778.215 与 150.0.7871.186 的 ZIP 摘要先精确复核；真实 PE、文件清单与三次私有 pipe 指纹探测核验。两构建 executable SHA 不同，未用两个 ID 指向同一包。 |
| 148→150 独立工作副本试用 | 实际通过 | 服务真实完整备份、解包、试运行、停止，再明确提交。持久 journal 的原目录与工作副本 NTFS identity 不同；两个构建在副本实际回读精确版本及同一 seed。试用阶段原已保存档案未变，普通克隆的新身份规则没有替代迁移身份。 |
| 迁移前 Cookie、LocalStorage、IndexedDB 在新构建中保留 | 实际通过 | 正常 148 浏览器写 `SYNTHETIC_BEFORE`；明确迁移后，正常 150 浏览器在禁止新写入的阶段读取同一站点的三项旧值。不是迁移后重新写一份 canary 来代替保留验证。 |
| 切换后完整撤回并重开 | 实际通过 | 150 写 `SYNTHETIC_AFTER`；通过原迁移的 `SelectRollback`、`PreviewRestore`、`ApplyRestore` 完整撤回，关闭服务并重开；原 ID、完整已保存档案、seed、精确 148 内核和数据引用恢复，再由真实正常浏览器回读三项 `SYNTHETIC_BEFORE`。未选环境档案完全不变。 |
| 切换中失败保护与回退 | 实际通过（受控故障注入） | 在实际 `new-installed` 目录切换阶段注入一次明确中断。任务准确 `failed`、`committed=false`、无 persistence pending，原 148 和原三存储实际可再打开读回。这个故障为自动化注入，不冒充用户 GUI 自然失败。 |
| 原资源终结与清理确认 | 实际通过 | 原试用正常退出；检查结束后，本测试 9 个网络 session 全部 `closed`、54 个资源全部 `released`，staging 无条目，按本测试路径筛选的 chrome/代理桥无残存。两个 journal 均 `finished`，原目标可重开。 |
| 失败试用副本及完整备份自动删除 | 不适用且有依据 | 当前恢复契约明确返回“原完整数据与原构建已核对保留；试用副本及已有备份保留”。本次失败 incoming 副本确实仍在，作为已停止的数据副本保留，未伪报文件已清空。进程、网络授权与维护保护解除另按上行确认。 |
| 新 exe 的迁移页面选择、试用、停止、提交与撤回点击 | 仍阻塞（preview7/8 实际失败） | 本补证没有操作 Wails GUI。整合者随后在新 preview7/8 实际操作迁移：旧 148 观测返回，新 150 真实进程已创建但 After 未返回，任务安全结束为原状态保留。preview8 安全原因已保存为 CONTROL_CHANNEL_LOST/control-read-ended；两次迁移各自 Windows Event1000 确认 chrome.dll150/80000003，但具体崩溃断言/根因未知。不得以本报告服务 PASS 顶替 GUI 失败。正式桌面计数及操作见[总验收](desktop-closeout.md)。 |
| 迁移来源 token 丢失时的同会话恢复 | 仍阻塞（已知契约限制） | 本补证只使用服务真实返回的原 token，不模拟或猜 token。缺来源的安全阻断与退出清理检查见[独立回收/清理报告](desktop-recycle-migration-closeout.md)。 |

“不适用”没有算成通过；失败注入、服务实跑和新桌面程序的 GUI 操作分别记录。

## 程序和证据来源

| 构建 | ZIP SHA-256 | 实际 executable SHA-256 |
| --- | --- | --- |
| 148.0.7778.215 | `9ef3f471b7a6641b4224532522b29141ce3746e27d55788d88e2fd951f362579` | `1867319e56bcabbc4681d8575c002106ce7b61b5290dc5eb34a37676805f6915` |
| 150.0.7871.186 | `4d549c326e51ebbabf562fd365eb5380d9d4a81200da2c60f075c688d9a77e03` | `65d0807599b5430ffbd9dffb60cd020464815f10557352959e3aa4c125a5c439` |

本地必要证据保留在 `output/mg-a79574/`：`run.json` 记录程序/检查来源与夹具调整，`test.log` 记录 51.24 秒的实际 PASS，`migration-evidence.json` 记录真实精确构建、指纹观测、切换失败、迁移和完整撤回，`state-summary.json` 记录原 dataRef、独立目录 identity 与 journal 状态，`network-closeout.json`/`resource-closeout.json` 记录资源及目录复核。含本机路径的详细原始证据留在 ignored output，不扩展公开截图。

网络使用生产保护链及受控本地上游，仅允许合成站点和 HTTPS 观察端，不允许任意公共转发。观察端返回的 TEST-NET IP 只是本次 HTTPS 读回的合成内容，不是公网出口证明。本次不扩展外部代理泄漏测试。

## 失败记录与最小夹具调整

1. 第一次直接执行旧夹具，将 TEMP 指向较长 `output/desktop-closeout/migration-native-6e51f647-dca1-44d9-8fa0-3e7b545cda60/temp`。148 安装的真实诊断进程无法启动，4.20 秒失败，尚未进入迁移。该执行未算通过；日志保留在该目录 `test.log`。
2. 第二次仅缩短 TEMP 为 `output/mg-ae403a/t`。真实 148 诊断进程启动，实际 executable 路径 246 字符、locale 文件 253 字符；90 秒诊断超时，94.17 秒失败，旧引用未改变，隔离临时目录清理后为空。日志与本测试进程观察仍保留在 `output/mg-ae403a/`。
3. 第三次为当前 PASS：Go `-overlay` 只替换测试 [`fixture`](../../internal/workspace/service_test.go) 的 `root := t.TempDir()`，有专用变量时使用短隔离根 `output/mg-a79574/w`。原测试名附加的长目录不再进入真实 Chromium 安装/数据路径。仓库测试文件和全部生产源码未被修改；overlay 原文与原始/替换文件摘要保留于 `run.json`。实际已安装 executable 路径 153 字符。精确测试通过后才作资源复核。

这些结果支持“长测试夹具路径与失败强相关”，没有确认每个底层 Win32/Chromium 失败原因，也没有据此宣称任意长工作区路径已支持。必要使用限制为：真实内核检查与工作区应选短、隔离、未被开发监听覆盖的路径；后续长路径兼容不能用本次短路径 PASS 代替。

这两次夹具路径失败不能解释后续 preview7 的 GUI 迁移失败；后者的 executable/profile 路径条件不同。没有确认根因时，不能继续以长路径推测代替可靠的内核错误证据。

迁移保留 seed 和身份引用，不承诺不同内核的全部输出相同。本次实际旧/新观测语言、时区和 CPU 相同，而 GPU 观测随内核从 Intel 项变为 NVIDIA 项；报告保留原值，没有捏造跨内核 GPU 一致。完整撤回核对原已保存档案与原三项站点数据恢复。

## 后续必要修复：安全保留失败分类

真实 preview7 失败暴露 [`finishMigrationRecovery`](../../internal/workspace/migration_recovery.go) 丢弃底层 cause 的问题。现只在已有“未提交、原完整数据与原构建保留”终态反馈中增加 `error.details.cause` 的 `code`、`reason`、`retryable`：使用既有 `runtimeError(cause)` 分类，只复制安全标识，完全不复制 Message、`Error()`、文件路径或 JavaScript 异常正文。普通未知错误为 `MIGRATION_CAUSE_UNKNOWN/unclassified`；没有 cause 或属于启动补偿时不生成过去失败原因。分类标识含非预期字符也退回安全未知分类。原未提交状态、正常恢复后的维护释放条件和活资源未确认的 protected 阻断不变。

新 [`migration_cause_test.go`](../../internal/workspace/migration_cause_test.go) 两项必要定向检查实际通过：从持久 journal 核对 `kernel.Problem` 的 Code/Reason 保留且原档案不变；检查原始错误、私有路径和脚本片段没有进入结果，未知/nil/启动补偿反馈准确，以及仍活跃的合成资源继续 protected、未提交并保持原拥有者。这些使用合成服务和生命周期状态，只证明诊断与保护契约，不能当成真实 preview7 失败已修复。

实际命令：`go test ./internal/workspace -run '^TestMigrationRecoveryCause' -count=1 -v`，2 项 / 5 个子场景通过，0.467 秒。当前修改尚未提交、未构建新 exe，也未停止或接管真实运行中的其他实例；整合者须在包含此改动的新程序上重现并取得实际原因后更新桌面验收。之前 51.24 秒真实服务检查和 preview7 实际失败都保留原日期、程序来源与结果。
