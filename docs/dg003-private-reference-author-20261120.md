# IM-DM-REFERENCE-AUTHOR-001：普通私聊历史引用丢失作者资料

2026-11-20，基线 `39c3e3fe5a32618058008494f90fd1b89fbaf8b1`、正式 Go1.25.12 制品 SHA `4d58d8d3a0e2e32faba9b2bad3bdcf27449965d19b48cb45283612384abb43bb`。DG003 normalflow 的固定 SDK 与现有原 MID 刷新后，嵌套引用作者退回“用户”；目录和被引主消息资料完整。独立判因原件在本机 `D:/ClaudeCodeProjects/output/dg003-full-qa-20261120/normalflow-quote-author-review.md`。

根因：`retrievePrivateReferenceMsg` 按既有 app/conversation/subchannel 重新取持久化原文，清除 `TargetUserInfo` 后替换嵌套引用，未恢复引用作者显示资料；SDK 嵌套引用只读该字段。不能用当前会话对端猜作者，因为引用自己也合法；不能信任客户端引文中的署名。

最小修复：只在普通私聊原文成功读取、删除标记还原之后，复用实时 `FillReferMsg`，按持久化原文 `SenderId` 取得当前可信显示资料，并按请求者还原 self/peer 方向。没有改历史内容、MID/索引、SQL、授权、Native fence、群引用或 SDK。

新增 `TestPrivateReferenceAuthorHistory` 复用现有隔离 MySQL/真实 IM 进程和正常 Native WS 查询：引用自己/对端、作者资料缺失、全局撤回/个人删除标记、不同会话/不同 app 不解析，以及 Native 历史19008屏障。含真实资料查询正控、同一测试 MID 连续两次重读、客户端伪造署名被持久化原文替换、历史/删除表全行不变。正式基线先红：5项作者失败、3项权限边界通过，原件 `output/dg003-full-qa-20261120/quote-author/red/`；首次过长测试 MID 初始化失败另存，未计作产品红例。

验证结果、固定补丁/候选 binary SHA 与工具链、清理及剩余原页面/两库验收由本机 `output/dg003-full-qa-20261120/quote-author/README.md` 汇总。normalflow 的原账号/原引用由其资源所有者在根独审后重读，实施者不改其资料、历史或服务；本地测试与候选构建不代表发布或现场验收。

最终隔离 Native WS 8子项全部通过、0跳过，同一测试MID重读两次，14条历史和1条删除标记全行不变。拟发布 Go1.25.12/Linux amd64/CGO0 binary SHA `6ed8824caa7543790c9bd4683c720dd156164f59ac2fbbdce617ff30d215dc7d`。一次未节流测试末尾遭11014限流，诊断原件捕获真实ACK后仅将测试查询间隔设为25ms，服务端100/s、burst10限制未改；该失败与诊断均保留，不把限流当权限拒绝或作者成功。
