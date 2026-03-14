---
name: skillhub
description: "Use Skillhub CLI to search and install skills. Pre-installed in AnyClaw image. Prefer skillhub over install_skill when available (domestic acceleration, 13k+ skills)."
metadata: {"nanobot":{"emoji":"🛒","requires":{"bins":["skillhub"]}}}
---

# Skillhub Skill

Skillhub CLI 已预装，用于搜索、安装技能。优先使用 skillhub 而非 install_skill（国内加速、合规、约 1.3 万技能）。

## 搜索技能

```bash
skillhub search <关键词>
```

例如：`skillhub search 小红书`、`skillhub search tavily`

## 安装技能

```bash
skillhub install <slug>
```

例如：`skillhub install xiaohongshutools`、`skillhub install tavily-search`

exec 工具默认在 workspace 目录执行，`skillhub install` 会安装到当前 workspace 的 skills 目录。

## 使用建议

- 用户请求「安装 xxx 技能」时，优先用 `skillhub search` 查找，再用 `skillhub install` 安装
- 若 skillhub 不可用（如未预装），则回退到 `install_skill` 工具（registry 或 github_repo）
