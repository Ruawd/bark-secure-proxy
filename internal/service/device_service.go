package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/bark-labs/bark-secure-proxy/internal/barkclient"
	"github.com/bark-labs/bark-secure-proxy/internal/config"
	"github.com/bark-labs/bark-secure-proxy/internal/crypto"
	"github.com/bark-labs/bark-secure-proxy/internal/model"
	"github.com/bark-labs/bark-secure-proxy/internal/storage"
)

// DeviceService manages encrypted device metadata.
type DeviceService struct {
	store storage.Store
	cfg   *config.Config
	bark  *barkclient.Client
}

// DeviceRequest describes upsert payload.
type DeviceRequest struct {
	DeviceToken string `json:"deviceToken"`
	DeviceKey   string `json:"deviceKey"`
	Name        string `json:"name"`
	Algorithm   string `json:"algorithm"`
	Mode        string `json:"mode"`
	Padding     string `json:"padding"`
	EncodeKey   string `json:"encodeKey"`
	IV          string `json:"iv"`
	Status      string `json:"status"`
	RegisterKey string `json:"registerKey"`
}

// NewDeviceService constructs DeviceService.
func NewDeviceService(store storage.Store, cfg *config.Config, bark *barkclient.Client) *DeviceService {
	return &DeviceService{store: store, cfg: cfg, bark: bark}
}

// RegisterDevice proxies Bark /register and caches the device key.
func (s *DeviceService) RegisterDevice(ctx context.Context, deviceToken, key string) (*barkclient.CommonResponse[barkclient.RegisterData], error) {
	if s.bark == nil {
		return nil, fmt.Errorf("bark client not configured")
	}
	if strings.TrimSpace(deviceToken) == "" {
		return nil, fmt.Errorf("deviceToken is required")
	}
	resp, err := s.bark.Register(ctx, deviceToken, key)
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Data.DeviceKey == "" {
		return resp, nil
	}
	device, err := s.store.GetDevice(ctx, resp.Data.DeviceToken)
	if err != nil {
		if err != storage.ErrNotFound {
			return nil, err
		}
		device = &model.Device{DeviceToken: resp.Data.DeviceToken}
	}
	device.DeviceKey = resp.Data.DeviceKey
	if device.Status == "" {
		device.Status = model.DeviceStatusActive
	}
	if err := s.store.UpsertDevice(ctx, device); err != nil {
		return nil, err
	}
	return resp, nil
}

// GenerateConfig aligns with /device/gen behaviour.
func (s *DeviceService) GenerateConfig(ctx context.Context, req DeviceRequest) (*model.Device, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("device name is required")
	}
	if strings.TrimSpace(req.DeviceKey) == "" {
		return nil, fmt.Errorf("device key is required")
	}
	return s.Upsert(ctx, req)
}

// Upsert stores/updates device metadata (used by admin UI + generator).
func (s *DeviceService) Upsert(ctx context.Context, req DeviceRequest) (*model.Device, error) {
	if strings.TrimSpace(req.DeviceToken) == "" {
		return nil, fmt.Errorf("deviceToken is required")
	}

	device, err := s.store.GetDevice(ctx, req.DeviceToken)
	if err != nil {
		if err != storage.ErrNotFound {
			return nil, err
		}
		device = &model.Device{DeviceToken: req.DeviceToken}
	}

	device.Name = req.Name
	device.Status = firstNonEmpty(strings.ToUpper(req.Status), model.DeviceStatusActive)
	device.Algorithm = firstNonEmpty(req.Algorithm, s.cfg.Crypto.DefaultAlgorithm)
	device.Mode = firstNonEmpty(req.Mode, s.cfg.Crypto.DefaultMode)
	device.Padding = firstNonEmpty(req.Padding, s.cfg.Crypto.DefaultPadding)

	if req.EncodeKey == "" {
		if device.EncodeKey == "" {
			generated, err := crypto.GenerateString(s.cfg.Crypto.KeyBytes)
			if err != nil {
				return nil, err
			}
			device.EncodeKey = generated
		}
	} else {
		device.EncodeKey = req.EncodeKey
	}

	if req.IV == "" {
		if device.IV == "" {
			generated, err := crypto.GenerateString(s.cfg.Crypto.IVBytes)
			if err != nil {
				return nil, err
			}
			device.IV = generated
		}
	} else {
		device.IV = req.IV
	}

	if req.DeviceKey != "" {
		device.DeviceKey = req.DeviceKey
	}

	if !isValidKeyLength(device.EncodeKey) {
		return nil, fmt.Errorf("encodeKey must be 16, 24 or 32 characters")
	}
	if len(device.IV) != s.cfg.Crypto.IVBytes {
		return nil, fmt.Errorf("iv must be %d characters", s.cfg.Crypto.IVBytes)
	}

	if device.DeviceKey == "" {
		if s.bark == nil {
			return nil, fmt.Errorf("bark client not configured, cannot register device")
		}
		resp, err := s.bark.Register(ctx, req.DeviceToken, req.RegisterKey)
		if err != nil {
			return nil, err
		}
		if resp == nil || resp.Data.DeviceKey == "" {
			return nil, fmt.Errorf("register failed: empty device key")
		}
		device.DeviceKey = resp.Data.DeviceKey
	}

	if err := s.store.UpsertDevice(ctx, device); err != nil {
		return nil, err
	}

	return device, nil
}

// List returns all devices.
func (s *DeviceService) List(ctx context.Context) ([]*model.Device, error) {
	return s.store.ListDevices(ctx)
}

// ListViews returns masked device views.
func (s *DeviceService) ListViews(ctx context.Context) ([]*model.DeviceView, error) {
	devices, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]*model.DeviceView, 0, len(devices))
	for _, device := range devices {
		views = append(views, toView(device))
	}
	return views, nil
}

// Get returns device by token.
func (s *DeviceService) Get(ctx context.Context, token string) (*model.Device, error) {
	return s.store.GetDevice(ctx, token)
}

// UpdateStatus toggles device activation.
func (s *DeviceService) UpdateStatus(ctx context.Context, token, status string) (*model.Device, error) {
	device, err := s.store.GetDevice(ctx, token)
	if err != nil {
		return nil, err
	}
	device.Status = strings.ToUpper(strings.TrimSpace(status))
	if device.Status == "" {
		device.Status = model.DeviceStatusActive
	}
	if err := s.store.UpsertDevice(ctx, device); err != nil {
		return nil, err
	}
	return device, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func isValidKeyLength(key string) bool {
	l := len(key)
	return l == 16 || l == 24 || l == 32
}

func toView(device *model.Device) *model.DeviceView {
	if device == nil {
		return nil
	}
	return &model.DeviceView{
		DeviceToken: maskValue(device.DeviceToken),
		Name:        device.Name,
		DeviceKey:   maskValue(device.DeviceKey),
		Algorithm:   device.Algorithm,
		Mode:        device.Mode,
		Padding:     device.Padding,
		EncodeKey:   maskValue(device.EncodeKey),
		IV:          maskValue(device.IV),
		Status:      device.Status,
	}
}

func maskValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	runes := []rune(value)
	length := len(runes)
	if length <= 4 {
		return value
	}
	masked := make([]rune, length-4)
	for i := range masked {
		masked[i] = '*'
	}
	return string(runes[:4]) + string(masked)
}
