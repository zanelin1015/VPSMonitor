## VPSMonitor 0.3.18

### Customer 订阅
- Clash/Mihomo 订阅增加固定链式代理支持，`999` 出口节点会通过 `IEPL` 前置节点拨号，便于把 IEPL 作为前置线路、999 作为落地出口。
- 保留 ACL4SSR 规则和现有代理分组，只调整代理节点自身的拨号关系。

### OpenClash DNS
- Customer 导出的订阅内置 OpenClash 友好的 DNS 配置，节点域名解析会优先使用国内可达 DNS。
- 避免依赖 `dns.google` 导致国内 OpenClash 环境出现 `dns resolve failed`、节点域名无法解析的问题。

### 验证
- Go 全量测试通过。
- Server 与 Client 多平台发布包构建通过。
