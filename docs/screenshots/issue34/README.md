[交付/检查记录](../../verification/issue34.md) · [实现画廊](index.html) · [逐张清单](manifest.json) · [唯一参考](../../UI_REFERENCE.md)

# #34 合成截图与差异说明

2026-10-06，**36状态×两视口=72张PNG**。每个ID在 `1440x900/` 和 `1280x800/` 各一张；DPR1、100%、viewport-only。`manifest.json` 记录实际几何、滚动、模式和SHA-256。

这里只发布本项目独立实现的demo / 合成native bridge图；**没有商业原图/资源、真实Cookie/凭据/用户目录，也不是真实桌面或全部1:1验收**。shared App接入由仓库外预览转换验证，正式落地由 MAIN 完成。原/实现对照36组已在私有目录逐组查看，不放入仓库。

## 完整状态映射

| 实现ID（两视口均有） | 冻结参考容器 | 适配/边界 |
| --- | --- | --- |
| `environment-create` | environment-create | 连续支持字段；660px右drawer、固定标题/底部。 |
| `environment-create-proxy` | environment-create-proxy | 滚到真实代理区，直连含义与失败不直连保留。 |
| `environment-create-proxy-manager` | environment-create-proxy-manager | 使用已有服务代理选择；不造第二导入窗口，#35唯一导入待终验。 |
| `environment-create-fingerprint` | environment-create-fingerprint | 服务内核、稳定seed与可支持偏好；裁剪无服务技术字段。 |
| `environment-create-bottom` | environment-create-bottom | 技术详情展开后滚到底，不填空白凑原3649px。 |
| `environment-edit` | environment-edit | 普通编辑沿用已保存身份、构建、引用。 |
| `environment-clone` | environment-clone | demo按模板新建草稿；新seed/空Cookie，代理继承。 |
| `fingerprint-randomized` | fingerprint-randomized | 实际“换一套”只改未保存草稿，不复制归档通知。 |
| `kernel-selector` | kernel-selector | 两个合成可用服务构建，菜单264px；归档版本不硬编码。 |
| `batch-create` | batch-create | 同一表单的正整数数量输入，没有产品实例配额。 |
| `environment-delete-confirm` | environment-delete-confirm | 400px demo记录移除，不提供真实文件删除checkbox。 |
| `cookie-populated` | proxy-import-preview | demo独立文本预览1050px；原创建期Cookie不扩展创建契约。 |
| `create-save-failure` | environment-create | demo真实存储失败/留草稿；没有同名原错误屏。 |
| `native-busy-running` / `native-busy-resources` / `native-busy-persistence` / `native-busy-reconcile` | environment-edit | 合成状态；仅元数据可改，关键字段/生成锁定。不证明真实进程或忙保存成功。 |
| `native-profile-history` | environment-edit | 支持档案/历史在drawer高级设置；无同名原历史屏。 |
| `cookie-import` | cookie-import | 520px归档ZIP上传容器映射到实际UTF-8 JSON/TXT选择；**不支持ZIP**。 |
| `native-cookie-text` | proxy-import-text | 1050px文本/说明布局；输入默认隐藏。 |
| `native-cookie-preview` | proxy-import-preview | 1050px安全行预览；原文已清除，无value。 |
| `native-cookie-partial` / `native-cookie-unknown` / `native-cookie-pending` / `native-cookie-unconfirmed` | proxy-import-preview | 合成任务/未知回执；不是归档原执行证据。原请求冻结、仅失败子集合并、取消不撤销已写入。 |
| `native-batch-clone` / `native-batch-create-plan` | proxy-bound | 1040px明确冻结计划；不创建真实环境/目录，不启动内核。 |
| `native-batch-current` / `native-batch-history` / `native-batch-page-error` | proxy-bound | 当前/旧冻结尝试及读页失败。普通视觉用3项；分页失败用26项，非当前页=全量。 |
| `batch-progress-fixture` | batch-progress-fixture | 400px实际合成批次结果；原fixture只人工显示，不代表执行。 |
| `recycle-list` | recycle-list | 页形密集native管理器，2条合成记录；非原12条商业数据。 |
| `recycle-delete-confirm` | recycle-delete-confirm | 400px具体ID/数据/备份/明确同意；inert完整回收背景加scrim。 |
| `recycle-restore` | recycle-restore | 440px，原分组只读；接口不接分组，不造假选择器。恢复原身份，不走clone。 |
| `native-recycle-remove` | environment-delete-confirm | 400px只读影响确认；截图未提交移除副作用。 |
| `native-recycle-protected` | batch-progress-fixture | 400px保护失败/原任务恢复容器，目录保护不伪装删除成功。 |

## 如何读差异

- **外框对齐**：drawer660/top40/bottom8/header40/footer71；外挂rail40；Cookie文本1050/文件520；批次明细1040/结果400；回收确认400/恢复440。详细x/y/高度见清单与交付记录。
- **支持内容裁剪**：新建默认内容1450px、技术展开2445px；原新建3649px。对应代理/指纹区按真实内容跳转，不假称与原778/1616的scrollTop相同，不补大块空白。
- **安全扩展**：具体ID、未知请求、备份影响、同意勾选和维护保护会增加确认/结果高度。400/440宽度保留；高度不宣称逐像素一致。
- **数据范围差异**：内核来自服务；native回收/批次数量来自fixture，不把原合成字典/12条数据硬塞入真实契约。
- **缺直接参考**：历史/保存失败/未知提交/执行pending/旧尝试/保护错误使用已冻结同类容器，不能计为“找到同名原屏且1:1完成”。
- **未接入状态**：400px未保存/force DOM确认没有本轮完成图，仍待MAIN保留原preview/session保护后挂载；#35最终代理导入往返、#37 shared全套检查和真实桌面均未由这些截图证明。

所有公开PNG的实际尺寸/DPR/scale/哈希已验证；pageErrors=0、非允许请求=0、native localStorage访问=0。浏览器图上的“本机桌面”只是native模式文案，**不把合成bridge升级为桌面验收**。
