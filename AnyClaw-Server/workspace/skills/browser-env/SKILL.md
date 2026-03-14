---
name: browser-env
description: "Headless browser environment guide for agent-browser / Playwright / Puppeteer inside Docker. Explains --no-sandbox requirement and system Chromium path."
metadata: {"nanobot":{"emoji":"🌐"}}
---

# Browser Environment (Docker)

AnyClaw 镜像（full 版）预装了 Alpine 系统 Chromium，路径：`/usr/bin/chromium-browser`。

## 关键：必须加 --no-sandbox

Docker 容器内无 SYS_ADMIN 权限，Chromium 必须加 `--no-sandbox`，否则启动失败：

```bash
chromium-browser --no-sandbox --headless --disable-gpu ...
```

## Playwright

环境变量已配置，Playwright 会自动使用系统 Chromium：

```
PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1
PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/usr/bin/chromium-browser
```

如果调用 Playwright API，启动参数需包含：

```js
const browser = await chromium.launch({
  executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH,
  args: ['--no-sandbox', '--disable-setuid-sandbox', '--disable-dev-shm-usage']
});
```

## Puppeteer

```
PUPPETEER_SKIP_DOWNLOAD=true
PUPPETEER_EXECUTABLE_PATH=/usr/bin/chromium-browser
```

启动时同样需要 `args: ['--no-sandbox', '--disable-setuid-sandbox']`。

## agent-browser 快速验证

```bash
chromium-browser --no-sandbox --headless --dump-dom https://example.com 2>/dev/null | head -20
```

能看到 HTML 则说明 Chromium 工作正常。
