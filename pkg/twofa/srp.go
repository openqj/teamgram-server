package twofa

import (
	"bytes"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/crypto"
)

type PasswordAlgo struct {
	Salt1 []byte `json:"salt1"`
	Salt2 []byte `json:"salt2"`
	G     int32  `json:"g"`
	P     []byte `json:"p"`
}

type PasswordState struct {
	Hint             string        `json:"hint,omitempty"`
	Email            string        `json:"email,omitempty"`
	EmailPending     bool          `json:"email_pending,omitempty"`
	EmailCode        string        `json:"email_code,omitempty"`
	RecoveryCode     string        `json:"recovery_code,omitempty"`
	RecoveryExpire   int64         `json:"recovery_expire,omitempty"`
	HasPassword      bool          `json:"has_password,omitempty"`
	ResetDeclined    bool          `json:"reset_declined,omitempty"`
	ResetRequestedAt int64         `json:"reset_requested_at,omitempty"`
	ResetUntilDate   int64         `json:"reset_until_date,omitempty"`
	ResetRetryDate   int64         `json:"reset_retry_date,omitempty"`
	Secret           []byte        `json:"secret,omitempty"`
	Algo             *PasswordAlgo `json:"algo,omitempty"`
}

type SRPChallenge struct {
	SrpID     int64  `json:"srp_id"`
	BNonce    []byte `json:"b_nonce"`
	B         []byte `json:"b"`
	ExpiresAt int64  `json:"expires_at"`
}

type ProofStore interface {
	Get(key string) (string, error)
	CompareAndDelete(key, expected string) (bool, error)
}

const supportedPrimeHex = "c71caeb9c6b1c9048e6c522f70f13f73980d40238e3e21c14934d037563d930f48198a0aa7c14058229493d22530f4dbfa336f6e0ac925139543aed44cce7c3720fd51f69458705ac68cd4fe6b6b13abdc9746512969328454f18faf8c595f642477fe96bb2a941d5bcd1d4ac8cc49880708fa9b378e3c4f3a9060bee67cf9a4a4a695811051907e162753b56b0f6b410dba74d8a84b2a14b3144e0ef1284754fd17ed950d5965b4b9dd46582db1178d169c6bc465b0d6ff9ca3928fef5b9ae4e418fc15e83ebea0f87fa9ff5eed70050ded2849f47bf959d956850ce929851f0d8115f635b105ee2e4e15d04b2454bf6f4fadf034b10403119cd8e3b92fcc5b"

func SupportedPrime() []byte {
	p, _ := hex.DecodeString(supportedPrimeHex)
	return p
}

func LegacyAlgo() PasswordAlgo {
	return PasswordAlgo{
		Salt1: []byte{0xEC, 0xF8, 0x73, 0x76, 0x65, 0xBC, 0x77, 0x5A},
		Salt2: []byte{0xBE, 0xDE, 0x48, 0x88, 0x8C, 0x0F, 0x42, 0xAC, 0x34, 0xFF, 0xD1, 0xD4, 0x93, 0x5D, 0x8B, 0x21},
		G:     3,
		P:     SupportedPrime(),
	}
}

func PasswordKey(userID int64) string {
	return "acct:" + strconv.FormatInt(userID, 10) + ":password"
}

func ChallengeKey(userID, srpID int64) string {
	return "acct:" + strconv.FormatInt(userID, 10) + ":srp:" + strconv.FormatInt(srpID, 10)
}

func ValidAlgo(algo PasswordAlgo) bool {
	return len(algo.Salt1) >= 8 && len(algo.Salt2) >= 8 && algo.G == 3 && bytes.Equal(algo.P, SupportedPrime())
}

func VerifySRP(algo PasswordAlgo, verifier []byte, challenge SRPChallenge, in *mtproto.InputCheckPasswordSRP) bool {
	if in == nil || !ValidAlgo(algo) || len(verifier) == 0 || in.SrpId != challenge.SrpID || len(in.A) == 0 || len(in.M1) != 32 || len(challenge.BNonce) == 0 || len(challenge.B) == 0 {
		return false
	}
	util := crypto.MakeSRPUtil(&crypto.PasswordKdfAlgoModPow{
		Salt1: algo.Salt1,
		Salt2: algo.Salt2,
		G:     algo.G,
		P:     algo.P,
	})
	expected := util.CalcM(algo.Salt1, verifier, in.A, challenge.BNonce, challenge.B)
	return len(expected) == len(in.M1) && subtle.ConstantTimeCompare(expected, in.M1) == 1
}

func LoadPasswordState(store interface{ Get(string) (string, error) }, userID int64) (PasswordState, error) {
	var state PasswordState
	raw, err := store.Get(PasswordKey(userID))
	if err != nil || raw == "" {
		return state, err
	}
	err = json.Unmarshal([]byte(raw), &state)
	return state, err
}

func VerifyPassword(store ProofStore, userID int64, state PasswordState, in *mtproto.InputCheckPasswordSRP) error {
	if len(state.Secret) == 0 {
		if in != nil && (in.SrpId != 0 || len(in.A) > 0 || len(in.M1) > 0) {
			return mtproto.ErrPasswordHashInvalid
		}
		return nil
	}
	if store == nil || in == nil || in.SrpId == 0 {
		return mtproto.ErrPasswordHashInvalid
	}
	key := ChallengeKey(userID, in.SrpId)
	raw, err := store.Get(key)
	if err != nil {
		return err
	}
	if raw == "" {
		return mtproto.ErrPasswordHashInvalid
	}
	var challenge SRPChallenge
	if err := json.Unmarshal([]byte(raw), &challenge); err != nil {
		return mtproto.ErrPasswordHashInvalid
	}
	consumed, err := store.CompareAndDelete(key, raw)
	if err != nil {
		return err
	}
	if !consumed || time.Now().Unix() > challenge.ExpiresAt {
		return mtproto.ErrPasswordHashInvalid
	}
	if !VerifySRP(passwordAlgoForState(state), state.Secret, challenge, in) {
		return mtproto.ErrPasswordHashInvalid
	}
	return nil
}

func passwordAlgoForState(state PasswordState) PasswordAlgo {
	if state.Algo == nil {
		return LegacyAlgo()
	}
	return *state.Algo
}
