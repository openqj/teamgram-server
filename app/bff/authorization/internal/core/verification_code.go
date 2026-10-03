package core

import (
	"errors"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/authorization/model"
	verification "github.com/teamgram/teamgram-server/pkg/code"
)

const (
	challengePurposeAuthLogin        = "auth.login"
	challengePurposeVerifyEmail      = "account.verify_email"
	challengePurposePasswordRecovery = "auth.password_recovery"
)

func (c *AuthorizationCore) challengeScope() string {
	if c == nil || c.MD == nil {
		return ""
	}
	return verification.ScopeID(c.MD.PermAuthKeyId)
}

func (c *AuthorizationCore) issuePhoneChallenge(data *model.PhoneCodeTransaction, channel verification.Channel, purpose, requestedCode string) (string, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Challenges == nil || c.MD == nil || data == nil {
		return "", mtproto.ErrSmsCodeCreateFailed
	}
	ttl := time.Until(time.Unix(int64(data.PhoneCodeExpired), 0))
	issued, err := c.svcCtx.Challenges.Issue(c.ctx, verification.IssueRequest{
		Channel: channel, Purpose: purpose, Subject: data.PhoneNumber,
		Scope: c.challengeScope(), ChallengeID: data.PhoneCodeHash,
		Code: requestedCode, CodeLength: 5, TTL: ttl,
	})
	if err != nil {
		return "", mapPhoneIssueError(err)
	}
	data.PhoneCode = ""
	data.PhoneCodeLength = len(issued.Code)
	data.PhoneCodeExtraData = ""
	data.PhoneCodeExpired = int32(issued.ExpiresAt.Unix())
	return issued.Code, nil
}

func (c *AuthorizationCore) consumePhoneChallenge(phone, id, value, purpose string) error {
	if c == nil || c.svcCtx == nil || c.svcCtx.Challenges == nil {
		return mtproto.ErrPhoneCodeExpired
	}
	normalizedPhone := normalizeChallengePhone(phone)
	err := c.checkAndConsumePhoneChallenge(normalizedPhone, id, value, purpose, verification.ChannelSMS)
	if errors.Is(err, verification.ErrChallengeNotFound) || errors.Is(err, verification.ErrChallengeExpired) {
		// App delivery uses the same challenge identity and storage.
		err = c.checkAndConsumePhoneChallenge(normalizedPhone, id, value, purpose, verification.ChannelApp)
	}
	switch {
	case err == nil:
		return nil
	case errors.Is(err, verification.ErrChallengeNotFound), errors.Is(err, verification.ErrChallengeExpired):
		return mtproto.ErrPhoneCodeExpired
	case errors.Is(err, verification.ErrChallengeInvalid):
		return mtproto.ErrPhoneCodeInvalid
	default:
		return mtproto.ErrInternalServerError
	}
}

func (c *AuthorizationCore) consumeEmailLoginChallenge(phone, id, value string) error {
	if c == nil || c.svcCtx == nil || c.svcCtx.Challenges == nil {
		return mtproto.ErrEmailVerifyExpired
	}
	request := verification.VerifyRequest{
		Channel: verification.ChannelEmail, Purpose: challengePurposeVerifyEmail,
		Scope:       c.challengeScope(),
		ChallengeID: verification.PurposeID(mtproto.Predicate_emailVerifyPurposeLoginSetup, phone, id),
		Code:        value,
	}
	if _, err := c.svcCtx.Challenges.Consume(c.ctx, request); err != nil {
		return mapEmailChallengeError(err)
	}
	return nil
}

func (c *AuthorizationCore) checkAndConsumePhoneChallenge(phone, id, value, purpose string, channel verification.Channel) error {
	if c == nil || c.svcCtx == nil || c.svcCtx.Challenges == nil || c.MD == nil {
		return verification.ErrChallengeNotFound
	}
	request := verification.VerifyRequest{
		Channel: channel, Purpose: purpose, Scope: c.challengeScope(),
		ChallengeID: id, Code: value,
	}
	challenge, err := c.svcCtx.Challenges.Check(c.ctx, request)
	if err != nil {
		return err
	}
	if challenge == nil || normalizeChallengePhone(challenge.Subject) != phone {
		return verification.ErrChallengeInvalid
	}
	_, err = c.svcCtx.Challenges.Consume(c.ctx, request)
	return err
}

func normalizeChallengePhone(phone string) string {
	phone = strings.TrimSpace(phone)
	if _, normalized, err := checkPhoneNumberInvalid(phone); err == nil {
		return normalized
	}
	return strings.ReplaceAll(phone, " ", "")
}

func mapPhoneIssueError(err error) error {
	switch {
	case errors.Is(err, verification.ErrRateLimited):
		return mtproto.ErrPhoneNumberFlood
	case verification.ProviderError(err):
		return mtproto.ErrSmsCodeCreateFailed
	default:
		return mtproto.ErrInternalServerError
	}
}

func emailPurposeID(purpose *mtproto.EmailVerifyPurpose) string {
	if purpose == nil {
		return ""
	}
	return verification.PurposeID(
		purpose.GetPredicateName(),
		purpose.GetPhoneNumber(),
		purpose.GetPhoneCodeHash(),
	)
}

func mapEmailChallengeError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, verification.ErrChallengeInvalid):
		return mtproto.ErrCodeInvalid
	case errors.Is(err, verification.ErrChallengeNotFound), errors.Is(err, verification.ErrChallengeExpired):
		return mtproto.ErrEmailVerifyExpired
	case errors.Is(err, verification.ErrRateLimited), verification.ProviderError(err):
		return mtproto.ErrSendCodeUnavailable
	default:
		return mtproto.ErrInternalServerError
	}
}
