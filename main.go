package main

import (
	"log"
	"os"
	"time"

	"my-monitor/libs"
)

func main() {
	cfg, err := libs.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

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

	if err := libs.RefreshHolidayCalendar(); err != nil {
		log.Printf("[WARN] 启动时获取节假日数据失败，将使用周末/工作日判断: %v", err)
	}
	libs.StartHolidayRefresher()

	log.Printf("监控启动 | 窗口=%dm TTFT阈值=%.0fs(%.0f%%) 失败率阈值=%.0f%% | 工作日10-22点每5分钟检查，其余每小时检查",
		cfg.WindowMinutes,
		cfg.TTFTThresholdSecs, cfg.TTFTAlertPercent, cfg.FailureRatePercent,
	)

	for {
		next := libs.NextCheckTime(time.Now())
		log.Printf("下次检查: %s", next.Format("01-02 15:04"))
		time.Sleep(time.Until(next))
		libs.RunCheck(cfg, next)
	}
}
