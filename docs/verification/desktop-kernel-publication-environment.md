# Windows 内核发布失败的隔离环境调查

日期：2026-10-07。范围：本轮合成 workspace 的真实 fingerprint-chromium 归档获取、隔离探测和安装发布。用户已取消严格 1:1 要求；本项不扩大视觉参考工作。

结论：首个 `.appdata/verification` 测试根的发布确实失败，应保留失败。相同原生产发布实现在 `output/goal/desktop-closeout` 的独立测试根，通过正式 `Kernel.Install` 成功安装精确 148 和 150。自有目录的监听开关实验重现了“后代目录被监听时，父目录移动被拒绝；关闭自有监听后，同一移动成功”。本轮没有修改 `internal/workspace/kernels.go`，没有降低目录 pins，也没有终止用户原 Vite 实例。

## 证据及判定

| 检查 | 来源、操作和实际结果 | 判定及计数边界 |
| --- | --- | --- |
| 首次真实安装 | `.appdata/verification/DesktopCloseout-c24f04d3-d838-4217-8013-3f7dea83658d` 内两次 148 安装完成真实探测后返回 `STORAGE_WRITE_FAILED`；数据库中对应操作为 failed、completedIds 为空，未注册可用内核 | **实际失败，保留**；不能因后续另一根成功而改写 |
| 原发布错误定位 | 新自有 `.appdata/verification/KernelPublication-*` 根，真实 Prepare 后直接调用原 `publishKernel`；底层 `*os.LinkError` 的操作为 rename，Windows 错误为 `Access is denied`，不是 `ERROR_SHARING_VIOLATION` | **实际失败，已定位阶段**；前端的存储错误标签不等于实际 SQLite 写入失败 |
| 排除仅限内核 payload 的问题 | 同一失败根中，额外创建只有普通文本文件的两层树，父目录移动同样被拒绝；源属性为普通 Directory，全部 79 个真实 payload 后代的读及 DELETE 打开检查均成功；测试结束后未见该根所属 Chromium 进程 | **实际观察**；不能凭这些观察断言当时没有外部后代监听句柄 |
| 候选对象移动实现 | 一度尝试现有受控 MoveTree 边界；合成小树通过，但相同 `.appdata` 真实 payload 仍因 AccessDenied 失败。候选已全部撤销 | **候选未解决，保留**；不纳入产品修复或通过计数 |
| 新根正式安装 148 | 根代理的 `output/goal/desktop-closeout/native-workspace-e1eb1ddc-b74a-41c2-8cb6-2857cabcae28` 使用原 `Kernel.Install`、真实 ChooseArchive；精确版本 148.0.7778.215，available=true、安装操作 completed，completedIds 只含对应内核 ID | **实际服务通过**；这是服务及真实内核准备证据，不能替代新桌面程序的点击、启动和关闭验收 |
| 新根正式安装 150 | 同一隔离根使用原 `Kernel.Install`；精确版本 150.0.7871.186，available=true、安装操作 completed，completedIds 只含对应内核 ID | **实际服务通过**；同上 |
| 监听因果对照 | 自有 output 两层文本树，对后代调用 `fs.watch`；移动父目录得到 `EPERM`。仅关闭自有 watcher 并等待 close，再执行同一移动成功。Node v24.14.0、libuv 1.51.0 | **实际对照通过**；证明本机后代监听足以造成该症状，不证明旧 Vite 进程的具体句柄归属 |
| 额外原发布对照 | `KernelPublicationOriginal-aeb5fbfb-6616-4e73-9f6e-543e05678a7e` 在 acquiring/extracting/verifying 后进入 probing，94.68 秒后 `context deadline exceeded`，未进入 publish | **该次探测仍失败**；发布检查未执行，不计发布通过，也不计发布失败。未用模拟或重试结果抹去 |

## 精确归档与发布记录

148 归档 SHA-256：`9ef3f471b7a6641b4224532522b29141ce3746e27d55788d88e2fd951f362579`。正式安装记录的 chrome.exe SHA-256：`1867319e56bcabbc4681d8575c002106ce7b61b5290dc5eb34a37676805f6915`，内核 ID 为 `24620122-863e-441f-8628-c7577a05d412`。

150 归档 SHA-256：`4d549c326e51ebbabf562fd365eb5380d9d4a81200da2c60f075c688d9a77e03`。正式安装记录的 chrome.exe SHA-256：`65d0807599b5430ffbd9dffb60cd020464815f10557352959e3aa4c125a5c439`，内核 ID 为 `1575b6aa-ee86-486f-9e83-87346ea6e3d6`。

本机合成证据保留在 ignored output：`kernel-install-first-failure.json`、`failed-test-root.txt`、`native-workspace-e1eb1ddc-b74a-41c2-8cb6-2857cabcae28/prepared-view.json`、`kernel148-source.json`、`kernel150-source.json`、`kernel-watch-control.json`、`kernel-publication-original.log`。临时诊断测试移至 `output/goal/desktop-closeout/kernel_publication_windows_test.go.txt` 保留原实验，不新增产品常规或 opt-in 测试分支。原失败目录没有手动移入成功位置；容量清理的范围如下。

因本轮磁盘剩余约 3 GB，根代理授权清理自有可重建诊断 payload。先保存精确路径、创建时间、属性、全部文件实算 SHA-256，确认目标在对应 named owned root 内、祖先及全部后代无 reparse，再移除两个副本：`KernelPublication-ca0677ca-56b4-45c5-bd7c-43a7270e0c26/staging/kernel-94f3ba7c-993f-4f27-83f0-df687b869ffd/payload` 与 `KernelPublication-5c3e8d36-94d3-46d3-9359-5ef557e8b0cf/kernels/84bccfaa-8057-4d31-bbff-9ad85233bb6f`。每个副本 76 文件、445242434 字节；两次移除实际成功，空闲空间由 2987266048 增至 3878035456 字节。根目录、数据库、小记录、归档、正式 native-workspace 和他人 Service 根均保留。清单与结果保存在 `kernel-publication-payload-cleanup-manifest.json`、`kernel-publication-payload-cleanup-result.json`，对应清理脚本一并保留在 ignored output。

## 最小修复及仍存边界

当前开发监听原本排除 output、.tools、build、wailsjs，却没有排除 `.appdata`。旧 5173 Vite 进程 PID 43972 正在本仓库运行。这与失败路径和自有监听对照吻合，故“旧 Vite 监听导致首次失败”是有依据的推断；本轮未停止该进程，未获取其句柄清单，不能将具体 PID 因果写为已证。

根代理已在 `vite.config.ts` 增加 `**/.appdata/**` 的最小监听排除，避免后续开发服务器监控可移动的 native 数据。配置不会动态改写旧 Vite 已建立的 watcher；真实桌面验收改用已排除的 output 隔离根，旧监听不再阻塞本轮该根的内核发布。任何其他程序持有测试目录后代句柄，仍可能导致 Windows 拒绝移动；产品继续安全返回失败，不冒充安装成功。

没有放宽 PinDirectories、改变内核删除合同、增加任意等待或通过裸 rename fallback 修补生产行为。程序 UI、环境启停、代理和 Cookie 等桌面结论另见本轮正式桌面验收记录。
