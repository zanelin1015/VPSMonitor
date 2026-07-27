## VPSMonitor 0.3.4

### 多平台 Client
- Linux 新增 `amd64`、`arm64`、`armv7` 构建，Windows 新增 `amd64`、`arm64` 构建。
- Debian、Ubuntu、CentOS 使用 systemd，Alpine 使用 OpenRC，iStoreOS/OpenWrt 使用 procd，Windows 安装为 Windows Service。
- 管理后台按系统分别提供 Linux、iStoreOS/OpenWrt、Windows PowerShell 和 Windows CMD 安装命令。
- Client 在线升级会校验目标操作系统和架构，并为 OpenWrt 自动切换专用安装脚本。

### Realm 可选安装
- Client 安装页面新增“同时安装 Realm”开关，默认关闭，仅 Linux 与 OpenWrt 生效。
- 支持固定 Realm 版本和配置自定义下载镜像目录，默认版本为 `v2.9.4`。
- 自动识别 glibc/musl 与 x86_64、ARM64、ARMv7，选择对应 Realm 官方包。
- 已安装 Realm 时直接复用，不覆盖现有二进制与转发配置。
- Realm 服务在收到首条转发规则后创建并启动；Client 在线升级继承管理员保存的 Realm 开关、版本和镜像参数。

### OpenWrt 兼容性
- 新增 `install-openwrt.sh`，支持 BusyBox shell、uclient-fetch/wget/curl 和 procd 开机自启。
- Realm 转发支持通过 procd 安装、启用和重启服务。
- OpenWrt 不自动应用端口策略，避免覆盖 firewall4、SQM、Cake 或 qosify 规则。
- 远程终端会根据目标系统选择可用 shell，并补充系统版本与架构识别。

### 验证
- Go 全量测试与 Go Vet 通过。
- 前端 TypeScript 检查、生产构建、客户端到期与财务回归测试通过。
- Linux `amd64/arm64/armv7` 和 Windows `amd64/arm64` 交叉编译通过。
- Realm glibc 与 musl 官方压缩包完成真实下载、解压和安装路径验证。
- 管理后台安装弹窗完成滚动、开关命令和浏览器控制台检查。
