## VPSMonitor 0.2.91

### 新增
- 新增公开官网页 `/site` / `/official`，展示脱敏后的 VPSMonitor 拓扑能力。
- 新增公开拓扑接口 `/api/v1/public/topology`，用于官网展示，不需要登录。
- 新增访问日志 v1：支持从 Xray access.log 采集元数据、Client 批量上报、后台查询。
- X-UI 托管配置新增访问日志开关、日志路径和保留天数。

### 安全与隐私
- 官网拓扑会脱敏 Client/VPS 原始名称、agent_id、客户客户端、IP、域名和 client_chains。
- 访问日志默认关闭，首次开启从文件末尾开始采集，避免历史日志一次性上传。

### 验证
- go test ./...
- npm run build
- npm run test:finance
