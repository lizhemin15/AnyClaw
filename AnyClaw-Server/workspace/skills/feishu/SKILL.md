---
name: feishu
description: AnyClaw 飞书绑定指南。AnyClaw 已内置飞书（基于飞书官方 Go SDK），配置即用，无需安装插件。
homepage: https://open.feishu.cn/
metadata: {"nanobot":{"emoji":"📋"}}
---

# AnyClaw 飞书绑定指南

当用户询问「怎么绑定飞书」「如何接入飞书」「飞书机器人怎么配置」时，用简洁、友好的方式一步步引导。

## 核心要点（先讲清楚）

1. **AnyClaw 已内置飞书**：基于飞书官方 Go SDK（larksuite/oapi-sdk-go），**无需安装任何插件**，配置即用
2. **全程无需扫码**：创建应用在飞书开放平台网页完成（登录即可）
3. **通过聊天绑定**：在 AnyClaw 页面（或任意已配置 channel）聊天中，用自然语言发送 app_id、app_secret，AI 会调用 `update_feishu_config` 工具自动写入配置

## 通过 AnyClaw 页面聊天绑定飞书（推荐）

用户可在 **AnyClaw 页面聊天**（或 Telegram、飞书等已配置 channel）中，用自然语言发送飞书凭证完成绑定：

**示例消息**（任选其一）：
- 「绑定飞书，app_id 是 cli_xxx，app_secret 是 xxx」
- 「配置飞书：cli_xxx / 我的secret」
- 「飞书 app_id=cli_xxx app_secret=xxx」

**AI 处理流程**：
1. 从用户消息中解析出 `app_id`（以 `cli_` 开头）和 `app_secret`
2. 调用 `update_feishu_config` 工具，将凭证写入配置并**自动重启** AnyClaw
3. 几秒后 AnyClaw 重启完成，飞书通道即可使用

**注意**：app_secret 为敏感信息，建议在私聊中完成。

## 循循善诱的回复模板

### 第一步：创建飞书应用（网页完成，无需扫码）

1. 打开 [飞书开放平台](https://open.feishu.cn/app) 或 [开发者后台](https://open.feishu.cn/)
2. 登录飞书账号（网页登录即可，**无需扫码**）
3. 进入「创建企业自建应用」，填写应用名称
4. 创建完成后，在「凭证与基础信息」中获取 **App ID**（以 `cli_` 开头）和 **App Secret**

### 第二步：配置权限与能力（简要）

- 在应用后台「权限管理」中，按需开通消息相关权限（如 `im:message`、`im:message:send_as_bot` 等）
- AnyClaw 使用 WebSocket/SDK 模式，无需配置 HTTP Webhook URL

### 第三步：通过聊天绑定（推荐）

在 AnyClaw 页面聊天中，直接发送：
> 绑定飞书，app_id 是 cli_你的AppID，app_secret 是 你的AppSecret

AI 会调用 `update_feishu_config` 自动写入配置。

### 第四步：自动重启

配置保存后，AnyClaw 会自动重启（约 3 秒）。重启完成后，在飞书中搜索你的应用并发送消息即可开始对话。

## 其他配置方式

### 手动编辑 config.json

编辑 `~/.anyclaw/config.json`（或 `ANYCLAW_CONFIG` 指定路径），在 `channels.feishu` 中填入：

```json
{
  "channels": {
    "feishu": {
      "enabled": true,
      "app_id": "cli_你的AppID",
      "app_secret": "你的AppSecret",
      "allow_from": []
    }
  }
}
```

### Docker 部署（环境变量）

```bash
docker run -d \
  -e ANYCLAW_CHANNELS_FEISHU_ENABLED=true \
  -e ANYCLAW_CHANNELS_FEISHU_APP_ID=cli_你的AppID \
  -e ANYCLAW_CHANNELS_FEISHU_APP_SECRET=你的AppSecret \
  ...
```

### anyclaw-launcher 或 AnyClaw-Web

在「通道配置」→「Feishu」中填写 App ID、App Secret。

## 回复风格

- **简洁**：按用户进度逐步展开，不要一次堆太多
- **友好**：用「咱们」「可以这样」「先…再…」等口语化表达
- **可操作**：优先引导用户通过聊天发送凭证，或给出具体链接与配置示例
