# 桌面收口：真实本机服务补充验证

本轮执行时间：2026-10-07 03:33–03:39 UTC（北京时间 11:33–11:39）。用户已取消严格 1:1 对齐要求；本报告不扩大商业参考、视觉截图或图库审计。

## 结论与证据层级

**实际通过：** 在独立的、已排除开发目录监视的 owned workspace 中，4 项已有 opt-in 检查使用真实 148 内核、真实 production Windows provider/launcher、私有通信管道、真实进程和目录。内核关闭重开、服务关闭重开、Cookie 边界、选中两个运行环境的备份恢复均通过。

**保留失败：** 首次在 `.appdata/verification` 下的真实内核准备因目录发布移动被 Windows 拒绝而失败；随后应用检查缺少已验证内核，未进入运行。两条原始失败日志均保留。改用独立 `output` 根有明确环境差异依据，不能把首次失败改成通过。

**本报告不验收新 Wails exe。** 全部是 Go 服务及内核检查，没有桌面鼠标/键盘输入，也没有独立外网出口观测。受控本机 HTTP 代理与 HTTPS observer 提供合成目标；不能据此声称真实外部代理供应商或桌面界面路径已通过。根任务另行负责新 exe 构建与实际桌面验证。

## 运行边界

- 保持当前 `codex/issue32-37-ui-integration` 分支及 `8688906`、PR #31 精确 `e170099` 基线。本补充验证没有修改生产代码、已有测试或共享验收文档，没有提交、推送、切换分支、重置或合并。
- archive：`.tools/fingerprint-chromium/148.0.7778.215/ungoogled-chromium_148.0.7778.215-1.1_windows_x64.zip`。
- archive SHA-256 合同：`9ef3f471b7a6641b4224532522b29141ce3746e27d55788d88e2fd951f362579`。本次 `kernel.Prepare` 按该合同实际校验、解压、逐文件验证与 probe。
- 真实 executable SHA-256：`1867319e56bcabbc4681d8575c002106ce7b61b5290dc5eb34a37676805f6915`；Windows build `26200`，内核 `148.0.7778.215`。
- 首次 failed root：`.appdata/verification/DesktopCloseout-Service-b92b4268-508e-4301-aeb6-8b2efd0d0f9d`。
- 成功 owned root：`output/desktop-closeout/service-b92b4268-508e-4301-aeb6-8b2efd0d0f9d/workspace`。
- 输出根：`output/desktop-closeout/service-b92b4268-508e-4301-aeb6-8b2efd0d0f9d`（ignored）。owned marker 为 `prism-owned-synthetic-v1`；未触碰用户数据或其它验证实例；没有终止任何进程。

## 实际结果

| 检查与来源 | 实际结果 | 可观察证据与范围 |
| --- | --- | --- |
| 首次 `TestRealProtectedProxyLaunchAndReopen` | **FAIL，9.38s** | `kernel.Prepare` 已进入 probing；随后 fixture 的 `os.Rename(prepared.Directory, destination)` 返回 Access denied。没有发布 verified-kernel record，也没有进入普通 protected session。 |
| 首次 `TestRealProtectedRuntimeApplicationReopen` | **FAIL，0.00s：前置条件未满足** | `run the real kernel fixture first`。没有进入 Runtime.Start，不能另算一次运行失败或不适用。 |
| 新 output owned root：`TestRealProtectedProxyLaunchAndReopen` | **实际通过，17.78s** | 完整真实 prepare/probe、2 次启动/正常退出 0；同一 seed `1256789`、实际 CPU 8；所有观测 Job 成员使用同一 AppContainer 且零 capabilities；受控 upstream 观察到 4 次页面请求；第 2 轮写入前读回上一轮 cookie/localStorage/IndexedDB。每轮 pending resources = 0、container absent。 |
| `TestRealProtectedRuntimeApplicationReopen` | **实际通过，9.35s** | 两轮真实应用 Runtime.Start/Stop、Service.Close/Open 后保存的 profile/seed、kernel ID、data reference 保持一致。第二轮实际页面读回此前三类存储。两个受保护 Cookie 空白启动 suppress 已保存 URLs；真实 empty/session HTTPOnly 与 persistent Secure Cookie 写读回，import request dedup，另一环境 Cookie 数为 0。该测试不包含普通环境编辑，不能由这项证据扩大为编辑路径通过。 |
| `TestRealLocalAcceptanceTwoRunningEnvironmentBackupAndRestore` | **实际通过，24.29s** | A、B、C 三个真实运行环境写入各自三类存储；仅选 A/B 导出，二者完整进程树正常退出 0，C 仍运行。包仅含 2 环境且不包含内核二进制；重复请求沿用 operation。A/B 实际改写三类数据及配置后 full restore，再 Service.Close/Open；三者分别实际读回 OLD 三类存储，原 ID/profile/data references 一致，未选 C 配置与数据保持。 |
| `TestRealLocalAcceptanceCookiePartitionsExpiryClearAndCancellation` | **实际通过，4.73s** | default 与两种 partition ancestor key 共 3 个真实 Cookie 写读回；相同项不重写；不完整 partition 与过期输入拒绝且不改同 key；merge 不删其它 partitions；明确 replace-all 实际 clear；重放 dedup；其它环境数据不变；普通响应不泄露 Cookie 值；取消后实际第 1 条已写/核实、第 2 条未发。取消时序使用 host scheduling barrier，单独说明见下文。 |

两项 local acceptance 在一个顺序 Go 测试进程中执行，package 总时间 29.184s；没有全套检查或无因重复。

## 重跑依据与未覆盖边界

根任务发现 `.appdata` 根新建普通两层文本目录移动同样被拒绝，而 `output` 根普通目录移动正常；当前开发目录监视没有排除 `.appdata`，`output` 已排除。本子任务没有把 watcher 因果关系写成已证实，也没有停止其它实例。为排除目录环境影响，创建了新的独立 output workspace，原失败根和日志未复用、未删除、未覆盖。新根下相同已有检查完整通过，因此记录为“有明确环境差异后通过”，不是原失败通过。

真实 kernel fixture 的初始化直接调用真实 `kernel.Prepare`、逐文件校验、probe 后移动并保存真实 record。三个 workspace 检查仅把这份已验证 record 注册到 owned 数据库，不替换 provider/launcher、不注入假内核能力。因此它们证明 Runtime/Cookie/Backup 的真实服务能力；**不证明 `Kernel.SelectArchive → Kernel.Install` 的桌面安装交互或 production 发布调用链**，这一部分需根任务自己的新 exe 验证。

Cookie 取消检查仅在第 1 条已完成真实写入/读回后暂停宿主调度，调用实际 `Operation.Cancel`；它使用真实 Cookie transport，准确证明取消后的已执行/未发送范围。该可控 barrier 属于自动化证据，不能冒充人手操作取消、不可控管道中断或桌面 UI 运行证据。

## 资源清理核实

- kernel fixture 两轮实际 Stop、Done、exit code 0、network journal pending 0、AppContainer absent 都是断言通过，JSON 逐轮记录。
- local acceptance 的 cleanup 实际调用 `Service.Close`，错误会令测试失败；备份检查另外断言选中完整进程树已正常退出，未选环境在预期期间仍活着。
- 2026-10-07 03:39:05 UTC 独立读取 Windows 进程清单：匹配本子任务成功/失败 owned root 的进程数为 **0**，已排除当前检查进程自身。owned marker、verified-kernel record 与 4 份成功 JSON 都存在。
- 本子任务不把“进程数为 0”泛化为所有 Windows 临时资源已清理；AppContainer/journal 的确定结论来自上述真实 kernel 断言。

## 证据索引与复现范围

以下输出均在上述 ignored 输出根。日志与 JSON 是本机补充证据，不提交真实目录/profile 到公开仓库：

- `kernel-protected.log`：首次真实失败，保留 Access denied。
- `workspace-prerequisite.log`：首次应用检查前置失败。
- `after-kernel-failure.json`、`publication-source-inspection.jsonl`：首次失败后的 owned 进程与目录核对；payload 为 Directory、无 read-only 属性，未证实为 probe 进程残留。
- `kernel-protected-output-root.log/json`：新根真实 kernel 两轮通过，03:37:22 UTC。
- `workspace-protected-output-root.log/json`：真实 Runtime 和服务重开通过，03:37:39 UTC。
- `cookie-and-backup-real.log`：两个实际 local acceptance PASS。
- `two-environment-backup-restore.json`：03:38:41 UTC 真实选择范围及三类存储恢复。
- `cookie-boundaries.json`：03:38:46 UTC Cookie 分区、过期、清除、取消及隔离。
- `after-service-tests.json`：03:39:05 UTC owned 进程为 0，未执行进程终止。
- `run.json`：原始启动清单；`supplemental-run.json`：首次失败与新独立根、环境重跑理由及最终状态。

实际运行仅限以下四个已有测试，Go 工具链使用仓库 `.tools/go/bin/go.exe`，本地 GOPATH/GOCACHE，`GOTOOLCHAIN=local`、`CGO_ENABLED=0`：

```text
go test -count=1 -v ./internal/kernel -run '^TestRealProtectedProxyLaunchAndReopen$'
go test -count=1 -v ./internal/workspace -run '^TestRealProtectedRuntimeApplicationReopen$'
go test -count=1 -v ./internal/workspace -run '^TestRealLocalAcceptance(CookiePartitionsExpiryClearAndCancellation|TwoRunningEnvironmentBackupAndRestore)$'
```

前两项以 `PRISM_PROTECTED_TEST_ROOT`、`PRISM_KERNEL_ARCHIVE` 指向本子任务 owned fixture；成功输出通过 `PRISM_PROTECTED_EVIDENCE` 与 `PRISM_PROTECTED_APP_EVIDENCE` 指定。后两项以 `PRISM_LOCAL_ACCEPTANCE_ROOT`、`PRISM_LOCAL_ACCEPTANCE_EVIDENCE` 指向同一 owned fixture 与独立证据输出根。只有 explicit owned marker 校验后才使用 fixture，未运行跳过分支。

源码来源：
[真实 kernel 保护通道及重开](../../internal/kernel/network_real_windows_test.go)、
[真实 Runtime 与服务重开](../../internal/workspace/runtime_protected_real_test.go)、
[真实 Cookie 边界](../../internal/workspace/cookie_boundaries_real_windows_test.go)、
[真实多环境备份恢复](../../internal/workspace/backup_multi_real_windows_test.go)、
[owned local acceptance fixture](../../internal/workspace/local_acceptance_real_windows_test.go)。

批量 not-executed/skipped、部分完成、取消及原 operation 重试的服务/DACL/UI模拟证据另见
[批量收口报告](desktop-batch-closeout.md)；不使用本报告扩大那些检查的证据层级。
