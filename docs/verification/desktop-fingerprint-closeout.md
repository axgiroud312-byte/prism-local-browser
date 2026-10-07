# 桌面可用性收口：指纹请求生命周期

日期：2026-10-07。分支：`codex/issue32-37-ui-integration`，沿用 `8688906` 基线。用户已取消严格 1:1 要求；本项只调查和修复功能、身份保护及失败恢复，不扩展视觉截图或参考审计。

初版覆盖实际 App + WailsAdapter 的合成桥自动化检查，该部分没有启动 Windows 桌面程序或 fingerprint-chromium，不进入正式桌面通过计数。本次另补根代理实际操作新 Windows Wails `0.3.0-preview.7/8` 程序、真实内核及 SQLite 的记录；本子代理只读核对相应证据。下面将自动化与真实桌面观察分开，历史未验记录不改写成通过，正式桌面计数由本轮总验收统一汇总。

## 发现与修复

关闭指纹预览仍在等待的编辑窗口后，旧请求保留全局 `fingerprintBusy` 和 `generating`。此时打开另一环境，新的自动预览无法发起，保存也持续被阻断；旧请求迟到的 `finally` 还可以改变当前窗口的等待状态。档案回滚共用同一等待标志，存在同类生命周期问题。

`src/App.tsx` 现在给每次生成/回滚读取分配单调请求序号。确定放弃草稿时，立即失效旧读取并释放当前窗口的等待；迟到的成功、失败、异常和 `finally` 只有匹配当前请求才可改变状态。新目标继续等待自己的请求，旧结果不覆盖当前 seed、内核、数据引用或错误。回滚读取异常也保留草稿并给出可重试反馈。

保留既有行为：生成中禁止保存和关键设备字段修改；取消丢弃预览；自动准备/失败重试使用 `regenerate: false`；显式“换一套”才使用 `true`。打开未保存确认后、尚未真正放弃的当前预览仍可接收自己的回复，确认内容变旧时拒绝放弃，不能误删新草稿。

## 按层判定

| 检查 | 判定 | 依据与边界 |
| --- | --- | --- |
| native 页面等待、保存保护、关闭后新目标可以独立生成 | 自动化实际通过 | 当前 App + 注入 Wails bridge；旧请求不返回时，第二目标仍发起自己的 Generate。不是桌面进程证据。 |
| 迟到成功、业务失败、桥异常不覆盖新目标、不解除新目标等待 | 自动化实际通过 | 三条独立结果检查；新目标保存继续禁用，直至自己的成功回复。 |
| 预览失败保留输入，原预览重试不换 seed | 自动化实际通过 | 同一 previewId、kernelId、templateId、白名单覆盖字段，`regenerate: false`；名称编辑不进入指纹生成白名单。 |
| 关闭回滚后迟到回复，以及取消“换一套”再重开 | 自动化实际通过 | 新目标等待不被旧回滚解除；重开仍显示原保存 seed、精确 coreId 与独立 dataRef，合成保存视图未改变。 |
| demo 的持久“生成中”异步分支 | 不适用且有依据 | DemoAdapter 同步生成预览，不为状态覆盖率改为异步。native 服务跨 bridge 的等待由上述检查覆盖。 |
| preview.7 普通编辑、退出程序和再次启动后的保存身份 | 真实桌面实际通过 | A 的配置 revision 1→2，seed `1279120188`、精确 150 kernelId、fingerprintId、dataRef 未变；退出/重启后 A/B 原引用及 seed 均一致。实际数据库与浏览器重开记录见下。 |
| preview.7 运行中关键设备字段保护 | 真实桌面实际通过 | 实际编辑窗口的内核、网站语言、时区、CPU 偏好及“换一套”均不可操作；普通保存仍可用。UIA 中只读种子显示 enabled=true，因此不把该属性误写为“种子控件禁用”；保存种子稳定由数据库对照证明。 |
| preview.8 B 的迁移失败后重开、身份与原数据恢复 | 真实桌面实际通过 | 同一个 B 用原 ID、seed `1562739753`、148 精确 kernelId、fingerprintId 和 dataRef 重开；实际网页 CPU14、148 UA、en-US/New York，以及测试 Cookie/LocalStorage/IndexedDB 原标记一致。这是失败恢复通过，不是迁移成功。 |
| preview.8 A 的普通 150 打开及原 ID 重试 | 仍阻塞：两次实际启动均失败 | 原 A 的普通直连打开真实创建主进程后失败，首次运行记录及独立 Application Error 1000 绑定同一 150 崩溃；一次正常重试仍为 CONTROL_CHANNEL_LOST、exitCode 80000003，没有实际 A 页面读回。程序重开后重算 ready 不能算打开成功。 |
| 新桌面程序中的持续生成等待或实际迟到 native 回复 | 仍阻塞：桌面场景未实测 | 本轮真实桌面创建/编辑/重开已运行，但没有捕获自然发生的长等待或迟到 native 回复。受控 bridge 延迟/故障检查只作为自动化证据；不能补成桌面长等待通过。 |
| preview.7/8 同一 B 的 148→150 GUI 迁移成功 | 仍阻塞 | 两次真实 GUI 试用的新 150 主进程失败，没有 after 采样及配置切换；已确认原状态保护和重开，150 的具体崩溃断言/上游原因仍未知。不得用短路径独立 Launcher 对照替代这两次 GUI 成功。 |

## 新桌面程序的真实身份与保护证据

根代理在隔离测试根的 preview.7 实际编辑 A、退出程序、重新启动并打开原环境。本子代理核对 `output/goal/desktop-closeout/running-a-db.json`、`after-exit.json`、`after-restart-open.json`：A 的 ID `92a3a98a-c62b-4e1f-b6c7-ff18e8d784d3`、seed `1279120188`、kernelId `1575b6aa-ee86-486f-9e83-87346ea6e3d6`、fingerprintId `b16a1c4a-9b9e-4fa8-8199-bcd833c96a94`、`environments/<A ID>/user-data` 都保持一致；B 的原 148 身份同样保持。运行中 UIA 记录 `running-field-protection.json` 对应实际 preview.7 窗口，上表仅认定已记录字段的保护。

preview.8 对同一个 B 的 GUI 迁移操作为 `3b36aa8c-93eb-40fa-bf6c-c96806c5daa1`，请求为 `b04ee028-a9b2-40f6-8ba2-f59b538e83d4`。真实旧 148 试用 PID78960 的采样通过，CPU14，三种 canary 读回一致；新 150 工作副本 PID50756 创建后失去控制通道。`migration8-running-db.json` 持久记录 `CONTROL_CHANNEL_LOST/control-read-ended`；trialExited=true、committed=false、stage=original-retained，completedIds 为空，没有把未完成迁移报为成功。

第 7 次的故障 PID32268 与第 8 次的故障 PID50756 分别绑定独立 Windows Application Error 1000：两者均为精确 `chrome.dll 150.0.7871.186`、异常 `80000003`、偏移 `77b71bd`。第 8 次记录 ID48682，进程创建 `2026-10-07T04:18:27.9544564Z` 与 journal 精确一致，崩溃事件 `04:18:28.9259334Z`，路径也匹配固定 150 内核。首次窄查询时事件尚未可见，第二次同 PID/同启动时间的窄查询查到并核对，不据第 7 次推断第 8 次。脱敏绑定证据为 `migration150-crash-owned.json` 与 `migration8-crash-owned.json`；没有扩扫 WER。

preview.8 再次实际打开原 B，`preview8-reopened-db.json` 的 ID `5fd3dd7c-6191-4238-9a95-a1ed4afce5f8`、seed `1562739753`、kernelId `24620122-863e-441f-8628-c7577a05d412`、fingerprintId `c59c8ba8-3a16-4377-afcb-6e762d920abe`、`environments/<B ID>/user-data` 均未改变。`preview8-reopened-browser-reports.json` 在 `04:20:15.980Z` 的真实网页读回测试 Cookie `SYNTHETIC-B-IMPORT`、LocalStorage/IndexedDB `SYNTHETIC-B-OLD`，CPU14、148 UA、en-US 与 America/New_York 保持一致。因此 B 的原身份和原数据失败恢复实际通过；这不消除 150 GUI 迁移阻塞。

**150 失败也发生在普通打开。** preview.8 的原 A 普通直连启动操作 `82eeadbf-c21b-4aa0-ba1c-b1f9ac8cc0b5`、会话 `516e8ab3-95c4-4da0-88fa-7ff133a32dd3`，主进程 PID47640 创建于 `04:20:29.78323Z`。实际运行记录为 error，操作 failed、completedIds 为空，原因 `CONTROL_CHANNEL_LOST/control-read-ended`、exitCode `80000003`；原 kernelId 和 dataRef 仍记录，资源退出后显示可重试。独立 Application Error 1000 记录 ID48684，于 `04:20:30.7997375Z` 报告精确同 PID、同完整创建时间及同 150 exe 的 `chrome.dll 150.0.7871.186 / 80000003 / 77b71bd`。脱敏绑定在 `preview8-a-start-failed-owned.json`，不是按迁移事件推断，也不能因 preview.7 曾普通打开成功而抹去本次失败。

只读系统整体内存计数在 `04:27:54.4508506Z` 显示提交量 63700459520 字节、上限 66278596608 字节（96.11%），剩余提交空间 2578137088 字节，可用物理内存 3846971392 字节；另存 `preview8-a-memory-observation.json`。这是当前提交压力的可靠线索，未记录失败时的数值或栈/断言，不能证明资源耗尽导致崩溃。没有枚举或停止其他程序，没有调整 GPU/sandbox 参数，也没有把正常重试预设为成功。

两个独立较短根的生产 Launcher 控制（同一 150、同 seed/CPUauto，新空 profile 与失败工作副本的独立拷贝）真实启动和三存储 canary 采样通过，并正常退出全部资源。其来源为 `migration150-launch-controls-result.json`，属于真实内核自动化控制证据，不计 GUI 迁移成功。它们说明不能认定该 seed 或副本内容必然在 150 崩溃；路径/时序与具体崩溃断言仍未分离或查明，没有猜参数、关闭 sandbox 或改 seed 来制造通过。

原 A 的正常重试操作 `bf383d55-294f-41cf-ba24-4aedd7f0e1d0`、会话 `a79b70ed-f2c4-42d6-bc77-0d3b5506762b`、主进程 PID41268，实际仍失败，原因 `CONTROL_CHANNEL_LOST/control-read-ended`、退出码 `80000003`。首次启动有精确 Windows 事件绑定；本次重试按实际 runtime/operation 记录判失败，未另取第二份事件，不能借首次事件推断第二次的 DLL 偏移。重开后的 ready 是陈旧运行记录重核结果，不是 A 浏览器实际读回。

## 实际检查及历史失败

1. 修复前运行单条 `closed native generation success`：失败。第二窗口等待第二个请求时，实际请求数为 1，预期为 2；证明旧窗口遗留等待阻断新目标。此失败不改写成历史通过。
2. 首轮修复检查：4/5 通过。失败断言错误地要求 native Generate 传 seed/coreId/名称覆盖字段；真实 WailsAdapter 仅传语言、时区、CPU 和窗口偏好，seed 保存在服务预览，kernelId 是独立参数。按真实契约修正断言，没有扩造服务分支。
3. 最终定向检查：`npx playwright test tests/ui/fingerprint-request-lifecycle.spec.ts tests/ui/environment-confirmation.spec.ts -g "closed native|native failed generation|a late fingerprint"`，6/6 通过。包含新增生命周期 5 条和原未保存确认迟到回复回归 1 条。
4. `npm run typecheck`：通过。`git diff --check`：通过。本子项没有运行全套检查、构建或新增截图验收；根代理的新桌面构建和 GUI 记录按上节独立补证。

已有 native 服务源代码检查明确：Generate 只改服务预览、不写数据库、不执行内核探测；普通预览保留 seed；提交使用 profileHash/expectedRevision 与忙租约；档案修订和浏览数据引用分离。对应既有 Go 检查为 `TestFingerprintGenerateIsReadOnlyAndUsesExactCapabilities`、`TestFingerprintRevisionRoundTripAndSameKernelRestorePreserveData`、`TestFingerprintCommitCannotBypassPreviewConflictOrBusyLease`。本子项只核对其源码，未把它们算成执行通过或真实桌面通过。

## 修改范围

- `src/App.tsx`：指纹请求归属、关闭失效与迟到回复/终结保护；回滚读取异常反馈。
- `tests/ui/fingerprint-request-lifecycle.spec.ts`：受控生成、失败、关闭重开、旧回复和回滚检查。

没有修改 demo 同步生成、指纹模型、内核合同、WailsAdapter 或桌面服务。本项不改变已保存身份和数据迁移规则。
