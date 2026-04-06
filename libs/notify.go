package libs

import (
	"encoding/json"
	"io"
	"log"
	"strings"
)

func SendFeishu(webhook, text string) {
	if webhook == "" {
		log.Println("[WARN] 飞书 webhook 未配置，跳过发送")
		return
	}
	body, _ := json.Marshal(map[string]any{
		"msg_type": "text",
		"content":  map[string]string{"text": text},
	})
	resp, err := httpClient.Post(webhook, "application/json", strings.NewReader(string(body)))
	if err != nil {
		log.Printf("[WARN] 飞书发送失败: %v", err)
		return
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	log.Printf("[INFO] 飞书响应: status=%d body=%s", resp.StatusCode, string(respBody))
}
