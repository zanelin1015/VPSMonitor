# VPSMonitor v0.2.74

## 修复
- 修复 x-ui/3x-ui 新增客户端在部分 v3 或 Docker 面板上因为 `/panel/api/inbounds/list` 返回 404 而失败的问题。
- 当下发新增客户端已带 `inbound_id` 时，优先直接调用 `/panel/api/clients/add`，不再强依赖读取入站列表。
- 保留旧版兼容：直接新增失败时，仍会回退到读取入站列表、`addClient` 或更新 inbound settings。

## 验证
- `go test ./...`
- `npm run build`
- `./scripts/build.sh`
