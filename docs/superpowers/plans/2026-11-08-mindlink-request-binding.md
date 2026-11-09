# Mindlink 请求事务绑定 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在真实 MySQL 事务内绑定原 operation/request/jti，并强制调用提交前权限检查；相同意图重试只返回原结果，权限失败或写失败不留半条操作。这是首条正式 POLICY HTTP 接线所需基础，不代替尚未接入的具体权限策略或原生消息链。

**Architecture:** 复用 `database/sql` 事务和已实现的 `Verifier`。所有权威读取、权限屏障与实际 SQL 效果必须使用同一个传入事务；核权 callback 不可省略。仅提交成功后返回结果；提交异常按结果未知处理，不能推断没写。Wire DTO 和完整 OperationResult 继续由 BIZ 唯一契约定义，内部保存的结果原件不另造公共协议。

**Tech Stack:** Go 标准库 database/sql、crypto/rand、SHA256、JSON；已有 MySQL driver 仅用于 integration 测试。MySQL8，两个新 InnoDB 元数据表。

**Spec:** 唯一契约 §11.1/11.4，原任务 DG-20261104-003；认证基础 `efcc96cd7721d14dbd1c43351cb2f5abed699cee`。原 actor、scope、target、policy、摘要、body hash、method/path 持久绑定不能被刷新票据更换。

## Global Constraints

- 新迁移 `commons/dbcommons/sqls/20261108.sql`，同步 `sql/imserver.sql`；不改 READ 的 006/007。
- `mindlink_operations` 唯一键 issuer/operation_id、完整 scope+actor+request_id 的 SHA256 键；保存完整不可变绑定、预留标识和原结果。
- `mindlink_tickets` 唯一键 issuer/jti，绑定完整已验证票据；拒绝跨原意图重放。
- 元数据标识采用二进制比较，不引入 MySQL 大小写或尾空格等价。
- 无有效认证对象、不同 Verifier 构造的对象、GET/query、缺核权/效果 callback 均失败关闭。
- 同一意图的重试仍先核当前权限，再返回原结果；不得再次执行 effect。完整业务结果查询路由另行精确定义。
- 不保存真实 Token/签名票据或消息全文；结果只应为操作元数据。不提供签发能力。
- 仅独立本地新容器/数据卷；不触 juggleim、READ、Rocky 或现有数据。未设置专用 DB 环境时 integration 明确 skip，不当通过。

## Review Focus

- 仅同 operation 的绑定相同不足以证明新 request/jti 没有被挪用，三种冲突均须拒绝且原数据不变。
- 并发重复请求与丢响应重试必须只有一个实际数据库效果。
- 权限检查必须和效果共用事务及锁；撤权提交后旧代际不能再写。
- SQL/核权/提交失败不得留下半条 operation/ticket；commit 失败不能说效果没发生。
- 存量异常空结果、零值认证、不同 verifier 和 query 不能被误当可执行 mutation。

---

### Task 1: 原意图与 jti 绑定事务

**Files:** Create `commons/mindlink/transaction.go`, `commons/mindlink/transaction_integration_test.go`, `commons/dbcommons/sqls/20261108.sql`; modify `commons/mindlink/auth.go`, `sql/imserver.sql`, `.github/workflows/ci.yml`。

**Interfaces:**
- `(*Verifier).Transact(context.Context, *sql.DB, *Authorization, func(*sql.Tx, Claims, bool) error, func(*sql.Tx) (json.RawMessage,error)) (*MutationResult,error)`。
- `MutationResult` 只含原 `json.RawMessage` 结果和本次 `Replayed bool`；完整 HTTP OperationResult 不由此类型冒称已定。
- 核权函数必须读取并锁定当前权威事实；第三参数由日志判定原操作是否已提交：复放核当前读取权限，新操作核提交权限及期望版本，不能因自身已更新版本而遗失原回执。禁止在事务外只验一次再写；effect 仅使用该 SQL 事务，不作外部投递。

- [ ] **Step 1:** 在专用 MySQL8 建测试库，先写真实权限失败/成功效果、同键冲突/重试、并发、写失败回滚、撤权锁顺序和假证明拒绝用例；新库/升级结构及旧数据保持测试。
- [ ] **Step 2:** 先记录缺实现，再用仅 API stub 实际观察允许的原请求没有写入效果的红例。
- [ ] **Step 3:** 实现两表迁移、原请求/jti 绑定、同事务核权及效果/原结果持久化；语义 DTO 的摘要复算仍由具体 handler 完成，不能省略。
- [ ] **Step 4:** `go test -tags=integration ./commons/mindlink -count=1` 在本次专用 DB 实际 PASS 且无 skip，核对 schema/行数与并发唯一效果；默认 auth/race 及 Linux 检查保持通过。
- [ ] **Step 5:** 独立审查、必要红绿修复及本地提交；报告 DB 基础的真实边界，继续首条 POLICY HTTP/原生拒绝链，不结原任务。
