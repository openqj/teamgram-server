package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type connectedBotInputValidationUserClient struct {
	userclient.UserClient
	profiles map[int64]*mtproto.ImmutableUser
}

func (c *connectedBotInputValidationUserClient) UserGetImmutableUser(_ context.Context, in *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	if profile := c.profiles[in.GetId()]; profile != nil {
		return profile, nil
	}
	return nil, mtproto.ErrUserIdInvalid
}

type connectedBotInputValidationStore struct {
	values map[string]string
	writes int
}

func (s *connectedBotInputValidationStore) Get(key string) (string, error) {
	return s.values[key], nil
}

func (s *connectedBotInputValidationStore) Set(key, value string) error {
	if s.values == nil {
		s.values = make(map[string]string)
	}
	s.writes++
	s.values[key] = value
	return nil
}

func connectedBotInputValidationProfile(id, accessHash int64, premium, bot bool) *mtproto.ImmutableUser {
	data := &mtproto.UserData{Id: id, AccessHash: accessHash, Premium: premium}
	if bot {
		data.Bot = &mtproto.BotData{}
	}
	return mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: data}).To_ImmutableUser()
}

func TestAccountUpdateConnectedBotRejectsInvalidTargetBeforeWrite(t *testing.T) {
	const (
		callerID   = int64(81026001)
		botID      = int64(81026002)
		accessHash = int64(91026002)
		staleHash  = accessHash + 1
	)
	tests := []struct {
		name    string
		profile *mtproto.ImmutableUser
		input   *mtproto.InputUser
		wantErr error
	}{
		{
			name:    "stale access hash",
			profile: connectedBotInputValidationProfile(botID, accessHash, false, true),
			input:   mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: botID, AccessHash: staleHash}).To_InputUser(),
			wantErr: mtproto.ErrUserIdInvalid,
		},
		{
			name:    "human target",
			profile: connectedBotInputValidationProfile(botID, accessHash, false, false),
			input:   mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: botID, AccessHash: accessHash}).To_InputUser(),
			wantErr: mtproto.ErrBotInvalid,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &connectedBotInputValidationStore{}
			previous := persist.Default
			persist.Use(store)
			t.Cleanup(func() { persist.Use(previous) })
			client := &connectedBotInputValidationUserClient{profiles: map[int64]*mtproto.ImmutableUser{
				callerID: connectedBotInputValidationProfile(callerID, callerID, true, false),
				botID:    test.profile,
			}}
			c := &ApiFullCore{
				ctx:    context.Background(),
				svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
				MD:     &metadata.RpcMetadata{UserId: callerID},
			}

			result, err := c.AccountUpdateConnectedBot(&mtproto.TLAccountUpdateConnectedBot{Bot: test.input})
			if result != nil || !errors.Is(err, test.wantErr) {
				t.Fatalf("update connected bot = (%v, %v), want nil and %v", result, err, test.wantErr)
			}
			if store.writes != 0 {
				t.Fatalf("rejected connected bot wrote local state %d times", store.writes)
			}
		})
	}
}

func TestVerifyConnectedBotResolvesInputUserSelf(t *testing.T) {
	const callerID = int64(81026003)
	client := &connectedBotInputValidationUserClient{profiles: map[int64]*mtproto.ImmutableUser{
		callerID: connectedBotInputValidationProfile(callerID, callerID, true, true),
	}}
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
		MD:     &metadata.RpcMetadata{UserId: callerID},
	}
	self := mtproto.MakeTLInputUserSelf(&mtproto.InputUser{}).To_InputUser()
	if got, err := c.verifyConnectedBot(self); err != nil || got != callerID {
		t.Fatalf("verify inputUserSelf = (%d, %v), want caller %d", got, err, callerID)
	}
}
