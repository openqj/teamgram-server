package core

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/pkg/twofa"
	"github.com/zeromicro/go-zero/core/logx"
)

type memoryPasswordStateStore struct {
	values map[string]string
}

func (s *memoryPasswordStateStore) Get(key string) (string, error) {
	return s.values[key], nil
}

func (s *memoryPasswordStateStore) Set(key, value string) error {
	s.values[key] = value
	return nil
}

func TestAccountUpdatePasswordSettingsClearsResetWait(t *testing.T) {
	const userID = int64(712348)
	store := &memoryPasswordStateStore{values: make(map[string]string)}
	state := twofa.PasswordState{
		ResetDeclined:    true,
		ResetRequestedAt: 100,
		ResetUntilDate:   100 + 7*24*60*60,
		ResetRetryDate:   200 + 7*24*60*60,
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	store.values[twofa.PasswordKey(userID)] = string(encoded)

	oldStore := persist.Default
	persist.Default = store
	t.Cleanup(func() { persist.Default = oldStore })

	ctx := context.Background()
	c := &ApiFullCore{
		ctx:    ctx,
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: userID},
	}
	algo := twofa.LegacyAlgo()
	settings := &mtproto.Account_PasswordInputSettings{
		NewAlgo: mtproto.MakeTLPasswordKdfAlgoModPow(&mtproto.PasswordKdfAlgo{
			Salt1: algo.Salt1,
			Salt2: algo.Salt2,
			G:     algo.G,
			P:     algo.P,
		}).To_PasswordKdfAlgo(),
		NewPasswordHash: []byte("new-password-verifier"),
	}
	if _, err = c.AccountUpdatePasswordSettings(&mtproto.TLAccountUpdatePasswordSettings{
		NewSettings: settings,
	}); err != nil {
		t.Fatal(err)
	}

	var saved twofa.PasswordState
	if err = json.Unmarshal([]byte(store.values[twofa.PasswordKey(userID)]), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.ResetDeclined || saved.ResetRequestedAt != 0 || saved.ResetUntilDate != 0 || saved.ResetRetryDate != 0 {
		t.Fatalf("reset wait survived password change: %+v", saved)
	}
}
