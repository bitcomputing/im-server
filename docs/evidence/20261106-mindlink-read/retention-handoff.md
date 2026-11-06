# F1-READ retention regression handoff

大当家当前派单 `pbyceyzlpwqhukeh` 明确：原 IM 管理在 F1-RELIABLE 内负责共享非复用持久序列/高水位及依赖提交；二管家继续负责 READ。这里交付复现材料，不转移共享链写权，也不要求将它转给二管家。收到可审查修复版本及交接后，由二管家跑红转绿、READ 回归和独立复审。禁止回退 seen 掩盖问题；合并和部署授权不变。

## 原始版本、路径和结果

原 managed worktree：`C:/Users/jogle/.codex/worktrees/ffc4/im-server`，分支 `feat/mindlink-read`。红例源码提交 `e3957eaea996c2f3b0e528386229fe915eb22de6`，文件 `simulator/probes/mindlinkread/retention_test.go`；初始 fixture 来自同包 `runtime_test.go`。前置 READ 实现为 `008bd1f`，clean-time SQL 修复为 `584f903`，Mongo 缺行错误修订为 `80bdd87`。

原红例的 IM 镜像为 `sha256:66f045dd93e2abb3d5190fcadb160dc75685b49637e23b8625570eb00da9133e`，运行 binary SHA256 为 `49d9e6b5f09b4afd382eb11343b7e3b5d73706b08e916c666d1ab736f2d91b87`。它对应 `008bd1f` 的源码快照，不是未来修复后的候选版本。

原件均在上述 worktree 的 `.local/f1-read/`：

| 文件 | 记录 |
| --- | --- |
| `runtime-before.log` / `runtime-after.log` | 原始三消息 fixture、普通 IM 重启验证；均 exit 0 |
| `retention-purge.log` | 实际 `GroupHisMsgDao.DelMsgsBaseTime` 清除该自有租户历史，核 DB 历史为 0、seen 仍为 3；exit 0 |
| `retention-restart.log` | 仅重启原自有 IM 的 Compose 输出 |
| `retention-red.log` | 冷启后真实发送成功，但读到 `latest=1, seen=3, has_new=false`；exit 1 |
| `db-before.log` / `db-after.log`、`version-before.json` / `version-after.json` | **清理前**普通重启的 DB/制品证据，不能替代 retention 红转绿 |

`private.env`、`runtime-private.json` 含随机测试密码、租户密钥或 Token，不进入消息、提交或共享附件。这里提供结构及生成步骤，不复制其值。原 `.local/f1-read/compose.json` 是原环境定位材料。原栈的容器、卷和数据仍归二管家并保持停止；原管理使用自己的隔离栈及新生成的私有值，不能直接启动或清理原栈。

## 必要的最小 fixture

| 项目 | 最小条件 |
| --- | --- |
| 存储/服务 | 隔离 MySQL `jim_db`；包含 `conversation_seen` 和真实消息生产链的重建 IM |
| 租户 | 测试自动新建随机 `sr` 前缀租户；不同运行独立，实际密钥由本地 fixture 私存 |
| 主体 | `read-owner`、`read-sender`、`read-peer` 三成员群；owner 用 Web 和 PC 两个真实 WS 会话 |
| 初始消息 | sender 顺序发三个存储消息，flags 为 10、8、72；基线历史序列为 1/2/3，最后一句为第三条 |
| 阅读位置 | owner 报告 2、再由另一端报 1/2/3/2/3，DB seen 最终为 3；首页 clear 不得改 seen |
| 私有文件结构 | `/evidence/runtime-private.json` 由 `TestSeenBeforeRestart` 自动生成：app_key、secret、token、group、seq=3、ids（三个真实消息 ID）；不手写假 ID/假回执 |
| 清理动作 | `TestPurgeSeenHistoryMySQL` 调用真实 DAO，cutoff 为该次测试当前毫秒+1；只清这个随机自有租户，不删除 seen；这是将保留阈值推进到测试消息之后，不需等 180 天 |
| 冷启与检查 | 仅重启 IM，MySQL 保持；`TestSeenAfterRetention` 发一个 flags=8 新消息，等可查询历史后检查 latest>seen 且 has_new=true |

原错误是新消息的 seq 复用为 1。正确目标是新消息使用大于既有 seen 的不可复用位置；不要求失败尝试之间连续无间隙，不允许把 seen 重置为 0/1。

## 复现命令和隔离映射

下面是原红例的命令形式。原管理应把四个路径/网络变量替换为自己的隔离资源；`readSource` 必须含上述 READ 提交及待验证的共享修复，不能只运行一个无关 IM。`readEvidence` 是自己的新私有目录，`readPrivateEnv` 是本栈新生成的私有 env 文件。不要复用原栈的容器/卷/私有文件。

现有 probe 只支持固定的隔离内部 DNS，且检查 `JIM_F1_READ_ISOLATED=im-f1-read-20261106`。自有 Compose 的 default 网络中为 mysql 和 im-server 设置如下 aliases；只在自己的私有网络映射，不修改 guard、不把原 F0/shared/pro/pre 服务挂上这些名字。fixture 不需要向宿主映射端口。

```yaml
services:
  mysql:
    networks:
      default:
        aliases: [im-f1-read-20261106-mysql-1]
  im-server:
    networks:
      default:
        aliases: [im-f1-read-20261106-im-server-1]
```

以下变量须事先设为原负责人自有且已核对的路径/网络：`readSource`（源码根）、`readEvidence`（私有证据目录）、`readPrivateEnv`（env 文件）、`readNetwork`（独立 Compose 网络）、`readCompose`（本栈 Compose 文件）。原实际值为 worktree、其 `.local/f1-read`、其 `private.env`、`im-f1-read-20261106_default`、其 `compose.json`，仅用于定位历史运行。

```powershell
$readRunnerArgs = @(
  'run', '--rm', '--name', 'im-reliable-read-probe',
  '--network', $readNetwork, '--env-file', $readPrivateEnv,
  '-e', 'JIM_F1_READ_ISOLATED=im-f1-read-20261106', '-e', 'CGO_ENABLED=0',
  '-v', "${readSource}:/src:ro", '-v', "${readEvidence}:/evidence",
  '-w', '/src', 'golang:1.25.12-alpine'
)
$readStackArgs = @('compose', '--env-file', $readPrivateEnv, '-f', $readCompose)

# 先自行重建候选镜像并启动此独立栈；确认 /health 为 ok，再生成全新的真实 fixture。
docker @readStackArgs exec -T im-server wget -qO- http://127.0.0.1:9003/health
docker @readRunnerArgs go test ./simulator/probes/mindlinkread -run '^TestSeenBeforeRestart$' -count=1 -v

# 普通重启检查：已有最后一句须先实际刷新到 DB，再重启 IM；不得重启 MySQL。
docker @readStackArgs restart im-server
docker @readStackArgs exec -T im-server wget -qO- http://127.0.0.1:9003/health
docker @readRunnerArgs go test ./simulator/probes/mindlinkread -run '^TestSeenAfterRestart$' -count=1 -v

# 核心 retention 红例：这会删除上一步新建的测试租户历史，需在普通重启检查之后单独运行。
docker @readRunnerArgs go test ./simulator/probes/mindlinkread -run '^TestPurgeSeenHistoryMySQL$' -count=1 -v
docker @readStackArgs restart im-server
docker @readStackArgs exec -T im-server wget -qO- http://127.0.0.1:9003/health
docker @readRunnerArgs go test ./simulator/probes/mindlinkread -run '^TestSeenAfterRetention$' -count=1 -v
```

每步核对退出码；健康/fixture/清理失败不得继续算验收。原始最后一步 exit 1，准确输出见原件；修复后应 exit 0 且真实新历史 ID/序列、seen 行和 has_new 相互对应。不要直接对整个 live probe 包不加 `-run` 运行，Go 的测试顺序不能代替外部 IM 重启或新 fixture 建立。

普通重启前须实际查询本次自有租户的 `conversations`：owner 行的 `latest_msg_id` 等于第三条 ACK ID，`latest_unread_msg_index/latest_read_msg_index` 均为 1；`conversation_seen` 对应行为 3，`g_hismsgs` 有该组三条真实 ID/序列。PERSIST 正常刷新周期为 60 秒，但应以真实 DB 行为准，不能只睡一段时间后假设已落库。

## 直接对齐的接口条件与依赖交付

- 提议的序列作用域是 `app_key + channel_type + GetConversationId(真实参与者/目标) + sub_channel`。群内成员共享消息排序；用户 seen 再增加 user 维度。私聊沿项目既有 canonical conversation 规则，不从客户端任选 conversation ID。需原管理确认实现范围和错误语义，不把此提议当已实现接口。
- 历史清理、缓存淘汰和 IM 重启不得复用已确认的排序位置。跨租户/会话/子通道要隔离，允许失败尝试留下 gap。
- 序列持久依赖出错不得降级到缓存/0 后成功受理；failed/unknown 按 RELIABLE 契约保留区分。READ 范围查询错误不能变成成功的 latest=0。
- 分配高水位可以领先实际历史；READ 的 latest 必须取同 scope、授权可见、已落库的历史，不能直接取预分配 floor。真实空历史和查询错误必须区分。
- BIZ §11.3 适配同一个原生 seen 权威源，不另建互不一致的业务游标。业务 seen HTTP、原 actor 认证、原生 RPC 各自登记；当前未注册的 HTTP 不因原生候选而可用。
- 原管理交付可审查 commit/range、具体写集、迁移/新库顺序、错误及 scope 语义、实际生产调用链/DB/失败/清理与重启证据和未覆盖项。修复版本进入 READ 验证前双方对齐源码/制品版本；本地验证不授予合并或部署权。

本材料只交接原件与可复现步骤；本轮没有重跑产品测试，retention 仍为阻断。私聊、跨子通道及 DB 错误的共享序列修复也需相应证据，不能由这条群聊红例推导全部通过。
