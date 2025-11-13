package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/bark-labs/bark-secure-proxy/internal/model"
	"github.com/bark-labs/bark-secure-proxy/internal/storage"
)

// NoticeLogService provides filtering and statistics similar to bark-api.
type NoticeLogService struct {
	store     storage.Store
	deviceSvc *DeviceService
}

// NewNoticeLogService builds the notice log service.
func NewNoticeLogService(store storage.Store, deviceSvc *DeviceService) *NoticeLogService {
	return &NoticeLogService{store: store, deviceSvc: deviceSvc}
}

// Query returns paginated logs.
func (s *NoticeLogService) Query(ctx context.Context, filter model.NoticeLogFilter) (*model.NoticeLogPage, error) {
	logs, err := s.filteredLogs(ctx, filter)
	if err != nil {
		return nil, err
	}

	total := len(logs)
	if filter.PageSize <= 0 {
		filter.PageSize = 10
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}

	start := (filter.Page - 1) * filter.PageSize
	if start > total {
		start = total
	}
	end := start + filter.PageSize
	if end > total {
		end = total
	}

	pageLogs := logs[start:end]
	pages := (total + filter.PageSize - 1) / filter.PageSize

	return &model.NoticeLogPage{
		Data:     pageLogs,
		Total:    total,
		Pages:    pages,
		PageNum:  filter.Page,
		PageSize: filter.PageSize,
	}, nil
}

// CountByDate aggregates logs per day/month/year.
func (s *NoticeLogService) CountByDate(ctx context.Context, dateType string, begin, end *time.Time) ([]map[string]any, error) {
	filter := model.NoticeLogFilter{
		BeginTime: begin,
		EndTime:   end,
	}
	logs, err := s.filteredLogs(ctx, filter)
	if err != nil {
		return nil, err
	}

	layout := "2006-01-02"
	switch strings.ToLower(dateType) {
	case "year":
		layout = "2006"
	case "month":
		layout = "2006-01"
	}

	counter := make(map[string]int)
	for _, log := range logs {
		key := log.CreatedAt.Format(layout)
		counter[key]++
	}

	var result []map[string]any
	for key, count := range counter {
		result = append(result, map[string]any{
			"date":  key,
			"count": count,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i]["date"].(string) < result[j]["date"].(string)
	})
	return result, nil
}

// CountByStatus aggregates by log status.
func (s *NoticeLogService) CountByStatus(ctx context.Context, begin, end *time.Time) ([]map[string]any, error) {
	filter := model.NoticeLogFilter{BeginTime: begin, EndTime: end}
	logs, err := s.filteredLogs(ctx, filter)
	if err != nil {
		return nil, err
	}
	counter := make(map[string]int)
	for _, log := range logs {
		status := log.Status
		if status == "" {
			status = "UNKNOWN"
		}
		counter[status]++
	}
	return mapToKV(counter, "status"), nil
}

// CountByGroup aggregates by notification group.
func (s *NoticeLogService) CountByGroup(ctx context.Context, begin, end *time.Time) ([]map[string]any, error) {
	filter := model.NoticeLogFilter{BeginTime: begin, EndTime: end}
	logs, err := s.filteredLogs(ctx, filter)
	if err != nil {
		return nil, err
	}
	counter := make(map[string]int)
	for _, log := range logs {
		group := strings.TrimSpace(log.Group)
		if group == "" {
			group = "DEFAULT"
		}
		counter[group]++
	}
	return mapToKV(counter, "group"), nil
}

// CountByDevice aggregates using device names when available.
func (s *NoticeLogService) CountByDevice(ctx context.Context, begin, end *time.Time) ([]map[string]any, error) {
	filter := model.NoticeLogFilter{BeginTime: begin, EndTime: end}
	logs, err := s.filteredLogs(ctx, filter)
	if err != nil {
		return nil, err
	}
	counter := make(map[string]int)
	deviceName := make(map[string]string)
	if s.deviceSvc != nil {
		devices, err := s.deviceSvc.List(ctx)
		if err == nil {
			for _, d := range devices {
				name := d.Name
				if name == "" {
					name = d.DeviceKey
				}
				deviceName[d.DeviceKey] = name
			}
		}
	}
	for _, log := range logs {
		name := deviceName[log.DeviceKey]
		if name == "" {
			name = log.DeviceKey
		}
		counter[name]++
	}
	return mapToKV(counter, "device"), nil
}

func (s *NoticeLogService) filteredLogs(ctx context.Context, filter model.NoticeLogFilter) ([]*model.NoticeLog, error) {
	all, err := s.store.ListNoticeLogs(ctx)
	if err != nil {
		return nil, err
	}
	matches := make([]*model.NoticeLog, 0, len(all))
	for _, log := range all {
		if filter.DeviceKey != "" && !strings.EqualFold(log.DeviceKey, filter.DeviceKey) {
			continue
		}
		if filter.Group != "" && !strings.EqualFold(log.Group, filter.Group) {
			continue
		}
		if filter.Status != "" && !strings.EqualFold(log.Status, filter.Status) {
			continue
		}
		if filter.BeginTime != nil && log.CreatedAt.Before(filter.BeginTime.UTC()) {
			continue
		}
		if filter.EndTime != nil && log.CreatedAt.After(filter.EndTime.UTC()) {
			continue
		}
		matches = append(matches, log)
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].CreatedAt.After(matches[j].CreatedAt)
	})
	return matches, nil
}

func mapToKV(counter map[string]int, key string) []map[string]any {
	var result []map[string]any
	for k, v := range counter {
		result = append(result, map[string]any{
			key:     k,
			"count": v,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i][key].(string) < result[j][key].(string)
	})
	return result
}
