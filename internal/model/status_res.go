package model

// StatusRes mirrors bark-api's /status/endpoint payload.
type StatusRes struct {
	Status          string `json:"status"`
	ActiveDeviceNum int    `json:"activeDeviceNum"`
	AllDeviceNum    int    `json:"allDeviceNum"`
}
