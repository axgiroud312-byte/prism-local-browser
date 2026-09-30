# Goal 当前执行位置

更新时间：2026-09-30 18:58 Asia/Shanghai。

- Goal：执行中；[规则](GOAL.md)；完成 **2/21**（验收任务计数）。
- 当前任务：T03 / [Issue #4](https://github.com/axgiroud312-byte/prism-local-browser/issues/4)，进行中。
- 当前步骤：54fe655的干净runner再次发现已保存快捷方式TargetPath为空；此前目录规范化没解决。已改为发布真实目标程序后由Go维护helper通过Windows COM创建链接，回读目标和永久工作目录后才集成注册；新鲜路径/Unicode/升级/占用回滚的独立PowerShell读回测试通过，相关desktopbase/vet通过（总24 Go）。只重建两个包并实际安装验收，随后推送必要修复。
- 现场：`goal/t03-windows-installer` / `54fe655`；未提交为当前票快捷方式产品修复、回归和记录，所有改动均本Goal。#4尚OPEN，正式仍2/21。

## 任务状态

| 任务 | Issue | 状态 | 交付提交 / 验证 |
| --- | --- | --- | --- |
| T01 | #2 | 已完成 | 代码 `3b9b0fa`、记录 `afaeecb`、合入 `6cd681b`；[记录](verification/T01.md)；[PR #23](https://github.com/axgiroud312-byte/prism-local-browser/pull/23) |
| T02 | #3 | 已完成 | 代码 `134dc66`、记录 `fc4aa47`、合入 `44517c5`；[记录](verification/T02.md)；[PR #24](https://github.com/axgiroud312-byte/prism-local-browser/pull/24)，最终 CI 全通过 |
| T03 | #4 | 进行中 | [验收清单](verification/T03.md)、[PR #25](https://github.com/axgiroud312-byte/prism-local-browser/pull/25)，本机闭环通过，干净runner快捷方式问题修复中 |
| T04 | #5 | 待开始 | — |
| T05 | #6 | 待开始 | — |
| T06 | #7 | 待开始 | — |
| T07 | #8 | 待开始 | — |
| T08 | #9 | 待开始 | — |
| T09 | #10 | 待开始 | — |
| T10 | #11 | 待开始 | — |
| T11 | #12 | 待开始 | — |
| T12 | #13 | 待开始 | — |
| T13 | #14 | 待开始 | — |
| T14 | #15 | 待开始 | — |
| T15 | #16 | 待开始 | — |
| T16 | #17 | 待开始 | — |
| T17 | #18 | 待开始 | — |
| T18 | #19 | 待开始 | — |
| T19 | #20 | 待开始 | — |
| T20 | #21 | 待开始 | — |
| T21 | #22 | 待开始 | — |

## 最近检查与当前工作

- 2026-09-30 13:48–13:52，版本 `5d19fe2`：`git status`、remote、log、fetch、HEAD 对比通过；无需回退。GitHub auth 有效；Issue #1/#2 全文、状态、T01 依赖已读取；main 无分支保护。
- Node 24.14.0、npm 11.9.0、Git、gh 可用；Go、Wails、NSIS 当前不在 PATH，留到对应票处理，不阻塞 T01。
- 2026-09-30 13:53，`5d19fe2`：`npm run check` 基线通过（23 项领域测试、构建、12 份文档）。已有非阻断警告为 Lucide `use client` 与 500 kB 主包提示；没有新功能失败。
- 2026-09-30 14:11，`5d19fe2 + T01 工作树`：14 项新增契约测试及 `npx tsc -b` 通过。TDD 首轮缺模块失败和取消对象被 freeze 的失败均已修复；未删除测试。
- 2026-09-30 14:18：首次 `typecheck` 发现测试 helper 的 assert narrowing；首次 UI 6/8 通过，2 项失败为测试按钮名不符和存储 mock 恢复删除了原方法。已修正具体夹具，需复跑；并未放宽验收。只读子代理评审进行中（不写入）。
- 2026-09-30 14:24：`npm run check` 全通过：37 项（23 领域 + 14 契约）、源码/测试类型检查、构建、24 份文档、8 条真实 Playwright UI；原型无新控制台错误。版本 `5d19fe2 + T01 工作树`；此前 UI/类型失败已关闭。后续加 main 的惰性 storage 包装以处理浏览器禁用存储的 getter 异常，需针对该变动确认。
- 2026-09-30 14:29 只读评审完成：创建途中恢复可破坏引用、storage getter 权限异常、revision 元数据类型异常、旧页写失败仍成功提示。已补工作区活跃任务保护/每批引用与提交验证、惰性存储端口、元数据对象校验、旧页提交结果检查与活动同次保存；新增失败回归，尚待复跑。额外保持批量名称前缀兼容和非空 Cookie 验证。
- 2026-09-30 14:36：`npm run check` 通过（40 测试、10 UI、源码/测试类型、构建、文档）。只读复核确认 4 阻断关闭；追加停止/创建竞态保护与契约回归，需最终检查。未进入 T02。
- 2026-09-30 14:42：最终 `npm run check` 通过（41 测试、10 UI、源码/测试类型、构建、24 文档），`git diff --check` 通过。代码版本 `5d19fe2 + T01 工作树`；评审 4 原问题和1新增竞态均已回归，无未解决功能失败。公开截图与 ACCEPTANCE 已补。
- 2026-09-30 14:46：本票代码/文档提交 `3b9b0fa`，已推送任务分支并创建 PR #23。本地最终检查覆盖该代码；远程 Node 22/24 正在检查。
- 2026-09-30 14:48：PR #23 的 [远程检查 run 36679953263](https://github.com/axgiroud312-byte/prism-local-browser/actions/runs/36679953263) 全通过，Windows Node 22.12.0 / 24 均执行 41 测试、类型、构建、文档和10条UI。当前仅补文档检查点，代码仍为 `3b9b0fa`。
- 2026-09-30 14:50–14:53：最终文档提交 `afaeecb` 的 PR [run 36680197805](https://github.com/axgiroud312-byte/prism-local-browser/actions/runs/36680197805) Node 22/24 全通过；PR #23 合入 `6cd681b`，#2 CLOSED 并同步四项验收勾选/完成评论，前置成果已重新核对。
- T02 工具现场：Go 1.27.1 官方 ZIP 已下载、实际 SHA-256 与官方值一致，解包 `.tools/go/`，`go version` 通过；Wails CLI v2.16.0 安装、doctor 通过。WebView2 154.0.4258.37 已存在；modernc SQLite v1.60.1 与锁文件已下载。工具环境仅项目脚本，不改系统 PATH。
- 2026-09-30 15:10 恢复检查：Git、当前 #3/依赖、Go 版本一致。之前中断未执行空 patch，没有文件被额外改动；SQLite 初稿与 wails.json/scripts 存在但尚未验证，不能统计完成。
- 2026-09-30 15:20–15:50：SQLite 初轮10条服务契约通过，Windows production exe 已成功构建（未冒充真实重开）；补连接重建仍启用外键、版本/坏配置保护、原生预览种子白名单、SQLite query_only 写失败重试后13条 Go 测试及 vet 通过。49 JS 测试通过；首次测试类型检查发现不存在的 compatibility 属性访问，已改为公开存在性检查，最终完整回归运行中。
- 原生构建用 Vite desktop 模式，桥接损坏也不能回退 demo；本机界面明确未就绪、暂不支持批量/真实启动/代理/Cookie/备份。只读评审已完成，无确认阻断；主代理为唯一写入者。
- 2026-09-30 15:50–16:13：首条原型 UI 冷启动超时已分析，保留时限/断言，单条及11条完整 UI 重跑均通过。go-webview2 主动屏蔽外部调试参数，改用 Windows UI Automation，不削弱产品设置；控件类型匹配和许可脚本 UTF-8 问题已修复。`npm run verify:desktop` **真实 production exe** 创建/编辑→正常关闭→重开→SQLite/UI核对→窄窗口→正常关闭通过，实际 id/seed/revision/全部偏好一致。无真实内核运行声明。
- 2026-09-30 16:20–16:27：最终 `npm run check` 通过（49 JS、类型、生产原型构建、25 文档、11 UI），13 Go 契约/vet 通过，最新 Windows production 构建及26模块许可通过；对新 exe 再执行真实 UIA/SQLite 关闭重开验证通过。公开合成 JSON/截图已核查，实机证据无私人路径；本 Goal 启动的测试桌面均已正常退出。`git diff --check`、gofmt 检查通过，尚未推送/远程验收。
- 2026-09-30 16:29–16:34：代码 `134dc66` / [run 36689998979](https://github.com/axgiroud312-byte/prism-local-browser/actions/runs/36689998979) 的 Windows Node 22.12.0、24 和 Go/Wails 构建三项全通过，已核对 headSha、实际步骤与结果。另以原型格式合成文本充当损坏 app.db 开真实 exe：显示 native 安全阻断，无演示重置/示例回退，正常关闭后原文未变；证据已脱敏。
- 第二轮只读复核完成：独立核对当前 production exe SHA-256、合成 SQLite 全字段与截图；26 个实际 Go 依赖许可和子组件声明齐全；UIA 只操作自身 PID、不注入调试参数，CI 没有以构建冒充实机。无确认阻断，不重复其未执行的测试声明。
- 2026-09-30 16:44–16:48：T02 最终 `fc4aa47` / [run 36691104701](https://github.com/axgiroud312-byte/prism-local-browser/actions/runs/36691104701) 三项 Windows Node 22/24、Go/Wails检查全通过；PR #24 合入 `44517c5`、#3 四项验收已勾选并 CLOSED。T03 #4 OPEN，无承担者，blocking #3 CLOSED 且成果可用，正式开始。
- T03 现场：Windows 11 Home x64、当前非管理员，无 Windows Sandbox，NSIS/Inno 不在 PATH。不操作未知既有 Windows 账号；先完成项目内便携编译工具和当前用户安装验收，干净 Windows 用户验收不能用改环境变量冒充，必要时交付可运行验收脚本并请求用户提供一次隔离用户环境。
- 2026-09-30 16:56–17:12：服务重启后核对现场，无重复合入/测试副作用。官方 NSIS 3.13（2026-09-27）ZIP 下载、SHA-256 `ba63dffc4410ee89193e1cb5a41989991bd77c61068da17e3156d136b7b0b3d8` 与发行元数据一致、makensis v3.13 实际通过。TDD 首轮缺模块失败符合预期；新增运行/安装互斥、真实 junction 拒绝/外部文件保持、幂等删除及版本前检共4 Go测试通过，已有13条/vet仍通过。
- 初版安装按用户固定程序根、同卷新版本目录切换；数据仍使用 Windows KnownFolder 默认根，测试环境覆盖不成为卸载删除输入。helper 不接受任意删除路径，链接/重解析点拒绝；程序启动和安装共享独占锁，旧T02运行也做保护。默认卸载保留数据，勾选删除需再次确认，静默删除仅显式 CONFIRMED。
- 干净用户环境备选：本机非管理员/Home无Sandbox；将尝试 GitHub Actions 的全新 Windows runner 用户目录实际安装/UIA闭环，保留真实 Windows 11 本机验证。若 runner 缺交互桌面会准确报告，不以改环境变量冒充新用户。
- 2026-09-30 17:12–17:22：NSIS 首轮报 UTF-8 无 BOM 输入编码错误，固定 `/INPUTCHARSET UTF8` 后直接编译初稿安装包成功（尚未运行）；Windows helper/26模块许可通过。已写默认KnownFolder的真实安装→UI创建编辑/重开→升级→拒绝降级→保留卸载→重装→显式删除测试，不允许环境重定向冒充用户隔离。
- 安装前默认根检查发现仅 `app.db`（77824字节，15:27创建，与T02首轮构建对应），只读SQLite核对为空环境/代理。Wails生成绑定时运行 main，原T02在 wails.Run 前开数据库造成这个副作用；先隔离绑定生成的初始化并保持原文件，不把它当私人数据删除。未知现有账号未动；当前没有运行的本产品进程。
- 2026-09-30 17:22–17:48：只读评审完成4项：卸载向导后换junction越界、启动未检内部链接、同版本缺exe仍成功、入口/注册失败忽略。主代理改为NSIS仅私有临时目录暂存；Go窄helper核验7文件哈希/固定KnownFolder、目录持有防替换、新版本发布/同版本修复、HKCU和快捷方式可逆集成与真实失败返回；卸载仅删已知文件，未知文件保留，全部必要操作成功才删注册。启动前查数据树并持有目录，binding tag不初始化用户数据。
- 新测试首次发现 READ_ATTRIBUTES 目录handle不参与Windows删除共享校验，换成GENERIC_READ后真实rename被拒。新增5条保护加原4条，共22 Go/vet通过；typecheck通过。安装脚本本轮逻辑未重建实测，评审修复不先冒称关闭。
- 17:49–18:01：旧空数据库及读取产生的wal/shm完整移动保留在忽略证据目录，原件hash记录，不删除；预览1/2安装包构建通过并确认binding不再新建默认数据根，实际均NotSigned。首次向导验收UIA将NSIS原生控件报告Pane而未观察到保留勾选，针对脚本改用同进程窗口ID/Win32真实按钮和BM_GETCHECK；相关闭环重跑全通过。当前默认用户产品安装和合成数据已按脚本明确删除，6个应用进程均正常退出，旧空数据库保持备存。
- 18:01：完整回归只在本票收尾运行一次并全通过：49JS、类型、原型构建、27文档、11UI；不因接下来的文档修改再重跑全量。用户明确调整节奏为功能优先、相关验证、票末完整回归与CI，已写入GOAL；工具/环境复用、关键真实操作仍保留。
- 18:08–18:12：只读复核确认四项关闭，新增唯一必要修复为尾部占用失败丢卸载重试入口；已分离最后finish-uninstall，快捷方式或数据处理失败保留卸载器/注册。仅跑相关desktopbase测试通过；新增合成真实文件占用→失败仍可重试→释放后完成用例，总Go23。快捷方式修为NoWorkingDir，并将最终真实桌面验证从直接exe改为分别从桌面/开始菜单.lnk启动、核对实际exePID。
- 18:11–18:13：代码提交12e8286；干净树两个预览包构建通过，真实从两条.lnk分别打开预期安装exe，每版正常关闭重开，完整安装→升级→保留卸载→重装→显式删除再次通过。公开合成证据与安装包实际hash已落盘；NotSigned，不含内核。确认Go VERSIONINFO fixed0.3.0.0与可选字符串空，元数据脚本按实际fixed字段记录，helper版本资源null如实保留。
- 18:23–18:27：[run36701706414](https://github.com/axgiroud312-byte/prism-local-browser/actions/runs/36701706414) Node22/24通过，desktop安装包构建失败。已读取失败日志定位Get-FileHash模块自动加载而非产品/数据库/UI失败；增加进程内本host原生模块路径优先的6入口共享bootstrap，模拟PS7-only PSModulePath的实际WinPS5验证Get-FileHash/Authenticode/Archive/Add-Type与NSIS缓存ZIP/hash/版本全部成功。仅相关脚本验证，未重复49JS全量；仍不计T03完成。
- 18:32–18:35：最新CI打包/前提已通过，真实安装exit0，快捷方式目标比较失败（尚未进入应用UI操作）。检查改为Windows物理DesktopDirectory（不是虚拟shell Desktop）与GetFullPath规范化，添加真实文件存在性检查及不带私人路径的错误细节；没有改产品代码或放宽预期exe检查。正在只复跑相关安装/快捷方式闭环；没有本地49JS全量回归。
- 18:44–18:58：[run36703818884](https://github.com/axgiroud312-byte/prism-local-browser/actions/runs/36703818884) Node22/24、Go与两包构建通过，默认首次安装exit0后链接文件存在但TargetPath空。先加失败回归，再改helper在publish成功后创建并回读Windows链接，取消安装前NSIS CreateShortcut；go-ole使用已有固定依赖/许可，没有引入新版本。物理目录修正保留，错误诊断不再对空路径调用Test-Path。独立PS实际读回两链接验证首次/升级及锁占用失败仍恢复旧链接/注册，desktopbase/vet通过；最终新包真实操作尚待。
- 当前阻塞：无外部资源阻塞；仅当前票干净runner真实流程未通过，不关闭本票。
- 下一步：提交产品修复，复用工具重建两个包并仅测实际安装闭环；推送一次必要修复CI，核对最新headSha/干净用户证据；成功后合入关闭#4，正式3/21后自动T04。无后台代理或命令。

## 恢复资源与 GitHub

- 已有其他 Node/Chrome 进程属于用户现场，不停止。已核对 5173 为本仓库旧 Vite 服务；4173 未监听（纠正初查格式误判）。本 Goal 尚未启动服务，UI 测试用独立端口 5183 和新浏览器上下文。
- `output/`、`.playwright-cli/`、`dist/`、`node_modules/` 为现有忽略目录；不删除旧成果。新测试输出使用 `output/goal/`，仅保留合成证据。
- `npx playwright install chromium` 已成功安装本票测试浏览器（仅前端检查，不是 fingerprint-chromium）。测试使用 5183，由 Playwright 启停独立 Vite，首次测试已退出；旧 5173 不动。Serena 本机索引 `.serena/` 已忽略。
- GitHub：T01/T02 已完成同步；T03 PR #25开放、#4未关闭；#1不关闭。当前无后台代理或命令需要等待。
