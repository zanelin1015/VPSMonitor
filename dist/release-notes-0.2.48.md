## v0.2.48

- 优化 Dashboard：拆分轻量总览与链路拓扑接口，普通总览不再计算 heavy topology。
- 新增 `/api/v1/dashboard/topology`，链路拓扑按需加载，并增加 45 秒服务端缓存与同请求单飞保护。
- DNS/GeoIP 查询移出 Dashboard/Topology 请求链路，仅在 Client 注册时刷新缓存；页面读取时只使用已缓存数据。
- 前端打开链路拓扑时才请求拓扑数据，增加加载中、失败重试与静默刷新处理。
- Customer 链路视图同步使用缓存解析数据，避免访问时触发外部 DNS/GeoIP 请求。
- 增加 Dashboard 缓存与无网络解析路径测试，提升慢接口稳定性。
