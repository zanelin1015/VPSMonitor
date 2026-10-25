package server

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"bridge-core/internal/model"
)

func (a *App) handleCustomerUsage(w http.ResponseWriter, r *http.Request) {
	user, _, ok := a.requireCustomer(w, r)
	if !ok {
		return
	}
	from, to, err := customerUsageRange(r, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	items, err := a.store.ListCustomerTrafficUsage(user.ID, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	overview, err := a.customerOverview(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	links := make(map[int64]model.CustomerLinkView, len(overview.Links))
	multipliers := make(map[int64]float64, len(overview.Links))
	var rawUpload, rawDownload, countedUpload, countedDownload, counted, quota uint64
	unlimited := false
	for _, link := range overview.Links {
		links[link.AssignmentID] = link
		multiplier := normalizeClientTrafficMultiplier(link.TrafficMultiplier)
		multipliers[link.AssignmentID] = multiplier
		rawUploadBytes := nonNegativeTrafficUint(link.TrafficUploadBytes)
		rawDownloadBytes := nonNegativeTrafficUint(link.TrafficDownloadBytes)
		rawUpload += rawUploadBytes
		rawDownload += rawDownloadBytes
		countedUpload += scaleTrafficUint(rawUploadBytes, multiplier)
		countedDownload += scaleTrafficUint(rawDownloadBytes, multiplier)
		counted += nonNegativeTrafficUint(link.TrafficUsedBytes)
		if link.TrafficLimitBytes <= 0 {
			unlimited = true
		} else {
			quota += nonNegativeTrafficUint(link.TrafficLimitBytes)
		}
	}
	// Detailed sampler records are intentionally limited to the latest three
	// days. Daily aggregates below remain available for the requested range.
	recordTo := time.Now().UTC()
	recordFrom := recordTo.Add(-72 * time.Hour)
	records, err := a.store.ListCustomerTrafficRecords(user.ID, recordFrom, recordTo, multipliers)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	dailyByDate := make(map[string]*model.CustomerDailyUsage)
	dailyLinksByDate := make(map[string]map[int64]*model.CustomerDailyLinkUsage)
	for _, item := range items {
		link := links[item.AssignmentID]
		multiplier := normalizeClientTrafficMultiplier(link.TrafficMultiplier)
		entry := dailyByDate[item.Date]
		if entry == nil {
			entry = &model.CustomerDailyUsage{Date: item.Date}
			dailyByDate[item.Date] = entry
		}
		linkEntries := dailyLinksByDate[item.Date]
		if linkEntries == nil {
			linkEntries = make(map[int64]*model.CustomerDailyLinkUsage)
			dailyLinksByDate[item.Date] = linkEntries
		}
		scaledUpload := scaleTrafficUint(item.UploadBytes, multiplier)
		scaledDownload := scaleTrafficUint(item.DownloadBytes, multiplier)
		entry.UploadBytes += scaledUpload
		entry.DownloadBytes += scaledDownload
		entry.TotalBytes += scaledUpload + scaledDownload
		entry.CountedBytes += scaledUpload + scaledDownload
		linkEntry := linkEntries[item.AssignmentID]
		if linkEntry == nil {
			linkEntry = &model.CustomerDailyLinkUsage{AssignmentID: item.AssignmentID}
			linkEntries[item.AssignmentID] = linkEntry
		}
		linkEntry.UploadBytes += scaledUpload
		linkEntry.DownloadBytes += scaledDownload
		linkEntry.TotalBytes += scaledUpload + scaledDownload
		linkEntry.CountedBytes += scaledUpload + scaledDownload
	}
	daily := make([]model.CustomerDailyUsage, 0, len(dailyByDate))
	for _, item := range dailyByDate {
		for _, linkEntry := range dailyLinksByDate[item.Date] {
			item.Links = append(item.Links, *linkEntry)
		}
		sort.Slice(item.Links, func(i, j int) bool { return item.Links[i].AssignmentID < item.Links[j].AssignmentID })
		daily = append(daily, *item)
	}
	sort.Slice(daily, func(i, j int) bool { return daily[i].Date < daily[j].Date })
	remaining := uint64(0)
	if !unlimited && quota > counted {
		remaining = quota - counted
	}
	writeJSON(w, http.StatusOK, model.CustomerUsageResponse{
		GeneratedAt:       time.Now().UTC(),
		RangeStart:        from,
		RangeEnd:          to,
		UploadBytes:       countedUpload,
		DownloadBytes:     countedDownload,
		RawTotalBytes:     rawUpload + rawDownload,
		CountedTotalBytes: counted,
		QuotaBytes:        quota,
		RemainingBytes:    remaining,
		Unlimited:         unlimited,
		Daily:             daily,
		Records:           records,
	})
}

func customerUsageRange(r *http.Request, now time.Time) (time.Time, time.Time, error) {
	location := model.BeijingLocation
	end := now.In(location)
	if raw := strings.TrimSpace(r.URL.Query().Get("to")); raw != "" {
		parsed, err := time.ParseInLocation("2006-01-02", raw, location)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid usage end date")
		}
		end = parsed.AddDate(0, 0, 1)
	}
	start := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, location).AddDate(0, 0, -29)
	if raw := strings.TrimSpace(r.URL.Query().Get("range")); raw != "" {
		switch raw {
		case "7d":
			start = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, location).AddDate(0, 0, -6)
		case "30d":
			start = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, location).AddDate(0, 0, -29)
		default:
			return time.Time{}, time.Time{}, fmt.Errorf("unsupported usage range")
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("from")); raw != "" {
		parsed, err := time.ParseInLocation("2006-01-02", raw, location)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid usage start date")
		}
		start = parsed
	}
	if !start.Before(end) {
		return time.Time{}, time.Time{}, fmt.Errorf("usage start date must be before end date")
	}
	return start, end, nil
}

func nonNegativeTrafficUint(value int64) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(value)
}

func scaleTrafficUint(value uint64, multiplier float64) uint64 {
	if value == 0 {
		return 0
	}
	if multiplier <= 0 {
		multiplier = 1
	}
	scaled := float64(value) * multiplier
	max := ^uint64(0)
	if scaled >= float64(max) {
		return max
	}
	return uint64(math.Round(scaled))
}
