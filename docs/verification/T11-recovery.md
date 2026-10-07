# T11 阶段3：正式资源故障恢复

2026-10-05；基于`bc56c90`正式provider追加。关联[Issue #12](https://github.com/axgiroud312-byte/prism-local-browser/issues/12)。

## 本机已运行

`TestRealProtectedFaultContainmentAndIndependence`：同一真实148构建同时启动A/B；撤去A的实际桥listener后普通HTTP服务绑定原端口。普通caller对照确实到达，A浏览器仍活着且没有访问替代服务；关闭A后B仍可经原代理访问。随后停止上游，B故障闭锁并退出准确Job，直连受控观察端仅收到普通caller对照，没有浏览器请求；最终journal pending=0。

`TestRealProtectedManagerCrashRecovery/prepared`及`/running`：父测试等待子进程明确准备完成，随后对自己创建的管理器helper调用Process.Kill，未运行defer收尾；分别覆盖实际ACL/容器已创建但浏览器未启动、真实浏览器已启动。确认准确Job退出、日志仍含旧session，重新打开store执行实际ACL/容器清理，原数据目录保留，pending=0。一次首轮清理返回file-not-found，保持占用后重试收敛；不把第一次不确定结果记作成功。

`TestNetworkResourceRecoveryKeepsOnlyAffectedEnvironmentBusy`：独立日志无app.db会话时也保留A占用、不影响B；缺失对象时显式重试失败并保留保护，恢复原对象后同一Service重试成功，没有伪造browser session。

## 修复

原`Runtime.Reconcile`只读取networkPending并拒绝，临时恢复错误必须重开管理器才能再试。现按准确资源session调用同一Recover，耗时工作在全局锁外；失败仍保持busy。journal-only记录通过独立`networkResources`投影显示“重试资源清理”，不伪造PID或运行会话。清理确认与操作结果保存分开，保存失败保留待写状态。

只读评审指出跨环境map无锁读取、孤立资源UI不刷新、关闭未汇报结果保存失败，均已修复并末审复核。SQLite触发器真实拒绝UPDATE的回归通过：仍占用且PersistencePending，Close返回错误。TypeScript检查、48份文档检查通过；完整回归及人工界面验收留后续阶段。

## 边界

- 生产桥与管理器同进程：listener撤去测试证明socket损失和旧端口接管；管理器硬退出同时移除桥和Job，不冒称独立生产桥进程崩溃。
- 全部为合成目录/本机受控HTTP上游与HTTPS观察端，没有自动点击、服务故障注入或真实凭据。
- 这不是外部全路径无泄漏证据；独立远端HTTP/HTTPS/WSS、DNS/IPv6/WebRTC/UDP/QUIC与系统跨登录/重启仍待验。缺失外部资源不阻塞后续独立实现，但正式验收仍4/21。
