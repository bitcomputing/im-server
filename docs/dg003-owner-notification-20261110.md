# DG003 主人实时同步通知

基线 6e70e7a；隔离候选增量，未发布。Bug ID：ML-DG003-OWNER-NTF-001。先前真实 WebSocket 探针连接、主动同步、游标续读成功，但自动通知失败；收件箱消费者没有调用实时通知路由。

收件箱事务成功后，后台以持久 outbox 的 inboxed 状态重试无正文 ntf；同步时间使用该消息实际收/发件箱位置。当前信任、原回执、主人读权限与会话归属核验后才路由，离线、超时及路由失败保留待办。成功路由标记 signalled，三人会话标记 quiet，不触发普通通知。重复消费不重建收件箱。

signalled 仅表示在线路由回执，并非终端已展示或用户已读；原生持久收件箱与重连同步仍是恢复来源。未增加手机厂商推送或桌面系统通知。

## 验证

- `im-notify-unit.log`：commons/mindlink、apigateway/services 两包通过。
- Linux 完整 launcher 构建通过；`im-notify-db-final.log` 在 Rocky 隔离 MySQL 中 6 个顶层测试通过，0 失败/跳过。覆盖四类收/发件箱、路由失败保留状态、撤读权不通知、重复回执不重复路由、三人会话静默、实际同步时间一致及已有历史/序列事务回归。
- `im-notify-ws-strong.log`：实际候选 launcher、注册 Token API、WebSocket 协议及 BIZ→签名 HTTP→IM 的测试中，自动通知在主动同步前收齐两个指定消息 ID，随后主动同步顺序和保存游标重连无重复通过。原 P1/C1 仍预置，不是首次接入全链。未宣称通知恰好一次。
- 重跑曾因未重建 pending 业务夹具失败；补建隔离夹具后通过，未修改断言。早期仅检查“收到至少一条”的结果不替代上述收齐断言。

日志在本机 `.local/dg003-transfer/` 与 Rocky `D:/projects/.evidence/`，跨项目 harness 位于忽略目录 `.local/dg003-solo/interop/`。未修改 13000/16006 服务，未推送、合并或 pre/pro 部署。浏览器实际 SDK 全链、断进程恢复和独立审查仍未覆盖。
