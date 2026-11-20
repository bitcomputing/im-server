# ML-DG003-RETIRED-PAIR-MEMBER-001：注销仍留在原三人群

2026-11-20，固定 IM `f45e78b67d991fabc2ddcbd90856963ea853bc52` 的正式 binary `c1ae3e3003c9bb991d5338590a8b808400b2aea5af5b4ab70c30466f70e9b1b0`，配 BIZ `081de8f44919e1bd344fd02c52e0b6b778914e78`。

隔离真实双库旅程经正式 Store、worker、签名 HTTP、独立业务来源程序和真实 IM 进程注册 A/B、建三人群、改名、跨机器接手，再正式注销 A。A 已停用、墓碑 retired、真群移除 A，但原三人群仍 active，groupmembers 仍有 owner+A+B。失败原件在本机 `D:/ClaudeCodeProjects/output/dg003-full-qa-20261120/lifecycle/attempt-3/`。

11-05 产品定稿 §3 要求“号停用、永不复用，出群”；唯一业务契约 §1 同样要求停号、出群，未给三人群豁免。主人及另一分身的历史读取不要求保留已注销 A 的活动成员行。根审查已按该要求确认缺口。

根因：BIZ 关闭涉及 A 的 pair 写入，旧 pair 不再形成新的 active C1 名单；N1 `transactPrincipalDisable` 只提交主体停用与退休墓碑，未移除该主体的剩余原生成员资格。真群 C1 的名单更新只关闭真群，不覆盖原 pair。

最小修复在 source-verified N1 的原事务内，按 appkey 与稳定 principal 删除 groupmembers。保留所有会话映射、原 C1 成员快照和历史正文，保留 owner、B 的原成员行及禁言属性；不改 schema、接口或契约。原事务已持有 owner barrier 与 principal 锁；C1 同样在 owner barrier 下锁定每个 principal 并要求 active，因此较晚 C1 不能在退休后重加 A。

验收入口：BIZ 独立分支的 `TestDG003LifecycleCurrentPair1120`，核实际双库及双方原回执；额外以 pair 成员 DELETE 的真实 SQL 故障验证停用、墓碑、退群和原回执共同回滚，撤故障后用原 N1 键恢复。继续核新同名身份与新 pair、旧原文不变、owner+B 历史读取、只读 owner、迟到原 P1 拒绝及原成员零残留。证据与完整结果保留在上述本机 lifecycle 输出目录；没有安装到共享环境或发布。
