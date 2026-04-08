package libs

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

func doRequest(cfg *Config, req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("New-API-User", "1")
	req.Header.Set("Accept", "application/json")
	return httpClient.Do(req)
}

// LogItem 对应 /api/log/ 接口返回的单条日志
type LogItem struct {
	ID               int    `json:"id"`
	CreatedAt        int64  `json:"created_at"`
	Type             int    `json:"type"`
	ChannelName      string `json:"channel_name"`
	ModelName        string `json:"model_name"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	Quota            int    `json:"quota"`
	UseTime          int    `json:"use_time"`
	IsStream         bool   `json:"is_stream"`
	Other            string `json:"other"`
}

// Frt 从 other JSON 中读取 frt（First Response Time，毫秒）
func (l LogItem) otherJSON() map[string]any {
	if l.Other == "" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(l.Other), &m); err != nil {
		return nil
	}
	return m
}

func (l LogItem) Frt() float64 {
	m := l.otherJSON()
	if m == nil {
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

func (l LogItem) StatusCode() int {
	m := l.otherJSON()
	if m == nil {
		return 0
	}
	switch v := m["status_code"].(type) {
	case float64:
		return int(v)
	case json.Number:
		i, _ := v.Int64()
		return int(i)
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

func fetchOnePage(cfg *Config, startTS, endTS int64, page int) (*logResponse, error) {
	const pageSize = 100
	u, err := url.Parse(cfg.BaseURL + "/api/log/")
	if err != nil {
		return nil, fmt.Errorf("解析 base_url 失败: %w", err)
	}
	q := u.Query()
	q.Set("p", strconv.Itoa(page))
	q.Set("page_size", strconv.Itoa(pageSize))
	q.Set("start_timestamp", strconv.FormatInt(startTS, 10))
	q.Set("end_timestamp", strconv.FormatInt(endTS, 10))
	q.Set("type", "0")
	u.RawQuery = q.Encode()

	req, _ := http.NewRequest(http.MethodGet, u.String(), nil)
	resp, err := doRequest(cfg, req)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	var result logResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w\nBody: %.200s", err, string(body))
	}
	if !result.Success {
		return nil, fmt.Errorf("接口返回失败: %s", result.Message)
	}
	return &result, nil
}

func FetchLogs(cfg *Config, startTS, endTS int64) ([]LogItem, error) {
	const pageSize = 100
	const maxConcurrency = 10

	first, err := fetchOnePage(cfg, startTS, endTS, 1)
	if err != nil {
		return nil, fmt.Errorf("请求第1页失败: %w", err)
	}
	allItems := append([]LogItem{}, first.Data.Items...)

	total := first.Data.Total
	totalPages := (total + pageSize - 1) / pageSize
	log.Printf("[INFO] 日志总条数=%d，共 %d 页", total, totalPages)

	if totalPages <= 1 {
		return allItems, nil
	}

	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, maxConcurrency)
	)

	for page := 2; page <= totalPages; page++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(p int) {
			defer wg.Done()
			defer func() { <-sem }()
			result, err := fetchOnePage(cfg, startTS, endTS, p)
			if err != nil {
				log.Printf("[WARN] 第%d页请求失败，跳过: %v", p, err)
				return
			}
			mu.Lock()
			allItems = append(allItems, result.Data.Items...)
			mu.Unlock()
		}(page)
	}

	wg.Wait()
	return allItems, nil
}
