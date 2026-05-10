package server

import (
	"fmt"
	"log"
	"strings"
	"time"

	"bridge-core/internal/model"
)

func (s *alertService) SendDailyTrafficReport(day time.Time) {
	if s == nil {
		return
	}
	day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	items, err := s.store.ListDailyTrafficUsage(day)
	if err != nil {
		log.Printf("daily traffic report: %v", err)
		return
	}
	bots, err := s.store.ListEnabledTelegramBotSecrets()
	if err != nil {
		log.Printf("daily traffic telegram bots: %v", err)
		return
	}
	if len(bots) == 0 {
		return
	}
	text := formatDailyTrafficReport(day, items)
	key := "daily_traffic:" + day.Format("2006-01-02")
	shouldSend, err := s.store.ShouldSendAlert(key, key, text, 48*time.Hour)
	if err != nil {
		log.Printf("daily traffic alert state: %v", err)
		return
	}
	if !shouldSend {
		return
	}
	for _, bot := range bots {
		if err := s.sendTelegramMessage(bot.BotToken, bot.ChatID, text); err != nil {
			log.Printf("send daily traffic report to %s: %v", bot.Name, err)
		}
	}
}

func formatDailyTrafficReport(day time.Time, items []model.DailyTrafficUsage) string {
	var upload, download uint64
	for _, item := range items {
		upload += item.Upload
		download += item.Download
	}
	lines := []string{
		fmt.Sprintf("📊 NanFengMonitor 昨日流量日报（%s）", day.Format("2006-01-02")),
		fmt.Sprintf("Client 数：%d", len(items)),
		fmt.Sprintf("总流量：%s", formatBytes(upload+download)),
		fmt.Sprintf("总上传：%s", formatBytes(upload)),
		fmt.Sprintf("总下载：%s", formatBytes(download)),
		"前三名：",
	}
	if len(items) == 0 {
		lines = append(lines, "暂无昨日快照数据")
	} else {
		limit := 3
		if len(items) < limit {
			limit = len(items)
		}
		for index := 0; index < limit; index++ {
			item := items[index]
			name := item.AgentName
			if name == "" {
				name = item.AgentID
			}
			lines = append(lines, fmt.Sprintf("%d. %s：%s（上传 %s / 下载 %s）", index+1, name, formatBytes(item.Total), formatBytes(item.Upload), formatBytes(item.Download)))
		}
	}
	lines = append(lines, "发送时间："+time.Now().Format("2006-01-02 15:04:05"))
	return strings.Join(lines, "\n")
}
