# 43Chat sta 部署执行记录

`sta` 与现有 `pre` 并行。用户要求覆盖 IM、43Chat 服务端、Web、Soul。先建独立 IM 验收环境，再接四端。本文件是本项目的部署入口，跨项目组合由各项目整合负责人核对同一组发布版本。

2026-11-20：现有 STA 的新版心灵传输启用配置、authority 对接、原件及准入要求见[STA Mindlink 运行合同](sta-mindlink-runtime_zh.md)。本页收录首次建库、IM 验收与公网接入步骤；现有 `im_sta` 升级不得执行新库 `init` 或重放 `sql/imserver.sql`。升级前在 STA 的 `secrets/config.yml` 中配置 `mysql.maxOpenConns`，模板值为 `16`。完成升级后，用同版 `db.py check` 核对配置和数据库。

## 2026-11-17 首次部署记录

| 组件 | 固定源或入口 | 当日阶段 |
| --- | --- | --- |
| IM | `e12609146c41150023874d63622fbdd39c76fc62` 和本分支 `deploy/sta` | 普通 IM 验收包已暂存到主机 `/opt/43chat-im-sta/staging/im-sta-smoke-b00a3f8.tar.gz`，上传和回查的 SHA256 一致。包内二进制使用 Go 1.26.1 交叉编译。启动前按 go.mod 的 Go 1.25.12 构建发布镜像，并核验 `im_sta` 专用账号。四端心灵传输以通过组合验收的固定版本为准 |
| 43Chat 服务端 | `chatbot43` 的 `sta` 分支 `55840f768` | 现有镜像固定复制 pre 配置。启动、接入协议、环境白名单和 worker 需支持独立 sta 配置。账户、数据库、Redis、队列及对象存储待准备 |
| Web | `43chat_web` 的构建入口 `package.json` 和 `src/config/agentAccess.ts` | 待固定 sta 源和绝对业务 API 地址。验收登录以及首页、账号面板的安装链接全部指向 sta |
| Soul | `43chat-soul` 的 macOS、Windows 发布入口 | 待开发负责人生成 sta 制品。Windows 使用独立机器或虚拟机验收，避免机器级任务与已有安装冲突 |

2026-11-17 只读检查目标主机 `39.98.68.96`。SSH 和 `sudo su` 可用，Docker 26.0.1、Compose 2.39.4，根盘余约 21 GiB。拟用 IM 回环端口 `19001/19002/19003/18090` 未发现监听，`6060` 已占用。两处拟定 sta 目录未发现。宿主机已实际连通 RDS TCP，实际账号和容器网络待核。暂存包写入后回查无上传临时文件，sta IM 容器数为 0。数据库边界单测 6 项通过，Compose 解析在继承模拟 pre 变量时仍选中 sta 项目和配置路径。

## 2026-11-18 IM 验收

- 验收版本为 `e9425c2cc79dc5f2611e950cb69dd1bdaa8e1ad7`，使用 Go 1.25.12 构建 Linux/amd64 制品。发布包、部署机解包及镜像中的二进制均核对了 SHA-256。
- 当次使用 `im_sta` 专用账号通过 VPC 内网连接 RDS MySQL 8.0.36，空库采用 `utf8mb4` 和 `utf8mb4_0900_ai_ci`。初始化后 `db.py check` 读到版本 `20260908`。
- 经 SSH 转发实际登录管理台。独立测试租户完成私聊、群聊和离线补收，三条消息的 ACK ID 与数据库历史行一致。会话摘要落库后仅重启 STA IM，历史与摘要均从服务接口读回，并用数据库核对了相同的消息 ID。
- 当次记录中，`pre` 业务容器的 ID、启动时间和重启计数在 STA IM 重启前后相同。STA 管理台、API、导航和 WebSocket 端口均绑定部署机回环地址。

管理台可经 `ssh -N -L 18090:127.0.0.1:18090 xudong@39.98.68.96` 转发，从本机访问 `http://127.0.0.1:18090/`。

## IM 公网入口

ALB 终止 TLS，按域名转发 HTTP 到部署机 Nginx 的 80 端口。将 `deploy/sta/nginx-im.conf` 安装为 `/etc/nginx/conf.d/43chat-im-sta.conf`，用 `nginx -t` 验证后重载。

- `https://im-sta.43chat.cn` 转发到 API 源站 `127.0.0.1:19001`。
- `wss://wsim-sta.43chat.cn/im` 转发到 WebSocket 源站 `127.0.0.1:19003`。Web SDK 在握手前请求同域名的 `/health`，该路径也由 Nginx 转发到 19003。

当次公网验收记录显示，两个 ALB 地址的 API 请求均返回 200，WebSocket 握手均返回 101。独立测试租户通过公网发起带签名的历史查询并读回原消息。

2026-11-19，补齐 `/health` 转发后，公网 GET 和 OPTIONS 探测、已登录页面的 SDK 连接及刷新重连通过。回滚经过和验收范围见 [STA-IM-003 验收记录](sta-web-reconnect-20261119.md)。

## 重建当前 STA IM 容器

从包含脚本的本机仓库目录执行。第一条检查现场配置和数据库，第二条会重建当前镜像对应的 STA IM 容器，客户端会短暂重连。

```powershell
Get-Content -Raw .\deploy\sta\redeploy.py | ssh xudong@39.98.68.96 'sudo -n python3 -B - --check'
Get-Content -Raw .\deploy\sta\redeploy.py | ssh xudong@39.98.68.96 'sudo -n python3 -B -'
```

脚本根据现有容器找到固定的 STA 发布目录和 Compose 配置，拿到发布锁并确认镜像 ID 与数据库版本匹配后，只重建 `im-server`。容器达到健康状态后才报告成功。发布新提交时按后面的构建和发布步骤准备新制品。

## 准备与启动

1. 在经核定的 RDS 实例上准备空库 `im_sta` 和同名账号，库字符集 `utf8mb4`、排序 `utf8mb4_0900_ai_ci`。账号绑定实测的容器出口。在准确转义的 `im\_sta` 库范围内授予 `SELECT, INSERT, UPDATE, DELETE, CREATE, INDEX, ALTER`，用于建表、写入和库内的启动迁移。STA 单实例的 IM、JuggleChat 和管理台各有一个连接池，`mysql.maxOpenConns` 接受 `1–32` 的整数。模板值为 `16`，单实例合计最多 48 条应用连接。调高前，按发布时最多同时运行的实例数乘以 3 个池和每池上限估算连接需求，核对 STA 账号上限及 RDS 实例余量。凭据经已有密钥渠道交付，受限配置目录权限 `0700`、文件 `0600`。用真实连接回查 TLS 和应用驱动能力，按部署时测得的 RDS 配置定连接方式。
2. 从同一提交构建 `linux/amd64` 镜像。校验发布包 SHA256、镜像 ID、实际运行二进制与该提交。将 `deploy/sta` 和该提交的 `sql/imserver.sql` 放入 `/opt/43chat-im-sta/releases/<commit>`，新建专用 volumes 与 `secrets`、`backups` 目录。按 `.env.example` 写发布配置，按 `config.example.json` 写受限 `config.yml`，写入 sta 独有的至少 32 字符随机 `adminSecret`，并由密钥渠道装入专用密码。
3. 在发布目录用 `db.py preflight` 验证账号、空库、库级授权、应用连接池配置、版本与字符集。核对容器到 RDS 的网络后才初始化。`init` 接受空库并校验同包 SQL 的版本，使用新管理员口令文件。确认安装日志和版本一致后再发布端口。

```sh
set -eu
cfg=/opt/43chat-im-sta/secrets/config.yml
dc() { docker compose -p im-server-sta --env-file deploy/sta/.env -f deploy/sta/compose.yml "$@"; }
dc config --quiet
python3 deploy/sta/db.py preflight --config "$cfg"
# 此处完成发布镜像的容器到 RDS 连通性核对。
python3 deploy/sta/db.py init --config "$cfg" \
  --admin-password-file /opt/43chat-im-sta/secrets/admin-password
dc up -d --wait --wait-timeout 120
curl --fail --max-time 5 http://127.0.0.1:19003/health
python3 deploy/sta/db.py check --config "$cfg"
```

用 SSH 本地转发同端口打开管理后台。测试租户跑真实登录、私聊、群聊、离线补收，核发送 ACK、接收者、历史与 SQL 中的同一消息 ID。确认摘要持久化后仅重启 IM，核历史与摘要恢复。先验证此段，再启动指向 sta IM 和 sta 依赖的业务 API。Web 与 Soul 跑真实接入码、绑定、收发和重连，检查 pre 凭据无法接入 sta。IM 公网入口见上文。

初次启动失败时停止 `im-server-sta` 中的 `im-server` 并保留 `im_sta` 和卷。库级备份只提交给受控目录，恢复使用 DBA 准备的空 `im_sta_recovery` 与专用账号。恢复账号还需该库的 `DROP` 权限。核对恢复结果再决定是否启用。保留故障发生后的原库和新消息用于对账。
