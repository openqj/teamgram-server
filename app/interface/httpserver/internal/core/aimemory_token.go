// Copyright 2026 The Teamgram Authors.
//
// Product Identity is the account authority for AIMemory. This communication
// gateway validates a real, encrypted MTProto request before resolving its
// server-side session and asking Identity for a bound product token; it never
// accepts a user id from the HTTP client or signs an account token itself.

package core

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/crypto"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
)

var (
	ErrAIMemoryTokenProof    = errors.New("invalid encrypted MTProto proof")
	ErrAIMemoryTokenReplay   = errors.New("encrypted MTProto proof has already been used")
	ErrAIMemoryTokenIdentity = errors.New("authenticated Teamgram user was not found")
	ErrAIMemoryTokenService  = errors.New("AIMemory token service is unavailable")
)

const (
	minimumEncryptedMTProtoRequest = 8 + 16 + 16
	maximumAIMemoryProofBytes      = 1 << 20
)

// IssueAIMemoryToken exchanges a proof made with an already authenticated
// MTProto session for a short-lived AIMemory JWT. The proof is deliberately a
// normal encrypted ping, so no new TL constructor or client-facing user-id
// parameter is needed.
func (c *HttpserverCore) IssueAIMemoryToken(in *mtproto.MTPRawMessage) (string, int64, error) {
	if c == nil || c.svcCtx == nil {
		return "", 0, ErrAIMemoryTokenService
	}
	proof, err := c.validateEncryptedPing(in)
	if err != nil {
		return "", 0, err
	}
	if c.svcCtx.IdentityReplayStore == nil {
		return "", 0, fmt.Errorf("%w: proof replay store is not configured", ErrAIMemoryTokenService)
	}
	ttl := time.Duration(c.svcCtx.Config.IdentityReplayTTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	claimed, err := c.svcCtx.IdentityReplayStore.Claim(c.ctx, proof.AuthKeyID, proof.PingID, ttl)
	if err != nil {
		return "", 0, fmt.Errorf("%w: proof replay store: %v", ErrAIMemoryTokenService, err)
	}
	if !claimed {
		return "", 0, ErrAIMemoryTokenReplay
	}

	if c.svcCtx.AuthsessionClient == nil {
		return "", 0, fmt.Errorf("%w: authsession client is not configured", ErrAIMemoryTokenIdentity)
	}
	userId, err := c.svcCtx.AuthsessionClient.AuthsessionGetUserId(c.ctx, &authsession.TLAuthsessionGetUserId{
		AuthKeyId: proof.AuthKeyID,
	})
	if err != nil || userId == nil || userId.GetV() <= 0 {
		return "", 0, fmt.Errorf("%w: auth key %d", ErrAIMemoryTokenIdentity, proof.AuthKeyID)
	}

	if c.svcCtx.AIMemoryTokenExchanger == nil {
		return "", 0, fmt.Errorf("%w: Identity token exchanger is not configured", ErrAIMemoryTokenService)
	}
	token, expiresAt, err := c.svcCtx.AIMemoryTokenExchanger.Exchange(c.ctx, userId.GetV())
	if err != nil {
		if errors.Is(err, svc.ErrIdentityTokenNotFound) {
			return "", 0, fmt.Errorf("%w: %v", ErrAIMemoryTokenIdentity, err)
		}
		return "", 0, fmt.Errorf("%w: %v", ErrAIMemoryTokenService, err)
	}
	return token, expiresAt, nil
}

type encryptedPingProof struct {
	AuthKeyID int64
	PingID    int64
}

func (c *HttpserverCore) validateEncryptedPing(in *mtproto.MTPRawMessage) (encryptedPingProof, error) {
	if in == nil {
		return encryptedPingProof{}, ErrAIMemoryTokenProof
	}
	if in.AuthKeyId() == 0 || len(in.Payload) < minimumEncryptedMTProtoRequest || len(in.Payload) > maximumAIMemoryProofBytes {
		return encryptedPingProof{}, ErrAIMemoryTokenProof
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil {
		return encryptedPingProof{}, fmt.Errorf("%w: Teamgram auth-key store is unavailable", ErrAIMemoryTokenProof)
	}

	keyInfo, err := c.svcCtx.Dao.GetCacheAuthKey(c.ctx, in.AuthKeyId())
	if err != nil || keyInfo == nil || len(keyInfo.GetAuthKey()) < 256 {
		return encryptedPingProof{}, ErrAIMemoryTokenProof
	}
	msgKey := in.Payload[8:24]
	ciphertext := in.Payload[24:]
	decrypted, err := crypto.NewAuthKey(keyInfo.GetAuthKeyId(), keyInfo.GetAuthKey()).AesIgeDecrypt(msgKey, ciphertext)
	if err != nil || len(decrypted) < 32 {
		return encryptedPingProof{}, ErrAIMemoryTokenProof
	}

	messageLength := int(binary.LittleEndian.Uint32(decrypted[28:32]))
	if messageLength < 4 || messageLength > len(decrypted)-32 {
		return encryptedPingProof{}, ErrAIMemoryTokenProof
	}
	message := decrypted[32 : 32+messageLength]
	decoded := mtproto.NewDecodeBuf(message)
	object := decoded.Object()
	if decoded.GetError() != nil || decoded.GetOffset() != messageLength {
		return encryptedPingProof{}, ErrAIMemoryTokenProof
	}
	ping, ok := object.(*mtproto.TLPing)
	if !ok || ping == nil || ping.GetPingId() == 0 {
		return encryptedPingProof{}, ErrAIMemoryTokenProof
	}
	return encryptedPingProof{AuthKeyID: in.AuthKeyId(), PingID: ping.GetPingId()}, nil
}
