package libs

import (
	"fmt"
	"log"
	"sort"
	"time"
)

func RunCheck(cfg *Config, windowEnd time.Time) {
	windowStart := windowEnd.Add(-time.Duration(cfg.WindowMinutes) * time.Minute)
	startTS := windowStart.Unix()
	endTS := windowEnd.Unix()
	startStr := windowStart.Format("15:04")
	endStr := windowEnd.Format("15:04")

	items, err := FetchLogs(cfg, startTS, endTS)
	if err != nil {
		msg := fmt.Sprintf("[ERROR] 🔴 NewAPI 请求失败 | 从 %s 到 %s | %v", startStr, endStr, err)
		log.Printf("%s", msg)
		SendFeishu(cfg.FeishuWebhook, msg)
		return
	}

	groups := GroupByChannel(items)

	channels := make([]string, 0, len(groups))
	for ch := range groups {
		channels = append(channels, ch)
	}
	sort.Strings(channels)

	for _, ch := range channels {
		chItems := groups[ch]
		stats := Analyze(chItems, cfg, ch)

		log.Printf("[INFO] %s~%s [%s] | API请求=%d 失败=%d(%.1f%%) | 流式=%d 慢TTFT=%d(%.1f%%) avg=%.2fs p95=%.2fs",
			startStr, endStr, ch,
			stats.Total, stats.Failed, stats.FailureRate,
			stats.StreamTotal, stats.SlowTTFT, stats.TTFTSlowRate,
			stats.TTFTAvgSecs, stats.TTFTP95Secs,
		)

		alerted := false

		if stats.StreamTotal >= cfg.MinRequests && stats.TTFTSlowRate >= cfg.TTFTAlertPercent {
			msg := fmt.Sprintf("[ALERT] 🚨 TTFT超阈值 [%s] | 从 %s 到 %s | 流式请求 %d 条, TTFT超长 %d 条(%.1f%%) | avg %.2fs, p95 %.2fs",
				ch, startStr, endStr,
				stats.StreamTotal, stats.SlowTTFT, stats.TTFTSlowRate,
				stats.TTFTAvgSecs, stats.TTFTP95Secs,
			)
			log.Printf("%s", msg)
			SendFeishu(cfg.FeishuWebhook, msg)
			alerted = true
		}

		if stats.Total > 0 && stats.Failed == stats.Total {
			msg := fmt.Sprintf("[ALERT] 🚨 全部请求失败 [%s] | 从 %s 到 %s | 请求 %d 条全部失败，可能是 API Key 失效或服务异常",
				ch, startStr, endStr, stats.Total,
			)
			log.Printf("%s", msg)
			SendFeishu(cfg.FeishuWebhook, msg)
			alerted = true
		} else if stats.Total >= cfg.MinRequests && stats.FailureRate >= cfg.FailureRatePercent {
			msg := fmt.Sprintf("[ALERT] 🚨 失败率超阈值 [%s] | 从 %s 到 %s | 请求 %d 条, 错误 %d 条(%.1f%%)",
				ch, startStr, endStr,
				stats.Total, stats.Failed, stats.FailureRate,
			)
			log.Printf("%s", msg)
			SendFeishu(cfg.FeishuWebhook, msg)
			alerted = true
		}

		if !alerted {
			log.Printf("[OK] ✅ [%s] 运行正常 | %s~%s | 请求=%d 错误=%d 错误率=%.1f%% | TTFT avg=%.2fs p95=%.2fs",
				ch, startStr, endStr,
				stats.Total, stats.Failed, stats.FailureRate,
				stats.TTFTAvgSecs, stats.TTFTP95Secs,
			)
		}
	}
}
