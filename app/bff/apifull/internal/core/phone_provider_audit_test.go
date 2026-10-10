package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

func TestPhoneGetCallConfigFailsClosedWithoutRelayProvider(t *testing.T) {
	previous := domain.Relay
	t.Cleanup(func() { domain.Relay = previous })
	domain.Relay = domain.RelayConfig{}

	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1001}}
	result, err := core.PhoneGetCallConfig(&mtproto.TLPhoneGetCallConfig{})
	if result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("phone.getCallConfig = (%#v, %v), want nil result and METHOD_NOT_IMPL", result, err)
	}
}

func TestPhoneGetCallConfigRejectsLoopbackRelay(t *testing.T) {
	previous := domain.Relay
	t.Cleanup(func() { domain.Relay = previous })
	domain.Relay = domain.RelayConfig{
		IP:       "127.0.0.1",
		Port:     3478,
		Username: "fixture-user",
		Password: "fixture-password",
	}

	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1002}}
	result, err := core.PhoneGetCallConfig(&mtproto.TLPhoneGetCallConfig{})
	if result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("loopback phone.getCallConfig = (%#v, %v), want nil result and METHOD_NOT_IMPL", result, err)
	}
}

func TestPhoneGetGroupCallStarsFailsClosedWithoutProvider(t *testing.T) {
	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1003}}
	result, err := core.PhoneGetGroupCallStars(&mtproto.TLPhoneGetGroupCallStars{
		Call: &mtproto.InputGroupCall{Id: 1, AccessHash: 2},
	})
	if result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("phone.getGroupCallStars = (%#v, %v), want nil result and METHOD_NOT_IMPL", result, err)
	}
}
