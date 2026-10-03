package dao

import (
	"context"
	"sync"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/authorization/model"
	"github.com/zeromicro/go-zero/core/stores/kv"
)

type phoneCodeTestStore struct {
	kv.Store
	mu     sync.Mutex
	values map[string]string
}

func newPhoneCodeTestStore() *phoneCodeTestStore {
	return &phoneCodeTestStore{values: make(map[string]string)}
}

func (s *phoneCodeTestStore) GetCtx(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[key], nil
}

func (s *phoneCodeTestStore) SetexCtx(_ context.Context, key, value string, _ int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
	return nil
}

func (s *phoneCodeTestStore) DelCtx(_ context.Context, keys ...string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	deleted := 0
	for _, key := range keys {
		if _, ok := s.values[key]; ok {
			delete(s.values, key)
			deleted++
		}
	}
	return deleted, nil
}

func TestDeletePhoneCodeRequiresMatchingHash(t *testing.T) {
	store := newPhoneCodeTestStore()
	d := &Dao{kv: store}
	const (
		authKeyID = int64(42)
		phone     = "15551234567"
		hash      = "current-hash"
	)
	if err := d.PutCachePhoneCode(context.Background(), authKeyID, phone, &model.PhoneCodeTransaction{
		AuthKeyId: authKeyID, PhoneNumber: phone, PhoneCodeHash: hash,
	}); err != nil {
		t.Fatalf("PutCachePhoneCode() error = %v", err)
	}

	if err := d.DeletePhoneCode(context.Background(), authKeyID, phone, "stale-hash"); err != mtproto.ErrPhoneCodeInvalid {
		t.Fatalf("DeletePhoneCode(stale hash) error = %v, want %v", err, mtproto.ErrPhoneCodeInvalid)
	}
	if current, err := d.GetCachePhoneCode(context.Background(), authKeyID, phone); err != nil || current == nil || current.PhoneCodeHash != hash {
		t.Fatalf("stale cancellation removed current challenge: value = %#v, error = %v", current, err)
	}

	if err := d.DeletePhoneCode(context.Background(), authKeyID, phone, hash); err != nil {
		t.Fatalf("DeletePhoneCode(current hash) error = %v", err)
	}
	if current, err := d.GetCachePhoneCode(context.Background(), authKeyID, phone); err != nil || current != nil {
		t.Fatalf("current cancellation left challenge: value = %#v, error = %v", current, err)
	}
}
