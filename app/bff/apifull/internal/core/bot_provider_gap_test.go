package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// These methods need a bot business/provider backend that is not part of the
// APIFull service yet. Keep their Layer 229 boundary fail-closed until one is
// available: returning a synthetic success would make clients believe that a
// bot setting or recommendation was applied.
func TestBotProviderGapsFailClosedWithoutPersistence(t *testing.T) {
	oldStore := persist.Default
	store := &botProviderGapStoreProbe{}
	persist.Use(store)
	t.Cleanup(func() { persist.Use(oldStore) })

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81019001}}
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
