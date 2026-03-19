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

`speak` 工具将文本合成为语音（MP3）并直接发送给用户，支持飞书和网页端。

### 配置方式

**调度器（AnyClaw-API）分轨配置 ASR + TTS：**
- `voice_api`：ASR（语音识别），如 ChatAnywhere、Groq
- `tts_api`：TTS（语音合成），如 ChatAnywhere、Xiaomi MiMo；空则回退到 voice_api 中非 Groq 的第一个

示例：ASR 用 ChatAnywhere、TTS 用 Xiaomi：
```json
"voice_api": [{"id":"asr","name":"ChatAnywhere","endpoint":"https://api.chatanywhere.org/v1","api_key":"sk-xxx","enabled":true}],
"tts_api": [{"id":"tts","name":"Xiaomi MiMo","endpoint":"https://platform.xiaomimimo.com/api/v1","api_key":"your-key","enabled":true}]
```

**方式一：环境变量**
- `XIAOMI_MIMO_API_KEY` 和 `XIAOMI_MIMO_API_BASE`（可选）：使用小米 MiMo TTS
- `ANYCLAW_VOICE_API_KEY` / `ANYCLAW_TTS_API_KEY`：调度器注入，分别用于 ASR 和 TTS

**方式二：config.json（本地运行）**
```json
"providers": {
  "xiaomi_mimo": {
    "api_key": "your-key",
    "api_base": "https://platform.xiaomimimo.com/api/v1",
    "tts_model": "mimo-v2-tts"
  }
}
```

**方式三：model_list**
```json
{"model": "xiaomi_mimo/mimo-v2-tts", "api_key": "your-key", "api_base": "https://platform.xiaomimimo.com/api/v1"}
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

**小米 MiMo TTS（platform.xiaomimimo.com）：**
| 音色 | 说明 |
|------|------|
| default | 默认音色 |
| female | 女声 |
| male | 男声 |
| 其他 | 以平台文档为准：https://platform.xiaomimimo.com/#/docs/usage-guide/speech-synthesis |

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

## 注意事项

- `text` 参数只传要说的内容，不要包含 markdown、表情符号等非朗读文本
- 长文本建议拆分为自然段落，分多次调用
- **调用 speak 后不要再额外输出文字**——语音本身就是回复，无需重复说一遍或解释"已为你合成语音"之类的话
- 用户要求语音回复时，直接调用 speak，不要先说"好的，我来……"等铺垫语
