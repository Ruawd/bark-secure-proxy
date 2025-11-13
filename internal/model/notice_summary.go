package model

// NoticeSummary mirrors bark-api's NoticeResponse payload.
type NoticeSummary struct {
	SendNum    int `json:"sendNum"`
	SuccessNum int `json:"successNum"`
}
