# T11 阶段2：正式隔离启动接入

日期：2026-10-05。任务：[Issue #12](https://github.com/axgiroud312-byte/prism-local-browser/issues/12)。本记录补充[T11历史记录](T11.md)，不把早期固定拒绝描述作为当前实现状态。

## 实现

- `NetworkStore`独立SQLite资源日志、目录固定与排他owner锁，在workspace维护恢复分支之前加载。配置库中没有对应runtime记录时仍保留资源占用；A清理不明不占用B。
- 创建容器、差量ACL、同package桥、原子Job绑定进程前提交意图。清理封存创建，核对准确Job已空，逆序撤销；失败继续占用。
- AppContainer零capabilities，启动材料绑定会话、环境、数据引用和内核；同通道前检后才能启动，实际Job成员及代理调用方token核对。保留Chromium沙箱，禁用网络服务无沙箱重启回退。
- 文件权限按现有DACL增删独有package SID，不恢复整份旧DACL；对象身份不符、reparse或硬链接拒绝。Windows继承传播属于已记账树意图。
- 窗口站由AuthenticationId推导、实际名字核对后授权；日志增加boot GUID，恢复区分旧boot、结束的登录及仍存在的登录。
- 迁移副本尚未使用这套owner，继续明确拒绝代理试用。

## 已运行的验证

1. kernel/proxy/workspace选择性测试：Network、Managed、EmptyProfile、未验证边界拒绝、Ingress、RuntimeProxy、SavedProxySession及空workspace重开相关测试通过。
2. `TestRealProtectedProxyLaunchAndReopen`通过：Windows build 26200、fingerprint-chromium 148.0.7778.215，真实浏览器两轮；CPU读回8、固定seed；正常退出码0，日志pending=0，容器mapping消失；第二轮Cookie、LocalStorage、IndexedDB先读后写均保留。[脱敏证据](T11-production-start-observations.json)。
3. `TestRealProtectedRuntimeApplicationReopen`通过：使用正式`Runtime.Start/Stop`及`Service`重开，真实provider和桥，两轮访问受控站点；保存身份和数据引用不变，三种存储保留。[脱敏证据](T11-production-application-observations.json)。
4. `npm run typecheck`、`npm run check:docs`（47份文档）及`git diff --check`通过。按用户阶段安排未运行含自动点击的完整`npm run check`，全量回归留阶段5。

测试使用显式`PRISM_PROTECTED_TEST_ROOT`合成目录、已有校验内核zip和本机HTTP上游/HTTPS观察端；没有自动点击。证据不包含真实凭据、用户profile或私有路径。

## 排错与评审

- 原目录锁拒绝WRITE共享，导致Chromium不能原子替换Local State，Cookie重开丢失。仅真实运行数据根允许WRITE共享，仍拒rename/delete；维护及批量创建维持全链严格锁。相关测试与真实三存储重开均通过。
- 文件ACL改用保留Windows继承语义的句柄API；测试确认原ACE与继承标志不变，并清除新建文件继承的临时SID。
- 只读评审指出旧登录恢复永久占用、共享pin helper影响批量目录创建，两项均修复；末次复核无新增可信P1/P2。

## 尚未验证与下一步

- 独立外部代理及远端观察、完整TCP/UDP/委托DNS无旁路、正式provider的上游断连/桥崩溃/管理器崩溃/旧端口接管矩阵，仍待验。
- 跨注销/重启恢复分支已实现，但本轮没有执行注销/重启；当前boot/logon只读身份稳定性测试通过。
- 运行期同用户对userdata根原地reparse不是分享锁能阻止的边界；host遍历及清理继续核对对象并拒绝不确定路径，不宣称抵抗同用户恶意并发修改。
- 人工产品界面闭环、队列/Cookie/迁移保护集成验收、全量回归与安装包仍按六阶段计划推进。Windows底层隔离服务失效属于已确认的后续加固范围。
- 正式验收计数保持4/21，无推送、PR或CI。GitHub认证恢复后同步issue证据。
