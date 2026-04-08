package libs

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	BaseURL               string  `yaml:"base_url"`
	APIKey                string  `yaml:"api_key"`
	FeishuWebhook         string  `yaml:"feishu_webhook"`
	WindowMinutes         int     `yaml:"window_minutes"`
	TTFTThresholdSecs     float64 `yaml:"ttft_threshold_secs"`
	TTFTAvgThresholdSecs  float64 `yaml:"ttft_avg_threshold_secs"`
	TTFTAlertPercent      float64 `yaml:"ttft_alert_percent"`
	FailureRatePercent    float64 `yaml:"failure_rate_percent"`
	MinRequests                int     `yaml:"min_requests"`
	PingAvgThresholdMs         float64 `yaml:"ping_avg_threshold_ms"`
	SpecificErrorRatePercent   float64 `yaml:"specific_error_rate_percent"`
}

func LoadConfig(path string) (*Config, error) {
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
		cfg.TTFTThresholdSecs = 40
	}
	if cfg.TTFTAvgThresholdSecs <= 0 {
		cfg.TTFTAvgThresholdSecs = 30
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
	if cfg.PingAvgThresholdMs <= 0 {
		cfg.PingAvgThresholdMs = 300
	}
	if cfg.SpecificErrorRatePercent <= 0 {
		cfg.SpecificErrorRatePercent = 1
	}
	return &cfg, nil
}
