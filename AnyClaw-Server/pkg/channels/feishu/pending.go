package feishu

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const successMsg = "飞书绑定成功！重启已完成，你现在可以在飞书中搜索应用并开始对话了。"

const feishuWelcomeMsg = "飞书绑定成功！欢迎使用～"

// pendingSuccess holds the source channel/chat to notify after restart.
type pendingSuccess struct {
	SourceChannel string `json:"source_channel"`
	SourceChatID  string `json:"source_chat_id"`
}

func anyclawDir() string {
	if home := os.Getenv("ANYCLAW_HOME"); home != "" {
		return home
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".anyclaw")
}

func pendingPath() string {
	return filepath.Join(anyclawDir(), "pending_feishu_binding_success.json")
}

func feishuWelcomePath() string {
	return filepath.Join(anyclawDir(), "pending_feishu_welcome")
}

// WriteBindingPending saves the source channel/chat_id and creates the Feishu welcome flag.
func WriteBindingPending(sourceChannel, sourceChatID string) error {
	p := pendingSuccess{
		SourceChannel: sourceChannel,
		SourceChatID:  sourceChatID,
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	path := pendingPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return os.WriteFile(feishuWelcomePath(), []byte("1"), 0o600)
}

// ReadAndClearBindingPending reads the pending notification and removes the file.
func ReadAndClearBindingPending() (channel, chatID string, ok bool) {
	path := pendingPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", false
	}
	var p pendingSuccess
	if err := json.Unmarshal(data, &p); err != nil {
		os.Remove(path)
		return "", "", false
	}
	os.Remove(path)
	if p.SourceChannel == "" || p.SourceChatID == "" {
		return "", "", false
	}
	return p.SourceChannel, p.SourceChatID, true
}

// BindingSuccessMessage returns the message to send on binding success.
func BindingSuccessMessage() string {
	return successMsg
}

// FeishuWelcomeMessage returns the message for first Feishu message after binding.
func FeishuWelcomeMessage() string {
	return feishuWelcomeMsg
}

// ConsumeFeishuWelcome checks if the Feishu welcome flag exists and removes it.
func ConsumeFeishuWelcome() bool {
	path := feishuWelcomePath()
	if _, err := os.Stat(path); err != nil {
		return false
	}
	os.Remove(path)
	return true
}
