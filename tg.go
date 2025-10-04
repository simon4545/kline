package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

func TelegramSendMessage(message string) error {
	if botToken == "" || chatID == "" {
		return nil
	}

	url := "https://api.telegram.org/bot" + botToken + "/sendMessage"

	data := map[string]string{
		"chat_id": chatID,
		"text":    message,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// 发送POST请求
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("发送Telegram消息失败，状态码: %d", resp.StatusCode)
	}

	return nil
}
