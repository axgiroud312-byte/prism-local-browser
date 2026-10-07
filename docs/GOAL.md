# 棱镜浏览器当前执行规则

更新：2026-10-06（Asia/Shanghai）。本文件记录当前用户目标与执行范围；实现状态见 [PROGRESS.md](PROGRESS.md)，实际验收见 [ACCEPTANCE.md](ACCEPTANCE.md)。

## 当前目标

产品围绕独立浏览器环境、一键生成并保存指纹、代理导入或直连、不同版本的 fingerprint-chromium 内核选择、多实例与分组管理组织日常操作。主要参考 [Ant Browser](https://github.com/black-ant/Ant-Browser) 的核心操作方式，现有六页布局不是必须保留的目标。

- [#28 精简 CI](https://github.com/axgiroud312-byte/prism-local-browser/issues/28)：日常只运行浏览器自动化点击测试，直接启动 Vite，不重复构建。
- [#29 核心操作纠偏](https://github.com/axgiroud312-byte/prism-local-browser/issues/29)：按 Ant Browser 收敛核心操作，具体范围和验收以该 issue 为准。

执行本次用户指派的 issue；完成后交付该范围的结果。历史 T01–T21 用于定位已有实现和待验收项，不再作为自动继续开发的任务队列。保留已有后台能力、无关改动与历史证据。

## 工作方式

1. 读取 [AGENTS](../AGENTS.md)、当前 issue 和涉及的源码，确认目标、依赖与现有工作；并行前明确文件责任。
2. 实现当前 issue 的可观察行为。页面通过应用服务使用能力，demo 与 native 数据保持分离。
3. 日常运行 `npm run check`，即浏览器自动化点击测试。Playwright 在独立 5183 端口启动 Vite，无需生产构建；报告在 `output/playwright/`。
4. 修复相关失败，记录实际检查和未验证边界，再交付当前任务。提交、推送与 issue 更新按本次授权执行。

默认 CI 采用单个 Node.js 24 Windows 任务，只安装依赖、Playwright Chromium 并执行页面测试。类型、后台、构建和桌面工具按需要单独运行，详见 [工程规范](ENGINEERING.md#6-检查与证据)。当前浏览器测试覆盖 demo 与模拟 bridge，通过不代表真实桌面能力通过。

用户本次已授权浏览器自动化点击测试。Windows UI Automation、真实安装、内核探针以及系统级实验属于另外的操作范围，不能由此推导为已授权执行。

## 保留的实现约束

- seed 首次保存后固定；普通编辑、启停与代理变化保持身份。克隆生成新身份与空数据，恢复保留原身份。
- 直连和代理按保存的策略执行；代理失败不静默直连。内核缺失或损坏准确报错，不自动切换版本。
- 数据目录、进程与凭据边界遵循 [工程规范](ENGINEERING.md) 和 [内核合同](KERNEL.md)，界面简化不移除后台保护。
- 测试与公开材料使用合成或脱敏数据；真实 profile、凭据和 Cookie 留在版本库外，第三方分发保留许可。

## 完成条件与历史

当前任务按自身验收项完成：用户能检查目标行为，实际执行的检查通过，未验证项明确。浏览器页面、模拟服务、本地服务与真实桌面的证据分别记录；既有未验收能力继续保持未验收。

旧任务拆分与依赖见 [ISSUES](ISSUES.md)，历史范围和执行决定见 [DECISIONS](DECISIONS.md)，原验收证据见 [ACCEPTANCE](ACCEPTANCE.md) 和各任务验证记录。历史中的六页约束、自动接续 21 票、禁止浏览器点击、逐票全量构建与 CI 节奏不再作为当前执行规则。
