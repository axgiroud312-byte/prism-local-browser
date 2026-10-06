[UI 合同](../UI_CONTRACT.md) · [窗口模块记录](issue34.md) · [实际源码截图](../screenshots/issue34-integration/README.md)

# #34 正式 App 挂载与安全确认接入

日期：2026-10-06。**正式源码已挂载，相关49项定向点击通过；不代表 #35/#36 最终接入、#37 全套终验或真实桌面验收。** 本记录补齐之前仅外部预览演示的 App seam，不把原72张图重新算成最终挂载证据。

后续：#35唯一parent导入、#36主页面已接入；本页保持 `12303a1` 的阶段事实，最新跨页/弹层/审查/共享检查和当前源码图见 [#37](issue37.md)。

## 已实现

- App 完整替换环境 drawer 内层为 `EnvironmentEditorWindow`，没有旧 drawer 包新 drawer；660px / header40 / 固定 footer71，唯一“换一套”在 footer，正文连续四区、独立滚动。参考 backdrop 不关闭。
- demo Cookie / 移除完整挂载新窗口；旧 body/footer、App `cookieFile` 与虚假的真实数据删除 checkbox/state 已删除。`saveDemoCookie` 沿用原 `mergeCookies` + `update` 回调，写失败先 return，不关闭、不发成功；解析有错误不能保存。移除仍验证准确 ID、运行状态和写入成功，日志只称删除示例记录/示例 Cookie，不操作真实文件。
- 未保存和 force 的浏览器原生 confirm 已换为 `EnvironmentConfirmation` 的400px DOM；业务状态、遮罩、ref、inert、焦点/键盘/滚动仍唯一由 App 管理。最高工作区阻断在确认上方，Escape 只取消警告，不能向下关闭表单。
- dirty 捕获 previewId、完整草稿 JSON、档案 hash 与数量，确认时重新核对同一当前草稿和 saving / creationOperationId / creationUnconfirmed。迟到档案或陈旧确认不丢任何预览；通过才丢原 previewId，保存身份/数据不变。数量单独变化也必须确认。cancel 保留输入；关闭后返回原新建按钮或行/创建菜单触发器。
- force 冻结 environmentId / sessionId，提交前重读 `application.getSnapshot()`，验证同一会话、canControl / canForce / needsReconcile 和工作区状态，沿用原 `beginRuntimeAction/endRuntimeAction` 与 `forceStopRuntime({environmentId,sessionId,requestId})`。cancel 无 force 调用；正常关闭失败不隐式升级为 force，旧会话不能控制新会话。
- 原 native Cookie / batch / recycle 的 props 和唯一 manager 生命周期保持；App 对它们不重复 trap。未知 Cookie 原请求、失败子集 merge、存储阻断盖住 native Cookie 的既有检查仍通过。代理导入仍保留原回调和同一表单上下文，未重造 importer。

## 实际检查

使用独立 Vite 端口进行以下**定向**检查，非 `npm run check`。配置和失败附件保留仓库外。

```text
npm run typecheck
npx playwright test environment-confirmation.spec.ts environment-windows.spec.ts environment-cookie-window.spec.ts environment.spec.ts native-boundary.spec.ts native-reference-shell.spec.ts modal-boundary.spec.ts --config <private-5207-config> --grep-invert "late cancellation of an old kernel|native kernel page waits"
npm run check:docs
git diff --check
```

- 源码定向 **49/49**，合入当前集成文档后再次实际运行 **1.2m**（合入前末次1.3m）；其中新增 `environment-confirmation.spec.ts` **15项**。收尾同步 #35 模块集成 `c6371eb` 为 `12303a1` 后，同范围再跑 **49/49，1.3m**，类型及61份文档本地链接/12需求/6路由/4内嵌文档通过（此前59文档为原范围）。既有3工作区/模态边界、精确跨页 ID/策略、分组失败子集以及原 ID 打开重试继续保留，不放宽断言。
- 新/编辑/换一套/取消、非法数量/重复名/网址、写失败重试、demo Cookie 写失败、普通 seed/内核/代理/空值保持、批次部分保存、代理取消/失败/成功返回、390×844创建与关闭均通过。
- 新守护测试：dirty cancel/Escape/Tab/焦点/同原 preview 丢弃；迟到档案拒绝陈旧确认；pending / accepted / unknown 创建不能关窗丢请求；未知创建重试同一 payload/requestId、已受理核实原 operationId；force取消0调用、确认仅一次准确三ID请求；旧session/失control/失force/需reconcile均0调用；普通stop失败不自动force；工作区fault高于force并能恢复确认焦点；demo精确移除运行保护/写失败及Cookie错误拒绝。
- RED→GREEN：初始两个新DOM确认缺失；之后合法迟到档案fixture修正；数量仅变化的 Escape 暴露旧回调闭包并用当前quantity ref修复；新测试菜单定位修正。未删除/跳过旧安全测试。`tdd` / `code-review` skill 不可用，使用实际行为回归和本次diff自审；#37独立审查仍待做。

## 视觉与剩余边界

16张**全新正式源截图**与仓库外16组PNG并排裁剪全部实际读取；同步 #35 后按 `12303a1` 重拍全部16张，12张SHA/几何与已读图一致，4张变化图及对应裁剪再次实际读取。两视口100%/DPR1，错误/非允许请求均0，逐张尺寸及SHA-256核对；详见[图表与差异](../screenshots/issue34-integration/README.md)。drawer精确容器已匹配，dirty163 / force217 / demo移除181px高度为公开安全适配。400px原参考来自 `group-delete-confirm`；未找到同名 dirty/force 执行参考，不记为同名直接1:1。保存失败同样仅容器映射。

#35 模块已合入但唯一代理导入最终替换、#36模块接入及 #37跨页/嵌套/最终视觉和共享一次全套检查仍由集成负责人收尾。当前demo代理往返已验证名称/分组/备注/数量/网址/内核/seed，但不代替 #35最终窗口验收；旧native嵌套导入frame须在MAIN接入时完整替换，不能与新self生命周期重复，49项不覆盖此新importer全链。未改变 ApplicationService、RPC、SQLite、后台、真实目录、精确构建或数据引用规则。

未运行生产 build/package、完整 npm check、Go/Wails、新桌面、安装/UIA、真实内核/网络/目录探针。合成native bridge不是Windows实跑；历史正式4/21、旧候选及历史未验项不改写。
