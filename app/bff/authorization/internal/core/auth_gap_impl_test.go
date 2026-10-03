package core

import (
	"encoding/json"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/twofa"
)

func TestApplyRecoveredPasswordStoresVerifier(t *testing.T) {
	algo := twofa.LegacyAlgo()
	state := acctPasswordState{
		Email:            "recovery@example.test",
		RecoveryCode:     "12345",
		RecoveryExpire:   10,
		ResetDeclined:    true,
		ResetRequestedAt: 100,
		ResetUntilDate:   200,
		ResetRetryDate:   300,
	}
	verifier := []byte("stored-client-verifier")
	settings := &mtproto.Account_PasswordInputSettings{
		NewAlgo: mtproto.MakeTLPasswordKdfAlgoModPow(&mtproto.PasswordKdfAlgo{
			Salt1: algo.Salt1,
			Salt2: algo.Salt2,
			G:     algo.G,
			P:     algo.P,
		}).To_PasswordKdfAlgo(),
		NewPasswordHash: verifier,
	}

	if err := applyRecoveredPassword(&state, settings); err != nil {
		t.Fatal(err)
	}
	if !state.HasPassword || string(state.Secret) != string(verifier) || state.Algo == nil {
		t.Fatalf("recovered verifier not stored: %+v", state)
	}
	if state.RecoveryCode != "" || state.RecoveryExpire != 0 {
		t.Fatalf("recovery code not consumed: %+v", state)
	}
	if state.ResetDeclined || state.ResetRequestedAt != 0 || state.ResetUntilDate != 0 || state.ResetRetryDate != 0 {
		t.Fatalf("password reset wait survived password replacement: %+v", state)
	}

	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var loaded twofa.PasswordState
	if err = json.Unmarshal(encoded, &loaded); err != nil {
		t.Fatal(err)
	}
	if string(loaded.Secret) != string(verifier) || loaded.Algo == nil || loaded.Email != state.Email {
		t.Fatalf("serialized password state lost fields: %+v", loaded)
	}
}

func TestApplyRecoveredPasswordWithoutSettingsClearsVerifier(t *testing.T) {
	state := acctPasswordState{
		HasPassword: true,
		Secret:      []byte("verifier"),
		Algo:        &twofa.PasswordAlgo{Salt1: []byte("salt1234")},
	}
	if err := applyRecoveredPassword(&state, nil); err != nil {
		t.Fatal(err)
	}
	if state.HasPassword || len(state.Secret) != 0 || state.Algo != nil {
		t.Fatalf("password verifier retained after password removal: %+v", state)
	}
}
