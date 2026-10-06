[视觉参考](UI_REFERENCE.md) · [工程规范](ENGINEERING.md) · [#33 结果](verification/issue33.md)

# #33 共用 UI 与后续接入约定

## 责任和修改边界

- #33 交付 shared shell/navigation、密集环境表与分组 UI，修改 `src/App.tsx`、`src/styles.css`、`src/components/native-proxy.css`，增加下列独立模块。
- **#33 合入后，集成负责人唯一修改 App、全局样式、应用入口、公共 UI 和 native-proxy.css。** 后续票在自己的页面/窗口组件和 ticket-local CSS 工作；提供 props/挂载位置说明，由集成负责人接入，不并发改 shared 文件。
- #34：`EnvironmentForm`、档案修订、Cookie、批次、回收和环境确认窗口。
- #35：代理/内核/迁移、demo 代理/内核页，**唯一代理导入组件**。环境窗口复用它。
- #36：备份/预检/恢复/诊断、活动/指南页及文档；`NativeRestoreExecution` 仍由 #36 维护，迁移页复用现有 props。
- #37：集成负责人完成跨页差异修复、独立审查和一次共享全套点击；本票仅执行定向检查。

## 公共组件（已提供，业务状态不移入框架）

### `src/components/ReferenceUi.tsx`

| 导出 | Props / 行为 | 调用方责任 |
| --- | --- | --- |
| `ReferenceButton` | 标准 `ButtonHTMLAttributes`；默认 `type="button"`，`className` 合并到 `.button` | disabled/操作/危险性；需要 submit 时显式传 type |
| `ReferencePopover` | `anchor: HTMLElement`、`onClose`、`children`、必填 `label`；可选 `width=176`、`align=start/end`、`role=menu/dialog`、`className` | 控制显隐，动作后关闭；被阻断/目标离开时卸载；dialog 角色仍为非模态，不能当模态框使用 |
| `ReferenceModalFrame` | `title`、`titleId`、`onClose`、`children`；可选 busy/footer/width=500/height/variant=dialog或drawer/className | **只提供视觉 frame**。调用方必须提供遮罩、inert、Escape/Tab/focus trap、body lock、返回焦点、未保存草稿保护及提交生命周期 |

Popover portal 到 `document.body`，不受下一行/表体 overflow 遮挡；按 anchor 定位并限制在视口内。负责外部 pointer 关闭、Escape 关闭后返回 anchor、menu 的上下/Home/End 键、外部滚动和 resize 重定位。非模态浮层不把背景设 inert、不添加第二个 Tab trap。模态框不得重复绑定 document Escape/Tab；嵌套窗口的 body lock/focus 由主生命周期协调。

`reference-ui.css` 提供 40px 标题、32px 下划线控件、可滚正文、固定 footer，drawer 为 top40/bottom8/主宽660 的调用约定。公共 token 为 `--blue/--ink/--muted/--border`；`.button.primary/.danger/.compact` 是已有兼容样式。后续特殊样式在自己的 CSS 内用页面前缀隔离。

### 列表/分组模块

- `EnvironmentFilters.tsx`：受控 `search/group/status/groups/total/running/errors/selected/pageSelected/native/blocked/searchRef`；回调为 `onSearch/onGroup/onStatus/onClear/onCreate/onOpen/onStop/onAssign/onRemove/onCancelSelection/onRefresh/onBackup/onHistory/onRecycle/onClone/onProxyAssign/onRetryReleased`。只管理浮层草稿/显示，不持有业务选择 ID、不调用 adapter。
- `EnvironmentGroups.tsx`：`groups: string[]`、`environments: Environment[]`、`native`、`onFilter(group)`、`onAssign(ids, group)`。分组是派生标签；native 明确显示“当前页”，不宣称整组改名/全量计数。给回调的 ID 为当时已读成员的副本。
- `EnvironmentRuntimeDetails.tsx`：`environment`、可选 `session: RuntimeSession`、`blocked`；只呈现错误/下一步/退出码/待核对/待保存/网络详情，不创造 runtime 状态。正确动作仍在 App 对应行。

环境 table 暂留 App，复用的冻结外观为 `.environment-table`（列 30/55/70/90/140/126/114/120、header40、row48、gap10）、两条 toolbar、独立 tbody 滚动和外围分页。后续若提取 table，由集成负责人搬移这一份，不重建第二份。

## 不变的服务和窗口接入

- `src/main.tsx` 只创建一个 ApplicationService。页面接收既有 `application/workspace`，不各自 new adapter，不直调 `window.go`/文件/进程/CDP。桌面 bridge 坏也保持 native 阻断，不落入 DemoAdapter。
- App 继续持有 route、精确所选 ID、drawer draft、previewId/requestId/expectedRevision、profile 预览、未知创建核实、创建后原 ID 打开重试、顶层焦点与跨页状态。关闭窗口不等于取消已受理任务。
- native 列表通过 `queryEnvironments({ page, pageSize: 10, search, group, status })` 读取；不是把当前页当全量缓存。选中跨页 ID 后，启动读取其准确 edit preview/修订和保存网络策略；读不到不猜 direct。分组只改准确 ID，逐项失败只重试该项。
- native manager 既有 props 保持兼容：`application/workspace`，按组件增加 `environment/selectedIds/input/onClose`。`NativeRestoreExecution` 保留 `application/workspace/preview/onConsumed(previewId)/onLockChange(locked)`。
- Cookie 明确环境/修订/网络策略，健康 resourcesPending 不误作故障；未知写入不可再次清空。备份/迁移/回收原请求与终态留在 adapter，不因换 UI 丢失。
- busy 不只看 running；PID/resourcesPending/needsReconcile/persistencePending/networkResources/maintenance 仍保护关键字段。force 只作用明确受控原会话并确认；FIFO 取消不变成结束所有进程。

## 交付给集成负责人的页面模块

每个后续票提供：导出/props、要插入的 route 或 overlay、由 App 传入的状态/回调、ticket-local CSS、已完成/缺参考的状态、两视口合成证据与定向检查。不调整契约、SQLite、RPC、备份格式或旧候选。通用 frame 可复用，但不把框架当保存/未知提交/取消业务控制器。

新增或修改窗口保留可检查标签：环境名称/分组/浏览器内核/绑定代理/固定指纹种子/创建数量/启动网址；创建/创建并打开/保存/换一套；行 `{name} 更多操作`；区域 `逐项操作结果`；工作区 `工作区需要处理`。可更新结构定位以适配真实参考，不放宽精确 ID、seed、代理不直连、原请求核实、失败恢复断言。
