# AnyClaw

与 AnyClaw-Web 对等的 Quicker 前端，含桌面小龙虾与完整宠舍。

## 功能

- **桌面小龙虾**：默认模式，在桌面右下角显示可拖拽的小龙虾图标，点击打开宠舍主界面
- **宠舍主界面**：金币、领养、宠舍列表，与 Web 端功能对等
- **聊天**：双击宠物卡片进入 WebSocket 实时聊天
- **弃养**：右键宠物卡片选择「弃养」
- **原生 Quicker 体验**：WPF 原生窗口，融入 Windows 桌面

## 使用

1. 首次运行会弹出登录窗口，输入邮箱和密码即可
2. 登录成功后 Token 会保存，下次无需再输
3. `mode` 变量：
   - `desktop`：显示桌面小龙虾，点击打开主界面
   - `main`：直接打开宠舍主界面

## 变量

| 变量 | 说明 |
|------|------|
| api_base | API 地址，默认 https://htwkumkjgrnz.sealosbja.site |
| email | 登录邮箱 |
| password | 登录密码 |
| mode | desktop=桌面小龙虾 / main=直接主界面 |
