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

## 2026-11-11 重开 v1：三人群静默误停同步

基线9335588，重开 ML-DG003-OWNER-NTF-001。第003轮真人A/B消息均persisted，B回复pb3hf3lqsdjmjw24已入库；Rocky真实驻留Web未刷新、未重新获焦、文档timeOrigin不变，A原文自动出现，但B独立回复未出现，历史只有5条，摘要仍A原文。原采样substring包含匹配会误把A请求里的B回复标记算为B消息；该匹配已排除，改按独立article段落和列表摘要精确核验，B仍缺失。首次匹配不能算通过。

现场两条IM outbox均quiet。源码SignalOwnerInbox遇avatar_pair_group不调用signal，直接标quiet；现有集成测试也要求pair不调用。这把禁止响铃/推送与无正文SDK同步提示混为一谈。定稿要求三人群不计首页未读、不推送，但“分身之间”的摘要和新消息应实时更新；SignalMindlinkOwner仅发ntf同步位置，不调用手机或桌面推送。

范围：复用既有同步提示，三类合法会话均可同步；保留pair原生flags=8和unread=0、权限/回执/归属检查。旧quiet行仅补发无正文同步提示，不重新入箱、不重放业务任务、不改变阅读游标。先改集成断言复现遗漏，并新增quiet恢复/去重/原消息不重复入箱回归，再最小修复。真实驻留Web必须以全新第004轮复测，重连加载不能算实时通过。系统通知实际点击另验。

## 重开 v2：源码与数据库回归

SignalOwnerInbox不再把pair标quiet并跳过同步，而是验证原有信任/读屏障/会话/持久入箱位置后，对三类合法会话发同一个无正文ntf。旧quiet只允许pair恢复，成功改signalled；失败保持原状态，遍历有界重试。不重新调用入箱投影、不调用推送、不改flags、未读或业务投递。既有重复成功、路由失败保留与撤销读权校验继续覆盖；新增旧quiet恢复一次、重复不重入箱、未读仍0。

Rocky专用MySQL日志：im-pair-sync-red-db.log只pair遗漏与quiet恢复两个目标失败，其余普通群/单聊/主人发送通过；修复后im-pair-sync-green-db.log为7个顶层场景通过、0失败/跳过（含历史、事务、并发、篡改、读屏障、消息序列）。im-pair-sync-unit.log为40通过/1跳过，im-pair-sync-gateway.log为2通过/0跳过，退出均0。首轮red.log因未设置fixture工作目录找不到SQL而失败，不计业务红例；修正cwd后复现上述目标失败。测试临时native schema回查0；只读残留查询曾因REGEXP二进制编码失败，转换字符集后得到0，不把失败算验证。

Linux完整launcher构建通过。候选imserver-pair-sync SHA256为22716d71bd8fffdad1764345291421604906109824912c5dd92f6d0074bba11a，Rocky暂存文件与本机构建相同。自动审批审查拒绝停止并替换dg003-im-e2e，返回仅blocked by policy，无具体理由；没有改用其他方式自动执行。回查原容器仍running，StartedAt为2026-11-11T04:05:11Z，实际/proc/1/exe仍旧SHA256 48c1d18a5d0047a1a0a03636dd4583eed91bd0c231b66ab5b27f198c30e2c479，更新备份尚不存在。候选更新未执行，真实第004轮仍pending。

已准备并上传供主人查看/手动执行的D:/projects/Apply-DG003-Pair-Sync.ps1；语法解析通过，没有执行。脚本限定同一候选容器、验证新旧文件哈希、保留旧二进制、更新后查health和活进程哈希；不改MySQL/Redis、旧13000/16006或pre/pro。自动审批被拒不是用户授权不足的推断；后续需主人手动操作或宿主外部状态变化，不能自动重试绕过。
