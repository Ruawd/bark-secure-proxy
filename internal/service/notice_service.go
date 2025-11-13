package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/bark-labs/bark-secure-proxy/internal/barkclient"
	"github.com/bark-labs/bark-secure-proxy/internal/crypto"
	"github.com/bark-labs/bark-secure-proxy/internal/model"
	"github.com/bark-labs/bark-secure-proxy/internal/storage"
)

// NoticeService encrypts plaintext payloads and forwards them to Bark.
type NoticeService struct {
	store storage.Store
	bark  *barkclient.Client
}

// NewNoticeService builds NoticeService.
func NewNoticeService(store storage.Store, bark *barkclient.Client) *NoticeService {
	return &NoticeService{store: store, bark: bark}
}

// Broadcast encrypts and pushes notifications.
func (s *NoticeService) Broadcast(ctx context.Context, req model.NoticeRequest) (model.NoticeSummary, []model.NoticeResult, error) {
	if strings.TrimSpace(req.Body) == "" {
		return model.NoticeSummary{}, nil, fmt.Errorf("body is required")
	}
	if s.bark == nil {
		return model.NoticeSummary{}, nil, fmt.Errorf("bark client not configured")
	}

	targets, lookupFailures := s.pickTargets(ctx, req.DeviceKeys)
	if len(targets) == 0 {
		return model.NoticeSummary{}, lookupFailures, fmt.Errorf("no target devices resolved")
	}

	payload := map[string]string{
		"title":    req.Title,
		"subtitle": req.Subtitle,
		"body":     req.Body,
		"group":    req.Group,
		"url":      req.Url,
	}
	if strings.TrimSpace(req.Icon) != "" {
		payload["icon"] = req.Icon
	}
	if strings.TrimSpace(req.Image) != "" {
		payload["image"] = req.Image
	}

	var (
		results    = make([]model.NoticeResult, 0, len(targets)+len(lookupFailures))
		mu         sync.Mutex
		wg         sync.WaitGroup
		successNum int
	)

	results = append(results, lookupFailures...)

	wg.Add(len(targets))
	for _, device := range targets {
		device := device
		go func() {
			defer wg.Done()
			deviceResults := model.NoticeResult{DeviceKey: device.DeviceKey}
			ciphertext, err := s.encryptPayload(payload, device)
			if err != nil {
				deviceResults.Status = "FAILED"
				deviceResults.Message = err.Error()
				s.appendLog(ctx, device, req, deviceResults.Status, err.Error())
			} else {
				resp, pushErr := s.bark.SendEncryptedPush(ctx, device.DeviceKey, ciphertext, device.IV)
				if pushErr != nil {
					deviceResults.Status = "FAILED"
					deviceResults.Message = pushErr.Error()
					s.appendLog(ctx, device, req, "FAILED", pushErr.Error())
				} else {
					if resp != nil && resp.Code == 200 {
						deviceResults.Status = "SUCCESS"
					} else {
						deviceResults.Status = "FAILED"
					}
					if resp != nil {
						deviceResults.Message = resp.Message
					}
					s.appendLog(ctx, device, req, deviceResults.Status, deviceResults.Message)
				}
			}
			mu.Lock()
			if deviceResults.Status == "SUCCESS" {
				successNum++
			}
			results = append(results, deviceResults)
			mu.Unlock()
		}()
	}
	wg.Wait()
	summary := model.NoticeSummary{
		SendNum:    len(targets),
		SuccessNum: successNum,
	}
	return summary, results, nil
}

func (s *NoticeService) encryptPayload(payload map[string]string, device *model.Device) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return crypto.EncryptToBase64(body, []byte(device.EncodeKey), []byte(device.IV))
}

func (s *NoticeService) pickTargets(ctx context.Context, deviceKeys []string) ([]*model.Device, []model.NoticeResult) {
	var (
		devices []*model.Device
		result  []model.NoticeResult
	)
	if len(deviceKeys) == 0 {
		list, err := s.store.ListActiveDevices(ctx)
		if err != nil {
			return nil, []model.NoticeResult{{
				Status:  "FAILED",
				Message: fmt.Sprintf("list devices: %v", err),
			}}
		}
		return list, nil
	}
	for _, key := range deviceKeys {
		device, err := s.store.GetDeviceByKey(ctx, key)
		if err != nil {
			result = append(result, model.NoticeResult{
				DeviceKey: key,
				Status:    "FAILED",
				Message:   err.Error(),
			})
			continue
		}
		devices = append(devices, device)
	}
	return devices, result
}

func (s *NoticeService) appendLog(ctx context.Context, device *model.Device, req model.NoticeRequest, status, result string) {
	logEntry := &model.NoticeLog{
		DeviceKey: device.DeviceKey,
		URL:       s.bark.DeviceEndpoint(device.DeviceKey),
		Title:     req.Title,
		Body:      req.Body,
		Group:     req.Group,
		Result:    result,
		Status:    status,
	}
	if err := s.store.AppendNoticeLog(ctx, logEntry); err != nil {
		log.Printf("append notice log failed: %v", err)
	}
}
