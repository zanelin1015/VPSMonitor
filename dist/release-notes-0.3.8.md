## VPSMonitor 0.3.8

### HAProxy 节点与客户端
- 纯 HAProxy Client 详情新增节点和客户端页签，展示主链路最终命中的 x-ui 入站及客户端。
- 节点和客户端链接使用 HAProxy 第一层入口的主域名与监听端口，支持在最终目标节点新增客户端。
- 主节点与备用节点指向同一最终入站时进行去重，避免节点和客户端重复展示。

### 停用配置隔离
- Realm 总开关关闭或 backend 为 `none` 时，保留的旧规则不再参与详情、全局拓扑、区域授权和用户入口匹配。
- HAProxy 主备目标会校验对应 Client 的 Realm 功能确实启用，避免使用已停用的中间转发规则。
- 修正停用 Realm 历史数据造成的额外节点、客户端和错误入口地址。

### 验证
- 使用线上广州 HAProxy、HK Realm 和 DMIT x-ui 数据验证，正确展示 1 个节点与 13 个客户端。
- Go 全量测试与 Go Vet 通过。
- 前端 TypeScript 生产构建、财务测试和客户端到期测试通过。
- Linux `amd64/arm64/armv7` 与 Windows `amd64/arm64` 的 Server、Client 发布包构建通过。
