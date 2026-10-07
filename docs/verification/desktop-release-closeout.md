# 桌面程序交付来源核对（2026-10-07）

用户已取消严格 1:1 要求。本次交付目标是桌面可用性收口；商业参考缺失不再阻塞这一目标。取消要求不等于历史 1:1 通过，原缺参考、FAIL、检查来源与正式 4/21 历史记录保留。当前可交付新的直接运行程序，但**桌面可用性未全部完成**：preview.8 的 A/150 普通启动及 B/148→150 迁移试用仍实际崩溃。本报告核对构建来源，并追加集成负责人已完成的实际桌面事实；完整流程见[总验收](desktop-closeout.md)。

## 可运行文件与版本

本机可直接运行的文件是 `build/bin/prism-browser.exe`。这是本轮新的 Windows amd64 开发预览，不是旧 preview.6 安装包。生成物留本机 ignored 目录，公开源码文档不链接到干净 checkout 中不存在的文件。

| 属性 | 本次核实 |
| --- | --- |
| 应用版本 | `0.3.0-preview.8`；来自根任务实际构建及该版本 exe 的实际桌面运行，二进制含该版本字符串 |
| channel | `development-preview` |
| source commit | `e349eeadaa92a83ce60d97ec2ef5b309fdf68d92` |
| source tree | `55581aa8953f7c8790e1925c1940bceeb1c8c518` |
| branch | `codex/issue32-37-ui-integration` |
| 基线保留 | `86889064d83bc098ad2f518be076fdffae45341a` 与 PR #31 精确 `e170099bc25aa15ecb9a8456d782394a2139ec20` 均实际核实为祖先 |
| 文件大小 | 21,003,776 bytes |
| SHA-256 | `d10c562bd732f0e3546440ebc0b331ec175bf0851f46c3887709d3a9125c8f4c` |
| PE architecture / version | 实读 `amd64` / `0.3.0.0`；PE 数字版本不编码 preview revision，不能用它区分 preview.6/.7/.8 |
| Authenticode | `NotSigned` |
| exe 最后写入时间 | 2026-10-07 04:12:39.8738662 UTC |
| Go / Wails | 二进制 build info：Go 1.27.1、Wails v2.16.0、windows/amd64、production、trimpath、CGO_ENABLED=0 |
| 本轮安装器 | 未生成 preview.8 安装器；本报告没有执行安装、升级、卸载或干净 Windows 验收 |
| 外部运行条件 | WebView2 Runtime >= 94.0.992.31；壳与内核分开，程序不捆绑 WebView2 或 fingerprint-chromium |

现有 `build/releases/v1-candidate/prism-browser-0.3.0-preview.6-release.json` 绑定旧 source `67c98dbe` 与不同 exe 哈希，**不适用于本程序**。本轮 `build/bin/release.json` 和 preview.8 installer manifest 均不存在。已在 ignored `output/goal/desktop-closeout/` 生成 `release-artifact-manifest.json`、`release-SHA256SUMS.txt` 作为当前直接运行文件的检查记录，artifactKind 明确为 `direct-executable-build`，不是安装器或正式候选验收清单。

来源边界：Wails exe 的 `go version -m` 没有 embedded VCS revision；其 source 关联来自根任务在上述 commit 的本轮构建。maintenance helper 实际嵌入同一 revision，`vcs.modified=true`。检查时 tracked diff 为空，但有未跟踪的本轮独立报告；因此不声明 clean candidate，也不把最后文档提交重绑成构建/测试来源。

## 运行说明与数据路径

日常直接打开上述 exe。在没有 `PRISM_WORKSPACE_ROOT` override 时，程序从 Windows Known Folder 获取 LocalAppData，使用 `%LOCALAPPDATA%\PrismBrowser`：已有 `app.db`、环境浏览目录和 `workbench-webview` 都属于真实工作区。直接启动会读取并可能写入该工作区，不能把它作为空白测试夹具。已有桌面实例先正常关闭；程序的全局单实例锁也会阻止并行工作台。

测试使用单独的 ignored 启动器：

```powershell
# 在仓库目录执行；只启动新合成 workspace，不使用默认数据根。
& ".\output\goal\desktop-closeout\launch-preview8-synthetic.ps1"
```

启动器先核对本次 exe 的精确 SHA-256，只允许 `output/goal/desktop-closeout/launcher-workspaces/` 下的新 UUID 子目录或带自己 marker 的目录；拒绝越界、链接路径和未标记已有目录。它只给 child process 设置 `PRISM_WORKSPACE_ROOT`，不改变调用者环境。遇到已有 `prism-browser` 进程会拒绝启动，绝不自动停止其它实例。

合成测试窗口正常关闭后，如需保留同一测试数据重开，可把启动器输出的 `syntheticWorkspace` 作为 `-WorkspaceRoot` 参数传回；不换目录、不复制用户 profile。该启动器与根任务当前桌面 root 不共用，也不会操作它。这里的空白 workspace 不预装内核或代理。

已执行 PowerShell 语法解析和 `-InspectOnly`：通过、没有创建目录、没有启动进程、没有使用默认根。**启动器的实际启动分支尚未执行**，避免干扰根任务正在运行的新 exe；不能把静态检查算作桌面启动通过。

## 必要伴随文件与许可

`build/bin/` 当前含以下 6 个文件。直接工作台入口是 exe；maintenance helper 服务于安装维护流程，并非此直接启动路径的先决调用，但本轮构建已产出并核实。分发这份构建时应保留现有 LICENSE 与全部通知文件。

| 文件 | bytes | SHA-256 |
| --- | ---: | --- |
| `prism-browser.exe` | 21,003,776 | `d10c562bd732f0e3546440ebc0b331ec175bf0851f46c3887709d3a9125c8f4c` |
| `prism-maintenance.exe` | 5,444,608 | `fed7f5bc96621621650ab12796ed8ca0b28bc6a6a012c0779bb1ad3dd27344b6` |
| `LICENSE` | 1,083 | `c13aa27bd8d539b6c9d91867735e35a7f6b1326d3171e458de611647ebae102b` |
| `THIRD_PARTY_NOTICES.md` | 21,186 | `426eca134d7063250ca326b2e434b72635ed4c3c332e0192de6803efdd0138cd` |
| `GO-THIRD-PARTY-NOTICES.txt` | 138,759 | `a032108e70e0da66211d8d469c86198fa4b15292f172bd6bc03809a940595978` |
| `FRONTEND-THIRD-PARTY-NOTICES.txt` | 237,352 | `5783d9159b9672dd53f05bb8faaf7f72890c573aa69dd81373387e2ef8fa2a8a` |

两份源码 LICENSE / THIRD_PARTY_NOTICES 与 payload 的 SHA-256 实际一致。原 MIT 不覆盖所有第三方组件；依据[第三方声明](../../THIRD_PARTY_NOTICES.md)，生产 Go 与前端通知已随构建生成。未复制内核二进制到 payload；内核许可与精确安装记录另按[内核合同](../KERNEL.md)核对。

preview.8 没有运行 NSIS 构建，所以 NSIS-LICENSE、安装器 INSTALLATION/USER_GUIDE 副本与 release.json 不属于当前 direct exe payload；**不适用且有依据，不计安装包测试通过**。已有[安装说明](../INSTALLATION.md)与[使用指南](../USER_GUIDE.md)可查，但旧安装器验收不能套给本程序。

## Issue / PR 首次只读快照与范围变更

发布前首次只读核对 GitHub 的历史快照：[PR #38](https://github.com/axgiroud312-byte/prism-local-browser/pull/38) OPEN / DRAFT，remote head 为 `8688906`，base 为 `codex/issue28-30-delivery`；该旧 head 的 Browser clicks 为 CANCELLED，不能标 remote CI passed。#32–#37 均 OPEN。这个快照由子任务在最终发布前读取，不能用来描述后续推送后的 head 或 CI。

首次快照中的 issue 正文仍使用严格 1:1、Vite only / 不构建的旧范围；本轮用户明确取消 1:1，并另行授权必要 Go/Wails 构建和真实 Windows 核验，已在本地目标与验收文档记录。最终回执须追加本轮范围变更，保留旧材料和失败来源，不将旧要求静默勾成通过。

| issue | 原要求中仍适用的功能重点 | 本轮草案处理 |
| --- | --- | --- |
| [#32](https://github.com/axgiroud312-byte/prism-local-browser/issues/32) | 现有功能、固定身份、失败恢复、原依赖与不自动合并 | 记录“用户取消 1:1”；不再以商业参考缺失阻塞桌面交付；不宣称原 strict 1:1 成功 |
| [#33](https://github.com/axgiroud312-byte/prism-local-browser/issues/33) | 可达列表/分组/启停、准确 ID 范围与逐项反馈 | native Batch not-executed / demo skipped 按真实合同分层；真实 preview.8 部分创建、关闭重开历史和原plan新attempt续作通过；实际桌面取消未截获，不能算通过 |
| [#34](https://github.com/axgiroud312-byte/prism-local-browser/issues/34) | 创建编辑、seed/内核/ref、生成等待、取消/迟到、Cookie/批次/回收 | 修复指纹请求归属；真实单条 Create 仅 count=1，多项按 Batch；原 B/148 失败后及应用重开三类存储实际读回通过；A/150 普通启动重试仍失败，真实迟到回复未截获 |
| [#35](https://github.com/axgiroud312-byte/prism-local-browser/issues/35) | 代理失败不直连、精确内核、迁移身份与来源清理 | 原正式服务安装 148/150 通过；受控代理真实服务证据分开；来源标识丢失同会话仍阻塞，不猜 token |
| [#36](https://github.com/axgiroud312-byte/prism-local-browser/issues/36) | 备份/恢复、预检/取消/清理、记录/帮助/诊断 | 完整商业指南参考不足改记取消要求；服务恢复与原 scratch 所有权证据单列；不因取消参考删现有入口 |
| [#37](https://github.com/axgiroud312-byte/prism-local-browser/issues/37) | 跨页闭环、失败恢复、可达性、准确轻量检查与交付来源 | 新 build/hash 明确；服务、自动化、桌面分别报告；真实桌面和当前正式计数由根任务整合，不提前关票 |

发布草案保存于 ignored `output/goal/desktop-closeout/pr-body.md`、`issue-comments.md`。均使用 Refs，没有 Closes。当前桌面验收未全部完成，不自动转 ready、关闭或合并；草案仅陈述已有事实和明确限制，没有悬空填写项。本轮版本、源码和哈希字段保留本次实际 build 来源，即使以后另增文档提交也不变。

## 实际验证与剩余边界

本报告实际执行：6 文件哈希/字节/PE/signature核对、两份 build info、两份源码通知一致、现有清单与新产物适用性、精确 Git 祖先、GitHub 当前状态和条款、启动器 parse/`InspectOnly`、本报告相关链接。没有重构建、复制/重打包 payload、运行程序、停止进程、上传文件或写外部应用。

真实服务补充结果见[服务报告](desktop-native-service-closeout.md)：4 项真实 production/内核检查通过，原 `.appdata` 发布失败及应用前置失败保留；[发布环境调查](desktop-kernel-publication-environment.md)保留实际失败、官方 Kernel.Install 成功与 watcher 对照边界。这些不能代替新 Wails exe。

当前交付程序已更新为 preview.8。preview.7 的真实桌面检查取得多项通过，但 148→150 迁移试用中真实进程崩溃：Windows Event 1000、chrome.dll、exception `80000003`。原目标保留、trial 已退出、committed=false；没有 minidump/stack/symbol 证据，不能宣称已找到内核或 seed 兼容根因。

已完成的基础流程按实际程序来源分别记录，不把同一功能重复观察累加为正式计数：

| 程序来源 | 已有实际通过的流程 | 具体证据范围 |
| --- | --- | --- |
| preview.7 / `be5b1768…` | 普通编辑保持身份；运行中关键设备字段保护；退出/重开保持原引用 | [指纹记录](desktop-fingerprint-closeout.md)。实际 saved revision 增加，seed/kernel/fingerprint/ref 不变；运行字段的可操作性按真实 UIA，不把只读 seed 的 enabled 属性误写成控件禁用。 |
| preview.7 / `be5b1768…` | 运行中回收拒绝；关闭后回收；原任务历史；找回原身份 | [回收记录](desktop-recycle-migration-closeout.md)。真实 `Recycle.ReadPage` 为一页一项，不扩大为真实桌面 25+2 分页；恢复后浏览数据由原 B 浏览器读回。 |
| preview.8 / `e349eead…` | 三项 Batch 部分创建；关闭重开历史；明确继续只补失败项 | [批次记录](desktop-batch-closeout.md)。自然 NAME_CONFLICT，原计划/新 attempt，2/1/0→3/0/0，原两项和失败项预配身份保持；不证明浏览器启动或实际取消。 |
| preview.8 / `e349eead…` | 已知来源只读预检与丢弃；失败迁移原 B/148 重开读回；工作区持久化重开；正常退出和资源清理 | [回收/清理记录](desktop-recycle-migration-closeout.md)与本文下列最终证据。没有执行预检恢复，不能把丢弃通过写成恢复通过；来源始终丢失的同会话仍阻塞。 |
| preview.8 / `e349eead…` | 用户授权 Clash7897 的实际 HTTPS 出口读回 | 仅代理入口与 native 前检一致；controller/node chain 未核实，范围见下文。 |

preview.8 在同一原 B 环境实际复验 148→150 迁移：结果仍为 `failed/original-retained`，safe cause 为 `CONTROL_CHANNEL_LOST/control-read-ended`；backupVerified=true、trialExited=true、committed=false、protected=false。精确trial PID/创建时间绑定的Windows Event1000确认与preview.7相同崩溃签名。这证明安全分类已显示和原目标未提交，不是迁移成功或内核崩溃根因已解决。

同程序的 A/150 首次普通打开与后续普通重试（operation `bf383d55…`）均实际失败，`CONTROL_CHANNEL_LOST/control-read-ended`、退出码 `80000003`。首次准确 runtime session/进程创建时间与 Event1000 绑定；重试按实际 runtime/operation 记录判失败，没有另取第二份 Event1000，不能借首次事件推断重试 DLL 偏移。首次 resourcesExited=true、needsReconcile=false，原环境没有发布 running 成功。当前内存观察不是进程失败时采样，不能据此归因内存。A/150 普通启动和迁移目标 150 的可用性仍阻塞，不写成恢复通过。

原 B/148 在迁移失败后及工作台正常关闭重开后实际打开成功，保留原 ID/seed/内核/ref。最终合成浏览器读回（04:33:31.773 UTC）确认 Cookie `SYNTHETIC-B-IMPORT`、LocalStorage/IndexedDB `SYNTHETIC-B-OLD`、实际 CPU14。工作台重开同时读回 73 个合成环境、两个已核验精确内核和 Clash7897 revision2。这里只确认原 B 和工作区的保留/读回，没有把 A/150 启动失败改成通过。

preview.8 真实 GUI Batch 三项计划的首尝试准确为 2 completed / 1 NAME_CONFLICT / 0 notExecuted；改原占名环境名称、关闭重开历史后点击明确继续，新 attempt 沿同一冻结计划只补 1 项。已创建两项 ID/seed/ref、失败项预配身份和旧 2/1/0 报告全部保持。此部分实际通过，详见[批次报告](desktop-batch-closeout.md)。另 64 项实际取消尝试在按钮可捕获前已完成 64/0/0、cancel_requested=0；**未截获取消，不计取消通过，也不改记 N/A**。真实指纹迟到回复同样未截获，自动化代次隔离证据不能冒充实际桌面迟到运行。

用户已授权接入本机 Clash 7897 当前 rule 模式。preview.8 的绑定代理直接指向7897，保持原环境seed、dataRef和已保存修订；真实150浏览器通过该入口打开HTTPS IP观察页，PrintWindow实际显示的出口与native前检相同。**这一条实际代理入口/HTTPS读回路径通过**；Clash controller unavailable，actual node chain未核实，不能扩大为当前商业节点链、所有规则或网络故障保护全部通过。实际出口值与本机原始观测留ignored evidence，公开报告只记一致性。之前受控loopback结果保持原来源，不冒充该真实路径证据。

preview.8 工作台两次正常关闭（PID42736、38840）均实际 exit0；这不单独证明清理。另核对最终资源证据：自有 Chrome 0、`backups/preflight` scratch 0、staging 0；3 个 network session 全 closed，18 项资源全 released。集成负责人只停止自有控制 origin/proxy helper（PID76592/70536），原用户 Clash（PID14936）仍存在。来源分别是 ignored `preview8-first-exit.json`、`preview8-final-exit.json`、`preview8-final-network-resources.json` 和 `preview8-final-browser-reports.json`。已知来源、正常结束的本次资源清理实际通过；迁移 source token 始终丢失的同会话安全恢复入口仍不支持，不能以本次正常清理推导该能力存在。

preview.7 的 manifest、SHA256、build info、来源报告和发布草案已单独存于 ignored `output/goal/desktop-closeout/*preview7*`，不重绑为 preview.8 证据。新程序的完整真实桌面结论以总报告为准。**正式 4/21 保持，不宣称全部桌面完成。**
