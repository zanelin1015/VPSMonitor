# VPSMonitor v0.2.80

## 修复
- 修复原生 3x-ui 使用 `/xui/` WebBasePath 时，VPSMonitor 误把 `/xui` 从 BaseURL 中截掉，导致新增客户端请求落到 `/panel/api/...` 并返回 404 的问题。
- 保留 1Panel / Docker 自定义路径兼容，例如 `/HK/` 仍会保留为 API 前缀。
- 继续支持从面板页面地址自动归一化，例如 `/xui/panel/inbounds` 会归一化为 `/xui`。

## 验证
- `go test ./...`
- 线上原生 x-ui 探测确认：`/panel/api/inbounds/list` 返回 404，`/xui/panel/api/inbounds/list` 返回 200。
