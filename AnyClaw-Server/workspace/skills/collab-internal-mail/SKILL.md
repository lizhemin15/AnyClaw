---
name: collab-internal-mail
description: 多员工内部邮件与拓扑协作（与 anyclaw-manager 配置一致时使用）
---

# 内部协作与邮件

## 何时使用

- 需要把任务转给**拓扑上的邻居**时，用工具 `internal_mail_send`（非邻居会被 API 拒绝）。
- 需要当前员工表或服务器上限时，用 `collab_get_roster`；需要邻居关系与拓扑版本时，用 `collab_get_topology`（两者返回的 `limits` 含 thread_id、主题、正文、列表分页等上限，发信与 Manager 拉列表时可对照）。
- 需要按线程或分页浏览内部邮件记录时，用 `collab_list_internal_mails`（返回 `mails`、`total`、`limits`；单封详情仍可用 bridge 按 id 拉取）。
- 只知道**展示名**不知道 `agent_slug` 时，先用 `collab_resolve_peer`；若有歧义须向用户确认。
- `spawn` / `subagent` 与内部邮件**并存**：长后台任务可用 spawn；要留痕、多跳转发用内部邮件。

## 参数约定

- `from_slug`：当前智能体在 `agents.list` 里的 **id**（与 manager 中 agent_slug 一致）。
- `thread_id`：同一线程复用同一 id，回复时填 `in_reply_to` 为上一封邮件的 `id`（见 API 返回或通知）。
- 收到 `[Internal mail id=…]` 类消息时：向用户汇报或处理已交代事项；**除非用户明确要求**与对方邻居继续协调、追问或多跳传话，否则**不要**再用 `internal_mail_send` 与对方自动多轮互发（单次传达或用户要求的一句回执即可）。

## 勿做

- **禁止**在无用户要求时与拓扑邻居自动多轮互发内部邮件（与系统规则第 7 条一致）。
- 勿向非邻居发内部邮件（会失败）。
- 勿把内部邮件内容当作已对用户可见；对用户说话请用其所在渠道（如网页）的 `message` 工具。
