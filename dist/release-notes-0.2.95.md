## VPSMonitor 0.2.95

### 新增
- 新增客户侧 Clash/Mihomo 订阅地址，支持区域/普通客户一键复制订阅。
- 新增订阅 token 与公开订阅接口，支持 `/clash.yaml`、`/mihomo.yaml`、`/clash.yml`、`/mihomo.yml`。
- 订阅内容支持将 VLESS、VMess、Shadowsocks、Trojan、Socks5、HTTP/HTTPS 链接转换为 Mihomo YAML。
- 订阅内置 Loyalsoldier clash-rules 分流规则：国内/局域网直连、广告拒绝、Google/Telegram/代理域名走代理、兜底代理。

### 优化
- 客户页面顶部和用户卡片增加“复制 Clash/Mihomo 订阅”入口。
- 线上 Server 未升级、暂未返回订阅地址时，入口不再灰掉，点击会提示需要发布新版 Server。
- 客户链路展示区分节点到期和客户端到期。

### 验证
- ./scripts/build.sh
