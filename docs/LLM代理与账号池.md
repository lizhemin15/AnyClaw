# LLM 代理与账号池

## 架构

- **原始 Key**：由管理员在「AI 配置」中配置，保存到数据库
- **分发的 Token**：创建实例时分配，仅用于容器向 API 鉴权
- **数据流**：容器 → API（Bearer token）→ 用配置的渠道 Key 调用 LLM → 返回

## AnyClaw-API 配置

### 管理后台

AI 渠道（OpenAI、Anthropic、OpenRouter 等）在管理后台「配置」→「AI 配置」中配置，支持多渠道、多模型，保存到数据库。

### 环境变量（可选）

| 变量 | 说明 |
|------|------|
| `ANYCLAW_API_PORT` | 服务端口，默认 8080 |
| `ANYCLAW_DB_DSN` | MySQL 连接串 |
| `ANYCLAW_JWT_SECRET` | JWT 密钥 |
| `ANYCLAW_API_URL` | 公网 API 地址，供容器连接 |

## 用量监管

每次 LLM 调用成功后，API 会：
1. 打印 usage 日志（instance_id、user_id、model、token 用量）
2. 写入 `usage_log` 表，按 token 扣用户金币
3. 用户可在「消耗记录」查看明细
