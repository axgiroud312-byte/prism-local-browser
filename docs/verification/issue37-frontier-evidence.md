# #32–#37：字节校验、迁移清理与原有限缺口补证

2026-10-07 · [图库总入口](../screenshots/issue37-continuation/index.html) · [源码与检查](issue37-frontier-code.json) · [独立动作](issue37-frontier-actions.json) · [保存合同索引](issue37-frontier-contracts.json) · [有限缺口续层](issue37-frontier-finite.json) · [公开来源批准身份](issue37-frontier-publication-approval.json)

**本轮交付可操作开发页、两项修复及独立增量证据；不是完整产品 1:1 或真实桌面验收。** 六票仍 OPEN，PR #38 仍为草稿；不自动合并 #31、#27、#38。正式验收仍 **4/21**，旧桌面二进制不含新 UI。

## 实际行为变化

- **代理 P07：** SOCKS5 替换认证和解码后的导入行共用 1–255 UTF-8 字节校验。超限拒绝但保留草稿，修正后只保存原选中行；认证内容不回显。HTTP/HTTPS、keep/clear、原未知请求核实及代理失败不直连回退规则不变。
- **迁移 M22：** 原来源与原预检的清理结果未确认时保留保护；隐藏、重开和重试仍使用原对，不新建预检。只有有效 discarded 回执释放。丢失来源回复且没有可用 token 时仍阻断，不猜来源或构造空 token。

两候选普通合入 `4eadba4`、`398314c`，保留 PR #31 的精确 `e170099` 基线。十个责任路径、候选 blobs、作者检查和 MAIN 类型检查分别绑定，见[源码记录](issue37-frontier-code.json)。

## 实际图片与记录

| 新包 | 图片 | 独立视觉结论 | 取证来源 |
| --- | ---: | --- | --- |
| [环境/批次底部](../screenshots/issue37-frontier-bottoms/index.html) | 4 | 2 ADAPTED / 2 MAPPED | b424b21，完整79输入 |
| [demo 预览失败与重试](../screenshots/issue37-frontier-demo-preview/index.html) | 2 | 2 ADAPTED | b424b21，完整79输入 |
| [代理字节边界](../screenshots/issue37-frontier-proxy-bytes/index.html) | 4 | 4 MAPPED | 398314c，完整81输入 |
| [迁移未知清理与等待](../screenshots/issue37-frontier-migration-cleanup/index.html) | 4 | 4 MAPPED | 398314c，完整81输入 |
| [创建后移交批次的部分结果](../screenshots/issue37-frontier-partial-create/index.html) | 2 | 2 ADAPTED | 398314c，完整81输入 |
| [指南首次 lazy 回退](../screenshots/issue37-frontier-cold-help/index.html) | 4 | 4 ADAPTED | b424b21，完整79输入 |

共 **20 张实际图片：10 ADAPTED / 10 MAPPED**。ADAPTED 表示本机功能或安全适配，MAPPED 只表示参考容器；都不是严格像素通过。同业务直接参考缺失及 `MISSING_DIRECT_HELP` 保留，不能把新包 0 FAIL 写成全产品通过。

另有 **4 条独立动作**、**16 条同图恢复链**（12 条普通回调及 4 条指南原请求恢复）。七份独立保存合同报告均为限定一致、0 P1/P2；这些记录不是额外图片或真实原生操作。新包来源、PNG、原记录签名、独立评审及安全投影分别绑定，不把 b424/79 重标为 398/81。

## 原47个有限缺口：实际派生44/47

继承原35项和 F13/F15 续层2项，本轮追加 F07、F32、B08、H19、P07、M22、F26 的7个独立决定，运行汇总得 **44已补证 / 3有剩余**。这是原47条有限语义定义的分母，不是47项完整功能验收。原定义、历史字段、37/47旧层及原274项113/60/101不回写；**whole-cell PASS新增0**。

剩余：

- **E18：** native 没有可观察的 skipped 分支，不能伪造。
- **F11：** demo 同步生成不能合法持有 busy/late；原 native 证据不能替代 demo。
- **R09：** 回收历史查询由 `Recycle.ReadPage` 所有，不能借 `Operation.Read` 异主结果补证。

F26 仅证明合成 native-reference 的 quantity2 **Create→Batch 移交与保留**；不认证单条 `Environment.Create` 多项部分完成、`completedIds` 分支、真实目录或落盘。M22 未知清理仍先保持未确认，不由有限补证认证原生清理安全。完整同版本原指南仍缺；六张设置/Local API候选和四本机文档不能替代。

## 检查与保留的失败

- MAIN `398314c` 两项 `tsc --noEmit` 实际成功。P07 候选21 UI；M22 原候选92 adapter/91 UI、固定候选14 adapter/18 UI分别归属，不重标为 MAIN 全套。
- 唯一共享 `npm run check` 仍为 `681f823` 的 **196/196**；本轮没有重跑。
- P07 首次 raw-JS TAB 解码拒绝为0准入，保留；新等价解码只处理合法 TAB，并核对原字节，不执行获取的代码或改缓存。
- M22 两轮各4对失败、0 PNG原样保留。私有诊断定位为取证工具把不存在的 `process` 存在性读取误拒；新工具先用描述符证明不存在，再封成不可改写的 undefined，其余原生阻断、预算和错误断言不变。新尝试实际4图/4回调、0FAIL，随后分别完成视觉、合同及有限审查。
- 工具预检发现用新 fork 修复并重审；旧工具、报告和失败不覆盖，没有把门禁发现写成已发生的产品故障或泄露。

没有运行生产构建、打包、安装卸载、Wails/UIA、真实内核、网络或目录恢复探针。商业归档只读，原代码/素材/原图、凭据、Cookie、profile、私人路径和原诊断不公开。发布依据独立最终数据批准，源工具批准不替代实际数据。
