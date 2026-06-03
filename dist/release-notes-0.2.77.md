# VPSMonitor v0.2.77

## 修复
- 固化 x-ui 客户端 UUID 不可变规则：除新增客户端外，所有客户端更新动作都会以 3x-ui 当前保存的数据为准。
- 更新 x-ui 客户端启停、到期时间等字段时，会在提交前强制恢复原始 `id` / `uuid` / `password`，避免误改 UUID 导致节点失效。
- 补充回归测试，覆盖 3x-ui 返回数据库行 ID 与真实 UUID 同时存在时的更新场景。

## 验证
- `go test ./...`
- `npm run build`
- `./scripts/build.sh`
