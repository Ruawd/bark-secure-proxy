package model

import "time"

// NoticeLog tracks each push attempt.
type NoticeLog struct {
	ID        uint64    `json:"id"`
	DeviceKey string    `json:"deviceKey"`
	URL       string    `json:"url"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Group     string    `json:"group"`
	Result    string    `json:"result"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// NoticeLogFilter describes query parameters for log searching.
type NoticeLogFilter struct {
	DeviceKey string
	Group     string
	Status    string
	BeginTime *time.Time
	EndTime   *time.Time
	Page      int
	PageSize  int
}
