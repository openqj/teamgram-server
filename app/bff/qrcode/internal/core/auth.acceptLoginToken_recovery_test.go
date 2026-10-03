package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
)

func TestEnsureQRLoginAuthKeyBinding(t *testing.T) {
	bindFailure := errors.New("bind transport failure")
	tests := []struct {
		name          string
		users         []int64
		bindErr       error
		wantErr       error
		wantBindCalls int
	}{
		{name: "already bound to claimant", users: []int64{42}},
		{name: "binds unowned key", users: []int64{0}, wantBindCalls: 1},
		{name: "confirms ambiguous bind", users: []int64{0, 42}, bindErr: bindFailure, wantBindCalls: 1},
		{name: "leaves unconfirmed bind recoverable", users: []int64{0, 0}, bindErr: bindFailure, wantErr: bindFailure, wantBindCalls: 1},
		{name: "rejects another owner", users: []int64{43}, wantErr: mtproto.ErrAuthTokenInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binder := &qrAuthKeyBinderStub{users: tt.users, bindErr: tt.bindErr}
			err := ensureQRLoginAuthKeyBinding(context.Background(), binder, 1001, 42)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ensureQRLoginAuthKeyBinding() error = %v, want %v", err, tt.wantErr)
			}
			if binder.bindCalls != tt.wantBindCalls {
				t.Fatalf("bind calls = %d, want %d", binder.bindCalls, tt.wantBindCalls)
			}
		})
	}
}

type qrAuthKeyBinderStub struct {
	users     []int64
	getCalls  int
	bindCalls int
	bindErr   error
}

func (s *qrAuthKeyBinderStub) AuthsessionGetUserId(context.Context, *authsession.TLAuthsessionGetUserId) (*mtproto.Int64, error) {
	index := s.getCalls
	s.getCalls++
	if index >= len(s.users) {
		index = len(s.users) - 1
	}
	return &mtproto.Int64{V: s.users[index]}, nil
}

func (s *qrAuthKeyBinderStub) AuthsessionBindAuthKeyUser(context.Context, *authsession.TLAuthsessionBindAuthKeyUser) (*mtproto.Int64, error) {
	s.bindCalls++
	if s.bindErr != nil {
		return nil, s.bindErr
	}
	return &mtproto.Int64{V: 99}, nil
}
