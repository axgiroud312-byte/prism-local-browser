# 第三方软件与来源声明

本仓库的 [MIT License](LICENSE) 适用于本项目原创代码与文档。第三方代码、字体、图标、依赖、浏览器内核及其组件仍适用各自许可证；本文件不将它们重新许可为 MIT。

## 前端直接依赖

下表根据当前安装包的 `package.json` 与许可证文件记录。实际安装版本由 [package-lock.json](package-lock.json) 固定；升级依赖时应重新核对许可证与通知。

| 软件                                                         | 本次记录版本 | 许可与归属                                                        | 使用方式                       |
| ------------------------------------------------------------ | ------------ | ----------------------------------------------------------------- | ------------------------------ |
| [React](https://github.com/react/react)                      | 19.3.0       | MIT；Meta Platforms, Inc. and affiliates                          | 页面组件及状态管理。           |
| [React DOM](https://github.com/react/react)                  | 19.3.0       | MIT；Meta Platforms, Inc. and affiliates                          | 浏览器 DOM 渲染。              |
| [Lucide React](https://github.com/lucide-icons/lucide)       | 1.49.0       | ISC；Lucide Icons and Contributors；部分 Feather 衍生图标使用 MIT | 界面图标；完整上游通知见下文。 |
| [react-markdown](https://github.com/remarkjs/react-markdown) | 10.1.0       | MIT；Espen Hovlandsdal                                            | 指南文档的 Markdown 渲染。     |
| [remark-gfm](https://github.com/remarkjs/remark-gfm)         | 4.0.1        | MIT；Titus Wormer                                                 | Markdown 表格等 GFM 语法。     |

下方保留五个直接依赖许可证，包括Lucide/Feather归属。首版候选另由[collect-frontend-notices.mjs](scripts/collect-frontend-notices.mjs)核对锁文件和安装版本，汇集107个生产直接/间接包以及Vite注入的预加载helper/polyfill许可（共108包），随包保留`FRONTEND-THIRD-PARTY-NOTICES.txt`。Vite helper适用MIT，2019-present VoidZero Inc. and Vite contributors；不因Vite是dev依赖而遗漏。CLI/测试工具不进入产品；缺许可或版本不一致使构建失败。该文件与实际Go依赖通知、NSIS许可和本项目LICENSE共同分发。

## 开发与构建工具

本次使用 TypeScript 7.0.2（Apache-2.0）和 Vite 8.3.1（MIT），以及类型声明等开发依赖。工具本体通常由 npm 安装，不以本仓库 MIT 条款重新授权。完整依赖版本见锁文件，各安装包附带的许可文件为核对入口。

- [TypeScript 许可证](https://github.com/microsoft/TypeScript/blob/main/LICENSE.txt)
- [Vite 许可证及第三方通知](https://github.com/vitejs/vite/blob/main/LICENSE)

T01 新增仅用于测试的 [Playwright Test](https://github.com/microsoft/playwright) 1.63.0（Apache-2.0；Microsoft Corporation）和 `@types/node` 26.6.3（MIT；DefinitelyTyped contributors）。Playwright 自带的测试 Chromium 由 `npx playwright install chromium` 安装到开发/CI 缓存，不进入应用分发，也不是产品指定的 fingerprint-chromium。保留 [Playwright 许可](https://github.com/microsoft/playwright/blob/main/LICENSE) 与实际包内通知，具体依赖版本以锁文件为准。

T11可行性调查仅在项目忽略工具目录使用 [Capstone](https://github.com/capstone-engine/capstone) **5.0.6** 静态读取固定自有内核的崩溃位置。实际包内 `LICENSE.TXT` 为 BSD-3-Clause，Copyright (c) 2013 COSEINC，设计实现 Nguyen Anh Quynh；不进入应用、实验exe或安装包依赖，未重新分发其库。许可入口为 [5.0.6 LICENSE.TXT](https://github.com/capstone-engine/capstone/blob/5.0.6/LICENSE.TXT)。

## Go/Wails 桌面底座（T02）

固定版本见 [go.mod](go.mod) 和 [go.sum](go.sum)。以下信息来自本机实际下载模块的许可证，不由本项目的 MIT 重新授权：

| 组件 | 固定版本 | 适用许可 |
| --- | --- | --- |
| Go runtime / standard library | 1.27.1 | BSD-3-Clause；The Go Authors |
| Wails | v2.16.0 | MIT；2018–Present Lea Anthony |
| modernc.org/sqlite | v1.60.1 | BSD-3-Clause；2017 The Sqlite Authors；SQLite 本体另保留其 public-domain 声明 |
| Wails Go WebView2 | v1.0.22 | MIT；John Chadwick、Serge Zaitsev；webviewloader 子组件 ISC，John Chadwick / Wails Project Developers |

`npm run build:windows` 使用 [collect-go-notices.ps1](scripts/collect-go-notices.ps1) 从固定 Windows production 依赖图提取**实际参与构建**的 Go 模块许可证、NOTICE 与子组件条款（包括 modernc libc 的第三方通知），输出 `build/bin/GO-THIRD-PARTY-NOTICES.txt`。缺少许可文件会使构建步骤失败；分发 exe 时必须同时保留该文件、本文件和本项目 LICENSE。开发 CLI/测试依赖不因此变成产品运行时组件。

代理IDNA与Cookie公共后缀使用已有固定`golang.org/x/net v0.56.0`（BSD-3-Clause，The Go Authors）；版本/go.sum未升级，许可按实际依赖图保留。Windows DPAPI、AppContainer、Job与ACL为系统API，不新增或复制第三方隔离组件。

Windows WebView2 Runtime 为微软单独许可的外部先决条件，T02 使用机器上已安装的 Runtime，没有复制其运行时安装包或将其重新许可为 MIT。后续安装包须说明其检测/安装方式和适用条款。WebView2 是桌面壳，**不是产品指定的 fingerprint-chromium**。

### Wails 许可证全文

```text
MIT License

Copyright (c) 2018-Present Lea Anthony

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

### modernc.org/sqlite 许可证全文

```text
Copyright (c) 2017 The Sqlite Authors. All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice, this
list of conditions and the following disclaimer.

2. Redistributions in binary form must reproduce the above copyright notice,
this list of conditions and the following disclaimer in the documentation
and/or other materials provided with the distribution.

3. Neither the name of the copyright holder nor the names of its contributors
may be used to endorse or promote products derived from this software without
specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

## NSIS 开发预览安装程序（T03）

固定 NSIS **3.13**；官方 [便携 ZIP 发行元数据](https://sourceforge.net/projects/nsis/files/NSIS%203/3.13/) 的 SHA-256 与实际下载一致：`ba63dffc4410ee89193e1cb5a41989991bd77c61068da17e3156d136b7b0b3d8`。仅下载到 `.tools/`，不分发完整编译工具。

安装/卸载 stub、System/nsDialogs 插件及 Modern UI 按 NSIS 的 **zlib/libpng** 条款使用，Copyright (C) 1999–2026 Contributors。本项目的安装脚本独立编写，不把 NSIS 引擎重新许可为 MIT。压缩显式选择 zlib；**没有链接 bzip2 或 LZMA 压缩模块**，其各自许可证不被误标为本安装器正在使用。官方 `COPYING` 全文随每个程序版本以 `NSIS-LICENSE.txt` 保留（其中包含上游其他可选压缩模块说明）；来源 [NSIS 许可](https://nsis.sourceforge.io/License)。

微软 WebView2 仍为外部运行先决条件：安装器检测并提示官方 Evergreen Runtime 获取方式，不复制安装器或静默下载微软运行时，也不在卸载时删除它。fingerprint-chromium 不随本开发预览安装包分发。

## 外部浏览器内核来源

**2026-10-08 新的直接运行桌面包：** 用户要求内置148，新 `build/bin/bundled/` 随附未修改的官方148.0.7778.215 ZIP，摘要为下文固定值；程序首次打开经原安装和真实探测合同登记。随包保留该tag的 `LICENSE.fingerprint-chromium.txt`、Chromium该版本的 `LICENSE.chromium.txt`，以及由精确主程序（SHA-256 `1867319e56bcabbc4681d8575c002106ce7b61b5290dc5eb34a37676805f6915`）实际 `chrome://credits` 导出的完整 `CHROMIUM-CREDITS.html`。归档资源不删改；本项目MIT不覆盖这些组件。下文“两版不随候选分发”描述历史候选，旧NSIS安装包不因本次直接运行包增量改变。

产品指定使用 [adryfish/fingerprint-chromium](https://github.com/adryfish/fingerprint-chromium)。148.0.7778.215归档SHA为`9ef3f471b7a6641b4224532522b29141ce3746e27d55788d88e2fd951f362579`；150.0.7871.186在10月5日已获取，实算SHA为`4d549c326e51ebbabf562fd365eb5380d9d4a81200da2c60f075c688d9a77e03`并真实迁移验证。**两版均不随首版候选重新分发。** 用户明确选择官方精确ZIP或可信本地ZIP，保留原包资源/组件；版本许可范围不能从148文本推导为150全部组件已获再分发许可。

如后续下载、修改或再分发内核，应检查所选具体版本的来源、许可证、Chromium 组件及第三方通知，并随分发材料保留要求的声明。不得以本仓库使用 MIT 为由认定整个内核、所有组件或相关品牌也适用 MIT。内核版本与许可审查应和构建产物一起记录。

148 tag的[上游LICENSE](https://github.com/adryfish/fingerprint-chromium/blob/148.0.7778.215/LICENSE)为BSD-3-Clause，归属The ungoogled-chromium Authors；全文保留如下。它仅覆盖该项目适用的代码，**不覆盖全部Chromium组件**。Chromium本体及各组件另见[Chromium许可](https://chromium.googlesource.com/chromium/src/+/148.0.7778.215/LICENSE)与选定浏览器内置`chrome://credits`。若将来重新分发内核，应提取并随附该具体包的完整组件通知；本票没有声称一份BSD文本可替代这些通知，也没有把第三方品牌许可包含其中。本地归档來源不明时必须用户明确确认可信，不能由文件名推导已获再分发授权。

```text
BSD 3-Clause License

Copyright (c) 2015-2026, The ungoogled-chromium Authors
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice, this
   list of conditions and the following disclaimer.

2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.

3. Neither the name of the copyright holder nor the names of its
   contributors may be used to endorse or promote products derived from
   this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

## 交互参考与原创边界

本项目从多环境管理的产品需求重新编写界面与业务演示逻辑。旧页面只作为信息结构和交互讨论的参考；没有在本仓库复制商业浏览器的发布代码、品牌图片、商业图标、云接口、用户资料或旧归档。

本仓库没有引入 Ant-Browser 源代码。参考一个公开可见项目，不代表取得复制其实现的许可；若未来引入相关代码，应先确认具体文件和版本的授权条件，并补充相应归属。

## 运行时直接依赖的许可证全文

以下文本来自本次实际安装的依赖包，按包分别保留。

### React

```text
MIT License

Copyright (c) Meta Platforms, Inc. and affiliates.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

### React DOM

```text
MIT License

Copyright (c) Meta Platforms, Inc. and affiliates.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

### Lucide React

```text
ISC License

Copyright (c) 2026 Lucide Icons and Contributors

Permission to use, copy, modify, and/or distribute this software for any
purpose with or without fee is hereby granted, provided that the above
copyright notice and this permission notice appear in all copies.

THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

---

The following Lucide icons are derived from the Feather project:

airplay, alert-circle, alert-octagon, alert-triangle, aperture, arrow-down-circle, arrow-down-left, arrow-down-right, arrow-down, arrow-left-circle, arrow-left, arrow-right-circle, arrow-right, arrow-up-circle, arrow-up-left, arrow-up-right, arrow-up, at-sign, calendar, cast, check, chevron-down, chevron-left, chevron-right, chevron-up, chevrons-down, chevrons-left, chevrons-right, chevrons-up, circle, clipboard, clock, code, columns, command, compass, corner-down-left, corner-down-right, corner-left-down, corner-left-up, corner-right-down, corner-right-up, corner-up-left, corner-up-right, crosshair, database, divide-circle, divide-square, dollar-sign, download, external-link, feather, frown, hash, headphones, help-circle, info, italic, key, layout, life-buoy, link-2, link, loader, lock, log-in, log-out, maximize, meh, minimize, minimize-2, minus-circle, minus-square, minus, monitor, moon, more-horizontal, more-vertical, move, music, navigation-2, navigation, octagon, pause-circle, percent, plus-circle, plus-square, plus, power, radio, rss, search, server, share, shopping-bag, sidebar, smartphone, smile, square, table-2, tablet, target, terminal, trash-2, trash, triangle, tv, type, upload, x-circle, x-octagon, x-square, x, zoom-in, zoom-out

The MIT License (MIT) (for the icons listed above)

Copyright (c) 2013-present Cole Bemis

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

### react-markdown

```text
The MIT License (MIT)

Copyright (c) Espen Hovlandsdal

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

### remark-gfm

```text
(The MIT License)

Copyright (c) Titus Wormer <tituswormer@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining
a copy of this software and associated documentation files (the
'Software'), to deal in the Software without restriction, including
without limitation the rights to use, copy, modify, merge, publish,
distribute, sublicense, and/or sell copies of the Software, and to
permit persons to whom the Software is furnished to do so, subject to
the following conditions:

The above copyright notice and this permission notice shall be
included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED 'AS IS', WITHOUT WARRANTY OF ANY KIND,
EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
```
