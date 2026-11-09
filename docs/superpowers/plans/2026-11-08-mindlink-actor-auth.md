# Mindlink 原 actor 认证 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现已确认票据契约的可复用请求验证，提供不可由业务 DTO 构造的认证结果，随后交给同事务权限屏障；本步骤本身不宣称 POLICY、HTTP 接线或 DB 闭环完成。

**Architecture:** 在 `commons/mindlink` 使用标准库验证紧凑 Ed25519 JWS；先校验严格 JSON/签名/信任绑定，再读取并固定实际请求原字节。认证结果只读携带 claims 和 body，语义摘要、原意图/jti 持久化及最终权限留给后续具体操作事务核验，不在认证阶段假造 ready。

**Tech Stack:** Go 1.25.12+；标准库 crypto/ed25519、encoding/json、net/http；不增加依赖。

**Spec:** `D:/ClaudeCodeProjects/chatbot43/docs/mindlink-conversations-contract.md` §11.4（本轮实际 HEAD `3eb7d2b7f53275582998914c3f50d1060c444963`）；原任务 `DG-20261104-003-IMPLEMENTATION.md`、产品定稿和原管理报告 §19。产品与本子集已获实施授权，不重开产品设计。

## Global Constraints

- header 固定 EdDSA / mindlink-actor+jwt / 受信 kid；IM 只持公钥。
- issuer+kid 固定 environment、tenant_id、appkey、audience 及 query/mutation 独立 allowlist。
- 22 个已确认 payload 字段全部显式必填；未知、重复、null、非整数时间/代际/版本拒绝。
- 原始 body SHA256、精确 method/path、固定 target/operation/request/digest 必须保持；GET 无 query/body。
- TTL 为 `0 < exp-iat <= 60` 秒，时钟偏差最多5秒；owner/system generation=0，avatar 后续匹配当前投影。
- 凭据与完整票据不进入错误、日志或交付材料；默认无信任配置时失败关闭。
- 隔离分支 `feat/mindlink-policy` 从 `fca18328483571fd1399f353fc2bdfcfde11668a` 开始；不改 READ 006/007 或已有工作区。

## Review Focus

- 合法签名的未知/重复/缺失/null 字段不能因 Go 零值或静默覆盖而获得身份。
- 签名正确但 issuer、scope、audience 或操作类别不符时拒绝，不能把管理密钥当授权证明。
- method、转义 path、query、body 或 Content-Type 改动必须使原票据失效。
- 时钟边界、整数溢出、损坏 UTF-8/孤立 surrogate、非规范 base64 不能变成合法原 actor。
- 认证对象不能被外部构造或通过返回的可变 body 修改；认证成功不替代提交前核权。

---

### Task 1: 严格票据验证与原请求绑定

**Files:**
- Create: `commons/mindlink/auth.go`
- Create: `commons/mindlink/strictjson.go`
- Test: `commons/mindlink/auth_test.go`

**Interfaces:**
- Consumes: 静态 `Trust` 公钥绑定列表、实际 `*http.Request`、handler 固定 operation_type、当前 `time.Time`。
- Produces: `NewVerifier([]Trust) (*Verifier, error)`；`(*Verifier).Verify(*http.Request, string, time.Time) (*Authorization, error)`；`(*Authorization).Claims() Claims` 与 `Body() []byte` 返回副本。
- `DecodeStrictObject([]byte, any, []string) error` 拒绝重复/未知/缺失/null 必填字段，后续 DTO 复用。

- [x] **Step 1:** 用测试内独立 Ed25519 签发器写合法 owner/avatar/system、无信任和逐字段攻击用例；预期行为是明确的成功身份或分类拒绝，不比较生产实现生成的期望。
- [x] **Step 2:** 运行 `go test ./commons/mindlink -count=1`。先记录缺实现的编译失败；只补公开类型/无验证 stub 后再次观察合法票据认证断言失败，后者才是行为红例。
- [x] **Step 3:** 实现严格 JSON、固定信任、签名与实际 HTTP 请求绑定；不创建签发接口、数据库或真实密钥。
- [x] **Step 4:** 同命令实际通过所有正反例并运行 `go test -race ./commons/mindlink -count=1`；`gofmt` 与 `git diff --check`。受影响集成包在 Linux 目标核对，保留本轮 Windows 基线已有 metrics.Swap* 编译失败。
- [x] **Step 5:** 只提交本计划及本任务源码/测试；回报认证子集实际验证，不把它记作权限事务、消息持久化或 DB 无新增已验。

## 后续原任务

原派单仍进行中：BIZ 正式落定第一条最小 POLICY DTO / operation_type / OperationResult 后，接公共 HTTP、MySQL 原意图/jti 和最终权限事务，跑越权拒绝及真实 DB 无新增，然后推进结果查询/可靠发送。认证步骤先行；不以 READ 整包或本文完成作为原任务结束。
