[返回 #35 交付记录](../../issue35.md) · [逐图几何](manifest.json) · [取证参数](capture.json) · [局部图矩形](crops.json) · [尺寸与SHA-256](integrity.json)

# #35 合成实现图索引

源码 `c096f9b4d01685871de4fc3ece04721d4ce8ea26`。**100张viewport PNG + 9张实现局部图；不是最终App或真实桌面验收。** 两目录分别1440×900和1280×800，DPR1/100%/zh-CN/Asia-Shanghai，正文和表体初始顶部；原归档/原图/私有比较拼图不在仓库中。

文件规则：`<视口>/<native或demo>-<状态>.png`。下表列出全部50个模式×状态组合；每组合均有两种尺寸。直接参考是外观参考，不移植其商业业务；“映射”只指同类容器。

## 两模式共有（16组合 × 2模式 × 2尺寸 = 64张）

| 状态后缀 | 状态 | 原参考 / 说明 |
| --- | --- | --- |
| `proxy-list` | 普通代理列表 | 直接 `proxy-list`；安全字段代替商业字段 |
| `proxy-add` | 单个添加 | 直接 `proxy-add`；裁剪无服务字段，先解析再保存 |
| `proxy-import-text` | 空文本导入 | 直接 `proxy-import-text` |
| `proxy-import-preview` | 两行有效预览 | 直接 `proxy-import-preview`；遮罩、原行号、明确所选 |
| `proxy-import-error` | 错误行预览 | 直接 `proxy-import-error`；保留坏行，不仿静默丢弃 |
| `proxy-import-duplicates` | 重复候选未默认选中 | 映射 `proxy-import-preview`；重复地址不等于同认证 |
| `proxy-import-file` | 未选文件 | 直接 `proxy-import-file`；UTF-8文本替代Excel |
| `proxy-import-file-selected` | 已读合成文件 | 映射 `proxy-import-file`；尚未解析/保存 |
| `proxy-import-file-error` | 非UTF-8失败 | 映射 `proxy-import-file`；原文件/输入保留、固定footer |
| `proxy-edit` | 保留认证 | 直接 `proxy-edit`；不回填旧值 |
| `proxy-edit-replace` | 明确替换认证 | 映射 `proxy-edit`；新账号/密码初始为空 |
| `proxy-edit-clear` | 明确清除认证 | 映射 `proxy-edit` |
| `proxy-bound` | 准确使用情况 | 直接 `proxy-bound`；demo6项/native服务返回项 |
| `proxy-delete-confirm` | 删除未引用节点确认 | 映射 `group-delete-confirm`，不是分组删除业务 |
| `kernel-list` | 服务内核列表 | 映射 `proxy-list`；demo仅元数据/native合成安全记录 |
| `kernel-details` | 内核只读能力drawer | 映射 `environment-edit` 的660px右贴边容器 |

例如：[native普通1440](1440x900/native-proxy-list.png) / [1280](1280x800/native-proxy-list.png)，[demo普通1440](1440x900/demo-proxy-list.png) / [1280](1280x800/demo-proxy-list.png)；其余完整文件名可在 [manifest](manifest.json) 查找。

## native独有（18组合 × 2尺寸 = 36张）

| 状态 | 1440×900 | 1280×800 | 原参考映射 / 范围 |
| --- | --- | --- | --- |
| 内核移除确认 | [PNG](1440x900/native-kernel-delete-confirm.png) | [PNG](1280x800/native-kernel-delete-confirm.png) | `group-delete-confirm`；没有执行移除 |
| 官方精确构建准备 | [PNG](1440x900/native-kernel-prepare.png) | [PNG](1280x800/native-kernel-prepare.png) | `proxy-add` 620表单；用户填写合成版本/摘要 |
| 可信ZIP准备 | [PNG](1440x900/native-kernel-prepare-local.png) | [PNG](1280x800/native-kernel-prepare-local.png) | `proxy-add`；假文件token，没有磁盘归档 |
| 内核进度 | [PNG](1440x900/native-kernel-progress.png) | [PNG](1280x800/native-kernel-progress.png) | `batch-progress-fixture`；模拟受理 |
| 内核待保存 | [PNG](1440x900/native-kernel-persistence-pending.png) | [PNG](1280x800/native-kernel-persistence-pending.png) | `batch-progress-fixture`；不是持久成功 |
| 内核失败 | [PNG](1440x900/native-kernel-error.png) | [PNG](1280x800/native-kernel-error.png) | `batch-progress-fixture` |
| 内核历史 | [PNG](1440x900/native-kernel-history.png) | [PNG](1280x800/native-kernel-history.png) | `proxy-edit` 620容器；合成持久记录 |
| 内核失败重试 | [PNG](1440x900/native-kernel-retry.png) | [PNG](1280x800/native-kernel-retry.png) | `batch-progress-fixture`；同精确输入的新attempt |
| 内核已取消 | [PNG](1440x900/native-kernel-cancelled.png) | [PNG](1280x800/native-kernel-cancelled.png) | `batch-progress-fixture`；仅原合成任务 |
| 已受理后刷新异常 | [PNG](1440x900/native-kernel-accepted-refresh-error.png) | [PNG](1280x800/native-kernel-accepted-refresh-error.png) | `batch-progress-fixture`；保原回执，不重复安装 |
| 代理保存失败 | [PNG](1440x900/native-proxy-save-error.png) | [PNG](1280x800/native-proxy-save-error.png) | `proxy-import-error`；固定footer，输入/预览保留 |
| 代理提交未知 | [PNG](1440x900/native-proxy-import-unknown.png) | [PNG](1280x800/native-proxy-import-unknown.png) | `proxy-import-error`；冻结原请求 |
| 代理未知换页返回 | [PNG](1440x900/native-proxy-import-return.png) | [PNG](1280x800/native-proxy-import-return.png) | `proxy-import-preview`；仍是原请求，无再次提交 |
| 迁移选择 | [PNG](1440x900/native-migration-select.png) | [PNG](1280x800/native-migration-select.png) | `proxy-bound` 1040容器 |
| 迁移差异 | [PNG](1440x900/native-migration-diff.png) | [PNG](1280x800/native-migration-diff.png) | `proxy-bound`；准确saved revision/policy |
| 迁移试用确认 | [PNG](1440x900/native-migration-confirm.png) | [PNG](1280x800/native-migration-confirm.png) | `group-delete-confirm` 400容器；未勾选/未准备 |
| 迁移进度 | [PNG](1440x900/native-migration-progress.png) | [PNG](1280x800/native-migration-progress.png) | `batch-progress-fixture`；预置合成owner，没启动副本 |
| 迁移取消保原状态 | [PNG](1440x900/native-migration-cancelled.png) | [PNG](1280x800/native-migration-cancelled.png) | `batch-progress-fixture`；没有commit/真实目录操作 |

## 实现局部图（仅裁自1280图，未缩放）

| 核对内容 | 局部图 |
| --- | --- |
| 两行安全预览 | [1050px dialog](crops/1280-native-proxy-import-preview-dialog.png) |
| 坏UTF-8不覆盖前一文件 | [500px dialog](crops/1280-native-proxy-import-file-error-dialog.png) |
| 保存失败始终可见 | [固定footer](crops/1280-native-proxy-save-error-dialogFooter.png) |
| 未知提交只能核实原请求 | [固定footer](crops/1280-native-proxy-import-unknown-dialogFooter.png) |
| 六行使用情况与分页可见 | [1040px dialog](crops/1280-demo-proxy-bound-dialog.png) |
| 官方准备不留旧空大卡 | [620×420](crops/1280-native-kernel-prepare-dialog.png) |
| 可信ZIP/摘要/信任说明 | [620×530](crops/1280-native-kernel-prepare-local-dialog.png) |
| 受理后刷新异常保原任务 | [400px任务窗](crops/1280-native-kernel-accepted-refresh-error-dialog.png) |
| 迁移准确策略与差异 | [1040px dialog](crops/1280-native-migration-diff-dialog.png) |

## 不能从图片推导的结论

- native图均为模拟bridge，demo图均为示例存储；无真实代理、凭据、profile、Cookie、私有绝对路径或原商业资源。
- 普通native使用数量不与原参考强行一致：只展示真实合成服务返回的引用。完整数据、范围、阶段和错误由服务契约决定，不为截图造记录。
- 安全适配会改变列名、字段数量、表单高度、footer高度和预览可滚区域。没有同名内核/迁移原图，不能宣称商业逐像素全通过。
- 原参考的返回/保存/实时检测/下载/取消证据不完整。最终App跨页、真实桌面和#37结论见后续记录，本目录不更新旧候选或历史验收计数。
