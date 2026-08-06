package server

import "time"

const notificationTimeLayout = "2006-01-02 15:04:05"

// Beijing time is always UTC+8 and has no daylight-saving transitions.
var beijingTimeLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

func formatBeijingTime(value time.Time) string {
	return value.In(beijingTimeLocation).Format(notificationTimeLayout)
}
