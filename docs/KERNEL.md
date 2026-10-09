# 浏览器内核适配合同

本项目选择 [fingerprint-chromium](https://github.com/adryfish/fingerprint-chromium/blob/main/README-ZH.md) 作为 Windows 桌面版的内核方向。本文定义配置生成、内核管理、独立环境启动和代理接入的实现边界，供后续桌面后端开发使用。

2026-10-08 用户要求内置148：新的直接运行包在程序旁 `bundled/` 随附精确148.0.7778.215官方ZIP及许可材料，首次打开离线准备，核验后供新建环境选择；这不是将前端版本号标成已安装。使用下文固定归档摘要并进行真实内核探测。已登记精确构建复用，初始默认不覆盖用户选择或旧环境引用；缺失/损坏不换版本。源码仓库不提交内核二进制，打包入口为 `scripts/desktop.ps1 -Action build -Bundle148 -PreviewRevision 15`；实际结果见[内置148记录](verification/bundled-kernel-148.md)。

**网页原型与native能力分别记录。** T04已正式验收；148/150、正式代理会话、故障恢复、三存储隔离、Cookie及完整恢复/迁移已有本机结果，详见[集中验收](verification/V1-final.md)。独立远端全路径、人工UI及其他Windows/内核组合仍待验证，不因诊断/参数/构建通过自动完成。原型GPU、摘要与启动状态仍为演示，不标成实测。

核实日期：2026-09-30。Ant-Browser 仅作为架构参考，本项目不复制其源码；旧比特浏览器材料仅用于理解交互与字段关系，不作为本项目内核、算法或资源池来源。

## 需求与页面对应

| 需求     | 本合同负责的行为                            | 原型入口          |
| -------- | ------------------------------------------- | ----------------- |
| FP-001   | 一键生成当前内核支持的指纹配置              | `/#/environments` |
| FP-002   | 按固定内核区分可配置、seed 生成与未验证能力 | `/#/kernels`      |
| CORE-001 | 记录、选择、核验内核；受控升级与恢复        | `/#/kernels`      |
| PRX-001  | 每个环境独立代理、认证桥接、失败阻断        | `/#/environments` |
| ENV-003  | 独立用户数据目录、进程生命周期与互斥锁      | `/#/environments` |

## 内核来源与版本选择

当前官方资料存在“说明已列出，资产尚不可获取”的差异，应按实际资产判断，不能只解析 README 的版本号。

| 核实对象                   | 本轮结果                                                        | 开发含义                                           |
| -------------------------- | --------------------------------------------------------------- | -------------------------------------------------- |
| README 中的 150.0.7871.186 | 已列出 Windows ZIP 与安装包链接，注明源码随 151 发布            | 只是上游说明，不能显示“已安装”或“可用”             |
| 150 的 Windows ZIP         | 9月30日查询404；10月5日已获取并实算归档SHA，真实150运行及迁移通过 | 保留旧查询时间，不自动回退；许可与能力仍按精确构建记录 |
| 148.0.7778.215             | Release API 可见 Windows x64 ZIP；T04 实算归档与PE/私有pipe真实探测通过 | 本票明确选定的实测候选；生产首选仍待兼容性等评估 |

148 的公开 tag 指向 `13b89eae304123f0710f2d33fd0816a2f61d7ffc`。发行页中 Windows ZIP 资产名为 `ungoogled-chromium_148.0.7778.215-1.1_windows_x64.zip`，GitHub API 声明其 SHA-256 为 `9ef3f471b7a6641b4224532522b29141ce3746e27d55788d88e2fd951f362579`。T04 已实际下载并核对同一归档摘要；主程序摘要为 `1867319e56bcabbc4681d8575c002106ce7b61b5290dc5eb34a37676805f6915`，PE文件版本与CDP实际版本均为148.0.7778.215，HTTP/网页UA与高熵UA-CH按本合同核对。详细采样、会话及后续页面验收见 [T04](verification/T04.md)，不把上游摘要或静态源码当实测。核实入口：[148 发行元数据](https://api.github.com/repos/adryfish/fingerprint-chromium/releases/tags/148.0.7778.215)、[148 源码](https://github.com/adryfish/fingerprint-chromium/tree/148.0.7778.215)、[150 发行查询](https://api.github.com/repos/adryfish/fingerprint-chromium/releases/tags/150.0.7871.186)、[README 所列 150 ZIP](https://github.com/adryfish/fingerprint-chromium/releases/download/150.0.7871.186/ungoogled-chromium_150.0.7871.186-1.1_windows_x64.zip)。

后续安装流程须重新核实资产可获取性、更新时效、目标网站兼容性与该版本能力。以“可审源码”直接推导“适合生产”不成立。上游采用延迟公开源码策略，`main` 不是完整源码目录；每个可用构建建立独立能力记录，不能把 148 的结果自动套到 150。[上游源码发布说明](https://github.com/adryfish/fingerprint-chromium/blob/main/README-ZH.md#从源码构建)

内核记录至少保存：`kernelId`、下载来源、发行 tag、源码 commit（未知则为 null）、安装包 SHA-256、实际可执行文件 SHA-256、实际内核版本、适配器版本、能力清单版本、安装与验收状态。版本应结合安装后的文件信息及受控启动结果核对，不能仅从 UA 或文件名反推；UA 本身可以被修改。

## 配置字段与启动参数

下表的“可控”表示公开说明或 148 补丁具有对应入口。真实可用仍需对选定二进制完成验收。第一版固定 Windows 桌面目标；UI 不提供跨系统伪装、任意启动参数或完整 UA 文本自由编辑。

| 配置字段                         | 映射或来源                                                    | 第一版规则                                                                         |
| -------------------------------- | ------------------------------------------------------------- | ---------------------------------------------------------------------------------- |
| `fingerprint.seed`               | `--fingerprint=<seed>`                                        | 由本地安全随机源生成 1 至 2147483647 的整数；保存并复用，已存环境内检查重复        |
| `fingerprint.platform`           | `--fingerprint-platform=windows`                              | 固定 Windows                                                                       |
| `fingerprint.platformVersion`    | `--fingerprint-platform-version=<value>`                      | 由已核验 Windows 预设选择并保存；这是 UA 平台版本语义，不直接拼接 Windows 营销名称 |
| `fingerprint.brand`              | `--fingerprint-brand=Chrome`                                  | 固定品牌策略；描述为 Chromium 内核的 Chrome 兼容声明                               |
| `fingerprint.brandVersion`       | `--fingerprint-brand-version=<actualVersion>`                 | 由已核验的实际内核版本生成，用户不能随意改；升级时显式迁移                         |
| `fingerprint.cpuCores`           | `--fingerprint-hardware-concurrency=<n>`                      | 可选受限整数预设；空值表示由 seed 决定。UI 和后端共同校验                          |
| `locale.uiLanguage`              | 上游 `--lang=<tag>`，本项目尚未验收菜单语言                   | T05固定为 `system`，不下发 `--lang`；网站语言不代表菜单语言已生效                    |
| `locale.acceptLanguages`         | `--accept-lang=<ordered-tags>`                                | 网站语言顺序去重后保存；后续核对请求头与网页 API                                   |
| `locale.timezone`                | `--timezone=<IANA-name>`                                      | 保存明确 IANA 时区，拒绝空值与 `Local`；跟随代理时先解析成功再保存，失败阻断         |
| `proxy.profileId`                | `--proxy-server=http://127.0.0.1:<bridgePort>`                | 后端解析为独立代理桥接实例；启动命令无上游凭据                                     |
| `network.webrtcPolicy`           | `--disable-non-proxied-udp`                                   | 固定策略；仍需覆盖 WebRTC、IPv6、UDP 等真实网络验收                                |
| `compatibility.disabledSpoofing` | `--disable-spoofing=font,audio,canvas,clientrects,gpu` 的子集 | 高级兼容项，默认空；变更要展示影响并保存配置版本                                   |
| `environment.profileId`          | `--user-data-dir=<backend-resolved-directory>`                | 由后端分配独立目录；UI 不能传任意绝对路径                                          |

依据：[官方参数说明](https://github.com/adryfish/fingerprint-chromium/blob/main/README-ZH.md#启用指纹功能的命令行参数)、[148 UA 补丁](https://github.com/adryfish/fingerprint-chromium/blob/148.0.7778.215/patches/extra/fingerprint/002-user-agent-fingerprint.patch)、[148 CPU 补丁](https://github.com/adryfish/fingerprint-chromium/blob/148.0.7778.215/patches/extra/fingerprint/005-hardware-concurrency-fingerprint.patch)、[148 时区补丁](https://github.com/adryfish/fingerprint-chromium/blob/148.0.7778.215/patches/extra/fingerprint/018-timezone.patch)。

### UA 与真实内核版本

148 的 UA 补丁默认可以根据 seed 从内置版本表选取版本；因此固定 seed 仍不足以满足本项目的版本一致性要求。适配器必须显式传入品牌与实际内核版本，随后核对 HTTP User-Agent、`navigator.userAgent`、`navigator.userAgentData`、高熵 Client Hints 与实际二进制版本。允许 Chromium 的 UA reduction 和 GREASE 表达，但真实品牌/主版本及完整版本字段须满足该内核能力记录中的规则。

若显式参数未生效或各通道不一致，对外返回 `KERNEL_INTEGRITY_FAILED`，在 `error.details.reason` 中记录 `identity-mismatch`，保留诊断并阻止该构建进入可用状态。不要用 UI 中写着“150”作为验证证据，也不把内核自动产生的品牌列表误写成全部由应用控制。

### 由 seed 驱动的字段

| 字段                            | 148 静态源码证据                             | UI 展示方式                                             |
| ------------------------------- | -------------------------------------------- | ------------------------------------------------------- |
| CPU 默认值                      | 未显式指定时为 `((seed % 13) + 4) * 2`       | “自动生成”；源推导值与实测值分开                        |
| 内存                            | 最终补丁从 `8 / 16 / 32` 中按 seed 选择      | 自动，只读；不提供任意内存数值框                        |
| GPU 与 WebGL 参数               | 按 seed 从平台配置池选整组参数，包含精度等值 | 实测前只显示“由内核生成”；不得把前端样例 GPU 标为已探测 |
| 字体、Canvas、音频、ClientRects | 补丁使用 seed 派生选择或扰动                 | 保存 seed 与配置；不复用其他内核独立噪声值的语义        |

依据：[最终内存补丁](https://github.com/adryfish/fingerprint-chromium/blob/148.0.7778.215/patches/extra/fingerprint/021-device-memory-cap.patch)、[GPU 配置选择](https://github.com/adryfish/fingerprint-chromium/blob/148.0.7778.215/patches/extra/fingerprint/011-gpu-info.patch)、[字体补丁](https://github.com/adryfish/fingerprint-chromium/blob/148.0.7778.215/patches/extra/fingerprint/006-font-fingerprint.patch)、[Canvas 文本补丁](https://github.com/adryfish/fingerprint-chromium/blob/148.0.7778.215/patches/extra/fingerprint/015-canvas-measure-text.patch)、[音频补丁](https://github.com/adryfish/fingerprint-chromium/blob/148.0.7778.215/patches/extra/fingerprint/003-audio-fingerprint.patch)、[ClientRects 补丁](https://github.com/adryfish/fingerprint-chromium/blob/148.0.7778.215/patches/extra/fingerprint/014-client-rects.patch)。

固定 seed 保证应用不主动更换生成输入，不承诺所有页面、机器和内核版本下的输出逐字一致。输出仍受页面内容、宿主环境和内核实现影响；验收使用固定测试条件。

### 尚未验证的入口

- `fingerprint-screen-width/height`、`fingerprint-device-scale-factor`、`fingerprint-location` 在 148 开关补丁中出现，但已查指纹补丁中只有声明与转发，未确认读取后生效的实现。第一版不开放这些“指纹”控件。普通窗口宽高属于窗口体验，可单独实现与验收。
- `fingerprint-gpu-vendor`、`fingerprint-gpu-renderer` 从 144 起被删除；GPU 参数采用内核配置池，界面不展示假有效的手填选项。
- 任意内存、MAC 地址、电脑名、TLS 特征、WebGPU 伪装、经纬度和任意字体列表均不作为首版已支持能力。需要这些字段时，先新增能力证据与实测，再扩展合同。

依据：[148 开关补丁](https://github.com/adryfish/fingerprint-chromium/blob/148.0.7778.215/patches/extra/fingerprint/000-add-fingerprint-switches.patch)、[144 参数变更](https://github.com/adryfish/fingerprint-chromium/releases/tag/144.0.7559.132)。

## 固定指纹的生成与持久化

### 第一版的生成规则池

应用只生成并保存自己能控制的输入；GPU、内存、字体和绘图噪声等具体算法使用固定内核的实现。无需把旧软件的显卡名单或随机函数复制成另一个算法池。不同 seed 不保证所有输出都互不相同，不能把有限内核池描述成无限种硬件。

| 池或规则   | 首版定义                                                                                                                 | 保存规则                                                          |
| ---------- | ------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------- |
| 设备模板   | `windows-desktop-v1`，只允许 Windows 桌面                                                                                | 模板 ID、版本和生成器版本一起保存                                 |
| CPU        | `auto` 或明确选择 4、8、12、16；默认 `auto`                                                                              | `auto` 表示交给已固定的内核按 seed 生成，不在管理端伪造读值       |
| 地区模板   | 原型内置 US/en-US/New_York、GB/en-GB/London、DE/de-DE/Berlin、JP/ja-JP/Tokyo、SG/en-SG/Singapore、CN/zh-CN/Shanghai 六组 | 保存完整语言 tag 和 IANA 时区；这是编辑建议，不是实际 IP 定位结果 |
| 窗口       | 默认 1280×800，可编辑 400–7680 内整数                                                                                    | 作为窗口偏好保存，不写成屏幕指纹                                  |
| seed       | 操作系统安全随机源生成正整数，检查已保存 seed，碰撞重试                                                                  | 首次保存后固定；正常启动、改代理不重新生成                        |
| 品牌与版本 | Windows + Chrome 品牌策略 + 核验得到的实际内核版本                                                                       | 从能力记录展开，不能随机拼接其他浏览器或主版本                    |
| 内核控制项 | GPU、内存、字体、Canvas、Audio、ClientRects 由具体内核与 seed 决定                                                       | 保存内核和能力版本；实际输出另做探测报告，不编造精细控制值        |

生成顺序为：确定模板及内核 → 校验显式选择 → 选择地区建议（或保留手动设置）→ 生成 seed → 形成完整预览 → 保存后冻结。重新生成默认只换 seed，保留已选代理、内核、语言、时区和窗口偏好；改变这些显式字段需要用户编辑并保存。

桌面版的实际代理归属可能与六组示例不同。先使用标准 IANA 时区和 BCP 47 语言标签校验手动输入，再逐步扩展有版本的地区模板；不能把国家代码直接当成唯一时区。模板升级只影响新建或明确迁移的档案，旧环境继续使用已保存的展开值。

`Fingerprint.Generate` 只生成预览：选择已验收内核 → 校验 Windows 预设与显式字段 → 生成 seed → 解析语言/时区 → 返回 `previewProfile` 与 `capabilityReport`。预览不修改已保存环境、不递增修订号。需要实际 GPU 等信息时，另行启动探测，不把摘要生成等同于内核探测。

用户明确保存后，`Fingerprint.CommitRevision` 校验预览、环境停止状态及 `expectedRevision`，在同一事务中保存完整档案和环境引用；提交成功才递增修订号并返回 `newRevision`。取消或关闭未保存编辑会丢弃预览，旧档案保持不变。新建环境由 `Profile.CreateBatch` 在创建事务中保存所选预览及环境记录。

T05实际增量以 [应用接口](DEVELOPMENT.md#t05-固定档案增量已实现完整验收待补) 和源码为准：当前单条新建仍是 `Environment.Create`；提交使用服务会话 `previewId` 和 `profileHash` 而非信任客户端完整档案。环境修订与档案修订分开，普通元数据保存只增加前者。回滚先由 `Fingerprint.PreviewRestore` 返回变化，再明确提交为新单调修订；只接受同一精确内核且编译输入与原历史一致。v1/v2迁移保留旧seed/生成器/pending，已有数据引用不变。[真实保存档案样本](verification/T05-saved-profile-observations.json) 仅证明受控临时诊断参数回读，不证明正常环境/Cookie/新页面操作。

能力编译器只下发所选不可变构建的observed能力：Windows/品牌/实际版本组、seed、网站语言顺序、明确时区，以及已核验的显式CPU偏好。GPU/字体/噪声具体算法留给内核，不下发假硬件值；窗口宽高只保存偏好，不编译成屏幕伪装。`Generate` 不执行探测，因此预览 `observedFingerprint` 为null、`canLaunchNative` 为false；构建安装时的能力证据明确标为非当前环境实测。正常启动监督器须接入host-only忙租约，租约单元测试不能代替真实运行证据。

每个指纹记录保存 `seed`、`generatorVersion`、`schemaVersion`、`configRevision`、`kernelId`、`coreActualVersion`、`coreExecutableSha256`、所有显式字段、配置规范化哈希。`generatorVersion` 标识应用生成规则；`configRevision` 随用户明确修改递增。打开详情、复制摘要、停止、重开和读取配置均不得触发重随机。

“重新生成”只在已停止环境中提供，先改变草稿并展示变更摘要；提交前再次检查停止状态。首次生成失败时不创建半成品；提交失败时保留旧有效配置。请求 ID 用于去重，保存采用修订号冲突检查；重新读取预览不得再次随机生成。

## 环境与代理的启动顺序

**2026-10-07 用户调整后的当前合同：**按 Ant-Browser 的共享内核、独立数据目录和逐环境代理方式启动，取消额外 AppContainer、共享内核/profile/窗口站 ACL 操作和内核副本。新代理会话使用本机认证桥及准确 Job 调用方核对；保留同桥前检、无 DIRECT 备选、故障停止与原身份重试。浏览器原沙箱保持开启。当前不再承诺操作系统强制全路径隔离或管理器崩溃窗口全路径零泄漏；旧容器/ACL 日志仍必须安全恢复。实际结果见[148 多开修复](verification/desktop-148-isolation-fix.md)。

历史条件（2026-10-05，已被上述用户调整取代）：Windows底层网络隔离正常工作时，代理掉线、桥崩溃、管理程序崩溃不得使浏览器直连，采用零网络能力 AppContainer、同身份桥及 Job/token 树。相关[首版记录](V1_DELIVERY.md)及失败均保留，不追认为新方案通过。

1. **锁定环境。**核对已保存配置的修订号，取得以环境 ID 为键的独占文件锁，记录 PID、进程创建时间与会话 ID。重复启动返回现有任务或会话信息；环境忙返回 `PROFILE_BUSY`，实际目录被占用返回 `DATA_DIR_LOCKED`。仅有 PID 不足以判断陈旧锁，不能凭锁文件年龄直接删除。
2. **核验内核与目录。**检查实际版本、文件哈希、能力验收状态。为每个环境分配独立 `user-data-dir`，解析后的绝对路径须处于应用数据根目录内，并拒绝通过路径穿越或重解析点指向其他环境。Chromium 的 `Default` 是用户数据目录内部子目录，不能仅靠共享数据根下的不同窗口形成隔离。[Chromium 用户数据目录说明](https://chromium.googlesource.com/chromium/src/+/HEAD/docs/user_data_dir.md)
3. **启动独立代理桥接。**HTTP、HTTPS 上游代理与 SOCKS5 统一由桥接层处理认证，浏览器只连接当前环境的 loopback 端口。HTTPS 表示到代理本身的 TLS 连接时须由桥接层明确支持，不能与“访问 HTTPS 网站的 CONNECT”混同。凭据经本地受保护存储按引用读取，不进入网页、命令行、普通日志或公开导出。
   **当前代理准备检查：**kernel仅接受绑定本次环境/会话/构建的实际通道 owner 和成功同桥前检，缺失则拒绝启动；不创建外层容器。独立检查不替代本次前检，不自动改直连。旧 [T11 容器启动证据](verification/T11-production.md)只对应旧实现。
4. **完成代理前检。**经同一个桥接实例检查连通、认证、出口 IP；按需解析并核对时区。代理不可用、认证失败、配置冲突或桥接异常均阻止启动。UI 中选了“必须代理”的环境不自动改为直连。
5. **启动浏览器并验证。**用参数数组调用已核验可执行文件，不经 shell 拼接；保留浏览器沙箱。确认进程、环境目录和身份配置；进程创建成功本身不等于环境就绪。
6. **发布会话就绪。**代理前检和启动检查均完成才发布 `EnvironmentStateChanged` 的 `running` 状态及操作完成事件。失败要关闭本次产生的进程树与桥接、释放已取得的锁，并保留已有环境数据。

上游 `--proxy-server` 明确不支持密码验证，因此本地桥接是本项目需要开发并验证的部分，不能宣称选定内核已经解决认证。每个环境有独立桥接生命周期；桥接崩溃、上游断线和认证过期进入阻断状态，停止或隔离浏览器网络并提示恢复。

T08编写配置/DPAPI与独立HTTP/HTTPS前检，T09接[`每会话桥接`](../internal/proxy/bridge.go)，T10接SOCKS5并使独立检查也走Bridge链；环境仍自己的新instance同通道重检，不凭历史成功。认证不进参数/普通响应。**T08–T10均未实际验收，不算T11全路径保护通过。** HTTP/SOCKS5不加密认证，HTTPS才TLS到代理；[T08](verification/T08.md)、[T09](verification/T09.md)、[T10](verification/T10.md)。

**当前故障行为须实测。**绑定代理失败应明确失败，应用不配置或执行直连回退；验证实际页面请求、代理中断和原 ID 重试。浏览器代理参数及 QUIC/WebRTC 限制不等于 OS 全路径隔离，本次不以页面请求成功或断线失败推导 DNS/IPv6/所有 UDP 均无旁路。旧全路径验收要求因用户调整退出本轮，不标为通过。

CDP 默认不开启。需要身份探测或本机自动化时，优先采用本地 pipe；采用 TCP 时仅允许 `127.0.0.1`，使用短生命周期动态端口并验证实际监听地址，不向局域网开放。loopback 不等于认证，控制接口还需限制本机调用方；CDP 地址不进入普通列表、日志和公开导出。无法核实绑定范围对外返回 `PROCESS_START_FAILED`，在 `error.details.reason` 中记录 `debug-endpoint-unsafe`。

### T06 正常会话的实际开发边界

[`长期会话`](../internal/kernel/runtime_windows.go)与[`目录锁`](../internal/kernel/profile_lock_windows.go)不复用探测临时profile，不删除正常浏览数据。T06提交时仅显式direct且阻断绑定代理；T09随后接认证桥，实际代理/泄漏保护验收仍待补。实际目录按服务派生UUID引用逐级创建并固定，目录链拒绝WRITE/DELETE、防原地junction转换，复核句柄重解析属性、最终规范化根内归属及锁文件类型/硬链接；既有浏览文件hardlink共享拒绝，分享模式0的实际锁覆盖重复目录打开，不声称防同用户恶意新增别名。

[`pipe`](../internal/kernel/pipe_windows.go)串行整个请求/响应并支持有界写，正常会话控制仅内部使用，不向UI开放任意CDP。短期启动context不控制已经就绪会话；就绪前单独核对主进程存活，不以Job尚未清空代替浏览器活着。正常停止先Browser.close，只有确认Job ActiveProcesses==0才释放pins/锁并报告完成。等待退出超时可重试，控制写端断开则返回不可正常重试的CONTROL_CHANNEL_LOST，不谎称通道可恢复。超时或清理未确认仍保持busy与目标身份，应用退出/失败启动只能终止本次自有Job。不以主PID退出、CreateProcess成功或UI状态作为全树退出/实际隔离证据。

[`runtime_real_test.go`](../internal/workspace/runtime_real_test.go)的实际A/B三存储隔离、停止、服务重开、根故障重试、回收找回及完整恢复已运行；显式direct测试与正式proxy测试分开，不互相冒充。原始证据及分支覆盖见[集中验收](verification/V1-final.md)，人工新页面仍未验证。

### T07 异常与重开的实际开发边界

监督器区分根存活、私有pipe可用及Job资源全部退出；根死亡不释放仍被子进程占用的目录，退出码/崩溃/断管/就绪与停止超时分别记录。主动失败清理有独立意图，保留原始原因。会话状态、任务终态和安全活动以schema4事务保存，存储失败保留保护和待写结果，不重复外部操作。

[`身份核对`](../internal/kernel/runtime_recovery_windows.go)只核对旧PID创建时间、session元数据、实际锁及[`指定Job资源`](../internal/kernel/runtime_job_windows.go)，不获取跨应用pipe，不按PID结束，不依靠锁文件年龄。正常Job使用仅当前SID/SYSTEM可访问的全局session身份、句柄不继承；重开仅取QUERY权，已有同名对象不接管。全局标识避免同用户不同Windows登录会话的Local命名空间差异；仅ActiveProcesses==0或对象确认已经销毁才通过树资源退出检查，主进程已死/应用锁空闲都不能代替。创建及销毁依据[Win32 Job合同](https://learn.microsoft.com/en-us/windows/win32/api/jobapi2/nf-jobapi2-createjobobjectw)，实际实测仍待补。

进入启动阶段但数据库PID0时读取锁元数据确认是否已创建；只有确实未创建才能使用安全中断分支。应用异常退出的kill-on-close不能代替重开后的实际核对；资源版本缺失、查询未知或尚未退出仍busy。指定强制结束只用于当前仍持有的Job、普通关闭已失败的会话；迟到worker结果与持久UPDATE须匹配session。源码及用例存在不代表Windows杀进程/崩溃恢复通过，[待验收](verification/T07.md)。

T18修订重开核对：[`现有目录检查`](../internal/kernel/profile_inspect_windows.go)固定实际目录链、仅`OPEN_EXISTING`读取原锁，不创建目录或锁、不扫描浏览数据；只有持久never-initialized/directory-prepared事实与明确未创建会话匹配时才容许对应缺项，仍查询准确Job。正常Start继续完整数据树校验。避免bootstrap在回滚至旧目录不存在后自行创建未知目录；新增回归未运行，[T18](verification/T18.md)。

### T09 认证通道的实际开发边界

独立TCP4 loopback监听不当作认证。内部前检同时验证当前host的TCP caller与随机token；浏览器用[`反向TCP四元组与准确Job`](../internal/kernel/proxy_guard_windows.go)判定客户端。QUERY副本在CreateProcess前绑定、句柄不继承，不泄漏副本影响kill-on-close；先收敛本次桥接/连接，再关闭副本并释放pins/实际目录，查询失败不改为放行或关闭沙箱。[D010边界](DECISIONS.md#d010--每会话代理通道只接纳其受控调用进程2026-10-01)不声称抵御管理员/恶意同SID注入或主动加入Job。

正常启动固定 `--proxy-server=http://127.0.0.1:<private-port>`、`--proxy-bypass-list=<-loopback>`，没有上游凭据/token或DIRECT备选；QUIC和非代理WebRTC UDP受限，但不能以参数替代T11实际泄漏验证。标准HTTP、CONNECT、HTTPS到代理TLS分别处理，明文HTTP Upgrade拒绝；同桥前检持久成功后才创建/报告进程就绪。

创建成功而首次时间读取失败也返回准确Job所有者和资源，不能只关原Job句柄就释放锁。记录实际PID及明确空时间异常；未确认全树退出仍busy。重开空时间不打开/控制barePID，只在Job空、实际锁空闲且原session元数据对应时证明旧树已退。当前均源码结论，[T09待验收](verification/T09.md)，没有实际API/普通用户/网络/浏览器证据。

### T10 SOCKS5与DNS边界

[`SOCKS5`](../internal/proxy/socks5.go)在T09准入/生命周期内仅协商保存单一方法，不降级。CONNECT域名以IDNA DOMAINNAME交上游，IPv4/IPv6按字节；只拨保存代理，代理host自身本机解析与目标DNS分开。完整BND帧不当出口IP，IPv6支持以实际回复为准。[D011](DECISIONS.md#d011--socks5目标域名固定远端解析且认证不降级2026-10-01)。

HTTP经目标流写origin-form，HTTPS经同本机CONNECT透传TLS；SOCKS5本身不加密认证。独立检查临时Bridge，环境自己的新桥同通道前检；只有安全策略/ID/修订/阶段进报告，RPC不能覆写DNS或直连。UDP ASSOCIATE/BIND未支持，明文HTTP Upgrade仍阻断。

无真实SOCKS5/DNS/Windows浏览器或新页面证据，[T10清单](verification/T10.md)。远端目标解析不证明后台DNS、UDP/QUIC、WebRTC或WebSocket/重连无泄漏；T11须后端关闭不满足路径/阻止运行并补实际证据，不能只凭参数/前检。

### T11 正式接入与剩余验收

正式provider现已接入；以下早期门禁/实验段落保留历史背景，当前状态以[正式接入](verification/T11-production.md)及[故障恢复](verification/T11-recovery.md)为准。目录运行锁仅userdata根允许WRITE共享以兼容Chromium原子写Local State，维护目录仍全链严格锁；不声称分享锁可阻止同用户在运行根原地修改reparse。

[`每桥故障监视`](../internal/proxy/bridge_watch.go)闭锁第一个故障，Failed事件交准确Job持有者独立安全结束本树，不等DB；单个请求取消或客户端上传异常只关闭该请求。30s同bridge巡检并非全路径隔离；确认桥/全树gone才收敛完整Done。停止失败保持pins/目录与准确会话，未终结Stop不释放后来Start的预留。

[`networkFault`](../internal/workspace/runtime_network_fault.go)保存network_error/安全根因/时刻/清理阶段，未返回process也记录闭锁故障；清理/重开不抹根因、不重建旧端口。当前[`门禁`](../internal/kernel/network_protection.go)只接受本次实际正式owner和同桥前检，缺失或未知拒绝，不能仅凭参数/API/管理员授权解除。

用户允许未来组件管理员安装，未现在提权/改系统。WFP独立程序路径可研究socket隔离，但ALE_ORIGINAL_APP_ID只定义连接重定向，不保证DNS Client委托查询，dynamic过滤生命周期也不保证host崩溃的拒绝边界；不能全局封DNS影响其他环境。[D012](DECISIONS.md#d012--安全边界缺失先阻止代理启动并允许隔离组件管理员安装2026-10-01)、[待完成清单](verification/T11.md)。完整组件/真实出口和故障验收均未实现通过，用户已选择先保存部分继续不依赖它的其他票。

2026-10-05增量：独立实验已证固定148在零能力AppContainer内正常桌面/原renderer限制、同package socket桥、委托DnsQueryEx局部拒绝、跨新SID三种合成存储及独立桥硬退出后的端口接管拒绝。用户另授临时非交互窗口站ACL和管理员只读采集；最终权限/资源已清理。只读结果显示本机BFE/MpsSvc宿主critical=true、相关运行期默认规则无persistent/boottime标志，不能替代系统故障窗口验证。[实验](verification/T11-feasibility-observations.json)与[系统只读观测](verification/T11-system-boundary-observations.json)都不是生产provider。随后仅新增一次BFE标准正常停止并立即恢复的明确授权；拒绝即结束，其他服务/宿主故障及系统改动未授权，实际执行位置见[T11记录](verification/T11.md)。

同日[`资源收尾源码`](../internal/kernel/runtime_lifecycle_windows.go)：根信号和准确Job空的读回成功后，只缓存“进程已退”；通道Close锁外成功后才释放pipe/guard/pins/目录并发布完整Done。失败仍能正常Stop重试清理，已退出时不再发CDP或操作已释放Job；在途尝试共享，未知查询不作成功。未创建process时通道仍由创建方收敛；真实ManagedProcess与workspace不重复拥有它。7内核+8服务回归及静态编译不是实际目录/恢复或OS隔离验收。

15:51获准的BFE正常停服实验被提升`OpenServiceW`以错误5拒绝组合QUERY/START/STOP权限申请，实际STOP/START均0次；所有采样仍在RUNNING状态。[结果与摘要](verification/T11-bfe-stop-observations.json)不能证明保护组件故障期间的隔离，当前管理员授权也不能解锁provider；完整代理门禁继续保持。

## 请求与返回合同

### T13 创建/克隆的空目录准备（源码已实现，未运行验收）

[`PrepareEmptyProfile`](../internal/kernel/empty_profile_windows.go)只消费服务已持久准备的plan/index/环境UUID/规范引用，host内部root不由RPC输入。父到子READ分享目录pins固定路径对象，独占创建/读回userdata外`.prism-batch.json`并Flush、保留同一marker句柄至SQL事务终结；拒reparse/可见硬链接/外来或坏标记，不复制源浏览数据、不创建userdata内runtime锁。既有目录只同journal且空集合才可续作，环境/项完成事务前再次用新枚举句柄核对可见空集合；不清空、覆盖或删除未知/非空目录。

空集合观测不阻止同SID外部创建子文件，Mkdir→pin也不是原子对象绑定；不称恶意本机writer强隔离。新envRoot已建但marker未完整持久时退出会留下准备残留，只保留拒认领，后续垃圾回收需独立规则。配置复制重新编译原精确构建并生成新seed，pending仍不可启动；批次不启动浏览器，不绕T11门禁。真实目录、重开/取消/资源故障证据尚未取得，[T13清单](verification/T13.md)、[D014](DECISIONS.md#d014--批次冻结计划逐项提交与空目录归属2026-10-01)。

### T12 窄Cookie控制与固定148语义

[`Cookie能力`](../internal/kernel/cookies_windows.go)仅从原准确ManagedProcess调用自己的匿名pipe，固定Storage.get/set/clearCookies，不开放通用CDP RPC、调试端点、浏览器数据库或任意contextId。根browserpipe的默认context属于该自有浏览器；应用sessionId不能当CDPcontext。每条读取→已匹配则不重写→单条set→完整get组合保持同commandGate，单命令超时后不能假称没有副作用。

读回包含值（仅内部）、domain前导点/路径/分区两成员及Secure/HttpOnly/SameSite缺省/Session/原Unix秒有效期。nonce不可序列化分区可能报告partitionKeyOpaque=false，presence仍排除普通Cookie匹配。原始响应不回显，不截断大集合计成功；2MiB帧上限会明确返回读回失败。

固定源码Storage.setCookies受理不以每条访问结果判成功，约400天等有效期限制导致读回差异不能计完整核对。writer不附URL，因为HTTPS URL会隐式改secure。写前拒绝可被URL规范化改成另一键的非规范path/host，以及多个域前导点、private PSL域Cookie；IPv6 Cookie域暂无闭合writer形态，明确不支持而非显示有效。JSON0与Netscape0时间语义分离，空value/session不改为默认未来时间。[官方依据/边界](verification/T12.md)、[D013](DECISIONS.md#d013--cookie命令只作用于指定会话重试先核对同键2026-10-01)。

Cookie空白用途仍在原Runtime.Start链，仅本次不恢复标签和URLs，保留修订/seed/精确内核/数据引用。代理入口不在页面永久禁用，仍由后端逐会话保护；直连仅未绑定且明确确认时可请求。清空明确选择、只作用当前context，失败重试合并不重清。双真实代理会话写后读取及A/B独立已验证；尚未实测的分区组合与人工UI保持待验，见[报告](verification/V1-final.md)。

### T15 完整导出的实际目录边界（源码已编写，未运行验收）

备份先取得原冻结环境ID的独立维护owner，阻重新启动/新Cookie写入；只用原正常Stop等待准确全树及持久观测，不按PID操作或升级ForceStop。独立[`初始化事实`](../internal/workspace/data_initialization.go)区分受理与launcher获得建立数据许可，旧记录迁移未知不推断空环境；停止后重读原ID，只有明确never允许缺目录，备份不通过创建空目录消除异常。

[`CaptureProfile`](../internal/backup/profile_windows.go)父到子固定实际路径对象、独占已有runtime.lock、固定每个文件READ/shareREAD至复制、集合/对象复核和发布；拒reparse/可见hardlink/变化，保留空目录，不截断或删除源文件，不调用会EnsureDirectory的运行检查来读取未初始化状态。文件固定/空集合并不保证排除任意恶意同SID新增子条目，不能宣称强隔离。

真实浏览数据和一致SQLite副本进入独立native包，逐文件SHA全量读回/Sync成功后以原输出文件句柄同目录不覆盖rename；不携带内核程序，清单保存精确版本/archive/executable hash，pending无捏造hash。DPAPI原ref/密文保存不称登录便携，浏览数据仍敏感。后续实际恢复须同用户上下文/准确内核与实际重开证据；T15静态和源码评审不证明这些已通过，[清单](verification/T15.md)、[D015](DECISIONS.md#d015--一致本机备份范围闭包与先核对再发布2026-10-01)。

以下均是**面向 UI 的目标外部应用接口示例**，以 [DEVELOPMENT.md 的本地应用接口](DEVELOPMENT.md#6-本地应用接口) 为统一契约；T04/T05/T06实际已接接口以该文档的增量说明及源码为准，T06尚未运行验收。成功返回 `{ ok: true, data, operationId? }`，失败返回 `{ ok: false, error: { code, message, retryable, details? }, operationId? }`。ID 均为虚构示例；应用后端按 ID 解析内核、数据目录与凭据，前端不得提交任意可执行文件路径。

生成预览请求；该调用不保存环境：

```json
{
  "method": "Fingerprint.Generate",
  "requestId": "request-example-001",
  "kernelId": "kernel-selected-by-user",
  "templateId": "windows-desktop-v1",
  "overrides": {
    "cpuCores": 8,
    "uiLanguage": "zh-CN",
    "acceptLanguages": ["en-US", "en"],
    "timezone": "America/New_York"
  }
}
```

演示预览返回必须保留证据边界；没有新的环境修订号，也不能被真实启动方法接受：

```json
{
  "ok": true,
  "data": {
    "mode": "demo",
    "previewProfile": {
      "id": "preview-example-001",
      "seed": 1256789,
      "generatorVersion": "windows-local-v1",
      "schemaVersion": 1,
      "platform": "windows"
    },
    "capabilityReport": {
      "kernelEvidence": {
        "coreActualVersion": null,
        "coreExecutableSha256": null,
        "status": "not-probed"
      },
      "observedFingerprint": null,
      "canLaunchNative": false
    }
  }
}
```

真实预览在 `data` 中使用 `mode: "native"`，补齐核验过的内核版本与 64 位十六进制 SHA-256，并返回全部解析后的显式字段和配置哈希。仅依据源码推导的字段使用 `source: "source-derived"`；探测回读使用 `source: "observed"` 及采样时间。没有探测时 `observedFingerprint` 保持 null。native 预览同样不会自动保存。

提交使用 `Fingerprint.CommitRevision({ environmentId, preview, expectedRevision, requestId })`；`preview` 为上一步完整预览，服务端重新校验其生成版本、内核证据和配置哈希，不能信任客户端任意改写。成功返回 `{ ok: true, data: { newRevision } }`，失败保留旧档案。演示预览仅允许提交到演示存储；真实服务拒绝演示数据，返回 `CAPABILITY_UNSUPPORTED`，原因设为 `demo-only`。

启动读取已保存环境的代理策略、内核和指纹，不接收临时预览或覆盖项。`Runtime.Start` 先返回包含 `operationId` 的成功受理结果；受理不代表浏览器已经运行。以下为请求及该操作的失败结果示例：

```json
{
  "method": "Runtime.Start",
  "requestId": "request-example-002",
  "environmentId": "env-example-001"
}
```

```json
{
  "ok": false,
  "operationId": "operation-example-002",
  "error": {
    "code": "PROXY_UNREACHABLE",
    "message": "代理连接失败，环境未启动。请检查代理后重试。",
    "retryable": true,
    "details": {
      "environmentId": "env-example-001",
      "stage": "proxy-preflight",
      "directFallbackAllowed": false
    }
  }
}
```

真实启动完成后，通过 `Runtime.Inspect` 的 `data` 及会话事件提供 `mode: "native"`、`sessionId`、`environmentId`、`state: "running"`、`revision`、PID、开始时间、内核核验摘要、代理检查摘要及各项检查状态。事件关联同一 `operationId` 并遵守开发方案的单调序号规则。CDP 地址仅通过受限内部通道返回。所有成功检查必须关联本次会话，不能复用上次的成功标记。

外部 `error.code` 使用开发方案中的统一词表。内核专有原因放入脱敏后的 `error.details.reason`；前端只处理统一错误码，不依赖适配器内部异常名称。

| 外部错误码                                               | 内核相关原因及处理                                                                                      |
| -------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| `VALIDATION_FAILED` / `REVISION_CONFLICT`                | 返回有问题的非敏感字段；保持旧配置，刷新后再提交                                                        |
| `CAPABILITY_UNSUPPORTED`                                 | 字段不支持、构建未验收或演示数据进入真实服务；原因分别为 `field-unsupported`、`not-probed`、`demo-only` |
| `KERNEL_MISSING`                                         | 固定内核未安装；资产不可获取时附 `asset-unavailable`，不改用其他版本                                    |
| `KERNEL_INTEGRITY_FAILED`                                | 文件哈希或身份不匹配；分别记录 `hash-mismatch`、`identity-mismatch`，隔离构建并阻止启动                 |
| `PROFILE_BUSY` / `DATA_DIR_LOCKED` / `PATH_OUTSIDE_ROOT` | 保留原会话和目录，明确忙状态、锁冲突或不合法路径                                                        |
| `PROXY_AUTH_FAILED` / `PROXY_UNREACHABLE`                | 启动前阻断；分别报告认证或连通失败                                                                      |
| `PROXY_ROUTE_LOST`                                       | 运行中转入阻断流程；不直连回退                                                                          |
| `PROCESS_START_FAILED` / `PROCESS_READY_TIMEOUT`         | 清理本次资源并保留用户数据；调试端点范围不安全时附 `debug-endpoint-unsafe`                              |
| `STORAGE_WRITE_FAILED` / `DISK_FULL`                     | 快照写入失败或空间不足时不进入升级；附对应阶段                                                          |
| `RESTORE_INCOMPLETE`                                     | 升级后需要回滚或恢复未完成；保持环境锁定，完成完整快照恢复后解锁                                        |

### T19 回收目录边界（源码已编写，未运行验收）

回收仅对已停止且退出确认的明确环境；CaptureProfile保存原树身份/文件清单，实际移动仍检查全部路径、重解析、可见硬链接和占用。移入与找回保持原目录对象与原精确身份，目标冲突不覆盖。共享[`rename`](../internal/backup/rename_windows.go)合并父路径pins，仅目标父对象允许write sharing以兼容系统隐式目标写访问，仍禁止delete sharing；临时输出创建和rename均用NT RootDirectory句柄及单leaf名称，不沿可写父路径重新创建。源数据遍历至最终核对保持严格pins，之后释放多余pins，仅按已固定对象句柄发布/移动；逻辑维护预约仍保持。覆盖备份发布和恢复/回收移动，父目录备份输出有专用回归。

[`永久删除`](../internal/backup/delete_windows.go)只消费已确认回收日志，原运行锁另记身份/摘要，不以文件名授予删除权。全部已存在对象验证并持DELETE句柄后才开始；部分重试可容许已授权缺项，变化/未知项不收编。只读文件先拒绝，根对象关闭后确认消失才成功。仍不承诺防恶意同SID新增子条目的强隔离或擦除既有备份/恢复副本；[T19](verification/T19.md)的Windows目录、硬退出及浏览器原数据读回均尚未执行。

## 内核升级与完整恢复

T20已接完整备份→独立副本旧/新试用→正常停止→明确切换；窄诊断仅自有pipe和受控合成三存储，不替代网站兼容或独立出口。正常结束要求准确Job全退且退出码0，取消只清理准确自有Job；WorkIdentity的缺锁恢复不推广普通环境。代理副本现共用正式保护链，真实148→150、切换失败回滚、成功切换及完整备份回退重开已有[运行证据](verification/T20-protected-observations.json)。实际硬中断与其余边界见[集中报告](verification/V1-final.md)。

T17本地源码加入同卷目录恢复：[`目录对象核对/移动`](../internal/backup/switch_windows.go)持有已验证根和完整可见文件集合，拒绝reparse/可见hardlink/未知对象；采用实际目录身份识别rename结果。旧目录留在本次previous，日志DB提交标记决定旧状态回滚或完整新状态确认。归档锁不导入，新锁不保存旧PID/session；实际安装目录不能继续标never-initialized。此处是源码边界，非恶意同SID新增写入的强隔离保证，实际完整浏览数据/精确内核重开待[T17验收](verification/T17.md)；T18接续硬中断恢复。

升级须先停止目标环境并确认进程树退出，取得环境锁，再制作完整、校验可恢复的文件系统快照。快照包括整个 `user-data-dir`、配套环境元数据、指纹配置、旧内核标识与配置版本；单独导出 Cookie 不构成完整快照。

新内核使用快照的工作副本执行迁移与验证，原目录保持可恢复。成功后原子切换环境当前目录和内核引用；失败时关闭新进程，从升级前完整快照恢复目录并绑定原版本配置与旧内核。**不得让旧内核直接打开已被新内核升级过的 profile。**

恢复范围是快照时刻，升级后产生的新登录状态或数据不会自动合并回旧快照。UI 在 `/#/kernels` 展示快照时间、目标环境和恢复影响，完成后记录恢复结果。该方案首先保证同一台 Windows 机器上的恢复；跨机器迁移需要另行验证本地加密数据兼容性。

## 实现完成条件

| 验收对象   | 必须看到的证据                                                                   |
| ---------- | -------------------------------------------------------------------------------- |
| 固定配置   | 同一环境重启仍使用同一 seed、生成版本与显式参数；读取详情不改变修订号            |
| 参数生效   | 用自有诊断页与回显服务核对允许字段、请求头和 Client Hints；不做隐蔽性评分        |
| 数据隔离   | 两环境并行操作时，Cookie、站点存储、缓存、历史与下载目录按设计独立；重复启动互斥 |
| 代理       | 分别验证 HTTP、HTTPS 上游和 SOCKS5 认证；前检失败不开环境，断线不直连            |
| 生命周期   | 手动关闭、管理器退出、崩溃恢复后进程与锁正确清理；无其他环境受影响               |
| 版本与恢复 | 实际版本/哈希匹配，身份字段满足该构建规则；用完整快照成功升级与恢复              |
| 证据显示   | 演示、源码推导、真实采样三种数据来源可区分；未经测试项目保持待验收               |

本合同的完成状态是“可实施的适配设计”。只有取得上述真实证据，才能把对应能力从待实现改为已验证。内核分发还需随对应构建保留适用许可证与第三方声明；本仓库未附带内核二进制。[fingerprint-chromium 许可证](https://github.com/adryfish/fingerprint-chromium/blob/main/LICENSE)
