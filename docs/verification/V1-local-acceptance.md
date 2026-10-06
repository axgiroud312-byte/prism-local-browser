# 首版本机缺口补验（2026-10-06）

[总报告](V1-final.md) · [源码摘要与脱敏观测](V1-local-acceptance.json) · [进度](../PROGRESS.md)

按用户“先把能做的验收都做了”，只使用新建自有合成工作区、精确148内核与受控本机上游。没有自动点击、提权、停Windows服务、填满系统盘或修改真实数据。**正式验收仍4/21，相关Issue保持OPEN。**

## 结果与边界

统一实跑12个顶层测试，其中11个新opt-in；备份权限另有2个子例。原始日志在`output/goal/V1-local-acceptance/`；[回执](V1-local-acceptance.json)包含12份观测、各项时长、源文件及测试二进制SHA，JSON存在本身不代替suite与Cleanup成功。

| 补验 | 已核对结果 | 不扩大为 |
| --- | --- | --- |
| T07/T11 普通停止失败→强制停止 | 只暂停自创的准确根进程线程；普通关闭实际超时→原Job ForceStop退出码1、资源释放；B活着，A重开保已落盘Cookie/档案/引用，旧session请求拒绝 | 应用崩溃/跨登录恢复 |
| T12 Cookie | 非分区/祖先false/true三个键实际读回；重复匹配不写；缺字段分区/过期拒绝且原Cookie不变；冲突合并保其他分区，明确清空全部分区，B不变；取消保第一条、第二条未发送 | 取消屏障位于首条真实写/读之后，不是任意pipe断线的未知写窗口；脱敏检查限已检查的Workspace响应 |
| T13 257项目录 | 第129项真实新增子目录拒绝，已完成128项/失败预备ID与seed保留；修权限后257个独立空目录、25项结果页/8项环境页及去重通过 | 百万实体/UI容量；内核元数据合成，目录/DACL是真实Windows |
| T14 12项真实队列 | 首条实际前检受控等待时12项排队；取消1不启动、实际目录拒绝1、其余10真实根按FIFO且保持运行；修权限只重试失败项成为11运行；全部正常停止 | OS内存/进程资源耗尽；无产品实例配额 |
| T15/T17 双运行环境完整包 | A/B各写Cookie/LocalStorage/IndexedDB；备份服务正常停止二者exit0，包只含A/B无内核；修改两者三存储/备注后恢复，服务重开真实读回旧数据/配置；包外C不变 | 跨用户/人工文件选择器 |
| T15 备份权限 | profile源读拒绝、输出新增文件拒绝均不发布包；源字节不变/响应脱敏；原DACL恢复后新请求完成 | NTFS空间耗尽 |
| T16 预检读取权限 | 自有备份实际不可读即拒绝；DB/WAL/SHM及浏览字节不变；修权限后可预检，丢弃仍不改库 | 跨SID解密/系统选择器 |
| T17/T18 切换/回滚权限 | checkpoint只加真实父目录拒绝并返回nil；两个完整侧/原目录对象保留，重开仍保护且拒绝启动/修改；删原包、修权限后原任务恢复精确旧侧，二次重开幂等 | 新浏览器升级或硬Kill切点 |
| T17/T18 SQLite容量 | 私有fixture `max_page_count`令正式配置事务实际SQLITE_FULL=13；committed始终0，实际worker回滚；放宽容量后原任务收尾，无半导入或包外损坏 | NTFS磁盘满、COMMIT刷盘失败；未填满系统盘 |
| T19 删除权限 | 同时拒绝对象DELETE/父DELETE_CHILD，独立DELETE-access open实际拒绝；删除前全树完整，重开保保护；恢复权限并释放测试自有恢复句柄后同任务只删A，B/历史包摘要不变 | 人工永久删除向导/其他资源故障 |

## 发现的缺陷与修复

生产`prepareBatchDirectory`把失败返回的nil指针直接转成`BatchDirectoryLease`接口，接口非nil导致失败清理`Close`崩溃。现在仅在转换前规范化nil；有效lease/空目录/归属核验和原错误保持，不改变身份、代理或网络保护。[默认回归](../../internal/workspace/batch_prepare_failure_test.go)、[无seam实际ACL与257项续跑](../../internal/workspace/safe_gap_windows_test.go)通过。初轮崩溃及失败日志保留；只精确撤销自有中断临时根上的本次Deny，保留测试字节，不改继承或系统权限。

其他首轮失败是测试观察：protected Close刻意返回保留警告但DB确已关闭；默认Workspace只含8行，页外会话须用ID `Runtime.Inspect`；原任务旧protected失败不能当新恢复终态；测试自己的ACL恢复句柄须在删除前释放。修订逐一核对这些事实，没有删/跳过测试或把原FAIL改记成功。

只读评审发现线程暂停检查有PID复用窗口；测试已改为核对创建时间后持续保留准确根进程句柄，cleanup对存活线程的恢复失败明确报错。动作仍只针对自有准确对象，不按进程名称、裸PID结束浏览器。

## 后台检查与候选

- `npm run check:background`：131/131、类型/build/50份文档通过；既有chunk/`use client`警告保留。新增报告后文档复验另记。
- `go test -p 1 -count=1 -json ./...`：exit0，342顶层PASS/30 opt-in或helper SKIP；11个新opt-in另行实际执行，不把默认SKIP算通过。
- `go vet -p 1 ./...`：完成回执exit0，不仅凭空日志判断。
- 未运行`npm run check`、Playwright或自动点击。

新干净67c98db来源候选`.5/.6`已构建/本机无点击安装复验通过，当前主`.6`见[总报告](V1-final.md)和[安装回执](V1-candidate-acceptance.json)。旧`.4`保留原哈希/证据但不再是最新；未复用旧版本/覆盖日常安装。默认CI也增加一次性Windows无点击安装，实际结果另记，不混用runner开发预览与本机候选。

runner首轮尚未安装便被dirty门禁拒绝；全新检出复现Go锁文件CRLF/LF差异。获用户批准仅固定两个锁文件LF后，全新检出两次production构建均保持干净/manifest sourceDirty=false；实际改动自有go.mod内容仍被原脚本exit1拒绝且未运行安装，精确恢复本测试改动后干净。[门禁证据](V1-ci-source-guard.json)明确物理换行SHA与规范化依赖的区别。产品及`.6`二进制未改变，远程实际安装结果单列。

eec3333[远程复验37409611901](https://github.com/axgiroud312-byte/prism-local-browser/actions/runs/37409611901)三job及真实无点击安装已通过；下载[安装回执](V1-ci-installation.json)/两安装包SHA核对。Server 2025已有WebView2上的开发预览`.1/.2`完成6次native正常exit0、升级/保留卸载/重装/自有删除；四点击分支跳过。该范围不替代精确交付`.6`的Home新用户/VM、缺WebView2或人工操作。原失败仍保留，正式4/21/全部blocking不变。

## 仍需外部条件或人工

1. 授权测试代理与独立HTTPS/WSS、权威DNS、IPv4/IPv6、UDP/STUN端及日志；凭据用本机受保护配置，不贴聊天/入仓库。
2. 人工native流程、保存/选择器取消与卸载默认复选框，沿用[最短人工清单](V1-final.md#最短人工补验只用测试用户与合成数据)。
3. 干净Windows/测试用户或VM、缺WebView2、跨Windows登录/重启，不在日常用户上制造。
4. 真正NTFS空间耗尽、OS进程/内存资源耗尽及更多系统/内核/目标网站兼容性。

没有新云功能、配额、依赖或Chromium再分发。Windows隔离服务损坏仍属后续加固；这些实跑只补对应证据，不解除其他blocking或自动关闭票。
