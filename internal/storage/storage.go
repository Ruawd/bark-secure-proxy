package storage

import (
	"context"

	"github.com/bark-labs/bark-secure-proxy/internal/model"
)

// Store abstracts device persistence.
type Store interface {
	UpsertDevice(ctx context.Context, device *model.Device) error
	GetDevice(ctx context.Context, token string) (*model.Device, error)
	GetDeviceByKey(ctx context.Context, key string) (*model.Device, error)
	ListDevices(ctx context.Context) ([]*model.Device, error)
	ListActiveDevices(ctx context.Context) ([]*model.Device, error)
	AppendNoticeLog(ctx context.Context, log *model.NoticeLog) error
	ListNoticeLogs(ctx context.Context) ([]*model.NoticeLog, error)
	Close() error
}
