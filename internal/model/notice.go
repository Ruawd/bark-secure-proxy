package model

// NoticeRequest models plaintext message clients send to the proxy.
type NoticeRequest struct {
	Title      string   `json:"title"`
	Subtitle   string   `json:"subtitle"`
	Body       string   `json:"body"`
	Group      string   `json:"group"`
	Url        string   `json:"url"`
	Icon       string   `json:"icon"`
	Image      string   `json:"image"`
	DeviceKeys []string `json:"deviceKeys"`
}

// NoticeResult summarises a push attempt.
type NoticeResult struct {
	DeviceKey string `json:"deviceKey"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
}
