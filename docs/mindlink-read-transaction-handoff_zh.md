# READ 与公共操作事务的调用交接

DG-20261104-003，回应 READ 的 `pbz2lkxhdzghukeh`，当前批次 `8ff3fa4cb1f9fdc8a36f32c2701396f3366b52d26ab0bdef2974437e29d11320`。ROOT 公共分支基线为 `92e312027fa4cf9887187a7046ee169d05c68bee`，READ 本轮只读核到 `d336dec4cae9eff4067938b18e05215a5a0c60fe`，均为干净工作树。没有改动 READ 写集或合并其分支。

## 已有生产者与缺口

公共事务函数位于 `commons/mindlink/transaction.go`（实现 d3fa119、时效修正 48037f8）：

```go
func (v *Verifier) Transact(
    ctx context.Context, db *sql.DB, auth *Authorization,
    authorize func(*sql.Tx, Claims, bool) error,
    effect func(*sql.Tx) (json.RawMessage, error),
) (*MutationResult, error)
```

authorize 的 bool 由原 journal 判定是否已提交复放；新效果核提交权限/期望版本，复放核当前读结果权限。原结果由 effect 返回的 JSON 在同一 SQL 事务内保存，commit 成功后才返回。它是通用 journal producer，不是正式 OperationResult 构造器。

本轮源码检索确认：ROOT 网关尚无 `QryConverSeenWithTx`/`SetConverSeenWithTx` 调用；OperationResult 仅有旧候选值类型，没有正式生产者或 READ HTTP 路由。Transact 显式拒绝 GET，不能直接用它承接独立 query 意图，更不能 Set(0) 代替 GET。生产 POLICY 权威、共享最终屏障、typed 回执与 query journal 均仍缺失；`92e3120` 的可信 RPC/屏障是设计候选。

## 已验证的 GORM 调用形式

在 Transact 的 effect 中，使用其传入的同一个 tx；构造只复用现有依赖，不创建另一个连接池：

```go
gdb, err := gorm.Open(gormmysql.New(gormmysql.Config{
    Conn: tx, SkipInitializeWithVersion: true,
}), &gorm.Config{
    DisableAutomaticPing: true,
    SkipDefaultTransaction: true,
    Logger: logger.Default.LogMode(logger.Silent),
})
if err != nil {
    return nil, err
}
gdb = gdb.WithContext(nativeCtx)
```

gormmysql 为 `gorm.io/driver/mysql` 别名；gorm/logger 为已有 GORM 依赖。日志示例采用 Silent，生产诊断需避免 SQL 参数、票据和消息正文泄露。nativeCtx 的 appkey/IM requester 必须由已核权、已锁定的映射生成，不能把业务 owner_id、request_id 或 URL 会话 ID直接当 IM requester/target。`bases.CreateRpcCtx(appkey, mappedIMRequester)` 的 requester 参数是 IM 用户身份，原 ToRpcCtx 的同名 requestId 参数也实际写入 requester。

READ 已有签名可以保持：

```go
QryConverSeenWithTx(ctx context.Context, tx *gorm.DB, req *pbobjs.QryConverSeenReq) (errs.IMErrorCode, *pbobjs.ConverSeenState)
SetConverSeenWithTx(ctx context.Context, tx *gorm.DB, req *pbobjs.SetConverSeenReq) (errs.IMErrorCode, *pbobjs.ConverSeenState)
```

将上面的 gdb 传入即可保留 `Statement.ConnPool == tx` 和 gorm.TxCommitter。不要传全局 `*gorm.DB`，不要再调用 gdb.Transaction/Commit。READ 在存储错误时执行 Rollback 必须终止同一外层事务；ROOT 必须停止 effect 并返回错误，不能换全局句柄继续写。即使上层误忽略下游错误，已回滚的同一 sql.Tx 也应让后续 journal 写入失败，不能变为自动提交。

## 本轮真实验证

新增 `commons/mindlink/gorm_transaction_integration_test.go`，只复用本方隔离的 `_mindlink_test_effects`；没有新增 seen 表、修改原生 READ 或运行整包 READ 验收。

- RED：错误地让 GORM 接收全局 sql.DB；真实 MySQL 观察到根事务提交前外部连接已读到 effects=1，测试确实失败。
- GREEN：改为 Conn:tx，`go test -race -tags=integration ./commons/mindlink -run TestMySQLGORMJoinsOriginalRequestTransaction -count=1 -v`，1 个顶层测试、3 个子场景通过，无失败/跳过。覆盖根事务提交、调用方拒绝后全部回滚、下游 SQL 失败后主动回滚且误继续也不能写 journal；事务内可读自写，外部连接提交前读不到。
- Windows Go1.26.1、GORM1.31.1、MySQL8.0.46，复用既有独立容器/数据卷和私有凭据文件，未新增外部资源。日志留忽略目录，SHA256 `af06175151cfb00d4fdd777c54e744270ae721215cc03b18374f50c342f31d56`。
- 最后回滚场景后于 2026-11-09 04:38:16.643934 UTC 独立 SQL 回查：operation/ticket/测试效果各0。这是测试元数据的时点快照，不是原生历史或 seen 结果。
- 默认 `go test ./commons/mindlink ./commons/errs ./commons/tools -count=1` 三包 exit0。没有重跑 Linux、READ 原生整包、页面或生产环境。

## 后续接线需要的产物

READ 的 d336dec 中 seen 状态查询/推进加入传入事务，当前 `seenLatest` 仍沿既有全局成员、设置、清理和历史存储查询；本桥接测试不证明这些可见性读取处于同一 SQL 快照。ROOT 尚无可交付的严格同事务历史生产者，不要求 READ 在缺少该生产者时重写历史链或返回假快照。

ROOT 后续仍负责：核定唯一 HTTP/typed 结果源，生产原 actor/目标/固定成员/代际/policy 权威及最终屏障；PUT 原操作回执与 seen 同事务；GET 独立 query 意图/state/观察时间原件；必要的可靠历史/latest 生产者。只有这些接入后才运行原请求冲突/重试/撤权、seen 与结果同提交回滚、响应丢失及真实页面验证。

实际推进位置须来自已授权 scope 的最终展示 DownMsg.MsgSeqNo（proto tag6），保持规范 int64 十进制字符串。MsgTime、消息 ACK、摘要 floor 都不是替代输入。当前 99f7ce5 旧字段提案与 BIZ N1 的时间/结果差异按 `mindlink-trusted-rpc-candidate_zh.md` 待同源核定；不要求 READ 并写公共 DTO、JWS、路由或生成另一份 seen 权威。

## READ 固定交付的原件复核（批次 24a14dc5）

本段回应 READ 交付消息 `pbz26y9qd5chukeh`。本批只读核对固定 d336dec 的源码及提供方既有原件，没有重跑 native、重启探针、实时 SQL 或页面；下列提供方实测记录不改称 ROOT 本轮实测。

| 核对对象 | ROOT 本批实际结果 |
| --- | --- |
| 固定 verification manifest | Git blob SHA256 `3eaddbf51d86b47df6354d97ee353ac04974535af62c1cb43df78c9b13f66633`；所列 26 份原件全部存在且哈希匹配 |
| 最终源包 | SHA256 `89620561bfe34b518b9b36690725624fb2424418d029f60e71f34603a1f9a83f`，与 manifest 匹配；772 个普通文件的集合及规范 LF 内容与基线 2387261 加声明的四个 Go 文件相符，无多缺文件或其他内容差异 |
| 四个 Go 改动 | seenservice.go、seen.go、seenservice_tx_test.go、transaction_test.go 的固定 Git blob、源包成员及 manifest 哈希一致 |
| 最终二进制 | 现存 imserver-final SHA256 `a78d0a99197fd762fa2805d42967578f5dd79ba78ebd1f0e30f2f821f6332ff1`；提供方记录中的重启前后 live hash 与其一致，未在本批重跑该二进制 |
| 测试与 runtime 记录 | 本批解析原日志为 12 个顶层 PASS / 11 SKIP / 0 FAIL；前后重启探针各有对应 PASS。11 个跳过仍未验证，不能计作通过 |
| DB 记录 | 提供方重启前后 DB 元数据文件字节一致，且各自哈希匹配；不是本批新的数据库查询结果 |
| 无效证据 | 1205 fixture 原件仍存在且哈希正确，继续排除在有效 1213 重现之外，不以文件存在计通过 |

首次直接比较 Windows Git archive 原字节时退出 1，出现 760 项差异。随后按交付已声明的 canonical LF 规则逐项复核，760 项全部仅为基线 CRLF→LF，非换行差异为零；没有修写提供方文件，也没有把首次失败直接计作通过。

本批另以只读 Docker inspect 核到 READ 三个容器均 exited，镜像与启动/停止时间匹配 manifest；IM exit137/OOMKilled=false、MySQL exit0，fresh 保留原 11月6日时间。此为查询时的资源快照，不表示优雅停机或持续保证。未启动或修改这些资源。

有限接收该固定版本的原生局部适配交付及其证据关联；ROOT 的公共 READ、真实权限屏障、正式回执和页面验收缺口保持。不能由材料哈希一致推导新增 HTTP 已可调用、全产品通过或已经部署。
