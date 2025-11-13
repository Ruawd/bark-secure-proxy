package bolt

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bark-labs/bark-secure-proxy/internal/model"
	"github.com/bark-labs/bark-secure-proxy/internal/storage"
	bolt "go.etcd.io/bbolt"
)

var _ storage.Store = (*Store)(nil)

var (
	bucketDevices   = []byte("devices")
	bucketNoticeLog = []byte("notice_logs")
	errStop         = errors.New("stop iteration")
)

// Store is a BoltDB-backed Store implementation.
type Store struct {
	db *bolt.DB
}

// New initialises the Bolt store.
func New(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, err
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(bucketDevices); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists(bucketNoticeLog)
		return err
	}); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close closes underlying Bolt DB.
func (s *Store) Close() error {
	return s.db.Close()
}

// UpsertDevice stores or updates a device record.
func (s *Store) UpsertDevice(ctx context.Context, device *model.Device) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	now := time.Now().UTC()
	if device.CreatedAt.IsZero() {
		device.CreatedAt = now
	}
	device.UpdatedAt = now
	payload, err := json.Marshal(device)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(bucketDevices)
		return bkt.Put([]byte(device.DeviceToken), payload)
	})
}

// GetDevice fetches device by token.
func (s *Store) GetDevice(ctx context.Context, token string) (*model.Device, error) {
	return s.getByKey(ctx, token, func(d *model.Device) bool { return true })
}

// GetDeviceByKey fetches device by Bark device key.
func (s *Store) GetDeviceByKey(ctx context.Context, key string) (*model.Device, error) {
	return s.getByKey(ctx, key, func(d *model.Device) bool {
		return d.DeviceKey == key
	})
}

func (s *Store) getByKey(ctx context.Context, key string, matcher func(*model.Device) bool) (*model.Device, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	var (
		result *model.Device
	)
	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(bucketDevices)
		return bkt.ForEach(func(k, v []byte) error {
			var device model.Device
			if err := json.Unmarshal(v, &device); err != nil {
				return err
			}
			if matcher(&device) {
				result = &device
				return errStop
			}
			return nil
		})
	})
	if err != nil && !errors.Is(err, errStop) {
		return nil, err
	}
	if result == nil {
		return nil, storage.ErrNotFound
	}
	return result, nil
}

// ListDevices returns all devices.
func (s *Store) ListDevices(ctx context.Context) ([]*model.Device, error) {
	return s.list(ctx, func(*model.Device) bool { return true })
}

// ListActiveDevices returns ACTIVE devices only.
func (s *Store) ListActiveDevices(ctx context.Context) ([]*model.Device, error) {
	return s.list(ctx, func(d *model.Device) bool {
		status := strings.ToUpper(strings.TrimSpace(d.Status))
		return status == "" || status == model.DeviceStatusActive
	})
}

func (s *Store) list(ctx context.Context, filter func(*model.Device) bool) ([]*model.Device, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	var devices []*model.Device
	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(bucketDevices)
		return bkt.ForEach(func(_, v []byte) error {
			var device model.Device
			if err := json.Unmarshal(v, &device); err != nil {
				return err
			}
			if filter(&device) {
				copied := device
				devices = append(devices, &copied)
			}
			return nil
		})
	})
	return devices, err
}

// AppendNoticeLog stores a push log entry.
func (s *Store) AppendNoticeLog(ctx context.Context, log *model.NoticeLog) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	now := time.Now().UTC()
	if log.CreatedAt.IsZero() {
		log.CreatedAt = now
	}
	log.UpdatedAt = now
	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(bucketNoticeLog)
		id, err := bkt.NextSequence()
		if err != nil {
			return err
		}
		log.ID = id
		payload, err := json.Marshal(log)
		if err != nil {
			return err
		}
		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, id)
		return bkt.Put(key, payload)
	})
}

// ListNoticeLogs returns all notice logs.
func (s *Store) ListNoticeLogs(ctx context.Context) ([]*model.NoticeLog, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	var logs []*model.NoticeLog
	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(bucketNoticeLog)
		return bkt.ForEach(func(_, v []byte) error {
			var log model.NoticeLog
			if err := json.Unmarshal(v, &log); err != nil {
				return err
			}
			copied := log
			logs = append(logs, &copied)
			return nil
		})
	})
	return logs, err
}
