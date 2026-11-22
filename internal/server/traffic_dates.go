package server

import (
	"fmt"
	"net/http"
	"time"

	"bridge-core/internal/model"
)

func parseTrafficDates(r *http.Request) (string, string, error) {
	today := time.Now().In(model.BeijingLocation)
	fromText, toText := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if fromText == "" {
		fromText = today.AddDate(0, 0, -6).Format("2006-01-02")
	}
	if toText == "" {
		toText = today.Format("2006-01-02")
	}
	from, fromErr := time.ParseInLocation("2006-01-02", fromText, model.BeijingLocation)
	to, toErr := time.ParseInLocation("2006-01-02", toText, model.BeijingLocation)
	if fromErr != nil || toErr != nil || to.Before(from) || to.Sub(from) > 365*24*time.Hour {
		return "", "", fmt.Errorf("请选择有效的起止日期，单次最多查询 366 天")
	}
	return fromText, toText, nil
}
