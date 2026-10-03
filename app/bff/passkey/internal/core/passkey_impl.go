// Copyright 2025 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	webauthn "github.com/go-webauthn/webauthn/webauthn"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/passkey/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/passkey/internal/dao"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type passkeyUser struct {
	id          int64
	name        string
	displayName string
	credentials []webauthn.Credential
}

func (u *passkeyUser) WebAuthnID() []byte {
	var id [8]byte
	binary.BigEndian.PutUint64(id[:], uint64(u.id))
	return id[:]
}

func (u *passkeyUser) WebAuthnName() string        { return u.name }
func (u *passkeyUser) WebAuthnDisplayName() string { return u.displayName }
func (u *passkeyUser) WebAuthnCredentials() []webauthn.Credential {
	return u.credentials
}

func (c *PasskeyCore) webAuthn() (*webauthn.WebAuthn, error) {
	if err := c.requireProvider(); err != nil {
		return nil, err
	}
	provider := c.svcCtx.Config.Provider
	return webauthn.New(&webauthn.Config{
		RPID:                  strings.TrimSpace(strings.ToLower(provider.RelyingPartyId)),
		RPDisplayName:         provider.RelyingPartyDisplayName,
		RPOrigins:             append([]string(nil), provider.Origins...),
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			RequireResidentKey: protocol.ResidentKeyRequired(),
			UserVerification:   protocol.VerificationPreferred,
		},
		Timeouts: webauthn.TimeoutsConfig{
			Login:        webauthn.TimeoutConfig{Enforce: true, Timeout: 2 * time.Minute},
			Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: 2 * time.Minute},
		},
	})
}

// requireProvider keeps every Passkey operation disabled until a real relying
// party is configured. Database availability alone must not make an
// unconfigured WebAuthn endpoint look usable.
func (c *PasskeyCore) requireProvider() error {
	if c == nil || c.svcCtx == nil || !passkeyProviderConfigured(c.svcCtx.Config.Provider) {
		return mtproto.ErrMethodNotImpl
	}
	return nil
}

func (c *PasskeyCore) loadUser(ctx context.Context, userID int64) (*passkeyUser, error) {
	if userID <= 0 || c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	immutable, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(ctx, (&structUserRequest{Id: userID}).TL())
	if err != nil {
		return nil, err
	}
	if immutable == nil || immutable.GetUser() == nil || immutable.GetUser().GetId() != userID || immutable.GetUser().GetDeleted() {
		return nil, mtproto.ErrUserIdInvalid
	}
	name := mtproto.GetUserName(immutable.ToSelfUser())
	if name == "" {
		name = strconv.FormatInt(userID, 10)
	}
	credentials, err := c.svcCtx.Dao.ListCredentials(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := &passkeyUser{id: userID, name: name, displayName: name}
	for _, record := range credentials {
		credential, err := decodeCredential(record)
		if err != nil {
			return nil, err
		}
		result.credentials = append(result.credentials, *credential)
	}
	return result, nil
}

// structUserRequest keeps the generated user package out of this helper's
// public surface while retaining the generated request type at the call site.
type structUserRequest struct{ Id int64 }

func (r *structUserRequest) TL() *userpb.TLUserGetImmutableUser {
	return &userpb.TLUserGetImmutableUser{Id: r.Id}
}

func decodeCredential(record dao.Credential) (*webauthn.Credential, error) {
	var credential webauthn.Credential
	if err := json.Unmarshal(record.Data, &credential); err != nil {
		return nil, fmt.Errorf("decode passkey credential: %w", err)
	}
	credential.ID = append([]byte(nil), record.ID...)
	credential.Authenticator.SignCount = record.SignCount
	return &credential, nil
}

func credentialResponsePayload(in *mtproto.InputPasskeyCredential, registration bool) ([]byte, error) {
	if in == nil || in.GetId() == "" || in.GetResponse() == nil || in.GetResponse().GetClientData() == nil {
		return nil, mtproto.ErrAuthTokenInvalid
	}
	response := in.GetResponse()
	id := in.GetId()
	rawID := in.GetRawId()
	if rawID == "" {
		rawID = id
	}
	clientData := response.GetClientData().GetData()
	if clientData == "" {
		return nil, mtproto.ErrAuthTokenInvalid
	}
	encode := func(value []byte) string { return base64.RawURLEncoding.EncodeToString(value) }
	payload := map[string]any{
		"id":    id,
		"rawId": rawID,
		"type":  "public-key",
	}
	if registration {
		if len(response.GetAttestationData()) == 0 {
			return nil, mtproto.ErrAuthTokenInvalid
		}
		payload["response"] = map[string]any{
			"clientDataJSON":    encode([]byte(clientData)),
			"attestationObject": encode(response.GetAttestationData()),
		}
	} else {
		if len(response.GetAuthenticatorData()) == 0 || len(response.GetSignature()) == 0 {
			return nil, mtproto.ErrAuthTokenInvalid
		}
		loginResponse := map[string]any{
			"clientDataJSON":    encode([]byte(clientData)),
			"authenticatorData": encode(response.GetAuthenticatorData()),
			"signature":         encode(response.GetSignature()),
		}
		if response.GetUserHandle() != "" {
			loginResponse["userHandle"] = response.GetUserHandle()
		}
		payload["response"] = loginResponse
	}
	return json.Marshal(payload)
}

func sessionChallengeRegistration(parsed *protocol.ParsedCredentialCreationData) string {
	if parsed == nil {
		return ""
	}
	return parsed.Response.CollectedClientData.Challenge
}

func sessionChallengeLogin(parsed *protocol.ParsedCredentialAssertionData) string {
	if parsed == nil {
		return ""
	}
	return parsed.Response.CollectedClientData.Challenge
}

func recordFromCredential(userID int64, credential *webauthn.Credential, name string) (dao.Credential, error) {
	data, err := json.Marshal(credential)
	if err != nil {
		return dao.Credential{}, err
	}
	return dao.Credential{
		ID:            append([]byte(nil), credential.ID...),
		UserID:        userID,
		Name:          name,
		Date:          time.Now().Unix(),
		LastUsageDate: 0,
		SignCount:     credential.Authenticator.SignCount,
		Data:          data,
	}, nil
}

func mapPasskeyVerificationError(err error) error {
	if err == nil {
		return nil
	}
	return mtproto.ErrAuthTokenInvalid
}

func userHandleID(handle []byte) (int64, bool) {
	if len(handle) == 8 {
		id := int64(binary.BigEndian.Uint64(handle))
		return id, id > 0
	}
	if id, err := strconv.ParseInt(string(handle), 10, 64); err == nil && id > 0 {
		return id, true
	}
	decoded, err := base64.RawURLEncoding.DecodeString(string(handle))
	if err == nil && len(decoded) == 8 {
		id := int64(binary.BigEndian.Uint64(decoded))
		return id, id > 0
	}
	return 0, false
}

func sameCredentialID(a, b []byte) bool { return len(a) > 0 && bytes.Equal(a, b) }

func passkeyRequireUser(c *PasskeyCore) (int64, error) {
	if c == nil || c.MD == nil || c.MD.UserId == 0 {
		return 0, mtproto.ErrAuthKeyUnregistered
	}
	return c.MD.UserId, nil
}

func passkeyProviderConfigured(provider config.ProviderConfig) bool {
	rpID := strings.TrimSpace(strings.ToLower(provider.RelyingPartyId))
	if rpID == "" || provider.RelyingPartyDisplayName == "" || net.ParseIP(rpID) != nil || rpID == "localhost" || len(provider.Origins) == 0 {
		return false
	}
	for _, rawOrigin := range provider.Origins {
		origin, err := url.Parse(rawOrigin)
		if err != nil || origin.Scheme != "https" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || origin.Path != "" {
			return false
		}
		host := strings.ToLower(origin.Hostname())
		if host != rpID && !strings.HasSuffix(host, "."+rpID) {
			return false
		}
	}
	return true
}

func passkeyTrustedApp(provider config.ProviderConfig, apiID int32, apiHash string) bool {
	if !validPasskeyAPIHash(apiHash) {
		return false
	}
	for _, app := range provider.TrustedApps {
		if app.ApiId == apiID && strings.EqualFold(app.ApiHash, apiHash) {
			return true
		}
	}
	return false
}

func validatePasskeySourceDc(localDCID, sourceDCID int32) error {
	if sourceDCID <= 0 || localDCID <= 0 || sourceDCID != localDCID {
		return mtproto.ErrDcIdInvalid
	}
	return nil
}
