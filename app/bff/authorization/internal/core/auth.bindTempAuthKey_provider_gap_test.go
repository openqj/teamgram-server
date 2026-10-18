package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	authsessionclient "github.com/teamgram/teamgram-server/app/service/authsession/client"
	"github.com/zeromicro/go-zero/core/logx"
)

type bindTempAuthKeyClientStub struct {
	authsessionclient.AuthsessionClient
	result  *mtproto.Bool
	err     error
	request *authsession.TLAuthsessionBindTempAuthKey
}

func (s *bindTempAuthKeyClientStub) AuthsessionBindTempAuthKey(_ context.Context, in *authsession.TLAuthsessionBindTempAuthKey) (*mtproto.Bool, error) {
	s.request = in
	return s.result, s.err
}

func newBindTempAuthKeyTestCore(stub authsessionclient.AuthsessionClient) *AuthorizationCore {
	ctx := context.Background()
	return &AuthorizationCore{
		ctx:    ctx,
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{AuthId: 92, PermAuthKeyId: 91},
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{AuthsessionClient: stub}},
	}
}

func validBindTempAuthKeyRequest() *mtproto.TLAuthBindTempAuthKey {
	return &mtproto.TLAuthBindTempAuthKey{
		PermAuthKeyId:    91,
		Nonce:            12,
		ExpiresAt:        34,
		EncryptedMessage: make([]byte, 56),
	}
}

func TestAuthBindTempAuthKeyRejectsMalformedProviderResult(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result *mtproto.Bool
	}{
		{name: "nil", result: nil},
		{name: "false", result: mtproto.BoolFalse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &bindTempAuthKeyClientStub{result: tc.result}
			result, err := newBindTempAuthKeyTestCore(stub).AuthBindTempAuthKey(validBindTempAuthKeyRequest())
			if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
				t.Fatalf("bind temp auth key = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", result, err)
			}
			if stub.request == nil || stub.request.GetPermAuthKeyId() != 91 || len(stub.request.GetEncryptedMessage()) != 56 {
				t.Fatalf("provider request = %v, want perm auth key 91 and 56-byte payload", stub.request)
			}
		})
	}
}

func TestAuthBindTempAuthKeyAcceptsProviderSuccess(t *testing.T) {
	stub := &bindTempAuthKeyClientStub{result: mtproto.BoolTrue}
	result, err := newBindTempAuthKeyTestCore(stub).AuthBindTempAuthKey(validBindTempAuthKeyRequest())
	if err != nil || result != mtproto.BoolTrue {
		t.Fatalf("bind temp auth key = (%v, %v), want BoolTrue", result, err)
	}
}
