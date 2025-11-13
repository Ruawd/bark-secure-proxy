package model

import "time"

// Device represents a Bark client device plus encryption material.
type Device struct {
	DeviceToken string    `json:"deviceToken"`
	DeviceKey   string    `json:"deviceKey"`
	Name        string    `json:"name"`
	Algorithm   string    `json:"algorithm"`
	Mode        string    `json:"model"`
	Padding     string    `json:"padding"`
	EncodeKey   string    `json:"encodeKey"`
	IV          string    `json:"iv"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

const (
	DeviceStatusActive = "ACTIVE"
	DeviceStatusStop   = "STOP"
)
