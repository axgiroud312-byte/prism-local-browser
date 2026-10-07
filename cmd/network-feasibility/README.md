# T11 独立可行性实验

此工具只探索系统隔离候选，不接产品服务，不解除代理启动门禁。固定 fingerprint-chromium **148.0.7778.215 Windows x64** 的官方归档与主程序 SHA-256；所有浏览数据均新建、合成。

已执行范围和实际问题见 [T11 记录](../../docs/verification/T11.md)。退出码 0 只表示本次采集及清理没有工具级错误；须逐项阅读 JSON，不能把它当作 T11 验收通过。

## 运行

需要 Windows x64、仓库 Go 工具链及已核验的固定 ZIP。工具会创建临时 AppContainer/profile、准确 Job、仅 loopback 的观测服务（包括 IPv4 DNS 53）。端口已被占用时直接报告，不能停止已有服务抢端口。不会打开普通工作区。

```powershell
$env:GOPATH = Join-Path $PWD '.tools/gopath'
$env:GOCACHE = Join-Path $PWD '.tools/gocache'
$env:GOTOOLCHAIN = 'local'
$env:CGO_ENABLED = '0'
& .\.tools\go\bin\go.exe build -p 1 -o .tools/network-feasibility.exe ./cmd/network-feasibility

$env:PRISM_NETWORK_FEASIBILITY = '1'
& .\.tools\network-feasibility.exe --archive .tools/fingerprint-chromium/148.0.7778.215/ungoogled-chromium_148.0.7778.215-1.1_windows_x64.zip --report output/goal/T11/new-experiment.json
```

报告必须是尚不存在的新文件。原始日志、清理记录和恢复用 DACL 留在忽略的 `output/goal/T11/`；其中可能含私有路径或系统标识，不能直接发布。

## 已单独授权的窗口站实验

`PRISM_NETWORK_STATION_ACL=1` 才允许临时给新生成的测试 SID 增加当前登录会话**非交互 Service 窗口站**的 `GENERIC_READ | WINSTA_CREATEDESKTOP`。先核对对象类型/可见性和保存原 DACL，结束时重新读取当前 DACL、撤销本次 SID 并读回确认。它不代表用户授权修改其他系统对象。后续操作者须取得对应授权后再设置开关。

`PRISM_NETWORK_FOCUS=station` 仅跑普通对照、上述 AC 配置和四阶段合成持久化/桥故障实验。其他调查模式保留原始失败，不将未成功的 Medium-token 尝试当作支持的方案。`PRISM_NETWORK_DEBUG=1` 额外调试本次 AC 进程；调试路径的结果与正常无调试器运行分开记录。

四阶段实际经过同一合成浏览目录：

1. 普通进程写持久 Cookie、LocalStorage、IndexedDB。
2. 零网络能力 AC 读回，更新标记；实际通道是 Chromium → 同 package socket → 容器外 Go 处理 → loopback origin。
3. 更换新的零能力 AC 重开读回；真正终止独立桥进程，由普通宿主接管原端口，再在仍存活浏览器中请求并观测连接/请求数。
4. 普通进程重开，再次读回 AC 写入值。

每阶段正常退出须主进程退出码 0 且准确 Job 全空；失败清理只终止本次自有 Job。进程快照核对 Job 成员、AppContainer/SID/capability、restricted SID 与 Win32k 状态。IPv4 TCP、IPv6 TCP、IPv4/IPv6 UDP 和 DnsQueryEx 使用受控本机观测器；这不替代外部出口抓包、全部解析 API、BFE/防火墙故障或全部 Windows 版本验证。

持久化 profile 先由普通浏览器写入，之后才逐对象追加本次 SID 访问权；此路径不改 integrity label，停止后撤销数据授权。`PRISM_NETWORK_VISIBLE=1` 去掉 headless 及实验用的后台网络/组件更新/sync禁用参数，请求 `--start-minimized`，只查询自有窗口。第21轮内核没有落实最小化请求，实际窗口可见并已正常关闭；窗口查询记录实际结果，不以参数当成效果。没有自动点击。

窗口站读改写在本工具遵循的同登录会话命名互斥内完成；该协议不控制其他外部权限写入者。独立桥进程硬退出后的原请求计数未采样，报告省略该字段，不写成零。

## 中断后的恢复

正常完成会撤销窗口站本次 SID、删除本次 AppContainer 和合成 root；失败在 JSON 明确保留。仅在核对记录、确认自有 Job 全退后，才可对本次工具创建的准确容器名使用：

```powershell
& .\.tools\network-feasibility.exe --cleanup-container prism-feasibility-<本次记录的UUID>
```

此入口只接受固定前缀和合法 UUID；不能据此删除其他目录。若进程被外部硬杀，尚未撤销的窗口站 ACE 需依据私有记录另行准确恢复；不能用通配符覆盖共享对象的完整权限。工具自身硬中断恢复尚未构成生产隔离组件的生命周期实现。

## 单独授权的一次 BFE 正常停止实验

2026-10-05仅获准当前电脑一次标准BFE STOP；被Windows拒绝立即结束，成功则短时合成探针并恢复原RUNNING。不调用依赖停服、宿主终止或规则/配置修改。后续操作者不能复用此记录作为新的停服授权。

- `PRISM_BFE_NORMAL_STOP=1`的非提升host运行`--bfe-prepare <output/goal/T11/bfe-stop-UUID>`：独占新目录，启动普通与零capability AppContainer持久helper，先完成TCP4/TCP6/UDP4/UDP6和指定loopback DNS的基线，记录准确Job/token。这一模式不启动Chromium、不改窗口站权限，也不是外部网络验收。
- `probe-ready.json`成功后，手动UAC提升同一已编译exe运行`--bfe-controller <同目录>`。一次性claim拒绝重放；核对非提升host原进程创建时间与同构建摘要，SCM只向BFE申请QUERY/STOP/START。
- controller先启动Job外的独立`--bfe-recover`子进程；该进程仅有QUERY/START服务句柄，READY时尚未进入恢复责任；随后单独arm/ACK成功才允许唯一一次`ControlService(STOP)`。未调用或明确拒绝时发送无副作用cancel，不能启动由其他原因停止的服务。拒绝记录原错误及重新查询的状态，不信任失败输出结构；不会为1051停止依赖服务，1052不支持控制也直接结束。
- 成功STOPPED窗口才触发合成探针；controller的收尾优先恢复RUNNING。guard在25秒或父进程丢失时持续尝试恢复，STOP_PENDING等待停止后再Start；父异常退出继续监测至少一分钟的晚到停止。此机制不能保证Windows服务调用与恢复在固定秒数内完成。
- STOP结果未知，或虽然受理但从未观测STOPPED时，不因一次RUNNING查询解除guard或标记恢复确认；报告保留`recoveryUncertain`，独立进程继续收尾。准备阶段可写含本次nonce的`probe-cancel-request.json`退出；仅在不存在controller claim时生效。
- JSON、摘要、PID/SID和日志仅在Git忽略的私有目录。全部自有helper退出后删除临时容器及`synthetic`目录；`probe-finished.json`记录清理。服务拒绝只能证明请求未获准，不能计为故障隔离通过。

SCM依据：[Stopping a Service](https://learn.microsoft.com/en-us/windows/win32/services/stopping-a-service)、[ControlService](https://learn.microsoft.com/en-us/windows/win32/api/winsvc/nf-winsvc-controlservice)、[StartServiceW](https://learn.microsoft.com/en-us/windows/win32/api/winsvc/nf-winsvc-startservicew)。`StartService`不会取消STOP_PENDING；其成功结果也必须后续读回RUNNING。探针中断、超时或尚未取得结果便离开STOPPED都作为工具错误，立即转入恢复。
