# LLM 代理与账号池

## 架构

- **原始 Key**：仅存在于 AnyClaw-API，由管理员配置
- **分发的 Token**：创建容器时分配，仅用于向 API 鉴权
- **数据流**：容器 → API（Bearer token）→ 用原始 Key 调用 LLM → 返回

## AnyClaw-API 配置

### 环境变量

| 变量 | 说明 |
|------|------|
| `ANYCLAW_API_PORT` | 服务端口，默认 8080 |
| `ANYCLAW_KEY_OPENAI_API_KEY` | OpenAI 原始 Key |
| `ANYCLAW_KEY_OPENAI_API_BASE` | OpenAI API 地址 |
| `ANYCLAW_KEY_ANTHROPIC_API_KEY` | Anthropic 原始 Key |
| `ANYCLAW_KEY_ANTHROPIC_API_BASE` | Anthropic API 地址 |
| `ANYCLAW_KEY_OPENROUTER_API_KEY` | OpenRouter Key（兜底） |
| `ANYCLAW_INSTANCE_TOKENS` | JSON：`{"token":{"instance_id":"x","user_id":"y"}}` |

### 配置文件

见 `AnyClaw-API/config.example.json`。`key_pool` 为原始 Key，`instance_map.tokens` 为分发的 token 与实例映射。

## 启动

```bash
cd AnyClaw-API
go run ./cmd/api -config config.json
```

## 用量监管

每次 LLM 调用成功后，API 会打印 usage 日志，包含 `instance_id`、`user_id`、`model`、token 用量。后续可接入数据库持久化。
