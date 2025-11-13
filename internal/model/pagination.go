package model

// NoticeLogPage reflects the paginated payload expected by bark-api frontend.
type NoticeLogPage struct {
	Data     []*NoticeLog `json:"data"`
	Total    int          `json:"total"`
	Pages    int          `json:"pages"`
	PageNum  int          `json:"pageNum"`
	PageSize int          `json:"pageSize"`
}
