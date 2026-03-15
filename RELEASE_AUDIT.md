# 发布前代码审计报告（二次审计）

**审计范围**：COS 上传、媒体类型区分、Web 配置与渲染相关改动  
**审计日期**：2025-03-15

---

## 一、已修复问题

### 1. API 媒体上传 JSON 响应（已修复）
- **问题**：`escapeJSONString` 仅转义 `\` 和 `"`，`filename` 含换行、制表符等会破坏 JSON
- **修复**：改用 `json.NewEncoder(w).Encode()` 输出，保证正确转义

### 2. Web Chat XSS 风险（已修复）
- **问题**：`[📎 x](javascript:alert(1))` 会渲染为可执行链接
- **修复**：增加 `isSafeHref`，仅允许 `https://`、`http://`、`data:`，否则渲染为 `<span>`
- **范围**：`<a>` 与 `<img>` 均做校验

### 3. API 错误响应 JSON 破坏（已修复）
- **问题**：`http.Error(w, "..."+err.Error()+")` 中 err 含 `"` 会破坏 JSON
- **修复**：改用 `json.NewEncoder(w).Encode(map[string]string{"error": ...})` 输出错误

### 4. COS path_prefix 路径穿越（已修复）
- **问题**：`path_prefix` 含 `..` 可能导致路径穿越
- **修复**：检测到 `..` 时回退为 `media`

### 5. COS domain 校验（已修复）
- **问题**：`domain` 未校验，可能配置 `javascript:` 等无效 URL
- **修复**：仅当 domain 以 `https://` 或 `http://` 开头时使用

### 6. Bridge 文件名破坏 markdown（已修复）
- **问题**：`filename` 含 `]`、`[`、`(`、`)`、`\` 会破坏 markdown 链接语法
- **修复**：`sanitizeFilenameForMarkdown` 将上述字符替换为 `-`

### 7. 前端 data: URL XSS（已修复）
- **问题**：`data:text/html,<script>...</script>` 或 `data:image/svg+xml` 可执行脚本
- **修复**：`isSafeHref` 限制 data: 仅允许 `image/*`（排除 svg）、`audio/*`、`video/*`、`application/octet-stream`

### 8. adminconfig 保存错误 JSON（已修复）
- **问题**：SaveAdminConfig 失败时 `http.Error` 含 err.Error() 可能破坏 JSON
- **修复**：改用 `json.NewEncoder(w).Encode()` 输出错误

### 9. adminconfig cos 未发送时被清空（已修复）
- **问题**：前端未发送 cos 时，后端会保存 cos=nil，覆盖已有配置
- **修复**：`req.COS == nil` 时保留 `cfg.COS`

### 10. API instance_id 解析未校验（已修复）
- **问题**：`strconv.ParseInt` 错误被忽略，非法 id 可能导致误判
- **修复**：校验 `ParseInt` 返回值，失败时返回 403

### 11. API Content-Type 注入（已修复）
- **问题**：Content-Type 含 `\r\n` 可能注入额外 HTTP 头
- **修复**：检测到 `\r\n` 时使用 `application/octet-stream`

### 12. API filename 路径穿越（已修复）
- **问题**：filename 含 `\x00`、`/`、`\` 可能影响存储或响应
- **修复**：使用 `filepath.Base` 提取纯文件名，并校验空串及 `.`、`..`

### 13. 前端 isSafeHref 大小写绕过（已修复）
- **问题**：`HTTPS://evil.com`、`DATA:text/html` 等大写协议可能绕过校验
- **修复**：对 URL 先 `toLowerCase()` 再判断协议

---

## 二、安全与兼容性检查

| 项目 | 状态 | 说明 |
|------|------|------|
| 上传鉴权 | ✅ | 使用 instance token，校验 `instance_id` 与 token 一致 |
| COS 密钥 | ✅ | 存 DB，GetConfig 脱敏，PutConfig 脱敏时保留原值 |
| 上传大小 | ✅ | 50MB 限制 |
| 向后兼容 | ✅ | 旧消息 `[📎 x](url)` 无 emoji 时按扩展名推断 |
| COS 未配置 | ✅ | Bridge 回退 base64，API 返回 503 |
| path_prefix | ✅ | 含 `..` 时回退为 `media` |
| domain | ✅ | 仅 https/http 生效 |
| filename | ✅ | 破坏 markdown 的字符已替换 |

---

## 三、部署注意事项

### 1. 配置迁移
- **COS 未配置**：现有行为不变，继续使用 base64 内嵌
- **首次启用 COS**：在管理后台填写 SecretID、SecretKey、Bucket、Region 等
- **path_prefix**：默认 `media/`，可按需修改

### 2. 依赖
- AnyClaw-API：`github.com/tencentyun/cos-go-sdk-v5`
- 需确认 `go mod tidy` 已执行

### 3. 回滚
- 关闭 COS：管理后台取消「启用 COS」
- Bridge 会自动回退到 base64
- 旧消息（含 base64）仍可正常显示

---

## 四、建议后续优化（非阻塞）

1. **上传限流**：对 `/instances/{id}/media` 做 QPS 或总量限制，防止滥用
2. **COS 跨域**：若使用自定义域名，需在 COS 控制台配置 CORS
3. **监控**：对 COS 上传失败率、延迟做监控

---

## 五、改动文件清单

- `AnyClaw-API/internal/media/handlers.go` - JSON 输出、错误响应
- `AnyClaw-API/internal/media/cos.go` - path_prefix 校验、domain 校验
- `AnyClaw-API/internal/config/config.go` - COSConfig
- `AnyClaw-API/internal/adminconfig/handlers.go` - COS 配置读写、错误响应
- `AnyClaw-API/cmd/api/main.go` - 注册 `/instances/{id}/media`
- `AnyClaw-Server/pkg/channels/anyclaw_bridge/bridge.go` - 上传、类型推断、emoji 格式、filename 脱敏
- `AnyClaw-Web/src/api.ts` - COSConfig 类型
- `AnyClaw-Web/src/pages/AdminConfig.tsx` - COS 配置表单
- `AnyClaw-Web/src/pages/Chat.tsx` - 媒体渲染、URL 校验、data: MIME 限制
