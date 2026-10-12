// Copyright 2026 Teamgram Authors
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

package core

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
)

const premiumProviderBodyLimit = 1 << 20

type premiumProviderEnvelope struct {
	RequestID string          `json:"request_id"`
	Operation string          `json:"operation"`
	Verified  bool            `json:"verified"`
	UserID    int64           `json:"user_id"`
	Error     string          `json:"error,omitempty"`
	Result    json.RawMessage `json:"result"`
}

func premiumPaymentProviderConfigured(c *PremiumCore) bool {
	return c != nil && c.svcCtx != nil &&
		strings.TrimSpace(c.svcCtx.Config.PaymentProviderEndpoint) != "" &&
		strings.TrimSpace(c.svcCtx.Config.PaymentProviderSigningKey) != ""
}

func paymentAssignmentError(err error) error {
	if err == mtproto.ErrMethodNotImpl {
		return mtproto.ErrPaymentUnsupported
	}
	return err
}

func normalizePremiumUpdates(updates *mtproto.Updates) bool {
	if updates == nil {
		return false
	}
	if updates.GetPredicateName() == "" {
		switch updates.GetConstructor() {
		case mtproto.TLConstructor_CRC32_updatesTooLong:
			updates.To_UpdatesTooLong()
		case mtproto.TLConstructor_CRC32_updateShortMessage:
			updates.To_UpdateShortMessage()
		case mtproto.TLConstructor_CRC32_updateShortChatMessage:
			updates.To_UpdateShortChatMessage()
		case mtproto.TLConstructor_CRC32_updateShort:
			updates.To_UpdateShort()
		case mtproto.TLConstructor_CRC32_updatesCombined:
			updates.To_UpdatesCombined()
		case mtproto.TLConstructor_CRC32_updates:
			updates.To_Updates()
		case mtproto.TLConstructor_CRC32_updateShortSentMessage:
			updates.To_UpdateShortSentMessage()
		default:
			// A provider may omit the generated union marker for a normal
			// updates value. Other unknown constructors remain invalid.
			if updates.GetConstructor() != mtproto.TLConstructor_CRC32_UNKNOWN {
				return false
			}
			updates.To_Updates()
		}
	}
	if updates.GetPredicateName() == "" {
		return false
	}
	return updates.Encode(mtproto.NewEncodeBuf(1<<16), 229) == nil
}

func normalizePremiumBool(value *mtproto.Bool) bool {
	if value == nil {
		return false
	}
	if value.GetPredicateName() == "" {
		switch value.GetConstructor() {
		case mtproto.TLConstructor_CRC32_boolTrue:
			value.To_BoolTrue()
		case mtproto.TLConstructor_CRC32_boolFalse:
			value.To_BoolFalse()
		}
	}
	if value.GetPredicateName() != mtproto.Predicate_boolTrue && value.GetPredicateName() != mtproto.Predicate_boolFalse {
		return false
	}
	return value.Encode(mtproto.NewEncodeBuf(16), 229) == nil
}

func normalizePremiumPromo(promo *mtproto.Help_PremiumPromo) bool {
	if promo == nil {
		return false
	}
	if promo.GetPredicateName() == "" {
		promo.To_HelpPremiumPromo()
	}
	if promo.GetPredicateName() != mtproto.Predicate_help_premiumPromo {
		return false
	}
	switch promo.GetConstructor() {
	case mtproto.TLConstructor_CRC32_UNKNOWN:
		promo.Constructor = mtproto.TLConstructor_CRC32_help_premiumPromo_5334759c
	case mtproto.TLConstructor_CRC32_help_premiumPromo_5334759c:
	default:
		return false
	}
	if len(promo.GetPeriodOptions()) == 0 {
		return false
	}
	for _, option := range promo.GetPeriodOptions() {
		if option == nil || option.GetMonths() <= 0 || option.GetMonths() > 120 ||
			strings.TrimSpace(option.GetCurrency()) == "" || option.GetAmount() < 0 {
			return false
		}
		if option.GetPredicateName() == "" {
			option.To_PremiumSubscriptionOption()
		}
		if option.GetPredicateName() != mtproto.Predicate_premiumSubscriptionOption {
			return false
		}
		switch option.GetConstructor() {
		case mtproto.TLConstructor_CRC32_UNKNOWN:
			option.Constructor = mtproto.TLConstructor_CRC32_premiumSubscriptionOption_5f2d1df2
		case mtproto.TLConstructor_CRC32_premiumSubscriptionOption_5f2d1df2:
		default:
			return false
		}
	}
	return promo.Encode(mtproto.NewEncodeBuf(1<<20), 229) == nil
}

type premiumUpdatesProviderResult struct {
	Accepted bool             `json:"accepted"`
	Updates  *mtproto.Updates `json:"updates"`
}

func (r *premiumUpdatesProviderResult) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if _, ok := fields["updates"]; ok {
		type plain premiumUpdatesProviderResult
		var value plain
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*r = premiumUpdatesProviderResult(value)
		return nil
	}
	if rawAccepted, present := fields["accepted"]; present {
		var accepted bool
		if err := json.Unmarshal(rawAccepted, &accepted); err != nil {
			return err
		}
		if _, hasPredicate := fields["predicate_name"]; !hasPredicate {
			if _, hasConstructor := fields["constructor"]; !hasConstructor {
				r.Accepted = accepted
				return nil
			}
		}
		r.Accepted = accepted
	}
	var updates mtproto.Updates
	if err := json.Unmarshal(data, &updates); err != nil {
		return err
	}
	if _, present := fields["accepted"]; !present {
		r.Accepted = true
	}
	r.Updates = &updates
	return nil
}

type premiumBoolProviderResult struct {
	Allowed bool          `json:"allowed"`
	Bool    *mtproto.Bool `json:"bool"`
}

func (r *premiumBoolProviderResult) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if _, allowed := fields["allowed"]; allowed {
		type plain premiumBoolProviderResult
		var value plain
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*r = premiumBoolProviderResult(value)
		return nil
	}
	if _, value := fields["bool"]; value {
		type plain premiumBoolProviderResult
		var result plain
		if err := json.Unmarshal(data, &result); err != nil {
			return err
		}
		*r = premiumBoolProviderResult(result)
		return nil
	}
	var direct mtproto.Bool
	if err := json.Unmarshal(data, &direct); err != nil {
		return err
	}
	r.Bool = &direct
	return nil
}

type premiumPromoProviderResult struct {
	Promo *mtproto.Help_PremiumPromo `json:"promo"`
}

func (r *premiumPromoProviderResult) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if _, wrapped := fields["promo"]; wrapped {
		type plain premiumPromoProviderResult
		var value plain
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*r = premiumPromoProviderResult(value)
		return nil
	}
	var promo mtproto.Help_PremiumPromo
	if err := json.Unmarshal(data, &promo); err != nil {
		return err
	}
	r.Promo = &promo
	return nil
}

func premiumProviderSignature(key string, body []byte) string {
	h := hmac.New(sha256.New, []byte(key))
	_, _ = h.Write(body)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

func verifyPremiumProviderSignature(key, signature string, body []byte) bool {
	signature = strings.TrimPrefix(strings.TrimSpace(signature), "sha256=")
	provided, err := hex.DecodeString(signature)
	if err != nil || len(provided) != sha256.Size || strings.TrimSpace(key) == "" {
		return false
	}
	h := hmac.New(sha256.New, []byte(key))
	_, _ = h.Write(body)
	return hmac.Equal(provided, h.Sum(nil))
}

func securePremiumProviderURL(parsed *url.URL, allowInsecureHTTP bool) bool {
	if parsed == nil {
		return false
	}
	if strings.EqualFold(parsed.Scheme, "https") {
		return true
	}
	if !strings.EqualFold(parsed.Scheme, "http") {
		return false
	}
	if allowInsecureHTTP {
		return true
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c *PremiumCore) callPaymentProvider(operation string, payload any, result any) error {
	if c == nil || c.svcCtx == nil || c.MD == nil || c.MD.GetUserId() <= 0 {
		return mtproto.ErrMethodNotImpl
	}
	endpoint := strings.TrimSpace(c.svcCtx.Config.PaymentProviderEndpoint)
	key := strings.TrimSpace(c.svcCtx.Config.PaymentProviderKey)
	signingKey := strings.TrimSpace(c.svcCtx.Config.PaymentProviderSigningKey)
	if endpoint == "" {
		return mtproto.ErrMethodNotImpl
	}
	if len(signingKey) < 32 {
		return mtproto.ErrPaymentProviderInvalid
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !securePremiumProviderURL(parsed, c.svcCtx.Config.AllowInsecureProviderHTTP) {
		return mtproto.ErrPaymentProviderInvalid
	}
	if c.svcCtx.Config.PaymentProviderTimeoutSeconds < 0 || c.svcCtx.Config.PaymentProviderTimeoutSeconds > 300 {
		return mtproto.ErrPaymentProviderInvalid
	}
	timeout := 5 * time.Second
	if c.svcCtx.Config.PaymentProviderTimeoutSeconds > 0 {
		timeout = time.Duration(c.svcCtx.Config.PaymentProviderTimeoutSeconds) * time.Second
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return mtproto.ErrInputRequestInvalid
	}
	// Request IDs are deterministic for a given operation and payload. This
	// gives the external processor a stable idempotency key across retries.
	digest := sha256.Sum256(append([]byte(operation+":"+strconv.FormatInt(c.MD.GetUserId(), 10)+":"), payloadJSON...))
	request := struct {
		RequestID string          `json:"request_id"`
		Operation string          `json:"operation"`
		UserID    int64           `json:"user_id"`
		Payload   json.RawMessage `json:"payload"`
	}{hex.EncodeToString(digest[:]), operation, c.MD.GetUserId(), payloadJSON}
	body, err := json.Marshal(request)
	if err != nil {
		return mtproto.ErrInternalServerError
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return mtproto.ErrPaymentProviderInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Idempotency-Key", request.RequestID)
	req.Header.Set("X-Teamgram-Payment-Provider-Request-Signature", premiumProviderSignature(signingKey, body))
	client := &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return mtproto.ErrMethodNotImpl
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, premiumProviderBodyLimit+1))
	if err != nil || len(responseBody) > premiumProviderBodyLimit || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return mtproto.ErrMethodNotImpl
	}
	if !verifyPremiumProviderSignature(signingKey, resp.Header.Get("X-Teamgram-Payment-Provider-Signature"), responseBody) {
		return mtproto.ErrPaymentProviderInvalid
	}
	var envelope premiumProviderEnvelope
	if err = json.Unmarshal(responseBody, &envelope); err != nil || !envelope.Verified || envelope.RequestID != request.RequestID || envelope.Operation != request.Operation || envelope.UserID != request.UserID {
		return mtproto.ErrPaymentProviderInvalid
	}
	if envelope.Error != "" || len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return mtproto.ErrMethodNotImpl
	}
	if err = json.Unmarshal(envelope.Result, result); err != nil {
		return mtproto.ErrPaymentProviderInvalid
	}
	return nil
}
