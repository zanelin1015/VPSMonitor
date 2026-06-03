# VPSMonitor v0.2.76

## 修复
- 客户端收费配置如果没有设置开始时间，将不再参与 x-ui 到期时间同步，避免历史 x-ui 到期时间把客户端自动设置为过期或禁用。
- 新建/保存收费配置时，不再默认把 x-ui 当前 `expiryTime` 写入系统 `expire_time`；只有明确设置开始时间后，系统才按收费周期同步 x-ui 到期时间。
- 自动续期任务同样要求存在开始时间，避免旧配置继续触发到期更新。
- 修复新版 3x-ui 客户端更新时可能把数据库行 ID 当作 VLESS/VMess UUID 下发的问题，确保更新启停/到期等字段时保留原 UUID。

## 验证
- `go test ./...`
- `npm run build`
- `./scripts/build.sh`
