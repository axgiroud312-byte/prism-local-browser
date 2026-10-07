# #34 实际源码挂载截图

[窗口接入记录](../../verification/issue34-integration.md) · [实现图库](index.html) · [测量与 SHA-256](manifest.json)

**8 个状态 × 2 视口，16 张全新截图。** 来自正式 `src/App.tsx` 的 Vite 页面，没有外部源码变换、旧 drawer 包新窗口或第二套 Cookie/移除 JSX。不是此前 #34 外部预览截图的重复验收，不代表 #37 或真实桌面完成。

所有图为合成数据，1440×900 / 1280×800 内容视口、DPR 1、100% 缩放、zh-CN / Asia-Shanghai。实现代码 `b3c6af0`；最终取证在同步 #35 模块后按源码 `12303a1` 重拍，完整提交见 manifest；不因此宣称 #35 最终挂载已完成。原参考 PNG、商业资源和私人并排图只留仓库外。

| 图片 ID | 冻结参考 ID | 对照性质 / 差异 |
| --- | --- | --- |
| `environment-create` | `environment-create` | 660px 连续右 drawer；支持字段裁剪，创建数量在基础区，Cookie 走独立流程 |
| `environment-fingerprint` | `environment-create-fingerprint` | 同一正文滚动，固定 footer；只用服务内核/稳定 seed/支持偏好，不仿原任意设备参数 |
| `environment-edit` | `environment-edit` | 同容器；保存/取消/换一套沿用原服务与草稿 |
| `environment-save-failed` | `environment-edit` | **映射**；未捕获同名原写入失败画面，实际 demo 失败保留草稿 |
| `dirty-confirm` | `group-delete-confirm` | **400px 容器映射**；原参考没有同名未保存确认，安全文案使高 163px |
| `force-confirm` | `group-delete-confirm` | **400px 容器映射**；环境/会话 ID 与影响说明使高 217px，不声称原强制结束画面已找到 |
| `demo-cookie-preview` | `proxy-import-preview` | **1050px 文本/说明/两项安全预览映射**；JSON/Netscape，不支持 ZIP，不能当作代理界面业务 |
| `demo-remove-confirm` | `environment-delete-confirm` | 400px；准确两项、示例记录与 Cookie，不操作真实文件；无假真实数据删除 checkbox，高 181px |

每个 ID 在 [1440×900](1440x900/environment-create.png) 和 [1280×800](1280x800/environment-create.png) 子目录各有一张。图库包含全部 16 张，可直接查看。

## 实测几何与裁剪

- drawer：x780 / 620、y40、宽660、高852 / 752，header40、body741 / 641、footer71、底距8；rail x739 / 579、宽40。仅正文滚动，点击 backdrop 不关闭。
- 新建默认正文 scrollHeight1450，编辑1370；服务不支持的商业字段裁剪，不用空白补成原3649px。指纹分区实拍 scrollTop709 / 809，底部夹取由实际内容长度决定，不宣称等于原1616。
- 400px 确认保留图标、标题、正文和固定 footer；dirty / force / demo 移除分别高163 / 217 / 181。原短确认约152.86px；额外安全说明及精确 ID 是公开差异，不虚称逐像素相等。
- Cookie 实拍1050×609.72，文本/说明/预览分区来自冻结同类容器。输入只有 `SYNTHETIC` 与空值，预览不显示值。
- 16张全部实际读取并人工核对；仓库外16组仅 PNG 的并排裁剪也已读取。最终重拍后12张SHA/几何不变，复用已读核对；4张变化图（两视口指纹/force）及其参考裁剪再次读取，容器几何不变。无原代码运行或原参考复制到本目录。

页面错误、console error、非允许请求均0；两张 force 警告截图在未确认状态，强制请求0。PNG尺寸、DPR、缩放和SHA-256逐张核对。视觉记录与点击检查独立，尚缺同名执行画面不算直接1:1证据。
