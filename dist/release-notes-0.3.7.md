## VPSMonitor 0.3.7

### HAProxy 链路映射
- HAProxy 主节点和备用节点保存前会解析完整 Realm 路径，并校验最终命中同一个 x-ui 节点，避免故障切换后落到不同服务。
- Client 详情、全局拓扑、区域账号授权和用户订阅均支持 HAProxy 入口，用户链接会替换为 HAProxy Client 的主域名与监听端口。
- HAProxy 主备链路按主节点路径展示最终节点与客户端，同时保留 UUID、Reality、SNI 和目标客户端信息。
- Realm 与 HAProxy 生成的同一最终客户端会进行流量与节点去重，避免重复统计。

### Realm 与 HAProxy 互斥
- 同一个 Client 的 Realm 与 HAProxy 功能开关强制二选一，切换模式时保留原规则并关闭另一套运行配置。
- 启用 HAProxy 时 Realm 会写入 `enabled: false` 与 `backend: none`，阻止本地 Realm 探测重新开启服务。
- Server API、Client 运行时、Realm 配置复制和旧数据归一均增加互斥保护。
- Linux、OpenWrt/iStoreOS 安装页面与安装脚本禁止同时自动安装 Realm 和 HAProxy。

### 区域账号与用户下发
- 区域账号支持按 `haproxy:<port>` 授权入口，并沿主链路校验最终节点及客户端权限。
- 用户节点链接、Mihomo 订阅和客户端导出支持 HAProxy 第一层入口地址替换。
- 修正多层转发场景下节点、客户端、流量、速度和授权范围的关联展示。

### 验证
- Go 全量测试与 Go Vet 通过。
- 前端 TypeScript 生产构建、财务测试和客户端到期测试通过。
- Linux 与 OpenWrt/iStoreOS 安装脚本语法及 Realm/HAProxy 互斥测试通过。
- Linux `amd64/arm64/armv7` 与 Windows `amd64/arm64` 的 Server、Client 发布包构建通过。
