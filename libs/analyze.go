package libs

import (
	"math"
	"sort"
)

type Stats struct {
	ChannelName  string
	Total        int
	Failed       int
	StreamTotal  int
	SlowTTFT     int
	FailureRate  float64
	TTFTSlowRate float64
	TTFTAvgSecs  float64
	TTFTP95Secs  float64
}

// GroupByChannel 按 ChannelName 分组日志
func GroupByChannel(items []LogItem) map[string][]LogItem {
	groups := make(map[string][]LogItem)
	for _, item := range items {
		ch := item.ChannelName
		if ch == "" {
			ch = "unknown"
		}
		groups[ch] = append(groups[ch], item)
	}
	return groups
}

func Analyze(items []LogItem, cfg *Config, channelName string) Stats {
	ttftThresholdMs := cfg.TTFTThresholdSecs * 1000
	s := Stats{ChannelName: channelName}
	var ttftValues []float64

	for _, item := range items {
		if item.Type != 2 {
			continue
		}
		if item.PromptTokens == 0 {
			continue
		}
		s.Total++

		if item.Quota == 0 {
			s.Failed++
		}

		if item.IsStream {
			frt := item.Frt()
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
