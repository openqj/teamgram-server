package logic

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/teamgram-server/app/bff/authorization/model"
)

type failingPhoneCodeStore struct {
	code      *model.PhoneCodeTransaction
	updateErr error
	deleted   []string
}

func (s *failingPhoneCodeStore) CreatePhoneCode(context.Context, int64, int64, string, bool, int, int, int) (*model.PhoneCodeTransaction, error) {
	return s.code, nil
}

func (s *failingPhoneCodeStore) GetPhoneCode(context.Context, int64, string, string) (*model.PhoneCodeTransaction, error) {
	return s.code, nil
}

func (s *failingPhoneCodeStore) UpdatePhoneCodeData(context.Context, int64, string, string, *model.PhoneCodeTransaction) error {
	return s.updateErr
}

func (s *failingPhoneCodeStore) DeleteCachePhoneCode(_ context.Context, _ int64, phone string) error {
	s.deleted = append(s.deleted, phone)
	return nil
}

func TestDoAuthSendCodeRollsBackAfterPhoneCodePersistFailure(t *testing.T) {
	store := &failingPhoneCodeStore{
		code:      &model.PhoneCodeTransaction{PhoneNumber: "15551234567", PhoneCodeHash: "challenge"},
		updateErr: errors.New("phone-code store unavailable"),
	}
	logic := &AuthLogic{phoneCodes: store}
	cleanupCalled := false
	_, err := logic.DoAuthSendCode(context.Background(), 1, 2, store.code.PhoneNumber, false, false, func(*model.PhoneCodeTransaction) error {
		return nil
	}, func(*model.PhoneCodeTransaction) { cleanupCalled = true })
	if !errors.Is(err, store.updateErr) {
		t.Fatalf("DoAuthSendCode() error = %v, want %v", err, store.updateErr)
	}
	if !cleanupCalled {
		t.Fatal("persist-failure callback was not called")
	}
	if len(store.deleted) != 1 || store.deleted[0] != store.code.PhoneNumber {
		t.Fatalf("deleted phone-code keys = %#v, want [%q]", store.deleted, store.code.PhoneNumber)
	}
}

func TestDoAuthResendCodeRollsBackAfterPhoneCodePersistFailure(t *testing.T) {
	store := &failingPhoneCodeStore{
		code: &model.PhoneCodeTransaction{
			PhoneNumber: "15551234567", PhoneCodeHash: "challenge",
			PhoneCodeExpired: 2_000_000_000, State: model.CodeStateSent,
		},
		updateErr: errors.New("phone-code store unavailable"),
	}
	logic := &AuthLogic{phoneCodes: store}
	cleanupCalled := false
	_, err := logic.DoAuthReSendCode(context.Background(), 1, store.code.PhoneNumber, store.code.PhoneCodeHash, func(*model.PhoneCodeTransaction) error {
		return nil
	}, func(*model.PhoneCodeTransaction) { cleanupCalled = true })
	if !errors.Is(err, store.updateErr) {
		t.Fatalf("DoAuthReSendCode() error = %v, want %v", err, store.updateErr)
	}
	if !cleanupCalled {
		t.Fatal("persist-failure callback was not called")
	}
	if len(store.deleted) != 1 || store.deleted[0] != store.code.PhoneNumber {
		t.Fatalf("deleted phone-code keys = %#v, want [%q]", store.deleted, store.code.PhoneNumber)
	}
}
