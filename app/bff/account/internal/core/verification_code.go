package core

import (
	"errors"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/authorization/model"
	verification "github.com/teamgram/teamgram-server/pkg/code"
)

const (
	challengePurposeChangePhone  = "account.change_phone"
	challengePurposeConfirmPhone = "account.confirm_phone"
)

func (c *AccountCore) challengeScope() string {
	if c == nil || c.MD == nil {
		return ""
	}
	return verification.ScopeID(c.MD.PermAuthKeyId)
}

func (c *AccountCore) issueSMSChallenge(data *model.PhoneCodeTransaction, purpose string) error {
	if c == nil || c.svcCtx == nil || c.svcCtx.Challenges == nil || c.MD == nil || data == nil {
		return mtproto.ErrSmsCodeCreateFailed
	}
	issued, err := c.svcCtx.Challenges.Issue(c.ctx, verification.IssueRequest{
		Channel: verification.ChannelSMS, Purpose: purpose, Subject: data.PhoneNumber,
		Scope: c.challengeScope(), ChallengeID: data.PhoneCodeHash,
		CodeLength: 5, TTL: time.Until(time.Unix(int64(data.PhoneCodeExpired), 0)),
	})
	if err != nil {
		switch {
		case errors.Is(err, verification.ErrRateLimited):
			return mtproto.ErrPhoneNumberFlood
		case verification.ProviderError(err):
			return mtproto.ErrSmsCodeCreateFailed
		default:
			return mtproto.ErrInternalServerError
		}
	}
	data.PhoneCode = ""
	data.PhoneCodeLength = len(issued.Code)
	data.PhoneCodeExtraData = ""
	data.PhoneCodeExpired = int32(issued.ExpiresAt.Unix())
	data.SentCodeType = model.SentCodeTypeSms
	data.NextCodeType = model.CodeTypeNone
	data.State = model.CodeStateSent
	return nil
}

func (c *AccountCore) consumeSMSChallenge(id, value, purpose string) error {
	if c == nil || c.svcCtx == nil || c.svcCtx.Challenges == nil || c.MD == nil {
		return mtproto.ErrPhoneCodeExpired
	}
	_, err := c.svcCtx.Challenges.Consume(c.ctx, verification.VerifyRequest{
		Channel: verification.ChannelSMS, Purpose: purpose, Scope: c.challengeScope(),
		ChallengeID: id, Code: value,
	})
	switch {
	case err == nil:
		return nil
	case errors.Is(err, verification.ErrChallengeInvalid):
		return mtproto.ErrPhoneCodeInvalid
	case errors.Is(err, verification.ErrChallengeNotFound), errors.Is(err, verification.ErrChallengeExpired):
		return mtproto.ErrPhoneCodeExpired
	default:
		return mtproto.ErrInternalServerError
	}
}
