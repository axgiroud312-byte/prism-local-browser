# 桌面可用性收口：批次范围与多项创建

日期：2026-10-07。执行分支 `codex/issue32-37-ui-integration`；初始服务/自动化核对的 HEAD 为 `86889064d83bc098ad2f518be076fdffae45341a`，保留 PR #31 的精确 `e170099` 基线及当前工作区改动。后续真实 preview.8 桌面证据在本文末独立追加，构建来源为 `e349eeadaa92a83ce60d97ec2ef5b309fdf68d92`。本子项没有切换、重置、合并或提交分支。

用户已取消严格 1:1 对齐要求。此次只核对真实服务契约、可操作的反馈和失败恢复；没有恢复商业参考、扩展视觉截图或继续图库审计。历史参考缺失及历史失败保持原记录；本报告只追加本次实际检查。

## 判定

| 检查项 | 判定 | 依据与边界 |
| --- | --- | --- |
| Batch 成功时准确列出全部目标，不把受理当成功 | 实际通过 | 原有 31 项 RPC/SQLite 回归及新增成功 UI 回归；逐项 ID 不重复、分页不漏项。UI 是浏览器中的模拟 Wails bridge。 |
| 同计划的逐项失败、资源暂停、未执行后缀 | 实际通过 | 新增 5 项真实服务回归：索引 `[0,1,2,3]` 曾准备，结果 `[completed,failed,completed,not-executed,not-executed]`；索引 4 没有准备。报告为 2 完成、1 失败、2 未执行，界面分别呈现。故障通过受控 seam 注入。 |
| 失败后继续，只处理失败和未执行项 | 实际通过 | 同一冻结计划重试只准备 `[1,3,4]`；本次新增完成 3，总完成 5。原已完成与已准备 ID、已完成 seed 和数据引用保持；原 commit/retry requestId 重发返回原任务，不产生重复环境。 |
| 取消准确保留已完成项与未执行范围 | 实际通过 | 4 项计划在索引 1 准备期间取消，只有索引 0 已事务提交，索引 `[1,2,3]` 均未执行，数据库只有 1 条环境。继续只准备 `[1,2,3]`，保留索引 1 的准备身份。另有原测试验证取消不会影响其他批次。 |
| 关闭重开批次窗口不自动执行；旧尝试结果不被重试覆盖 | 实际通过 | 新增实际 App UI 自动化验证取消后关闭重开仍为 1 完成、4 未执行，没有 `Batch.Retry`；用户明确继续才对原 operationId 请求 Retry。旧失败尝试仍显示 2/1/2，其未执行行不暴露后续身份。服务历史回归也核对旧页。 |
| 实际 Windows 目录拒绝写入后的部分完成与重试 | 实际通过 | 显式开启 `PRISM_SAFE_GAP_VERIFY=1`。production prepare 小项真实拒绝写入后返回 `DIRECTORY_WRITE_FAILED`；257 项在实际 Win32 DACL 拒绝前完成 128 项，恢复原 DACL 后完成 257 项；ID/seed 唯一，准备身份、已完成身份保持，空目录与分页核对。属于真实文件系统和应用服务的自动化证据；不是新 Wails exe 的人工 UI 验收。 |
| Batch 的 `skipped` 返回分支 | 不适用且有依据 | 当前 [`BatchItem`](../../internal/workspace/batch_types.go) RPC 状态只有 `completed/failed/not-executed`；内部 `prepared` 在 [`projectBatchItem`](../../internal/workspace/batch_preview.go) 投影为 `not-executed`。重试通过未完成索引略过已完成项，不生成 `skipped` 业务结果。不能把该 N/A 算成测试通过。 |
| native Runtime 的“批量 skipped 响应” | 不适用且有依据 | Runtime 接口每次处理明确的单个 environmentId，批量打开是逐 ID 受理的任务队列。原有 FIFO 回归核对实际启动顺序 `[A,C,D]`，B 等待项被取消且从未启动，C 失败只重试 C。结果通过各自 Operation 呈现，没有服务端批量 `skipped` 字段。不能因此跳过逐项取消/失败证据。 |
| 单条 `Environment.Create` 的多项部分完成 | 不适用且有依据 | [`service.go`](../../internal/workspace/service.go) 明确拒绝 `count != 1`。新增真实 RPC 回归发送 count=2，收到 `CAPABILITY_UNSUPPORTED`，数据库 0 条环境，没有部分提交。真正的多项创建使用 Batch，其部分成功/失败/取消/重试已按上表检查；Create→Batch 移交不能充当单条 Create 部分完成证据。 |
| 批次创建后“打开失败只重试原 ID” | 此批次入口不适用 | [`NativeBatchDialog`](../../src/components/NativeBatchDialog.tsx) 和 Batch worker 不启动浏览器。新增 3 条批次 UI 均断言 `Runtime.Start`、`Environment.Create`、`Batch.Commit` 没有因读取历史或 Retry 被调用。单条“创建并打开”的实际链路由本次总桌面收口报告单独核对，不能用此处 N/A 代替。 |
| 新 exe 中的批次 UI、真实 fingerprint-chromium 会话 | 见下文真实桌面追加 | 初始子项没有操控新 Wails exe 或实际 Chromium；后续集成负责人实际操控 preview.8 的批次部分完成与续作已取得证据。Batch 本身不启动浏览器；不能把创建通过算作 150 内核启动通过。 |

demo 的 `skipped` 分支确实存在：[`App.tsx`](../../src/App.tsx) 的 demo 打开取消、已运行/关闭中、存储失败后尚未执行、关闭已停止环境均使用该 UI 状态。上表两个 N/A 只适用于当前 native 服务的不存在分支，不能推导为所有模式都没有 skipped，也不能取消 demo 本身的实际检查要求；demo 的执行来源由总回归记录分别保留。

## 实际运行的检查

1. 新增测试前执行 `go test -count=1 -v ./internal/workspace -run '^TestBatch|^TestRuntimeQueue'`：18 项通过。包括 production 空目录创建、逐项身份、资源暂停、取消、克隆、绑定失败、最终日志保存、重开不自动续作、去重、历史及 FIFO/迟到就绪取消保护。运行时使用项目固定 Go、本地 GOPATH/GOCACHE、`GOTOOLCHAIN=local`、`CGO_ENABLED=0`，没有额外构建发布产物。
2. `go test -count=1 -v ./internal/workspace -run '^TestBatchCloseout'`：最终 3 项通过，新增源为 [`batch_closeout_test.go`](../../internal/workspace/batch_closeout_test.go)。精确范围、取消续作、单条 Create 不支持多项均走真实 RPC/SQLite；目录等待与资源错误为自动化控制。
3. `PRISM_SAFE_GAP_VERIFY=1`，执行 `go test -count=1 -v ./internal/workspace -run '^TestV1SafeGapBatch(ProductionACLFailureNoTypedNil|257ActualACLRetry)$'`：2 项通过。安全证据写入 `output/desktop-closeout/batch-service/batch-production-acl.json`（03:23:47 UTC / 北京时间 11:23:47）与 `batch-257-acl.json`（03:23:49 UTC / 北京时间 11:23:49）。测试只改变新建自有临时根的 DACL，并恢复原值；真实私有工作区、浏览数据及凭据未参与。
4. `npx playwright test tests/ui/batch-desktop-closeout.spec.ts`：最终 3 项通过。实际 App 与 WailsAdapter、合成 native bridge，覆盖完整成功、混合失败/未执行/重试、取消关闭重开；无 native I/O。新增源为 [`batch-desktop-closeout.spec.ts`](../../tests/ui/batch-desktop-closeout.spec.ts)。
5. `npx tsc -p tsconfig.tests.json`：通过。

## 本次失败与修正来源

新增 UI 夹具初轮 3 项失败，原因是夹具变量 `planId`/`planID` 拼写不一致；修正后最终 3 项通过。夹具后续使用 `DOMContentLoaded` 安装覆盖，避免独立 init scripts 的不保证顺序；这里不是产品的迟到回复修复。本次没有把初轮失败改写为产品测试已通过。

单条 Create 不适用性回归首轮错误地给 `Environment.Preview` 空请求，服务正确拒绝为“环境预览请求无效”；改用已有 `preview(...,"create",...)` 夹具后最终 3 项 Go 回归全部通过。首轮夹具失败不算 Count 多项拒绝检查通过。

本子项未发现需要修改 production 批次 worker 或 NativeBatchDialog 的行为问题；新增的是之前没有明确核对的精确范围和可观察反馈检查。历史 1:1 视觉项、远端出口、真实 Chromium 与真实 exe 的未验证内容仍保留各自边界。

## preview.8 真实桌面追加

来源是集成负责人使用新 `0.3.0-preview.8` Windows exe 的实际 UI 操作、同工作区 SQLite 读回与 UI 动作记录。本子项另独立核对读回字段与身份一致性；不是模拟 bridge、直接向数据库写入故障或 DACL 自动化的替代记录。exe SHA-256 为 `d10c562bd732f0e3546440ebc0b331ec175bf0851f46c3887709d3a9125c8f4c`，详细来源见[交付核对](desktop-release-closeout.md)。

| 真实桌面检查 | 判定 | 实际范围与边界 |
| --- | --- | --- |
| 三项计划遇到已存在名称后的部分完成 | 实际通过 | GUI 先建立合成占名环境 `Desktop-8 2`，ID `001af4d3-2ea0-4903-bcc2-83beff056902`。另在真实 Batch 冻结三项计划 `2e25e5bc-45c3-46ae-a7cf-1307bb936803`。首尝试 `064f4b97-a390-4ce5-b342-7bf9bc11c75e` 是 failed，准确反馈 2 completed、1 NAME_CONFLICT、0 notExecuted。失败项没有被算完成，原占名环境没有被覆盖；不是为了覆盖率添加服务分支。 |
| 关闭重开历史与明确继续 | 实际通过 | GUI 修改原占名环境为 `Desktop-8 Occupant`，保留原 ID、seed `1795200787` 与数据引用；关闭重开批次历史不执行。点击明确继续后，原计划生成新 attempt `9b668c54-a586-4975-b35c-7364ca52bc01`，最终 3 completed、0 failed、0 notExecuted，attemptCompletedCount=1。不是重新发起三项创建。 |
| 已创建与已准备身份不重复、不重生成 | 实际通过 | 首次已创建的索引 0：ID `8bd3e50b-e8b9-43f3-ab89-0cf4b82754ab`、seed `1780375053`；索引 2：ID `2cd1da78-5517-4805-8b75-026bdacb1367`、seed `1770851980`。失败索引 1 的预配 ID `ff111180-4cc4-448c-b44e-373f78bc8cb4`、seed `416870583` 在续作保持并创建成功。独立比较两份 SQLite 证据的全部三个 prepared identity 精确一致，前两项 ID/kernel/fingerprint/revision/ref 精确一致。旧失败 operation 仍保留原 2/1/0 报告。 |
| 实际桌面取消后的执行/未执行范围 | 仍阻塞：未截获取消 | 另试真实 64 项计划 `8643d228-5558-47bb-ab29-c576155eaab3` / operation `8da08f29-91a8-450e-853d-3d81dff718d6`。UIA 找不到取消按钮时任务已 64 completed、0 failed、0 notExecuted，cancel_requested=0；没有实际取消请求或取消后的执行范围。**未截获 cancel，不计 cancel 通过**，也不能改记 N/A。上文服务取消和受控 UI 自动化的通过保持原来源。 |

原始证据留 ignored `output/goal/desktop-closeout/batch8-first-attempt.json`（04:24:54.448 UTC）、`batch8-retry-attempt.json`（04:26:29.095 UTC）、`batch8-cancel-attempt.json`（04:27:42.075 UTC），以及 `ui-actions.jsonl`。公开报告只使用合成名称、身份与范围。

当前真实批次入口只准备和创建，不调用 Runtime.Start。Create→Batch 移交仍不能证明不存在的单条 Create 多项部分提交；批次成功也不能解除真实 150 普通启动和迁移试用崩溃。实际桌面取消与真实指纹迟到回复尚未截获，完整桌面收口没有全部完成，正式历史 4/21 保持；总判断见[总验收](desktop-closeout.md)。
