package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/svc"
	verification "github.com/teamgram/teamgram-server/pkg/code"
)

func TestVerifyPassportEmailDoesNotConsumeChallengeForWrongEmail(t *testing.T) {
	store := &phoneChallengeStore{}
	service := verification.NewChallengeService(store, verification.ChallengeSettings{
		TTL: time.Minute, RateLimit: 5, RateWindow: time.Minute, MaxAttempts: 3,
	}, phoneChallengeProvider{}, phoneChallengeProvider{})
	c := &AuthorizationCore{
		ctx:    context.Background(),
		MD:     &metadata.RpcMetadata{PermAuthKeyId: 91},
		svcCtx: &svc.ServiceContext{Challenges: service},
	}
	const code = "123456"
	challengeID := verification.PurposeID(mtproto.Predicate_emailVerifyPurposePassport, "", "")
	if _, err := service.Issue(context.Background(), verification.IssueRequest{
		Channel: verification.ChannelEmail, Purpose: challengePurposeVerifyEmail,
		Subject: "right@example.com", Scope: c.challengeScope(), ChallengeID: challengeID, Code: code,
	}); err != nil {
		t.Fatal(err)
	}

	if result, err := c.AccountVerifyEmailECBA39DB(&mtproto.TLAccountVerifyEmailECBA39DB{Email: "wrong@example.com", Code: code}); result != nil || !errors.Is(err, mtproto.ErrCodeInvalid) {
		t.Fatalf("wrong email = (%v, %v), want CODE_INVALID", result, err)
	}
	if result, err := c.AccountVerifyEmailECBA39DB(&mtproto.TLAccountVerifyEmailECBA39DB{Email: "right@example.com", Code: code}); result != mtproto.BoolTrue || err != nil {
		t.Fatalf("correct email = (%v, %v), want TRUE", result, err)
	}
}

func TestEmailAndRecoveryHandlersRejectNilRequests(t *testing.T) {
	c := &AuthorizationCore{}
	if result, err := c.AccountSendVerifyEmailCode(nil); result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("send email nil = (%v, %v), want INPUT_REQUEST_INVALID", result, err)
	}
	if result, err := c.AccountVerifyEmail32DA4CF(nil); result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("verify email purpose nil = (%v, %v), want INPUT_REQUEST_INVALID", result, err)
	}
	if result, err := c.AccountVerifyEmailECBA39DB(nil); result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("verify email passport nil = (%v, %v), want INPUT_REQUEST_INVALID", result, err)
	}
	if result, err := c.AuthCheckRecoveryPassword(nil); result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("check recovery nil = (%v, %v), want INPUT_REQUEST_INVALID", result, err)
	}
	if result, err := c.AuthRecoverPassword(nil); result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("recover password nil = (%v, %v), want INPUT_REQUEST_INVALID", result, err)
	}
}

func TestEmailChallengeHandlersRejectMissingAuthKey(t *testing.T) {
	c := &AuthorizationCore{}
	purpose := mtproto.MakeTLEmailVerifyPurposePassport(nil).To_EmailVerifyPurpose()
	verificationInput := mtproto.MakeTLEmailVerificationCode(nil).To_EmailVerification()

	if result, err := c.AccountSendVerifyEmailCode(&mtproto.TLAccountSendVerifyEmailCode{
		Purpose: purpose,
		Email:   "user@example.com",
	}); result != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("send email without auth key = (%v, %v), want AUTH_KEY_UNREGISTERED", result, err)
	}
	if result, err := c.AccountVerifyEmail32DA4CF(&mtproto.TLAccountVerifyEmail32DA4CF{
		Purpose:      purpose,
		Verification: verificationInput,
	}); result != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("verify email purpose without auth key = (%v, %v), want AUTH_KEY_UNREGISTERED", result, err)
	}
	if result, err := c.AccountVerifyEmailECBA39DB(&mtproto.TLAccountVerifyEmailECBA39DB{
		Email: "user@example.com",
		Code:  "123456",
	}); result != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("verify passport email without auth key = (%v, %v), want AUTH_KEY_UNREGISTERED", result, err)
	}
}
