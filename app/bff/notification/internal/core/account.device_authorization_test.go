package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
)

type deviceAuthorizationStore struct {
	values map[string]string
	writes int
}

func (s *deviceAuthorizationStore) Get(key string) (string, error) {
	return s.values[key], nil
}

func (s *deviceAuthorizationStore) Set(key, value string) error {
	s.values[key] = value
	s.writes++
	return nil
}

func TestAccountDeviceMethodsRejectOtherAccountWithoutWrites(t *testing.T) {
	const (
		callerID = int64(81001)
		otherID  = int64(81002)
		token    = "device-token"
	)

	tests := []struct {
		name string
		call func(*NotificationCore) (*mtproto.Bool, error)
	}{
		{
			name: "register",
			call: func(c *NotificationCore) (*mtproto.Bool, error) {
				return c.AccountRegisterDevice(&mtproto.TLAccountRegisterDevice{
					TokenType: 10,
					Token:     token,
					OtherUids: []int64{callerID, otherID},
				})
			},
		},
		{
			name: "unregister",
			call: func(c *NotificationCore) (*mtproto.Bool, error) {
				return c.AccountUnregisterDevice(&mtproto.TLAccountUnregisterDevice{
					TokenType: 10,
					Token:     token,
					OtherUids: []int64{callerID, otherID},
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callerKey := deviceStoreKey(callerID, 10, token)
			otherKey := deviceStoreKey(otherID, 10, token)
			store := &deviceAuthorizationStore{values: map[string]string{
				callerKey: "caller registration",
				otherKey:  "other registration",
			}}
			previous := persist.Default
			persist.Default = store
			t.Cleanup(func() { persist.Default = previous })

			c := &NotificationCore{MD: &metadata.RpcMetadata{UserId: callerID}}
			got, err := tt.call(c)
			if !errors.Is(err, mtproto.ErrUserIdInvalid) {
				t.Fatalf("device method error = %v, want USER_ID_INVALID", err)
			}
			if got != nil {
				t.Fatalf("device method result = %v, want nil", got)
			}
			if store.writes != 0 {
				t.Fatalf("store writes = %d, want 0", store.writes)
			}
			if store.values[callerKey] != "caller registration" {
				t.Fatalf("caller registration changed to %q", store.values[callerKey])
			}
			if store.values[otherKey] != "other registration" {
				t.Fatalf("other registration changed to %q", store.values[otherKey])
			}
		})
	}
}

func TestAccountDeviceMethodsRequireAuthenticatedUser(t *testing.T) {
	callers := []struct {
		name string
		md   *metadata.RpcMetadata
	}{
		{name: "missing metadata"},
		{name: "zero user", md: &metadata.RpcMetadata{}},
	}
	methods := []struct {
		name string
		call func(*NotificationCore) (*mtproto.Bool, error)
	}{
		{
			name: "register",
			call: func(c *NotificationCore) (*mtproto.Bool, error) {
				return c.AccountRegisterDevice(&mtproto.TLAccountRegisterDevice{Token: "device-token"})
			},
		},
		{
			name: "unregister",
			call: func(c *NotificationCore) (*mtproto.Bool, error) {
				return c.AccountUnregisterDevice(&mtproto.TLAccountUnregisterDevice{Token: "device-token"})
			},
		},
	}

	for _, method := range methods {
		for _, caller := range callers {
			t.Run(method.name+"/"+caller.name, func(t *testing.T) {
				store := &deviceAuthorizationStore{values: make(map[string]string)}
				previous := persist.Default
				persist.Default = store
				t.Cleanup(func() { persist.Default = previous })

				c := &NotificationCore{MD: caller.md}
				got, err := method.call(c)
				if !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
					t.Fatalf("device method error = %v, want AUTH_KEY_UNREGISTERED", err)
				}
				if got != nil || store.writes != 0 {
					t.Fatalf("device method result = %v, writes = %d; want nil and 0 writes", got, store.writes)
				}
			})
		}
	}
}
