---
name: feishu
description: AnyClaw 飞书绑定指南。内置飞书通道（Go SDK）；推荐扫码绑定（与官方 openclaw-lark「新建机器人」同接口），也可手填凭证。
homepage: https://open.feishu.cn/
metadata: {"nanobot":{"emoji":"📋"}}
---

# AnyClaw 飞书绑定指南

当用户询问「怎么绑定飞书」「如何接入飞书」「飞书机器人怎么配置」时，用简洁、友好的方式一步步引导。

## 核心要点（先讲清楚）

1. **AnyClaw 已内置飞书**：基于飞书官方 Go SDK（larksuite/oapi-sdk-go），配置即用
2. **推荐：扫码绑定（等价官方插件「新建机器人」）**：用户说「绑定飞书」等时，AI 应调用 **`bind_feishu_scan`**：与 `npx @larksuite/openclaw-lark install` 使用同一设备注册接口，**无需手填 app_id/app_secret**，会把**终端风格二维码 + 链接**发到当前聊天，用户在飞书里扫码创建机器人；完成后自动写入 `config.json` 并触发网关重启（Linux/macOS；Windows 需用户手动重启网关）
3. **备选：手填凭证**：用户已有开放平台应用时，可自然语言提供 app_id、app_secret，调用 **`update_feishu_config`**

## 扫码绑定（推荐）

**触发**：用户表达绑定飞书、不想手填密钥、要扫码等。

**AI 动作**：调用 `bind_feishu_scan`（可选参数 `env`: prod|boe|pre；`lane`: 与官方 `--lane` 一致）。

**用户侧**：用飞书扫消息里的码或打开链接完成「一键创建」；完成后等待助手提示成功。若终端扫码困难（如 Windows 终端分辨率），可只看消息里的**链接**或用 Cmder 等终端。

**与官方文档一致的建议**（可在成功后简短提醒）：飞书里发任意消息开始对话；需要代用户操作文档/多维表/日历等可发 `/feishu auth`；验证安装发 `/feishu start`。

## 手填凭证绑定

在 AnyClaw 页面聊天（或 Telegram 等已配置 channel）中发送 app_id、app_secret，AI 调用 `update_feishu_config`，保存后自动重启（Unix）。

示例：「绑定飞书，app_id 是 cli_xxx，app_secret 是 xxx」

## 手动编辑 config / Docker / Launcher

与原先相同：在 `channels.feishu` 填写 `enabled`、`app_id`、`app_secret`、`allow_from`；或使用环境变量 `ANYCLAW_CHANNELS_FEISHU_*`。

## 回复风格

简洁、友好、按用户进度逐步展开；优先引导 `bind_feishu_scan`，其次手填或文档链接。
