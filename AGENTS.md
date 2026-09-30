# 棱镜浏览器开发指引

本文件适用于本仓库。保留与当前任务无关的改动；并行工作先明确文件责任。
产品是 Windows 本机多环境管理工具；不增加云账号、组织权限、计费或实例数量配额。
指定内核为 fingerprint-chromium。当前前端的模拟启动和检查不代表桌面能力已实现。

## 按任务读取

- 领取、实现或评审开发 issue：读 [工程规范](docs/ENGINEERING.md)，核对目标、交付层级和 blocking 依赖。
- 修改用户流程、功能范围或验收条件：读 [PRD](docs/PRD.md) 对应需求及原型差异表。
- 修改数据模型、应用接口、异步任务或持久化：读 [开发方案](docs/DEVELOPMENT.md) 对应章节。
- 修改指纹、内核、代理通道、进程或真实目录：读 [内核合同](docs/KERNEL.md) 和工程规范的桌面边界。
- 修改已实现行为：更新 [需求追踪](docs/TRACEABILITY.md)；核实结果写入对应 issue/PR，交付验收记入 [验收记录](docs/ACCEPTANCE.md)。

## 领取和实现

- 任务源是 [GitHub Issues](https://github.com/axgiroud312-byte/prism-local-browser/issues)。
- `spec` 表示总规范，拆成有边界的实现任务，不作为一张可直接完成的开发任务。
- `ready-for-agent` 只表示说明清楚；领取前仍须确认所有 blocking 依赖均已完成且成果可用。
- 用户已明确指派的任务按现有授权执行；常规实现、排错和验证不增加确认步骤。
- 先查看当前源码与配置；把设计、原型、本地服务实现、真实桌面验证分别记录。
- 演示数据和 native 数据明确分离；页面通过应用服务调用真实能力，保持当前原型可运行。
- 固定 seed、明确代理策略、固定内核与独立数据引用遵循工程规范；普通编辑不得悄悄重生成身份。
- 公开代码、测试和截图使用合成数据；真实凭据、Cookie、profile、私有路径和旧商业资源留在仓库外。

## 完成与交付

- 检查命令以 `package.json` 为准；行为修改运行 `npm run check`，文档修改至少运行 `npm run check:docs`。
- UI 流程修改做真实页面操作；新增桌面能力按内核合同补进程、目录或网络证据。
- 测试针对可观察行为与失败恢复；简单文案或样式调整不机械补测试。
- 修复失败检查后再交付；尚未验证的能力如实保留为未验证，不用截图代替后台证据。
- PR 关联 issue，说明行为变化、实际检查和剩余边界；格式见 [.github/pull_request_template.md](.github/pull_request_template.md)。
- 新依赖或内核分发涉及许可时，更新 [第三方声明](THIRD_PARTY_NOTICES.md)，保留实际组件的许可范围。
