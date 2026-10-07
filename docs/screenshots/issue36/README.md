# #36 合成截图

[验证记录](../../verification/issue36.md) · [字段/尺寸/哈希索引](screenshots.json) · [本地查看页](index.html)

最终 **39状态 × 2视口 = 78张PNG**，均标记 **SYNTHETIC / 非真实桌面验收**，仅用独立演示数据或内存 native bridge。两种内容视口为 **1440×900 / 1280×800**，DPR1、100%、viewport-only。没有原商业图/资产、真实凭据、Cookie 值或私人路径。

页面由实际组件/按钮到达，任务结果是作者定义的合成情形。除最后两种工作区安全边界外，均为本票 extracted wiring harness；**不表示 MAIN App 的 demo/记录/帮助已完成接入**。原图留在仓库外，不在查看页中混入原参考。

每个下列 ID 在两个同名视口目录都有 PNG；`screenshots.json` 分别记录直接参考/容器映射、原参考 ID、实际 App 范围、测量与哈希，另记有效遮罩数量和最上层窗口的实际层级。最终文件已核对尺寸与SHA-256，页面错误0；下层窗口不会靠双重遮罩变黑，故障窗口里的诊断真实绘制在最上层。

| 分类 | 状态 ID |
| --- | --- |
| 操作记录 | `activity-list`（直接布局/字段适配）、`activity-empty`、`activity-details` |
| 帮助 | `guide-user`、`guide-prd`、`guide-development`、`guide-kernel`、`guide-kernel-bottom` |
| 演示备份 | `demo-backup-list`、`demo-backup-create`、`demo-backup-save-error`、`demo-backup-import`、`demo-backup-import-error` |
| 演示恢复 | `demo-restore-preview`、`demo-restore-confirm`、`demo-restore-save-error` |
| 本机备份 | `backup-list`、`backup-create`、`backup-import`、`backup-cancel`、`backup-progress`、`backup-unknown`、`backup-failed` |
| 本机恢复 | `restore-preview`、`restore-conflict`、`restore-missing-build`、`restore-confirm`、`restore-progress`、`restore-rollback`、`restore-protected`、`restore-unknown` |
| 诊断 | `diagnostics-preview`、`diagnostics-cancel`、`diagnostics-unknown`、`diagnostics-end-confirm`、`diagnostics-ended`、`diagnostics-minimal` |
| 实际 App 安全边界 | `workspace-blocker-over-restore`、`workspace-blocker-diagnostics`（仍有待移除旧页标题，不用于主页面最终1:1结论） |

只有操作记录列表有同名直接布局参考。备份/恢复/诊断映射到500/1040/400/620px容器；帮助仅映射shell/标题/正文规则，原设置页的覆盖层不是完整帮助参考。安全字段与正文数量不同，确认/结果高度不伪装为原短删除弹窗。

代表性截图：[记录1440](1440x900/activity-list.png) / [记录1280](1280x800/activity-list.png) · [恢复预检1440](1440x900/restore-preview.png) / [恢复预检1280](1280x800/restore-preview.png) · [维护保护1440](1440x900/restore-protected.png) / [维护保护1280](1280x800/restore-protected.png) · [诊断1440](1440x900/diagnostics-preview.png) / [诊断1280](1280x800/diagnostics-preview.png)。
