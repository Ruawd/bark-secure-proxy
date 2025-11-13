package model

// DeviceView hides sensitive fields when returning devices to clients.
type DeviceView struct {
	DeviceToken string `json:"deviceToken"`
	Name        string `json:"name"`
	DeviceKey   string `json:"deviceKey"`
	Algorithm   string `json:"algorithm"`
	Mode        string `json:"model"`
	Padding     string `json:"padding"`
	EncodeKey   string `json:"encodeKey"`
	IV          string `json:"iv"`
	Status      string `json:"status"`
}
