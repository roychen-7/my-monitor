package libs

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"
	"time"
)

type holidayCalendar struct {
	mu       sync.RWMutex
	holidays map[string]bool
	workdays map[string]bool
}

var holidayCal = &holidayCalendar{}

type holidayJSON struct {
	Holidays map[string]string `json:"holidays"`
	Workdays map[string]string `json:"workdays"`
}

func fetchHolidayYear(year int) (*holidayJSON, error) {
	url := fmt.Sprintf("https://cdn.jsdelivr.net/npm/chinese-days/dist/years/%d.json", year)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %.200s", resp.StatusCode, string(body))
	}
	var data holidayJSON
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

func RefreshHolidayCalendar() error {
	now := time.Now()
	years := []int{now.Year(), now.Year() + 1}

	holidays := make(map[string]bool)
	workdays := make(map[string]bool)

	for _, y := range years {
		data, err := fetchHolidayYear(y)
		if err != nil {
			log.Printf("[WARN] 拉取 %d 年节假日数据失败: %v", y, err)
			continue
		}
		for d := range data.Holidays {
			holidays[d] = true
		}
		for d := range data.Workdays {
			workdays[d] = true
		}
	}

	if len(holidays) == 0 {
		return fmt.Errorf("未获取到任何节假日数据")
	}

	holidayCal.mu.Lock()
	holidayCal.holidays = holidays
	holidayCal.workdays = workdays
	holidayCal.mu.Unlock()
	log.Printf("[INFO] 节假日数据已更新: %d 个假日, %d 个调休上班日", len(holidays), len(workdays))
	return nil
}

func StartHolidayRefresher() {
	go func() {
		for {
			now := time.Now()
			next3am := time.Date(now.Year(), now.Month(), now.Day()+1, 3, 0, 0, 0, now.Location())
			time.Sleep(time.Until(next3am))
			if err := RefreshHolidayCalendar(); err != nil {
				log.Printf("[WARN] 凌晨刷新节假日失败，沿用已有数据: %v", err)
			}
		}
	}()
}

// IsPeakTime 判断是否为工作日高峰期（工作日 10:00-22:00）
func IsPeakTime(t time.Time) bool {
	if !IsWorkday(t) {
		return false
	}
	h := t.Hour()
	return h >= 10 && h < 22
}

// NextCheckTime 计算下一次检查时间：
// 高峰期对齐到 5 分钟整点，其余时间对齐到整点小时
func NextCheckTime(now time.Time) time.Time {
	next5 := now.Truncate(5 * time.Minute).Add(5 * time.Minute)
	if IsPeakTime(next5) {
		return next5
	}
	return now.Truncate(time.Hour).Add(time.Hour)
}

func IsWorkday(t time.Time) bool {
	dateStr := t.Format("2006-01-02")

	holidayCal.mu.RLock()
	hasData := len(holidayCal.holidays) > 0
	isHoliday := holidayCal.holidays[dateStr]
	isWorkdayOverride := holidayCal.workdays[dateStr]
	holidayCal.mu.RUnlock()

	if hasData {
		if isHoliday {
			return false
		}
		if isWorkdayOverride {
			return true
		}
	}

	wd := t.Weekday()
	return wd != time.Saturday && wd != time.Sunday
}
