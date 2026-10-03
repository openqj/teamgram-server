package twofa

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/crypto"
)

type memoryProofStore struct {
	mu     sync.Mutex
	values map[string]string
}

func (s *memoryProofStore) Get(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[key], nil
}

func (s *memoryProofStore) CompareAndDelete(key, expected string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values[key] != expected {
		return false, nil
	}
	delete(s.values, key)
	return true, nil
}

func TestVerifyPasswordProofAndRejectReplay(t *testing.T) {
	state, challenge, proof := makeSRPTestProof(t, time.Now().Add(time.Minute))
	store := makeMemorySRPStore(t, challenge, state)

	if err := VerifyPassword(store, 42, state, proof); err != nil {
		t.Fatalf("valid proof rejected: %v", err)
	}
	if err := VerifyPassword(store, 42, state, proof); !errors.Is(err, mtproto.ErrPasswordHashInvalid) {
		t.Fatalf("replayed proof error = %v, want PASSWORD_HASH_INVALID", err)
	}
}

func TestVerifyPasswordRejectsWrongProof(t *testing.T) {
	state, challenge, proof := makeSRPTestProof(t, time.Now().Add(time.Minute))
	proof.M1[0] ^= 0xff
	store := makeMemorySRPStore(t, challenge, state)

	if err := VerifyPassword(store, 42, state, proof); !errors.Is(err, mtproto.ErrPasswordHashInvalid) {
		t.Fatalf("wrong proof error = %v, want PASSWORD_HASH_INVALID", err)
	}
}

func TestVerifyPasswordRejectsExpiredChallenge(t *testing.T) {
	state, challenge, proof := makeSRPTestProof(t, time.Now().Add(-time.Minute))
	store := makeMemorySRPStore(t, challenge, state)

	if err := VerifyPassword(store, 42, state, proof); !errors.Is(err, mtproto.ErrPasswordHashInvalid) {
		t.Fatalf("expired challenge error = %v, want PASSWORD_HASH_INVALID", err)
	}
}

func makeSRPTestProof(t *testing.T, expires time.Time) (PasswordState, SRPChallenge, *mtproto.InputCheckPasswordSRP) {
	t.Helper()
	algo := LegacyAlgo()
	util := crypto.MakeSRPUtil(&crypto.PasswordKdfAlgoModPow{
		Salt1: algo.Salt1,
		Salt2: algo.Salt2,
		G:     algo.G,
		P:     algo.P,
	})
	verifier := util.GetVBytes(algo.Salt1, []byte("correct horse battery staple"))
	bNonce, b := util.CalcSRPB(verifier)
	challenge := SRPChallenge{SrpID: 103, BNonce: bNonce, B: b, ExpiresAt: expires.Unix()}
	x := util.GetX(algo.Salt1, []byte("correct horse battery staple"))
	a, m1 := util.CalcClientM(algo.Salt1, x, b)
	if len(a) == 0 || len(m1) == 0 {
		t.Fatal("failed to construct test SRP proof")
	}
	state := PasswordState{Secret: verifier, Algo: &algo}
	proof := &mtproto.InputCheckPasswordSRP{SrpId: challenge.SrpID, A: a, M1: m1}
	return state, challenge, proof
}

func makeMemorySRPStore(t *testing.T, challenge SRPChallenge, state PasswordState) *memoryProofStore {
	t.Helper()
	challengeJSON, err := json.Marshal(challenge)
	if err != nil {
		t.Fatal(err)
	}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	return &memoryProofStore{values: map[string]string{
		ChallengeKey(42, challenge.SrpID): string(challengeJSON),
		PasswordKey(42):                   string(stateJSON),
	}}
}
