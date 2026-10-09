# 内置148开发版（2026-10-08）

## 远程同步（2026-10-09）

按用户“全部同步”要求，源码与文档保存到 [codex/bundled-kernel-148-sync](https://github.com/axgiroud312-byte/prism-local-browser/tree/codex/bundled-kernel-148-sync)，基于已合并 PR #38 的远程 main；[v0.3.0-preview.15](https://github.com/axgiroud312-byte/prism-local-browser/releases/tag/v0.3.0-preview.15) 提供 Windows x64 程序包、SHA256SUMS.txt 和 release-manifest.json。程序包直接使用下文已经实测的现有 exe，主程序摘要再次核对为 `8d666acd34d6a43a5be015d3b3e5395481bc0f38d8e331a5b09297b24c62cd3a`；归档摘要与固定官方摘要一致。来源清单分别标明原二进制构建基线 `311a47f`、已提交源码、完整文件摘要及实际 NotSigned 状态。

同步前重新运行 `go test ./internal/workspace ./internal/kernel -run '^(TestBundledKernel|TestPhysicalExecutable)' -count=1 -v`，5/5通过；`npm run check:docs` 检查87份文档、相关本地链接、12条需求、6个路由和4份内嵌文档，通过。3份相关PowerShell脚本语法检查通过，6份Go文件gofmt检查无输出。公开源码与包只含代码、文档、当前程序、精确内核和许可材料；本机工作区、profile、凭据及原始系统轨迹保留在忽略目录。历史真实桌面结果仍以各自记录的来源和范围为准。

## 原始交付与验证

用户追加要求：程序内置148。交付为 `build/bin/prism-browser.exe` 与同目录 `bundled/`，版本0.3.0-preview.15，基于311a47f和本次本地增量；不是旧安装器，也不宣称历史21项验收全通过。

程序随附官方 fingerprint-chromium148.0.7778.215 ZIP，189,767,686 bytes，SHA-256 `9ef3f471b7a6641b4224532522b29141ce3746e27d55788d88e2fd951f362579`。本次重新读取官方Release API并实算本机ZIP，名称、大小和摘要一致。许可保留精确tag的上游BSD文本、Chromium版本文本以及由该包真实主程序导出的19,717,592-byte完整组件Credits；没有将其重新许可为项目MIT。

首次打开走既有安装worker：摘要、解包边界、PE、实际CDP pipe身份/能力探测、事务发布和持久操作终态均保留。只在初始默认为kernel-pending时设置148；重开复用已登记构建，不覆盖显式默认或旧环境引用。目录维护恢复优先；已登记文件损坏后保持原ID并标不可用，错误归档不执行、不发布。

定向模块检查：`go test ./internal/workspace -run '^TestBundledKernel' -count=1 -v`，3/3通过。覆盖首次默认、关闭重开只安装一次、原构建损坏不替换、显式默认保持及修改归档拒绝/失败记录。使用合成test seam，不作为真实Chromium运行证据。

首次桌面尝试（preview12–14）失败，Windows返回14001。SxSTrace定位为148版本程序集清单无法解析；Process Monitor进一步确认：从MSIX打包的宿主启动时，AppData写入被重定向到宿主包的 `LocalCache/Local/`，而CSRSS仍在未重定向路径查找版本清单，得到PATH NOT FOUND。同一份文件在Temp可用，在AppData逻辑路径失败；从已打开文件句柄取得物理路径后，激活上下文成功。不是清单缺失或归档损坏。微软对[MSIX目录重定向](https://learn.microsoft.com/en-us/windows/msix/desktop/flexible-virtualization)的说明与本机轨迹一致。

修复位于 [`pipe_windows.go`](../../internal/kernel/pipe_windows.go)：启动前从已打开的同一程序文件取得真实物理路径，并把文件句柄保持到CreateProcess完成；既有归档/文件核验、目录固定、Job和私有pipe保持。没有修改内核字节、Windows目录权限或浏览器沙箱，也不硬编码任何宿主包名。失败说明保留具体启动步骤及Windows错误号。修改工作目录的中间尝试无效，已撤回。

本次实际结果：

- 定向模块检查5/5通过：3项内置准备检查，以及物理路径仍指向同一文件、启动期间拒绝替换和硬链接拒绝检查。真实归档的可选回归入口改用自有LocalAppData目录，避免Temp豁免目录重定向掩盖问题。
- 修复后在原失败工作区执行真实148探针，3次版本/网页参数核对均通过、正常退出；没有发布诊断用构建。随后正式程序首次自动准备成功：76个内核文件、官方归档及程序摘要一致、3次真实观察、沙箱开启、继承私有pipe，安装操作终态completed。
- `scripts/desktop.ps1 -Action build -PreviewRevision 15 -Bundle148` 成功；主程序SHA-256为 `8d666acd34d6a43a5be015d3b3e5395481bc0f38d8e331a5b09297b24c62cd3a`。随程序保留精确ZIP、三份许可材料和manifest.json。
- 实际Windows窗口标题为0.3.0-preview.15；内核管理显示148.0.7778.215「真实诊断已核验」「当前默认」，新建环境的下拉框显示「148.0.7778.215 · 已核验」，固定指纹草稿可准备。本次未保存验收环境。
- 正常关闭后重新打开，无准备失败弹窗；原内核ID、默认ID/修订2均保持，安装操作仍为5条（历史失败4条、完成1条），没有新增安装，环境数仍为0。最终保留可见原生窗口。

原始失败、目录对照与本机只读数据库摘要留在忽略目录 `output/bundled-kernel-148/`；包含本机路径的原始系统轨迹不提交。临时大型轨迹和诊断程序的清理被执行策略拒绝，仍留在该目录。此次只验收随附148准备、真实诊断及上述UI入口；没有重跑整套前端检查或扩大150、普通环境网络/代理、跨版本迁移和历史正式4/21的结论。
