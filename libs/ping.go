package libs

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var pingAvgRe = regexp.MustCompile(`(?:round-trip|rtt)\s+\S+\s*=\s*[\d.]+/([\d.]+)/`)

type channelItem struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Status  int    `json:"status"`
	BaseURL string `json:"base_url"`
}

type channelResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Items []channelItem `json:"items"`
	} `json:"data"`
}

func fetchActiveChannels(cfg *Config) ([]channelItem, error) {
	req, _ := http.NewRequest(http.MethodGet,
		cfg.BaseURL+"/api/channel/?p=1&page_size=100&id_sort=false&tag_mode=false", nil)
	resp, err := doRequest(cfg, req)
	if err != nil {
		return nil, err
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var result channelResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析渠道列表失败: %w", err)
	}
	var active []channelItem
	for _, ch := range result.Data.Items {
		if ch.Status == 1 {
			active = append(active, ch)
		}
	}
	return active, nil
}

func pingHost(host string) (float64, error) {
	out, _ := exec.Command("ping", "-c", "10", "-i", "1", host).CombinedOutput()
	m := pingAvgRe.FindSubmatch(out)
	if m == nil {
		return 0, fmt.Errorf("无法解析 ping 输出: %s", strings.TrimSpace(string(out)))
	}
	return strconv.ParseFloat(string(m[1]), 64)
}

func RunPingCheck(cfg *Config) {
	channels, err := fetchActiveChannels(cfg)
	if err != nil {
		log.Printf("[WARN] 获取渠道列表失败，跳过 ping 检查: %v", err)
		return
	}

	// 按 hostname 去重，同一 hostname 合并渠道名
	hostNames := map[string][]string{}
	hostOrder := []string{}
	for _, ch := range channels {
		u, err := url.Parse(ch.BaseURL)
		if err != nil || u.Hostname() == "" {
			continue
		}
		h := u.Hostname()
		if _, exists := hostNames[h]; !exists {
			hostOrder = append(hostOrder, h)
		}
		hostNames[h] = append(hostNames[h], ch.Name)
	}

	for _, host := range hostOrder {
		names := strings.Join(hostNames[host], ", ")
		avgMs, err := pingHost(host)
		if err != nil {
			log.Printf("[WARN] ping %s (%s) 失败: %v", host, names, err)
			continue
		}
		if avgMs > cfg.PingAvgThresholdMs {
			msg := fmt.Sprintf("[ALERT] 🚨 Ping延迟超阈值 | %s (%s) | 平均延迟 %.1fms > %.0fms",
				host, names, avgMs, cfg.PingAvgThresholdMs)
			log.Printf("%s", msg)
			SendFeishu(cfg.FeishuWebhook, msg)
		} else {
			log.Printf("[INFO] ping %s (%s) avg=%.1fms", host, names, avgMs)
		}
	}
}
