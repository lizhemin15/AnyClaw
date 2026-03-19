---
name: voice
description: 语音回复指南。当用户发送语音消息或请求语音回复时，使用 speak 工具合成语音并发送。
metadata: {"nanobot":{"emoji":"🔊"}}
---

# 语音回复指南

## 语音识别（ASR）说明

**重要：** 当消息中出现 `[voice: 某段文字]` 时，表示用户发送了语音消息，系统已自动转成文字。这段文字就是用户说的内容。

- **直接按文字内容回复**，当作用户用文字说了同样的话
- **不要说**「我无法识别语音」「需要安装语音识别技能」——识别已完成，无需安装任何技能
- 若看到 `[audio]` 且无后续文字，才表示转录失败；此时可简短说明「语音暂时无法识别」

## speak 工具说明

`speak` 工具将文本合成为语音（MP3/WAV，依 TTS 提供商）并直接发送给用户，支持飞书和网页端。

**小米 MiMo 专用参数 `style`（可选）**：与 `text` 并列传入时，服务端会前置 `<style>…</style>`（若 `text` 本身已以 `<style>` 开头则不再重复包裹）。便于模型只填情感/场景词，例如 `style="Whisper"`、`style="唱歌"`。

### 配置方式

**调度器（AnyClaw-API）分轨配置 ASR + TTS：**
- `voice_api`：ASR（语音识别），如 ChatAnywhere、Groq
- `tts_api`：TTS（语音合成），如 ChatAnywhere、Xiaomi MiMo；空则回退到 voice_api 中非 Groq 的第一个

示例：ASR 用 ChatAnywhere、TTS 用 Xiaomi：
```json
"voice_api": [{"id":"asr","name":"ChatAnywhere","endpoint":"https://api.chatanywhere.org/v1","api_key":"sk-xxx","enabled":true}],
"tts_api": [{"id":"tts","name":"Xiaomi MiMo","endpoint":"https://api.xiaomimimo.com/v1","api_key":"your-key","enabled":true}]
```

**方式一：环境变量**
- `XIAOMI_MIMO_API_KEY` 和 `XIAOMI_MIMO_API_BASE`（可选）：使用小米 MiMo TTS
- `ANYCLAW_VOICE_API_KEY` / `ANYCLAW_TTS_API_KEY`：调度器注入，分别用于 ASR 和 TTS

**方式二：config.json（本地运行）**
```json
"providers": {
  "xiaomi_mimo": {
    "api_key": "your-key",
    "api_base": "https://api.xiaomimimo.com/v1",
    "tts_model": "mimo-v2-tts"
  }
}
```

**方式三：model_list**
```json
{"model": "xiaomi_mimo/mimo-v2-tts", "api_key": "your-key", "api_base": "https://api.xiaomimimo.com/v1"}
```

### 可用音色（voice 参数）

**OpenAI TTS（alloy/echo/fable/onyx/nova/shimmer）：**
| 音色 | 风格 |
|------|------|
| alloy | 中性、平衡（默认） |
| echo | 男声、沉稳 |
| fable | 英式、叙事 |
| onyx | 男声、深沉 |
| nova | 女声、活泼 |
| shimmer | 女声、温柔 |

**小米 MiMo TTS（api.xiaomimimo.com）：**
| 音色 | 说明 |
|------|------|
| mimo_default | 默认音色 |
| default_zh | 中文女声 |
| default_en | 英文女声 |

### 小米 MiMo TTS 接口说明（内联自平台文档）

- **Endpoint**：`POST {api_base}/chat/completions`（如 `https://api.xiaomimimo.com/v1/chat/completions`）
- **Headers**：`Content-Type: application/json`，`api-key: {api_key}`（**不是** Authorization: Bearer）
- **Request body**：
  | 参数 | 类型 | 说明 |
  |------|------|------|
  | model | string | 模型，`mimo-v2-tts` |
  | messages | array | 含 user 与 assistant，assistant.content 为待合成文本 |
  | audio | object | `{ "format": "wav", "voice": "mimo_default" }` |
- **Response**：JSON，音频在 `choices[0].message.audio.data`（base64）
- **API Key**：在 platform.xiaomimimo.com 控制台创建，使用 `api-key` 请求头，**不是** OAuth 或小米账号授权

**curl 示例**（与平台文档一致）：
```bash
curl -X POST "https://api.xiaomimimo.com/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "api-key: $MIMO_API_KEY" \
  -d '{"model":"mimo-v2-tts","messages":[{"role":"user","content":"Hello"},{"role":"assistant","content":"你好"}],"audio":{"format":"wav","voice":"mimo_default"}}'
```

### 小米 MiMo 风格与语气控制（内联自平台文档）

**整体风格**：在文本开头加 `<style>风格名</style>`，可组合多个风格：`<style>风格1 风格2</style>内容`。下表为推荐风格，未列出的风格也可尝试。

| 风格 | 说明 | 示例 |
|------|------|------|
| Happy | 开心、愉悦 | `<style>Happy</style>明天周五啦，好开心！` |
| Whisper | 低语、悄悄话 | `<style>Whisper</style>天哪，今天好冷啊！那风呼呼的，像刀子割脸一样！` |
| 唱歌 | 歌声合成 | `<style>唱歌</style>歌词内容`（**唱歌必须**在文本最开头加此标签） |

**细粒度音效标签**：用 `[标签]` 或 `(标签)` 控制咳嗽、喘息、叹气、抽泣、笑声等。

| 标签 | 效果 | 示例 |
|------|------|------|
| [cough] | 咳嗽 | `阿嚏！咳。我—我真是 [cough] 觉得要感冒了 [cough] 重感冒。` |
| [heavy breathing] | 喘息 | `[heavy breathing] 等……等一下。我……从车站……一路跑过来的。` |
| long sigh | 长叹 | `我就是觉得…… long sigh ……像一直在踩水，你知道吗？` |
| (sobbing) | 抽泣 | `太蠢了！(sobbing) 我们花那么多钱买蛋糕，狗却……(sudden laugh) 一口全吃光了！` |
| (sudden laugh) | 突然笑 | 见上例 |

## 何时调用 speak

### 必须调用
- 用户明确要求语音回复：「用语音回复我」「发语音」「voice reply」「speak to me」
- 用户发来了语音消息，并要求你用语音回复

### 可以调用
- 用户发来语音消息，根据对话氛围判断语音回复更自然时
- 用户询问「你能说话吗」「能发语音吗」（先确认后调用）

### 不需要调用
- 普通文字聊天，用户没有提到语音
- 用户发语音但只是想让你识别内容：消息中会显示 `[voice: 转录文字]`，直接按该文字正常回复即可，无需安装技能

## 使用示例

**用户说：** 「用语音告诉我今天天气怎么样」

**正确做法：** 先查天气，再调用 speak：
```
speak(text="今天北京天气晴，气温 18 到 26 度，适合出行。", voice="nova")
```

**用户说：** 「换个好听的声音说一遍」

**正确做法：**
```
speak(text="你好，我是你的 AI 助手。", voice="shimmer")
```

**小米 MiMo TTS 调用示例**（接口格式见上方「小米 MiMo TTS 接口说明」）：
```
speak(text="你好，欢迎使用小米语音。", voice="mimo_default")
speak(text="这是一段中文女声朗读。", voice="default_zh")
speak(text="This is English female voice.", voice="default_en")
```

**小米 MiMo 风格示例**（唱歌、语气、情感）：
```
speak(text="明天周五啦，好开心！", style="Happy", voice="mimo_default")
speak(text="<style>Happy</style>明天周五啦，好开心！", voice="mimo_default")
speak(text="<style>Whisper</style>天哪，今天好冷啊！那风呼呼的，像刀子割脸一样！", voice="mimo_default")
speak(text="<style>唱歌</style>一闪一闪亮晶晶，满天都是小星星。", voice="mimo_default")
speak(text="阿嚏！咳。我真是 [cough] 觉得要感冒了 [cough] 重感冒。", voice="mimo_default")
speak(text="[heavy breathing] 等……等一下。我从车站一路跑过来的。", voice="mimo_default")
speak(text="我就是觉得…… long sigh ……像一直在踩水，你知道吗？", voice="mimo_default")
speak(text="太蠢了！(sobbing) 我们花那么多钱买蛋糕，狗却……(sudden laugh) 一口全吃光了！", voice="mimo_default")
```

## 与定时任务（cron）结合

当用户要求定时发语音提醒时，**必须将 `deliver` 设为 `false`**，让 agent 在触发时处理消息并调用 speak，而不是直接推送文字。

**用户说：** 「每天早上9点用语音提醒我喝水」

**正确做法：**
```
cron(
  action="add",
  message="用语音提醒用户喝水，说：记得喝水哦，健康第一！",
  cron_expr="0 9 * * *",
  deliver=false
)
```
触发时 agent 处理该 message，自动调用 speak 发出语音。

**错误做法（deliver=true）：** 会直接发文字，不会触发语音合成。

## speak 失败时的正确说明

**重要：** 当 speak 工具返回错误时，**必须**按以下说明回复，**禁止编造或联想**：

- **小米 MiMo TTS** 使用 **API Key**，**不是** OAuth、Token 或「小米账号授权」。**严禁**使用「Token 过期」「待授权」「token过期待授权」「点击链接重新登录」「小米账号授权」等表述——这些均不适用于 MiMo API。
- **正确回复模板**：「语音合成暂时不可用。请管理员在管理后台 → 配置 → 语音合成 (TTS) 中检查 Xiaomi MiMo 的 API Key 和 Endpoint 是否有效。Endpoint 需为 https://api.xiaomimimo.com/v1。」

## 注意事项

- `text` 参数只传要说的内容，不要包含 markdown、表情符号等非朗读文本（**小米 MiMo 除外**：可含 `<style>风格</style>` 和 `[cough]`、`(sobbing)` 等音效标签）
- 长文本建议拆分为自然段落，分多次调用
- **调用 speak 后不要再额外输出文字**——语音本身就是回复，无需重复说一遍或解释"已为你合成语音"之类的话
- 用户要求语音回复时，直接调用 speak，不要先说"好的，我来……"等铺垫语
