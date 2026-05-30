## v0.2.59

- Client 执行系统命令时不再依赖 systemd 环境中的 `PATH`，会主动查找 `/usr/sbin`、`/sbin` 等常见目录。
- 修复部分 VPS 上 `vpsmonitor-client` 由 systemd 启动时找不到 `ufw`，导致 UFW 白名单无法采集和显示的问题。
- 该兼容同样覆盖 `tc`、`iptables`、`ip` 等端口策略相关命令查找。
