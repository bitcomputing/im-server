# STA-IM-003 · Web 聊天持续重连

2026-11-19，部署配置基线 `559bde3`。用户报告 STA `/chat` 持续“重连中”。Chrome Beta 登录后复现：SDK `providerConnected=false`，请求 `https://wsim-sta.43chat.cn/health` 失败。独立 HTTP 查询该路径为 404，回环源站 `127.0.0.1:19003/health` 为 200、正文 `{"status":"ok"}`，带跨域响应头；部署配置摘要与基线一致。

实际 Web SDK 的 `serverList` 模式先经 `Network.detect` 请求 `/health`，成功后才建立 `/im` WebSocket。STA Nginx 只转发 `/im`，其余请求直接 404，遗漏健康探测路由。导航路径的 404 同时存在，但该模式不使用 `navList`，不据此扩大修改。

修订仅在 `deploy/sta/nginx-im.conf` 的 `wsim-sta.43chat.cn` 虚拟主机增加精确 `/health` 到同一 WS 源站的代理；保留原 `/im` 升级配置和其他路径的 404。不涉及业务源码、SDK、数据库或容器配置。

部署前须核对实际旧配置摘要、备份原件，对候选运行 Nginx 语法检查；失败恢复原件。部署后验证公网 GET／OPTIONS 健康探测、真实已登录页面 SDK 连接及刷新重连，并比较其余 Nginx 配置和业务／IM 容器身份快照。

现有证据在本机协调目录 `output/playwright/sta-chat-reconnect-20261119/`：`authenticated-before-network.json`、`other-nginx-before.txt`、`containers-before.txt`。当时页面采集为首次连接阶段，不能把它写成持续重连已恢复；修订尚待部署和页面复测。

## 发布和验收

独立只读审查确认修复与 SDK 的 serverList 链路相符；健康路由同时转发 OPTIONS，CORS 由源服务提供，原 WebSocket、API 和兜底路由保持。冻结候选配置 SHA-256 为 `e24ce6d90b560f17658f7e16619e06afd80be80c71da5d0a20fa74bd3b53b63b`。

首轮候选及完整 Nginx 语法检查通过，但 `systemctl reload` 返回后立即探测仍收到 404，脚本退出 1 并恢复两份原配置。已回查目标、源副本和备份均为旧摘要 `78940cf2a4c8036db07471ca32376b2661b67c58452b6ea5606589a4871e10ff`，没有 health-new 残留；该次不计发布通过。systemd 的 reload 是发送重载信号，需等待新 worker 生效。第二轮使用有限就绪等待（最多 10 轮，每轮 HTTP 超时 3 秒，轮间 1 秒），严格要求 HTTP 200 且正文为 `{"status":"ok"}`；超时仍回滚。该轮第 1 次仍为 404，第 2 次就绪，脚本退出 0。两份 STA 配置均核对为候选摘要，备份留在 `/opt/43chat-im-sta/nginx/`。

公网 GET 与带真实 SDK 请求头名的 OPTIONS 均返回 200、正文 `{"status":"ok"}`，跨域允许源和头部为 `*`，见 `public-health-after.json`。部署前同一已登录页面明确显示“重连中”、providerConnected=false（`authenticated-before-retry-state.json`）。04:01:38 UTC 刷新后，实际 WebSocket 地址为 `wss://wsim-sta.43chat.cn/im`，SDK `providerConnected=true` 且无连接异常提示；04:03:09 UTC 观察仍连接；04:03:33 UTC 再刷新、新建 WebSocket 后仍连接，见 `authenticated-after-network.json`、`authenticated-stable-state.json`、`authenticated-refresh-network.json`。

其余 35 份 Nginx 配置摘要、业务 pre／业务 sta／IM sta 三个容器的 ID 与启动时间均与本轮部署前相同，见 `server-preservation.json`。第一次对比控制器误并行读取尚未生成的 after 文件而失败，未计通过；顺序等待文件生成后重新比较才通过。

本项仅关闭已登录 Web 聊天连接持续重试故障；未发送真实消息，不把连接通过写成消息收发、心灵传输全流程或长期可用性保证。没有数据库变更，未重建或重启应用／IM 容器；未对生产作发布。Nginx 原配置回滚文件保留，临时上传件和检查配置在完成后清理。
