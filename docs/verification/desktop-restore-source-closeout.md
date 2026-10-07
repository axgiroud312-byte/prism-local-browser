# 普通备份恢复来源与原预检安全恢复增量

本增量继续当前 `codex/issue32-37-ui-integration` 分支，用户当前只使用精确 148 内核，不验证 150 或跨版本迁移。用户取消严格 1:1 要求；本增量不扩展视觉截图、商业参考或图库审计。此前实际桌面通过、失败和能力限制保留在 [回收与迁移收口记录](desktop-recycle-migration-closeout.md)，下列源码/自动化通过不改写 preview7/8 的历史结果。

## 问题与修复

原来源 token 只由页面保存。选择回复缺失或页面卸载后的 best-effort 丢弃失败时，后端虽然保留原预检 worker/准确 scratch，页面仍丢失了可重试入口。现在 `Backup.SelectRestoreSource({requestId})` 在服务登记原选择请求；`Backup.ReadRestoreSource({requestId})` 从同一实际服务所有权查回原 sourceToken、名字、原预检是否仍在进行、准确 scratch 清理状态及尚有效的原预览。查询只读且绕过持久任务 flush，不接受路径或包内容，也不按任意目录扫描认领资源。

选择请求在途时不能报告丢弃完成；同请求重复选择不打开第二个系统选择器。选择明确失败/取消也有原请求终态，可通过查询可靠确认没有创建来源。来源过期仍保留准确清理所有权。`DiscardRestore` 可接原 requestId，由服务解析真实 token；caller 同时提供不同 token 会被拒绝。只有原预检 worker 结束且原 scratch 的实际删除确认后，才返回 discarded 并允许另选文件。已受理恢复的来源移交原 restoreTask，不用页面丢弃解除任务保护。

adapter 保留原 requestId 与清理意图，跨页面隐藏/卸载不丢 owner；失败回执、transport 未确认和迟到结果都不能释放或替换新 owner。原选择或原查询已被准确 discarded 后，它们的迟到 selected 回复被拒绝。页面返回时核实原来源；丢 token 时显示“核实原来源”，清理失败可“重试取消只读预检”/“仅关闭窗口”。准确已确认来源换包时先丢弃原来源，确认后才新选，不将普通选错文件误称来源未知。

## 当前验证分层

| 范围 | 判定 | 实际证据与边界 |
| --- | --- | --- |
| 原 requestId 查回准确来源/原预览，重复选择幂等，错误 requestId 拒绝，过期来源准确丢弃 | 实际通过（Go 服务自动化） | 新 `restore_source_owner_test.go` 使用实际 Service/RPC、生成的合成原生备份和只读预检；没有启动桌面或真实内核。 |
| 原选择仍在途时保护，结束后按原请求查回并丢弃 | 实际通过（Go 服务自动化） | 实际服务选择器调用由通道受控阻塞，明确属于可控迟到注入；不冒充真实 Windows 系统选择器卡顿。 |
| scratch 占用失败后保留原 owner，解除实际文件占用才确认清理 | 实际通过（Go 服务/Windows 文件检查） | 测试在独立测试 root 生成合成 scratch，持有实际 `backup.FreezeFile` 句柄，使原准确删除失败；释放后重试并核对 `os.Stat` 目录不存在。不是本轮新 Wails GUI 故障场景验收。 |
| 丢 token/IPC、拒绝 foreign requestId、失败清理跨订阅保留、晚选择/晚查询不接入新目标 | 实际通过（adapter 自动化） | 5 个专属 Node 用例；只使用合成 bridge。 |
| 隐藏/离开/返回页仍可核实与重试原清理，旧预检迟到不重开，换包先原丢弃再新选择 | 实际通过（页面自动化） | 3 个新增 Playwright 用例，Vite 5184，合成 Wails bridge，无真实浏览器内核。 |
| 既有普通预检/取消/准确完整恢复受理页面流程 | 实际通过（既有定向页面回归） | 6 个用例，两种窗口尺寸；模拟完整报告，只证明页面接入，不证明实际恢复完整浏览数据。 |
| 新 exe 上丢 token、占用失败、重开恢复与原实际数据 | 仍待实际验收 | 由整合者构建并执行新 Wails 桌面验证；本子任务未构建、未启动桌面/内核、未操作其他实例。没有预设真实桌面通过数。 |
| 进程异常终结后认领未知旧 scratch/重用旧短期 token | 不支持；不计通过 | 选择所有权仅在原 Service 生命周期有效。正常退出须原 Close 可靠确认 worker/准确 scratch 清理；未知异常残留没有按目录扫描或猜 token 恢复功能。此增量没有放宽该边界。 |

## 实际执行的必要检查

均使用当前源码，未提交/推送，未生成新桌面构建。

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| Go 普通来源/预检/丢弃/关闭 | `.tools/go/bin/go.exe test ./internal/workspace -run 'TestRestore(Source\|Preview\|Discard\|Shutdown)' -count=1` | 9 个用例通过，末次 0.766s；Go 环境沿项目 `.tools/gopath` / `.tools/gocache` / `GOTOOLCHAIN=local` / `CGO_ENABLED=0`。 |
| 新 adapter 所有权 | `node --experimental-strip-types --test tests/restore-source-owner.test.ts` | 5/5，118.6108ms。新文件已纳入 `package.json` 的固定 npm test 列表；不为接线重跑全套。 |
| 现有 adapter 准确入口与只读 | `node --experimental-strip-types --test --test-name-pattern='M22 definite SelectRollback PROFILE_BUSY\|restore preflight never' tests/wails-adapter.test.ts` | 2/2，132.7598ms。迁移只运行既有合成入口兼容检查，不做跨版本迁移。 |
| 新 UI 所有权 | `npx playwright test --config output/playwright/recycle-closeout.config.ts restore-source-owner.spec.ts` | 3/3，7.8s。 |
| 既有普通恢复 UI | `npx playwright test --config output/playwright/recycle-closeout.config.ts local-pages.spec.ts -g 'native preflight and discard\|native chooser cancellation\|native restore success'` | 6/6，9.5s。 |
| 类型/差异 | `npx tsc -p tsconfig.json --noEmit`；`npx tsc -p tsconfig.tests.json --noEmit`；`git diff --check` | 通过，未构建。 |

首轮新增 UI 的一条用例失败：合成 fixture 在清理 pending 时仍返回 preview，与生产查询合同不符；严格 validator 拒绝了该回复。已修正 fixture 为清理 pending 时没有可用 preview，保留这个失败来源，并在最后 3/3 中核实。首轮旧 Go fixture 在新选择前未丢弃仍保留的原 source 而被准确拒绝；fixture 现在显式丢弃其准确来源后再选下一个合成包。没有降低真实 requestId 校验，也没有制造额外 skipped 分支。
