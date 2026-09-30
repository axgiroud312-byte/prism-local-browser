# 开发票据与依赖索引

[开发总规格](SPEC.md) · [开发规范](ENGINEERING.md) · [产品需求](PRD.md) · [需求追踪](TRACEABILITY.md)

发布记录：2026-09-30。用户已确认测试边界和票据拆分；已发布 1 张总规格、21 张开发 Issue、3 个里程碑和 29 条 GitHub 原生 blocking 依赖。

总规格：[Spec #1](https://github.com/axgiroud312-byte/prism-local-browser/issues/1)。它是共同范围依据，具体实现从下面的任务领取。当前完成的是规划配置；桌面功能仍需逐票实现和验收。

主要自动测试边界采用应用服务公开契约，沿用已有领域样本。真实浏览器、代理故障、备份恢复与安装包另做 Windows 实测。

## 领取规则

- 初始实施入口是 [T01 · #2](https://github.com/axgiroud312-byte/prism-local-browser/issues/2)。后续仅在 blocking 依赖的成果已完成且可用时领取；总规格不作为一张实现任务。
- `ready-for-agent` 表示任务说明和验收已明确，并不表示所有任务现在都能开工；`native-validation` 表示需要真实 Windows 证据。
- 本页是发布时的范围与依赖索引，不镜像实时完成状态。开始前查看 [GitHub Issues](https://github.com/axgiroud312-byte/prism-local-browser/issues) 的最新状态、关联 PR 和原生依赖。
- 每张 Issue 包含目标、需求 ID、验收清单、测试方式和真正的阻塞项。变更范围或依赖时同步本页与需求追踪，保留总规格的共同边界。

## [M1 · 本机环境与真实内核](https://github.com/axgiroud312-byte/prism-local-browser/milestone/1)

完成契约先导、本地持久工作区、可安装桌面壳及固定内核的真实隔离会话。以实际重开证据验收，不设截止日期。

| 开发 Issue                                                                   | 完成后的行为                                  | 直接阻塞项                                                                   |
| ---------------------------------------------------------------------------- | --------------------------------------------- | ---------------------------------------------------------------------------- |
| [T01 · #2](https://github.com/axgiroud312-byte/prism-local-browser/issues/2) | 提取环境应用契约，保持原型创建与编辑闭环      | 无                                                                           |
| [T02 · #3](https://github.com/axgiroud312-byte/prism-local-browser/issues/3) | 在 Windows 桌面创建并重开本地环境档案         | [T01 · #2](https://github.com/axgiroud312-byte/prism-local-browser/issues/2) |
| [T03 · #4](https://github.com/axgiroud312-byte/prism-local-browser/issues/4) | 安装、升级和卸载 Windows 桌面壳并保留工作区   | [T02 · #3](https://github.com/axgiroud312-byte/prism-local-browser/issues/3) |
| [T04 · #5](https://github.com/axgiroud312-byte/prism-local-browser/issues/5) | 安装并核验一份精确版本的 fingerprint-chromium | [T02 · #3](https://github.com/axgiroud312-byte/prism-local-browser/issues/3) |
| [T05 · #6](https://github.com/axgiroud312-byte/prism-local-browser/issues/6) | 预览、保存并回滚固定设备档案修订              | [T04 · #5](https://github.com/axgiroud312-byte/prism-local-browser/issues/5) |
| [T06 · #7](https://github.com/axgiroud312-byte/prism-local-browser/issues/7) | 真实启停两个环境并验证浏览数据隔离            | [T05 · #6](https://github.com/axgiroud312-byte/prism-local-browser/issues/6) |
| [T07 · #8](https://github.com/axgiroud312-byte/prism-local-browser/issues/8) | 处理应用重开、浏览器崩溃和停止超时            | [T06 · #7](https://github.com/axgiroud312-byte/prism-local-browser/issues/7) |

## [M2 · 独立网络与日常操作](https://github.com/axgiroud312-byte/prism-local-browser/milestone/2)

完成代理认证与运行期故障阻断、Cookie 写入、批量任务及取消重试。所有网络结论来自受控实际流量。

| 开发 Issue                                                                     | 完成后的行为                                 | 直接阻塞项                                                                                                                                                   |
| ------------------------------------------------------------------------------ | -------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| [T08 · #9](https://github.com/axgiroud312-byte/prism-local-browser/issues/9)   | 导入代理、保护凭据并显示真实检查结果         | [T02 · #3](https://github.com/axgiroud312-byte/prism-local-browser/issues/3)                                                                                 |
| [T09 · #10](https://github.com/axgiroud312-byte/prism-local-browser/issues/10) | 为指定环境接入 HTTP 和 HTTPS 认证代理        | [T06 · #7](https://github.com/axgiroud312-byte/prism-local-browser/issues/7)、[T08 · #9](https://github.com/axgiroud312-byte/prism-local-browser/issues/9)   |
| [T10 · #11](https://github.com/axgiroud312-byte/prism-local-browser/issues/11) | 接入 SOCKS5 认证并固定 DNS 解析策略          | [T09 · #10](https://github.com/axgiroud312-byte/prism-local-browser/issues/10)                                                                               |
| [T11 · #12](https://github.com/axgiroud312-byte/prism-local-browser/issues/12) | 代理运行中断线时阻断非预期网络并恢复原会话   | [T10 · #11](https://github.com/axgiroud312-byte/prism-local-browser/issues/11)                                                                               |
| [T12 · #13](https://github.com/axgiroud312-byte/prism-local-browser/issues/13) | 把 Cookie 导入指定真实环境并读回核对         | [T06 · #7](https://github.com/axgiroud312-byte/prism-local-browser/issues/7)                                                                                 |
| [T13 · #14](https://github.com/axgiroud312-byte/prism-local-browser/issues/14) | 持久化批量创建、配置复制与代理分配任务       | [T07 · #8](https://github.com/axgiroud312-byte/prism-local-browser/issues/8)、[T08 · #9](https://github.com/axgiroud312-byte/prism-local-browser/issues/9)   |
| [T14 · #15](https://github.com/axgiroud312-byte/prism-local-browser/issues/15) | 排队启动、取消和重试真实环境且不设置运行配额 | [T07 · #8](https://github.com/axgiroud312-byte/prism-local-browser/issues/8)、[T11 · #12](https://github.com/axgiroud312-byte/prism-local-browser/issues/12) |

## [M3 · 完整恢复与桌面交付](https://github.com/axgiroud312-byte/prism-local-browser/milestone/3)

完成备份、预检、原子恢复、崩溃恢复、回收区、内核迁移和完整 Windows 使用闭环。

| 开发 Issue                                                                     | 完成后的行为                           | 直接阻塞项                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| ------------------------------------------------------------------------------ | -------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [T15 · #16](https://github.com/axgiroud312-byte/prism-local-browser/issues/16) | 导出全量或选定环境的完整本机备份       | [T07 · #8](https://github.com/axgiroud312-byte/prism-local-browser/issues/8)、[T08 · #9](https://github.com/axgiroud312-byte/prism-local-browser/issues/9)                                                                                                                                                                                                                                                                                                                                   |
| [T16 · #17](https://github.com/axgiroud312-byte/prism-local-browser/issues/17) | 导入备份前只读预检并展示恢复影响       | [T15 · #16](https://github.com/axgiroud312-byte/prism-local-browser/issues/16)                                                                                                                                                                                                                                                                                                                                                                                                               |
| [T17 · #18](https://github.com/axgiroud312-byte/prism-local-browser/issues/18) | 提交完整恢复并在执行失败时回滚         | [T16 · #17](https://github.com/axgiroud312-byte/prism-local-browser/issues/17)                                                                                                                                                                                                                                                                                                                                                                                                               |
| [T18 · #19](https://github.com/axgiroud312-byte/prism-local-browser/issues/19) | 恢复中断后重开应用自动找回一致工作区   | [T17 · #18](https://github.com/axgiroud312-byte/prism-local-browser/issues/18)                                                                                                                                                                                                                                                                                                                                                                                                               |
| [T19 · #20](https://github.com/axgiroud312-byte/prism-local-browser/issues/20) | 移除环境进入回收区并可找回原身份       | [T07 · #8](https://github.com/axgiroud312-byte/prism-local-browser/issues/8)                                                                                                                                                                                                                                                                                                                                                                                                                 |
| [T20 · #21](https://github.com/axgiroud312-byte/prism-local-browser/issues/21) | 选择环境迁移内核并从升级前完整备份恢复 | [T18 · #19](https://github.com/axgiroud312-byte/prism-local-browser/issues/19)                                                                                                                                                                                                                                                                                                                                                                                                               |
| [T21 · #22](https://github.com/axgiroud312-byte/prism-local-browser/issues/22) | 完成安装包上的首版日常使用与恢复验收   | [T03 · #4](https://github.com/axgiroud312-byte/prism-local-browser/issues/4)、[T12 · #13](https://github.com/axgiroud312-byte/prism-local-browser/issues/13)、[T13 · #14](https://github.com/axgiroud312-byte/prism-local-browser/issues/14)、[T14 · #15](https://github.com/axgiroud312-byte/prism-local-browser/issues/15)、[T19 · #20](https://github.com/axgiroud312-byte/prism-local-browser/issues/20)、[T20 · #21](https://github.com/axgiroud312-byte/prism-local-browser/issues/21) |

## 交付与验收

PR 关联对应开发 Issue，并按 [PR 模板](../.github/pull_request_template.md) 记录变化与实际验证。自动检查当前覆盖前端领域测试、构建与文档；新增桌面模块时补上可执行检查和 Windows 观测证据。

每项功能关联的原型入口及桌面 Issue 见 [需求追踪](TRACEABILITY.md)。完成声明写明实际版本、条件、结果和未验证项；实际交付记录保留在 [验收记录](ACCEPTANCE.md)。
