# VPSMonitor v0.2.79

## 修复
- 修复区域账号通过广州 Realm 虚拟节点给 HK x-ui 节点新增客户端时，最终回退到 `/panel/api/inbounds/update/:id` 并返回 404 的问题。
- 新增客户端时，`/panel/api/inbounds/addClient` 优先使用新版 3x-ui 的 `inboundId + client` 请求格式。
- 保留旧版兼容：新版请求失败后再尝试旧版 `id + settings` 格式，最后才回退整条 inbound 更新。
- 补充测试覆盖新版 `addClient` 请求格式，避免新增客户端误走旧更新接口。

## 验证
- `go test ./...`
- `npm run build`
- `./scripts/build.sh`
