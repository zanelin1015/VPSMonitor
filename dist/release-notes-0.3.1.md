## VPSMonitor 0.3.1

### 区域账号出站授权
- 将“添加已授权 Client 的节点作为落地”和“使用当前 Client 已有出站规则”拆分为两类独立权限。
- 区域账号新增出站时只能选择已授权 Client 下的节点客户端，不再提供手工 JSON 或公共出口链接库入口。
- Server 会校验来源 Client、节点、客户端、协议、地址、端口和认证信息，阻止越权构造或覆盖已有出站。
- 出站规则授权不再自动扩大区域账号的 Client 范围，保存账号时会拒绝未授权 Client 的出站范围。

### Clash / Mihomo 订阅
- 订阅改用 ACL4SSR Online 配置结构，保留参考配置的 11 个策略组和 3528 条分流规则。
- 生成订阅时仅替换当前用户的代理节点，并同步更新各策略组中的节点成员。
- VLESS、VMess、Shadowsocks、Trojan、HTTP 和 SOCKS 节点转换能力保持不变。
- 模板内嵌在 Server，不依赖第三方订阅转换服务，也不会向外部服务发送用户节点凭据。

### 验证
- Go 全量测试与 Go Vet 通过。
- 前端 TypeScript 检查、生产构建、客户端到期和财务回归测试通过。
- 生成的 Clash / Mihomo YAML 结构校验通过，参考节点域名、UUID 和 Reality 公钥未进入发布代码。
- Linux / Windows Server 与 Client 0.3.1 安装包生成成功。
