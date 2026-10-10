package core

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	mtcrypto "github.com/teamgram/proto/mtproto/crypto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/pkg/twofa"
)

func TestAccountGetTmpPasswordPostgresRoundTrip(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	if err := domain.OpenPostgresReadOnly(dsn); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = domain.Close() })
	userID := time.Now().UnixNano()
	password := []byte("temporary-password-fixture")
	algo := twofa.LegacyAlgo()
	util := mtcrypto.MakeSRPUtil(&mtcrypto.PasswordKdfAlgoModPow{
		Salt1: algo.Salt1, Salt2: algo.Salt2, G: algo.G, P: algo.P,
	})
	state := twofa.PasswordState{Secret: util.GetVBytes(algo.Salt1, password), Algo: &algo}
	stateRaw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err = persist.Default.Set(twofa.PasswordKey(userID), string(stateRaw)); err != nil {
		t.Fatal(err)
	}
	challenge, err := createSRPChallenge(userID, acctPassword(state), algo)
	if err != nil {
		t.Fatal(err)
	}
	challengeRaw, err := json.Marshal(challenge)
	if err != nil {
		t.Fatal(err)
	}
	if err = persist.Default.Set(twofa.ChallengeKey(userID, challenge.SrpID), string(challengeRaw)); err != nil {
		t.Fatal(err)
	}
	clientX := util.GetX(algo.Salt1, password)
	a, m1 := util.CalcClientM(algo.Salt1, clientX, challenge.B)
	core := &ApiFullCore{ctx: context.Background(), MD: &metadata.RpcMetadata{UserId: userID}}
	result, err := core.AccountGetTmpPassword(&mtproto.TLAccountGetTmpPassword{
		Password: &mtproto.InputCheckPasswordSRP{SrpId: challenge.SrpID, A: a, M1: m1},
		Period:   300,
	})
	if err != nil || result == nil || len(result.GetTmpPassword()) != 32 || result.GetValidUntil() <= int32(time.Now().Unix()) {
		t.Fatalf("temporary password = %#v err=%v", result, err)
	}
	ok, err := domain.ValidateTmpPassword(context.Background(), userID, result.GetTmpPassword())
	if err != nil || !ok {
		t.Fatalf("issued token validation: ok=%v err=%v", ok, err)
	}
	if ok, err = domain.ValidateTmpPassword(context.Background(), userID, []byte("wrong")); err != nil || ok {
		t.Fatalf("wrong token validation: ok=%v err=%v", ok, err)
	}
	if err = domain.StoreTmpPassword(context.Background(), userID, result.GetTmpPassword(), time.Now().Add(time.Minute).Unix()); err != nil {
		t.Fatal(err)
	}
	if err = persist.Default.Set(twofa.PasswordKey(userID), ""); err != nil {
		t.Fatal(err)
	}
}
