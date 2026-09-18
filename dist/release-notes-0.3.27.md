# VPSMonitor v0.3.27

- 优化 Client 详情页收费控件布局，保持金额、周期及保存按钮在同一行。
- 调整开始/到期列宽及控件间距，减少按钮换行。
- 移除 Client 详情页冗余的“返回首页”按钮。
- 返回 Client 列表时清空标签与搜索条件。

Server 和 Client 安装包版本统一为 0.3.27。本次功能变更位于 Server 内嵌前端，Client 无新增功能，无需为这些界面调整升级 Client。

本次未修复 Android Clash 的 REALITY authentication failed 问题，该问题仍需进一步诊断。

验证：前端 TypeScript/Vite 构建、go test ./...、client-expiry、dashboard-network、finance 测试通过。
