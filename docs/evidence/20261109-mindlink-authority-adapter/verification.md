# BIZ只读来源接收增量

DG-20261104-003，批次8ce3abd7 / 消息pb2lyhh2gxahukeh。IM基线1cfcbbbb；BIZ固定75739e949b7c1b5f08bf9f5f7013ed8c01eb4cba。机器记录、源/日志哈希及明确未覆盖见[verification.json](verification.json)。

新增可直接构造调用的 `NewDisableAuthoritySource(DisableAuthoritySourceConfig)` / `Check(ctx, verifier, authorization) error`，只验证退役观察及其原N1绑定。配置默认关闭，Program/ConfigPath只取服务端固定绝对路径；实际exec不经shell、最多32KiB stdout和10秒，保留更短调用方deadline。复用N1解析器独立核15槽摘要；严格21字段，限定固定版本/原system actor及全部scope/owner/parent/wire/body hash/目标/代际/policy，拒绝无效/多份JSON及非零退出，调用前后核票据有效期，不缓存成功。

这个API不返回可传递grant，不写owner/principal/回执，不接生产SQL/HTTP，不启用公开POST；既有028原语和所有配置/迁移保持。BIZ账户锁在输出前已释放，一次成功检查不是跨库权限租约。

## 本方实际验证

Windows，Go1.26.1。进程协议测试启动真实子测试程序；其成功JSON是协议夹具，不能当成真实业务SQL读取。另实际构建并调用BIZ原命令，只验证默认关闭，不连接业务DB。

| 验证 | 当前结果 |
| --- | --- |
| 无实现时新增源测试 | exit1，缺少新类型/函数的编译RED；不作为运行测试证据 |
| `go test -race ./commons/mindlink -count=1 -json`，提供自有BIZ程序/私有disabled夹具 | exit0；30顶层+304子例PASS、0FAIL、0SKIP；含真实BIZ默认关闭、独立10秒上限、调用中票据过期、早期取消、所有字段缺失/null/异绑定、合法JSON但非零exit、超限/重复/尾随/数字词法等 |
| 较早同增量 `go test -race ./commons/mindlink ./commons/errs ./commons/tools -count=1 -json` | exit0；最终mindlink计数以上一行为准，不重复相加；errs为1顶层+8子例，tools为2顶层+8子例，均0FAIL/SKIP |
| 跳过绑定验证的独立Go overlay | exit1，`avatar_id/different`实际失败；未改生产工作树 |
| 忽略子进程退出码的独立Go overlay | exit1，`valid_json_nonzero`实际失败；未改生产工作树 |
| 最终定向vet：mindlink/errs/tools | exit0 |
| `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c ./commons/mindlink` | exit0；交叉编译，不是Linux执行或Go1.25.12运行 |
| BIZ精确75739独立worktree `go build -mod=readonly ... ./cmd/mindlink_authority_source` | exit0；制品位于Root忽略目录，SHA256 e0354ec8bf8e26f654375aa9775a0dd843ac233035fa6b06a3f61a479d4c26a7 |
| BIZ真实制品+自有私有 `{"enabled":false}` 配置 | adapter返回NOT_READY；直接进程exit3且stdout空，未读取DSN或查询DB。只对新建夹具设当前用户私有ACL，不改真实服务配置/凭据 |
| 固定来源清单/协议 | 实核16文件：4生产Go、5测试Go、7文档/证据/STATE；对应21字段与接收器逐字匹配，N1机器块SHA87290e1f…87f9f未变 |
| 独立审查review_authority_source_adapter | C0/I0，1项旧文档描述Minor已修复复核关闭；新增时间测试已读审。审查方另跑源race通过、其未提供BIZ环境故可选真实进程测试SKIP，不计入本方pass |

## 未覆盖和后续接线

本批只读Docker `/version` 返回HTTP500/exit1，未进行容器/数据库生命周期操作。没有真实enabled BIZ SQL成功快照被IM消费、owner/principal生产投影writer或mutation grant、跨库撤权竞争、真实SQL effect/HTTP/WS/SDK/UI/重启、Linux执行或部署验收。提供方的SQL生命周期/6个exe/历史大套测试及资源清理仅作背景，不记本方重跑。

当前Source只给原意图关联的退役事实，不给可在IM最终提交时比较的owner撤权版本/栅栏，也不创建可信既有主体/owner授权。后续writer必须取得真实、可追溯的独立投影及提交授权来源，并证明所有相关撤权与effect/receipt提交共享可验证顺序；一次新鲜Check不能代替此部分。缺投影或屏障继续NOT_READY，不能把fixture或合法票据转换为grant、先预写disabled，或开启ensure。

本批未改BIZ/W1/W2/READ、现有SQL事务、HTTP/proto/配置/迁移；没有合并、推送或发布。自有build/log/disabled夹具保留于忽略.local目录；未触碰提供方被策略阻止删除而保留的数据目录。原066/c82及完整F1/F3继续在途。
