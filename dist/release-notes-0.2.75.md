# VPSMonitor v0.2.75

## 修复
- 修复广州 Realm 转发到 HK 后，通过广州虚拟节点给 HK x-ui 节点新增客户端时仍可能报 `/panel/api/inbounds/list` 404 的问题。
- 新增客户端现在会在 `/panel/api/clients/add` 失败后，直接尝试旧版 `/panel/api/inbounds/addClient`，不再先强依赖入站列表接口。
- 当 3x-ui/1Panel Docker 面板的 `/panel/api/inbounds/list` 不可用时，会回退读取本机 x-ui SQLite 数据库解析真实 inbound id，再重新通过 API 下发新增客户端。
- 兼容 Realm 虚拟节点中 `inbound_id` 被端口号替代的场景，可通过节点 tag/端口映射到真实 HK inbound。

## 验证
- `go test ./...`
- `npm run build`
- `./scripts/build.sh`
