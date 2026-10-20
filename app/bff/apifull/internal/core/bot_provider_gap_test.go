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
)

// Bot recommendations and the star referral program require an authoritative
// bot business provider. Keep their Layer 229 boundary fail-closed when neither
// a published control-plane result nor a signed provider is available.
func TestBotProviderGapsFailClosedWithoutPersistence(t *testing.T) {
	oldStore := persist.Default
	store := &botProviderGapStoreProbe{}
	persist.Use(store)
	t.Cleanup(func() { persist.Use(oldStore) })

	const (
		callerID   = int64(81019001)
		botID      = int64(81019002)
		accessHash = int64(81019003)
	)
	client := &miniBotAppsUserClient{users: map[int64]*mtproto.ImmutableUser{
		botID: miniBotProfile(botID, accessHash, true),
	}}
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
		MD:     &metadata.RpcMetadata{UserId: callerID},
	}
	bot := &mtproto.InputUser{PredicateName: mtproto.Predicate_inputUser, UserId: botID, AccessHash: accessHash}
	calls := []struct {
		name string
		call func() error
	}{
		{"bot recommendations", func() error {
			_, err := c.BotsGetBotRecommendations(&mtproto.TLBotsGetBotRecommendations{Bot: bot})
			return err
		}},
		{"star referral program", func() error {
			_, err := c.BotsUpdateStarRefProgram(&mtproto.TLBotsUpdateStarRefProgram{Bot: bot})
			return err
		}},
	}
	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); !errors.Is(err, mtproto.ErrMethodNotImpl) {
				t.Fatalf("got %v, want METHOD_NOT_IMPL", err)
			}
		})
	}
	if store.reads != 0 || store.writes != 0 {
		t.Fatalf("provider gaps touched persistence: reads=%d writes=%d", store.reads, store.writes)
	}
}

func TestBotProviderGapsAuthenticateBeforeProviderCheck(t *testing.T) {
	c := &ApiFullCore{}
	calls := []struct {
		name string
		call func() error
	}{
		{"bot recommendations", func() error {
			_, err := c.BotsGetBotRecommendations(nil)
			return err
		}},
		{"bot custom verification", func() error {
			_, err := c.BotsSetCustomVerification(nil)
			return err
		}},
		{"bot emoji status", func() error {
			_, err := c.BotsUpdateUserEmojiStatus(nil)
			return err
		}},
		{"bot emoji permission", func() error {
			_, err := c.BotsToggleUserEmojiStatusPermission(nil)
			return err
		}},
		{"star referral program", func() error {
			_, err := c.BotsUpdateStarRefProgram(nil)
			return err
		}},
	}
	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
				t.Fatalf("got %v, want AUTH_KEY_UNREGISTERED", err)
			}
		})
	}
}

type botProviderGapStoreProbe struct {
	reads  int
	writes int
}

func (s *botProviderGapStoreProbe) Get(string) (string, error) {
	s.reads++
	return "", nil
}

func (s *botProviderGapStoreProbe) Set(string, string) error {
	s.writes++
	return nil
}
