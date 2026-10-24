# 管理台第一期结构

管理台保留 React 18、Vite、Ant Design 5、现有 HTTP API 与 URL 格式，增量接入 ProComponents 2。

## 入口与渲染

- `web/src/App.tsx`：区分管理端、客户入口与公开推荐页，按需加载对应应用。
- `web/src/app/AdminApp.tsx`：组合业务 Hook，将数据和操作绑定到页面；不直接发起 HTTP 请求。
- `AdminLayout.tsx`：ProLayout + PageContainer，统一桌面与移动导航、页面标题和全局操作。
- `adminMenu.ts`：菜单、分组、权限条件与页面文案的单一配置，`adminMenuHref` 保留根目录和 `/zanelin` 部署路径。
- `AdminPageRouter.tsx`：页面分发；工作台与资产页分别按需加载。
- `AdminModalHost.tsx`：管理全局账号、安装、升级与授权等弹窗的挂载。
- root 管理员负责系统、财务、区域账号和运营管理员；区域账号只看到授权范围内的基础设施；operator 账号只进入用户管理和在线客服。

## 状态与请求边界

`web/src/hooks/admin/` 按业务分工：

| 模块 | 职责 |
| --- | --- |
| `useAdminNavigationState` | 当前页面、选中 Client、标签、搜索、详情 Tab 和定位锚点 |
| `useDashboardData` | 仪表盘、拓扑、客户展示名缓存、请求去重、实时 WebSocket |
| `useDashboardRealtime` | 按账号身份维持实时连接、开发模式握手清理、断连后的退避重连 |
| `useAdminFinance` | 财务账号、汇率、显示币种 |
| `useAgentDetailData` | 当前 Client 概览、配置草稿、操作记录、日志与配置保存 |
| `useAgentOperations` | 采集、删除、替换与 Realm 配置复制 |
| `useXUIActionComposer` | XUI 操作表单、来源节点加载、链接复制 |
| `useAdminLifecycle` | 会话初始化、页面加载、权限回退、轮询和清理 |
| `buildAgentDetailProps` / `buildConsoleModalProps` | 将上述业务域绑定为已有组件的类型化接口 |

现有 `useAdminSession`、`useAdminSystemTools`、`useAdminRouteSync`、`useXUIRemoteActions` 继续负责各自领域。新增页面应放在 `pages/admin`，对应加载行为进入业务 Hook，避免重新集中到应用入口。

## 资产与详情

- `AssetsPage.tsx`：ProTable 处理排序、分页、列设置和选择状态；筛选与实时数据继续使用现有数据源。
- 列设置按账号和权限范围隔离保存，区域账号不显示 CPU/内存/硬盘、主机位置与实时网速。
- 767px 以下采用卡片列表，同一套搜索、标签和健康筛选适用于两种布局。
- `AgentDetailDrawer.tsx`：详情从资产页打开，关闭时保留搜索和筛选。弹窗层级高于详情抽屉，避免全局操作表单被遮挡。
- 详情深链接在首个仪表盘请求完成前保留；确认 Client 不存在后才清除选择。
- operator 访问工作台、资产、拓扑、财务或系统深链接时，会在路由归一化阶段回退到用户管理，避免先加载无权限页面。

## 主题

`web/src/theme/tokens.ts` 同时生成 Ant Design token 和旧业务组件需要的 CSS 变量。`AdminConsoleTheme` 应用并在卸载时恢复变量，支持明暗模式与减少动画偏好。组件样式放在 `admin-layout.css`，不要再次定义同一套主色、状态色或背景变量。

## 菜单与实时连接回归

- 分组必须使用 `children`。当前 ProLayout 的转换器会先过滤掉同时缺少 `path` 和 `children` 的项，给无路径分组设置旧式 `routes` 会让整组菜单消失。测试同时检查转换结果和实际布局中的菜单链接，避免只检查源配置。
- 实时连接按账号 ID、用户名和角色维持，不因同账号对象更新而断开。开发模式的 StrictMode 探测不立即创建握手，已开始的握手在打开后正常关闭，避免直接取消连接。失败重连从 3 秒开始退避，最长 30 秒；HTTP 轮询继续作为回退。
- Vite 的 `EPIPE` / `ECONNRESET` 表示代理连接被对端关闭或重置，与菜单渲染无关。客户端减少多余重连不会消除真实上游网络故障，代理测试覆盖消息转发、正常关闭与上游重置后的再次连接。

## 验证

```bash
cd web
npm run test:admin-phase1
npm run test:admin-state
npm run test:admin-realtime
npm run test:dev-proxy
npm run test:console-theme
npm run test:app-recovery
npm run build
```

新增测试覆盖 ProLayout 菜单渲染与转换、菜单权限、部署前缀、资产组件渲染、实时连接生命周期及重置恢复、首屏深链接、区域账号权限回退、轮询清理、切换 Client 时的延迟响应与未保存草稿保护。本次环境的 Work 设置禁用了本地浏览器控制，尚未执行 375/768/1440px 的真实浏览器点击检查和截图验收。
