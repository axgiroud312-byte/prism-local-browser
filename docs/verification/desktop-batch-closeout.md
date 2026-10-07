# 桌面可用性收口：批次范围与多项创建

日期：2026-10-07。执行分支 `codex/issue32-37-ui-integration`；本次核对的 HEAD 为 `86889064d83bc098ad2f518be076fdffae45341a`，保留 PR #31 的精确 `e170099` 基线及当前工作区改动。本子项没有切换、重置、合并或提交分支。

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
| 新 exe 中的批次 UI、真实 fingerprint-chromium 会话 | 本子项仍未验证 | 本子项没有操控新 Wails exe 或实际 Chromium。构建及真实桌面验证由总收口执行；这些结果须独立追加，不能以本报告的 UI 模拟或 DACL 自动化冒充。 |

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
