package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ─── 配置 ─────────────────────────────────────────────────────────────────────

type Config struct {
	BaseURL            string  `yaml:"base_url"`
	APIKey             string  `yaml:"api_key"`
	FeishuWebhook      string  `yaml:"feishu_webhook"`
	WindowMinutes      int     `yaml:"window_minutes"`
	TTFTThresholdSecs  float64 `yaml:"ttft_threshold_secs"`
	TTFTAlertPercent   float64 `yaml:"ttft_alert_percent"`
	FailureRatePercent float64 `yaml:"failure_rate_percent"`
	MinRequests        int     `yaml:"min_requests"`
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}
	if cfg.WindowMinutes <= 0 {
		cfg.WindowMinutes = 5
	}
	if cfg.TTFTThresholdSecs <= 0 {
		cfg.TTFTThresholdSecs = 10
	}
	if cfg.TTFTAlertPercent <= 0 {
		cfg.TTFTAlertPercent = 30
	}
	if cfg.FailureRatePercent <= 0 {
		cfg.FailureRatePercent = 10
	}
	if cfg.MinRequests <= 0 {
		cfg.MinRequests = 5
	}
	return &cfg, nil
}

// ─── HTTP 客户端 ───────────────────────────────────────────────────────────────

var httpClient = &http.Client{Timeout: 30 * time.Second}

func doRequest(cfg *Config, req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("New-API-User", "1")
	req.Header.Set("Accept", "application/json")
	return httpClient.Do(req)
}

// ─── 日志结构 ─────────────────────────────────────────────────────────────────

// LogItem 对应 /api/log/ 接口返回的单条日志
// TTFT 存在 other 字段的 JSON 中，字段名 frt，单位毫秒
// 失败判断：quota == 0（未扣费代表请求未成功完成）
type LogItem struct {
	ID               int    `json:"id"`
	CreatedAt        int64  `json:"created_at"`
	Type             int    `json:"type"`
	ModelName        string `json:"model_name"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	Quota            int    `json:"quota"`
	UseTime          int    `json:"use_time"` // ms
	IsStream         bool   `json:"is_stream"`
	Other            string `json:"other"` // JSON string，包含 frt 字段
}

// frt 从 other JSON 中读取 frt（First Response Time，毫秒）
func (l LogItem) frt() float64 {
	if l.Other == "" {
		return 0
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(l.Other), &m); err != nil {
		return 0
	}
	switch v := m["frt"].(type) {
	case float64:
		return v
	case json.Number:
		f, _ := v.Float64()
		return f
	}
	return 0
}

type logResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Page     int       `json:"page"`
		PageSize int       `json:"page_size"`
		Total    int       `json:"total"`
		Items    []LogItem `json:"items"`
	} `json:"data"`
}

// ─── 飞书通知 ─────────────────────────────────────────────────────────────────

func sendFeishu(webhook, text string) {
	if webhook == "" {
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
	resp.Body.Close()
}

// ─── 拉取日志 ─────────────────────────────────────────────────────────────────

func fetchLogs(cfg *Config, startTS, endTS int64) ([]LogItem, error) {
	const pageSize = 500
	var allItems []LogItem

	for page := 1; ; page++ {
		u, err := url.Parse(cfg.BaseURL + "/api/log/")
		if err != nil {
			return nil, fmt.Errorf("解析 base_url 失败: %w", err)
		}
		q := u.Query()
		q.Set("p", strconv.Itoa(page))
		q.Set("page_size", strconv.Itoa(pageSize))
		q.Set("start_timestamp", strconv.FormatInt(startTS, 10))
		q.Set("end_timestamp", strconv.FormatInt(endTS, 10))
		q.Set("type", "0") // 0 = 全部类型
		u.RawQuery = q.Encode()

		req, _ := http.NewRequest(http.MethodGet, u.String(), nil)
		resp, err := doRequest(cfg, req)
		if err != nil {
			return nil, fmt.Errorf("请求日志失败: %w", err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		var result logResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("解析日志响应失败: %w\nBody: %.200s", err, string(body))
		}
		if !result.Success {
			return nil, fmt.Errorf("日志接口返回失败: %s", result.Message)
		}

		allItems = append(allItems, result.Data.Items...)

		if len(result.Data.Items) < pageSize {
			break
		}
		if len(allItems) >= 20000 {
			log.Printf("[WARN] 日志条数达到 20000 上限，停止翻页")
			break
		}
	}

	return allItems, nil
}

// ─── 统计分析 ─────────────────────────────────────────────────────────────────

type Stats struct {
	Total        int
	Failed       int
	StreamTotal  int
	SlowTTFT     int
	FailureRate  float64
	TTFTSlowRate float64
	TTFTAvgSecs  float64
	TTFTP95Secs  float64
}

func analyze(items []LogItem, cfg *Config) Stats {
	ttftThresholdMs := cfg.TTFTThresholdSecs * 1000
	var s Stats
	var ttftValues []float64

	for _, item := range items {
		// 只统计 API 调用（type=2）
		if item.Type != 2 {
			continue
		}
		s.Total++

		// 失败：quota == 0（请求未成功完成，未扣费）
		if item.Quota == 0 {
			s.Failed++
		}

		// TTFT：仅流式请求，从 other.frt 读取（毫秒）
		if item.IsStream {
			frt := item.frt()
			if frt > 0 {
				s.StreamTotal++
				ttftValues = append(ttftValues, frt)
				if frt > ttftThresholdMs {
					s.SlowTTFT++
				}
			}
		}
	}

	if s.Total > 0 {
		s.FailureRate = float64(s.Failed) / float64(s.Total) * 100
	}
	if s.StreamTotal > 0 {
		s.TTFTSlowRate = float64(s.SlowTTFT) / float64(s.StreamTotal) * 100

		// 平均值和 P95（毫秒转秒）
		var sum float64
		for _, v := range ttftValues {
			sum += v
		}
		s.TTFTAvgSecs = sum / float64(len(ttftValues)) / 1000

		sort.Float64s(ttftValues)
		idx := int(math.Ceil(0.95*float64(len(ttftValues)))) - 1
		if idx < 0 {
			idx = 0
		}
		s.TTFTP95Secs = ttftValues[idx] / 1000
	}
	return s
}

// ─── 主检查逻辑 ───────────────────────────────────────────────────────────────

func runCheck(cfg *Config, windowEnd time.Time) {
	windowStart := windowEnd.Add(-time.Duration(cfg.WindowMinutes) * time.Minute)
	startTS := windowStart.Unix()
	endTS := windowEnd.Unix()
	startStr := windowStart.Format("15:04")
	endStr := windowEnd.Format("15:04")

	items, err := fetchLogs(cfg, startTS, endTS)
	if err != nil {
		log.Printf("[ERROR] 拉取日志失败: %v", err)
		return
	}

	stats := analyze(items, cfg)
	log.Printf("[INFO] %s~%s | API请求=%d 失败=%d(%.1f%%) | 流式=%d 慢TTFT=%d(%.1f%%) avg=%.2fs p95=%.2fs",
		startStr, endStr,
		stats.Total, stats.Failed, stats.FailureRate,
		stats.StreamTotal, stats.SlowTTFT, stats.TTFTSlowRate,
		stats.TTFTAvgSecs, stats.TTFTP95Secs,
	)

	if stats.StreamTotal >= cfg.MinRequests && stats.TTFTSlowRate >= cfg.TTFTAlertPercent {
		msg := fmt.Sprintf("[ALERT] 🚨 TTFT超阈值 | 从 %s 到 %s | 流式请求 %d 条, TTFT超长 %d 条(%.1f%%) | avg %.2fs, p95 %.2fs",
			startStr, endStr,
			stats.StreamTotal, stats.SlowTTFT, stats.TTFTSlowRate,
			stats.TTFTAvgSecs, stats.TTFTP95Secs,
		)
		fmt.Println(msg)
		sendFeishu(cfg.FeishuWebhook, msg)
	}

	if stats.Total >= cfg.MinRequests && stats.FailureRate >= cfg.FailureRatePercent {
		msg := fmt.Sprintf("[ALERT] 🚨 失败率超阈值 | 从 %s 到 %s | 请求 %d 条, 错误 %d 条(%.1f%%)",
			startStr, endStr,
			stats.Total, stats.Failed, stats.FailureRate,
		)
		fmt.Println(msg)
		sendFeishu(cfg.FeishuWebhook, msg)
	}

	log.Printf("[OK] %s~%s | 请求=%d 错误=%d 错误率=%.1f%% | TTFT avg=%.2fs p95=%.2fs",
		startStr, endStr,
		stats.Total, stats.Failed, stats.FailureRate,
		stats.TTFTAvgSecs, stats.TTFTP95Secs,
	)
}

// ─── 调度 ─────────────────────────────────────────────────────────────────────

// isPeakTime 判断是否为工作日高峰期（周一至周五 10:00-22:00）
func isPeakTime(t time.Time) bool {
	wd := t.Weekday()
	if wd == time.Saturday || wd == time.Sunday {
		return false
	}
	h := t.Hour()
	return h >= 10 && h < 22
}

// nextCheckTime 计算下一次检查时间：
// 高峰期对齐到 5 分钟整点，其余时间对齐到整点小时
func nextCheckTime(now time.Time) time.Time {
	next5 := now.Truncate(5 * time.Minute).Add(5 * time.Minute)
	if isPeakTime(next5) {
		return next5
	}
	return now.Truncate(time.Hour).Add(time.Hour)
}

// ─── 入口 ─────────────────────────────────────────────────────────────────────

func main() {
	cfg, err := loadConfig("config.yaml")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 环境变量优先级高于配置文件
	if v := os.Getenv("BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("FEISHU_WEBHOOK"); v != "" {
		cfg.FeishuWebhook = v
	}
	if cfg.APIKey == "" {
		log.Fatalf("缺少 api_key，请在配置文件或环境变量 API_KEY 中设置")
	}

	log.Printf("监控启动 | 窗口=%dm TTFT阈值=%.0fs(%.0f%%) 失败率阈值=%.0f%% | 工作日10-22点每5分钟检查，其余每小时检查",
		cfg.WindowMinutes,
		cfg.TTFTThresholdSecs, cfg.TTFTAlertPercent, cfg.FailureRatePercent,
	)

	for {
		next := nextCheckTime(time.Now())
		log.Printf("下次检查: %s", next.Format("01-02 15:04"))
		time.Sleep(time.Until(next))
		runCheck(cfg, next)
	}
}
