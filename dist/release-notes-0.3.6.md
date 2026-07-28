## VPSMonitor 0.3.6

### HAProxy 高可用转发
- Client 安装流程新增可选 HAProxy 安装，支持 Debian、Ubuntu、CentOS 系及 OpenWrt/iStoreOS。
- Client 功能开关新增 HAProxy，可从系统中已有的 Realm 规则选择一个主目标和多个有序备用目标。
- HAProxy 通过 TCP 健康检查自动摘除异常目标，并在主目标恢复后自动切回。
- Server 保存前会解析并校验 Realm 目标，Client 使用配置校验、原子替换和服务 reload 安全下发。
- 已有 Client 默认保持 HAProxy 关闭，不会因升级自动安装或接管现有网络配置。

### 区域账号权限与流量
- 区域账号为下属用户授权时，仅可选择自身已授权或由自身创建的节点客户端。
- 修正区域账号节点、客户端、出站规则、IP、流量和实时速度的权限范围，避免展示未授权数据。
- 区域账号创建的客户端会纳入后续授权范围，并继续保留 Realm 多层转发后的入口替换能力。

### 工作台与前端结构
- 管理工作台固定展示全部 Client 的全局统计，不再跟随 Client 页选中的分组变化。
- Client 分组筛选仍只作用于 Client 列表、详情和拓扑范围。
- 拆分登录会话、后台工具、路由同步、x-ui 远程操作、工作台汇总和访问日志页面，`App.tsx` 从 2776 行降至约 2000 行。

### 验证
- Go 全量测试与 Go Vet 通过。
- 前端 TypeScript 生产构建、财务测试和客户端到期测试通过。
- 安装脚本语法检查通过。
- Linux `amd64/arm64/armv7` 与 Windows `amd64/arm64` 的 Server、Client 发布包构建通过。
