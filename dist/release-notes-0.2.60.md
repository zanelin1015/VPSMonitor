## v0.2.60

- Dashboard agent 数据新增返回 `network_policy` 快照，端口策略页可直接使用 dashboard 当前选中节点的数据。
- 修复 `/api/v1/agents` 已有 UFW/tc 端口策略数据，但页面通过 `/api/v1/dashboard` 渲染时显示 `端口策略(0)` 的问题。
- 区域账号视图继续隐藏端口策略快照，避免泄露未授权配置细节。
