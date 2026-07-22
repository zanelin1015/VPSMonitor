## VPSMonitor 0.2.100

### x-ui 客户端同步保护
- 修改收费开始时间或周期后，只同步 x-ui 客户端的 `expiryTime`，不改动 UUID、Email、启停状态、流量、Flow、Sub ID 或备注。
- 客户端到期动作与普通编辑继续复用 x-ui 中已经存在的 UUID，只有新建客户端时才生成唯一随机 UUID。
- Admin 客户端列表始终显示 x-ui 原始 1 倍流量，流量倍率只应用于 Customer 页面和用户订阅侧展示。

### 多层 Realm Customer 链路
- Customer 授权第一层 Realm 入口时，可沿多层 Realm 拓扑解析到最终 x-ui 节点和指定客户端，不再显示“待解析”。
- 下发链接使用授权入口的主域名和监听端口，同时保留最终节点的 UUID、密码、Reality、SNI 和传输参数。
- Realm 映射严格按入口端口和客户端标识匹配，不会暴露同一最终节点中未授权的其他客户端。

### HTTP 账号型代理
- 支持读取 x-ui HTTP/SOCKS 入站的 `settings.accounts` 用户名和密码，并在节点、客户端、授权和 Customer 页面中展示。
- 支持生成 HTTP 代理导入 URL，经 Realm 转发时仅替换公开入口地址和端口，账号密码保持不变。
- Mihomo 订阅支持输出 HTTP 代理的 `type`、`server`、`port`、`username` 和 `password`。
- 新增和删除 HTTP/SOCKS 账号直接更新 `settings.accounts`，不调用 UUID Client API，也不会生成或修改 UUID。
- x-ui 不支持 HTTP 单账号启停、在线统计和到期时间，因此管理端按节点账号展示，并避免下发无效的单账号启停或到期操作。

### 验证
- 新增 HTTP 账号采集、导入 URL、Mihomo 订阅、账号新增/删除和多层 Realm 链路测试。
- 新增第一层 Realm Customer 下发、最终客户端凭据保留和未授权客户端隔离测试。
- Go 全量测试、Go Vet、前端 TypeScript 检查、生产构建、财务测试和客户端到期测试通过。
- Linux/Windows Server 与 Client 0.2.100 安装包生成成功。
