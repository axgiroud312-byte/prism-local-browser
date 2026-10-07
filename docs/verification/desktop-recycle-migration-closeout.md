# 桌面收口：回收历史与迁移来源清理

日期：2026-10-07。继续 `codex/issue32-37-ui-integration` 的当前工作区；不切换、重置、重做或合并 PR。用户取消严格 1:1 要求，本记录只核对实际服务契约和必要失败恢复；不扩展商业参考、截图或图库审计。

本文件先记录本轮专属服务与自动化证据，再追加本轮新 exe 的实际桌面结果，各层级分别标明。正式计数由[总验收](desktop-closeout.md)记录；服务测试、合成桥或受控故障注入不代替新 exe 的真实桌面运行。

## 判定

| 范围 | 判定 | 实际依据与边界 |
| --- | --- | --- |
| `Recycle.ReadPage` 历史按原 operationId、offset、pageSize 查询 | 实际通过（服务/自动化） | 新 Go 定向用例通过实际 SQLite 服务回收 27 个合成环境；从正确 RPC 查询 25+2 冻结条目，再找回其中 2 项、关闭服务/重开，旧历史与原顺序/结果保持。正确执行 ID 与页位置进入 Wails RPC，界面逐项结果显示失败和未执行且能翻页。 |
| 回收错身份、少项、重复项、错误 total 或混合列表/历史响应 | 实际通过（自动化） | adapter 拒绝接入；不发布错误历史或冒称成功。返回原回收列表后选择的是回收项 ID；移入回收的旧历史 ID 是原环境 ID，两种契约没有混用。 |
| 回收迟到历史回复与重开列表 | 实际通过（自动化） | 实际 App 在合成桥里关闭历史读取、重开列表；旧回复不能替换新的列表目标。控制迟到回复属于测试注入。 |
| `Operation.Read` 替代回收历史分页 | 不适用且有依据 | 真实历史明细接口是 `Recycle.ReadPage`。普通任务读取只返回 Operation；旧一般任务测试不具备分页和逐项身份的验收能力。新 UI 历史用例明确没有 `Operation.Read` 调用。旧未证记录保留。 |
| 已知来源的预检取消/清理 | 实际通过（服务/自动化） | 原 worker 尚未终结时，`DiscardRestore` 只取消并返回可重试 `PROFILE_BUSY`，不报告 discarded。终结后仅原 source token 可重试清理其准确私有暂存；占用失败保持来源，实际解除占用并确认目录消失后才成功。 |
| 暂存清理失败时另选新备份或新预检 | 实际通过（服务/自动化） | 本机选择器前和返回后、迁移升级前来源选择、恢复预检受理都在服务锁内拒绝未清理暂存。原 source→scratch 拥有链保留；无关来源不能解除原保护。原 token 也必须先通过原 `DiscardRestore` 完成清理，不能创建新预检来代替原清理。 |
| 迁移来源标识丢失：同会话安全恢复 | 仍阻塞（能力限制） | 当前服务没有按迁移编号找回 source token 的接口。原选择请求发出后标识始终没有返回时，保持阻断，不猜来源、不造 token、不另选来源、不把隐藏/离页/重开页面当清理成功。界面已明确说明“本次会话没有安全恢复入口”。 |
| 来源丢失后的完整程序退出/重开 | 实际通过（服务层受控检查）；桌面待总验收 | 正常关闭先取消并等待原 worker，再清理服务内部准确记录的暂存；实际占用失败返回错误并保留拥有链，解除占用后的同一关闭重试确认目录消失、清空会话来源，随后服务重开保留原环境身份。这里不靠猜 token，也不依据新来源释放旧资源。真实 exe 必须另核对旧进程/worker 终结和原暂存清理。 |

“不适用”没有算进测试通过；“仍阻塞”没有改成成功。缺来源时没有添加替代 RPC 或虚构新的契约分支。

## 必要修复

1. `recycle-model.ts` 校验真实分页完整长度、唯一条目、精确视图、operation 总数与请求目标，防止遗漏或接入无关结果。
2. `NativeRecycleManager.tsx` 在切换结果页时保持用户展开的逐项结果，避免每次翻页都隐藏结果正文。
3. `restore_preview.go` 让取消和清理确认分别反馈；保留原 token 直到 worker 退出，保存原 source 的准确失败暂存，按原拥有链重试。普通来源选择、迁移升级前来源选择和恢复预检都阻止新来源抹掉或接管原清理身份；删除旧的“新预检先清全局旧暂存”路径。
4. `Service.Close` 只在原 worker 全部终结后清理原准确暂存；清理错误保持可见、可按原关闭重试。结束后清空会话来源，正常服务重开不复用旧来源授权。
5. `NativeMigrationManager.tsx` 直接说明来源丢失时同会话无法安全继续的限制。

## 本轮执行检查

| 检查 | 结果 | 证据性质 |
| --- | --- | --- |
| Node：`--test-name-pattern='M22|migration rollback|Recycle.ReadPage|recycle page' tests/wails-adapter.test.ts` | 15/15 | adapter 及注入响应检查；不是真实 native 运行。 |
| Playwright：回收历史 3 项 + 迁移原来源清理/迟到/缺 token 定向 16 项 | 19/19 | 当前 App、合成 Wails bridge；独立 5184 端口，未停止占用 5183 的其他测试。没有常规截图扩展。 |
| Go：`TestRestoreDiscard*`、`TestRestoreShutdown*`、`TestRestoreReadPage*`、`TestRestorePreviewCleansOwnedScratch*`、`TestRecycleHistoryReadPage*` | 6/6 | 本机真实 SQLite、合成记录和隔离文件/Windows 占用检查；worker 终结测试使用可控服务状态。不是新 exe 的 GUI 操作证据。 |
| Go：既有等待关闭/关闭超时/已观测会话退出定向检查 | 3/3 | 确认新预检清理没有破坏原关闭等待和资源完成保护。 |
| 两份 TypeScript `--noEmit` 类型检查 | 通过 | 未生成前端构建产物。 |
| Git 空白检查 | 通过 | 无 commit/push；新 exe 和源码提交由整合者统一交付。 |

首轮新 UI 用例实际失败：历史翻页后逐项结果收起；已修并在新执行的 19/19 中验证。迟到回复用例首轮用旧“列表”标题定位关闭按钮，而活动历史已切换结果窗口；纠正为当前可访问关闭按钮后通过，不把该夹具定位错误宣称为产品修复。首轮 Go 历史用例将 remove 历史的原环境 ID 当成 restore 所需的回收项 ID，服务准确拒绝；改由当前回收列表取 trash ID 后按真实契约验证。首轮失败事实保留于本记录与本轮会话输出；重新执行生成当前检查结果，历史旧证据没有改写成通过。

## 追加桌面证据前的待验记录（历史保留）

新 exe 上回收→找回后的原 ID、seed、精确内核、真实 Cookie/目录读回，迁移原身份/独立副本及失败保护由总验收完成。Go 测试、可控迟到/故障注入和 Vite 合成 UI 都没有计作这些桌面结果。

`main.OnShutdown` 仍忽略 `Service.Close` 返回错误；进程 exit 0 单独不能证明预检暂存清理完成。本轮实际桌面验证需同时确认旧工作台进程已终结、原 worker 不再运行、原私有预检目录已清理。若清理失败或没有可靠确认，保留阻塞，不建议以重开继续绕过。

没有既存 startup 扫描/接管任意旧预检暂存的恢复功能；本轮也没有新增这种扫描或认领。来源始终没有返回且不能可靠确认旧程序/资源结束时，仍属于明确使用限制。

## 本轮真实新桌面程序补充结果

以下操作由整合者在本轮新 Windows/Wails 程序上执行；本子任务只读核对已有证据，不创建新测试、构建或进程。本次来源分别为 `0.3.0-preview.7` / `be5b1768c96ffc6f448a695f6e430dc8c828402b` 与 `0.3.0-preview.8` / `e349eeadaa92a83ce60d97ec2ef5b309fdf68d92`。`output/goal/desktop-closeout/release-artifact-manifest-preview7.json` 与 `release-artifact-manifest.json` 记录版本和源码来源；不能以此前的 Vite、旧 exe 或自动化 bridge 代替。

| 范围 | 判定 | 实际操作、结果和证据边界 |
| --- | --- | --- |
| 运行中环境移入回收保护 | 实际通过（preview7 桌面） | 整合者对运行中的合成 B 操作移入回收，服务返回 `PROFILE_BUSY`，没有执行回收写入；正常关闭 B 后才执行下行操作。该失败受理事实按整合者本轮实际操作记录，不能把失败反馈记作回收成功。 |
| 移入回收后查原历史逐项结果 | 实际通过（preview7 桌面） | B 正常关闭后回收任务 `2f3627b8-9b11-4714-a60f-bac2d741ed0d` 完成，仅 B 一项，completed=1、failed=0、notExecuted=0。界面选择最近回收任务，经实际 `Recycle.ReadPage` 显示原冻结任务和 B 原环境 ID、“已回收”、共 1 项/本页 1 项。`recycle-history-native-ui.json` 保存真实 UI Automation 文本，`after-recycle-db.json` 保存原任务结果。没有用 `Operation.Read` 结果代替历史明细。真实桌面只有这一页；25+2 分页仍是前述服务/自动化证据。 |
| 找回环境保留原身份与真实浏览数据 | 实际通过（preview7 桌面及后续真实读回） | 找回任务 `2fd12d89-3108-40d9-a224-29b06bc950e3` 实际 completed。B 保留 ID `5fd3dd7c-6191-4238-9a95-a1ed4afce5f8`、seed `1562739753`、精确 `148.0.7778.215`、内核 ID `24620122-863e-441f-8628-c7577a05d412` 和原 `environments/5fd3dd7c-6191-4238-9a95-a1ed4afce5f8/user-data` 引用；普通恢复增加环境 revision，没有换身份。`after-recycle-restore-db.json` 核对原引用，真实 B 浏览器随后读回 Cookie `prism=SYNTHETIC-B-IMPORT`、LocalStorage/IndexedDB `SYNTHETIC-B-OLD`。`preview8-reopened-browser-reports.json` 保留 03:55 UTC 的恢复后读回，以及迁移失败后的 04:20 UTC 再读回。 |
| 正确来源的只读预检、明确丢弃、正常退出和暂存清理 | 实际通过（preview8 桌面，已知来源） | 实际选择原 `desktop-preview7.prismbackup`，预检显示“包校验通过 · 尚未恢复”，新增 0、覆盖 2、冲突 0、缺失精确内核 0；实际尚未执行恢复。点击“丢弃此预览”，确认返回后正常退出。`preview8-preflight-ui.json` 保存原包 SHA 和预检文本，`preview8-preflight-canceled-db.json` 保存丢弃后状态。`preview8-first-exit.json` 同时确认原程序正常关闭 exit 0、本次拥有的 Chrome 为 0、`backups/preflight` 暂存子项为 0、staging 为 0；这次清理有目录证据，不只依据 exit 0。 |
| 只读预检前后全部已保存环境未受污染 | 全部 73 项身份最终比对仍待总验收确认 | 已读的丢弃后、关闭前和退出后数据库快照均有 73 条环境、完整性检查 ok、外键检查无错误；B 的原 seed/内核/dataRef 已单独核对。其余环境及全部 73 项身份的最终完整比对由整合者统一确认，本记录不以数量相同冒称全部身份相同。 |
| 迁移失败的原数据保护与安全原因持久记录 | 实际通过（preview8 桌面失败恢复） | 迁移 `3b36aa8c-93eb-40fa-bf6c-c96806c5daa1` 实际 failed/original-retained：完整备份已 verified、试用已退出、未 committed、无遗留 protected/persistence pending。外层仍为 `MIGRATION_INCOMPLETE` 和原完整状态已保留，安全底层 `CONTROL_CHANNEL_LOST/control-read-ended` 已真实保存。B 的原 ID/seed/148/dataRef 保持；真实 148 重开读回原三存储。`preview8-before-close-db.json`、`preview8-first-exit-db.json` 和 `preview8-final-browser-reports.json` 保留持久结果与 04:33 UTC 读回。失败保护通过不等于迁移成功。 |
| 150 试用、明确提交及桌面兼容性 | 仍阻塞（真实内核失败） | preview7 和 preview8 的 150 均真实创建试用进程，随后发生绑定本次准确进程/创建时间的 `chrome.dll` / `80000003` 崩溃，新构建 After 未返回。`migration150-crash-owned.json`、`migration8-crash-owned.json` 保存精确归属与崩溃分类。尚无堆栈/assert 等证据定位供应方根因；不以源码猜测、短目录对照或受控服务 PASS 宣称此桌面失败已修复。 |
| 原来源 token 丢失的同会话恢复 | 仍阻塞（能力限制，未做真实 GUI 丢 token 验收） | 当前无安全的同会话来源找回入口。已知 token 的预检丢弃及完整正常退出不构成丢 token 后的真实 GUI 恢复证据；此前 active/迟到/丢 token 防护仍只按服务/自动化层记录。继续保持原请求/原资源保护，不猜 token、不造 token、不静默释放。 |

本节全部原始文件位于 ignored `output/goal/desktop-closeout/`，使用合成环境及测试 Cookie；不扩展截图或图库审计。普通回收/找回和已知来源丢弃的真实通过可纳入总验收相应实际项，150 桌面迁移与丢来源的限制继续保留。

两个旧数据库快照 `after-recycle-db.json`、`after-recycle-restore-db.json` 的 `trash` 子查询实际记录 `no such table: trash`：这是检查工具用了错误表名，旧失败来源保留，不能把错误对象算作“一条回收项”或宣称该子查询通过。上面的回收历史判断使用正确 RPC 的真实 UI 与原任务结果，找回判断使用恢复任务、原身份引用和真实浏览器读回；后续 preview8 快照已读取实际回收表。本节没有将失败的旧查询改写成通过。
