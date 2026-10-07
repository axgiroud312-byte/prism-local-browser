# 桌面可用性收口：指纹请求生命周期

日期：2026-10-07。分支：`codex/issue32-37-ui-integration`，沿用 `8688906` 基线。用户已取消严格 1:1 要求；本项只调查和修复功能、身份保护及失败恢复，不扩展视觉截图或参考审计。

本报告覆盖实际 App + WailsAdapter 的合成桥自动化检查。它没有启动 Windows 桌面程序或 fingerprint-chromium，不进入正式桌面通过计数。新桌面程序的启动、SQLite 重开、正常环境内核读值与运行中关键字段保护，由本轮桌面总验收另行记录。历史未验记录保持原范围。

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
| 本轮新桌面程序中真实重开、身份稳定与运行中保护 | 此报告仍未验证 | 需要本轮新 exe、真实 SQLite/受控内核证据；合成 bridge 和延迟注入不能替代。由桌面总报告补判，不计本报告通过。 |

## 实际检查及历史失败

1. 修复前运行单条 `closed native generation success`：失败。第二窗口等待第二个请求时，实际请求数为 1，预期为 2；证明旧窗口遗留等待阻断新目标。此失败不改写成历史通过。
2. 首轮修复检查：4/5 通过。失败断言错误地要求 native Generate 传 seed/coreId/名称覆盖字段；真实 WailsAdapter 仅传语言、时区、CPU 和窗口偏好，seed 保存在服务预览，kernelId 是独立参数。按真实契约修正断言，没有扩造服务分支。
3. 最终定向检查：`npx playwright test tests/ui/fingerprint-request-lifecycle.spec.ts tests/ui/environment-confirmation.spec.ts -g "closed native|native failed generation|a late fingerprint"`，6/6 通过。包含新增生命周期 5 条和原未保存确认迟到回复回归 1 条。
4. `npm run typecheck`：通过。`git diff --check`：通过。没有运行全套检查、构建或新增截图验收。

已有 native 服务源代码检查明确：Generate 只改服务预览、不写数据库、不执行内核探测；普通预览保留 seed；提交使用 profileHash/expectedRevision 与忙租约；档案修订和浏览数据引用分离。对应既有 Go 检查为 `TestFingerprintGenerateIsReadOnlyAndUsesExactCapabilities`、`TestFingerprintRevisionRoundTripAndSameKernelRestorePreserveData`、`TestFingerprintCommitCannotBypassPreviewConflictOrBusyLease`。本子项只核对其源码，未把它们算成执行通过或真实桌面通过。

## 修改范围

- `src/App.tsx`：指纹请求归属、关闭失效与迟到回复/终结保护；回滚读取异常反馈。
- `tests/ui/fingerprint-request-lifecycle.spec.ts`：受控生成、失败、关闭重开、旧回复和回滚检查。

没有修改 demo 同步生成、指纹模型、内核合同、WailsAdapter 或桌面服务。本项不改变已保存身份和数据迁移规则。
