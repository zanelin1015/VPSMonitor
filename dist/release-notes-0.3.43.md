## VPSMonitor v0.3.43

### 修复

- 完整对齐第三方 Clash/Mihomo 配置头。
- 补齐 `geosite:cn` 的 119.29.29.29 与 223.5.5.5 双 DNS 配置。
- 动态生成的节点、代理组与规则保持不变。

### 验证

- `go test ./internal/server`
- `./scripts/build.sh`

升级后请确认宿主机的 53 端口可用，并确保运行 Clash/Mihomo 的进程具备监听该端口所需权限。
