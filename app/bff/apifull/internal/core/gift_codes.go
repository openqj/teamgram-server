// Copyright 2026 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)

package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RPCGiftCodesServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) PaymentsCheckGiftCode(in *mtproto.TLPaymentsCheckGiftCode) (*mtproto.Payments_CheckedGiftCode, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || strings.TrimSpace(in.GetSlug()) == "" {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	var response struct {
		Verified bool                              `json:"verified"`
		UserID   int64                             `json:"user_id"`
		Slug     string                            `json:"slug"`
		Code     *mtproto.Payments_CheckedGiftCode `json:"code"`
	}
	if err = c.callStarsRevenueProvider(c.secretContext(), struct {
		Operation string `json:"operation"`
		UserID    int64  `json:"user_id"`
		Slug      string `json:"slug"`
	}{"check_gift_code", uid, strings.TrimSpace(in.GetSlug())}, &response); err != nil {
		return nil, err
	}
	if !response.Verified || response.UserID != uid || response.Slug != strings.TrimSpace(in.GetSlug()) || response.Code == nil {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	code := response.Code
	if code.GetDays() < 0 || code.GetMonths() < 0 || code.GetDate() < 0 {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	if code.GetPredicateName() == "" {
		mtproto.MakeTLPaymentsCheckedGiftCode(code).To_Payments_CheckedGiftCode()
	}
	if code.GetPredicateName() != mtproto.Predicate_payments_checkedGiftCode {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	for _, user := range code.GetUsers() {
		if user == nil || user.GetPredicateName() == "" {
			return nil, mtproto.ErrPaymentProviderInvalid
		}
	}
	for _, chat := range code.GetChats() {
		if chat == nil || chat.GetPredicateName() == "" {
			return nil, mtproto.ErrPaymentProviderInvalid
		}
	}
	return mtproto.MakeTLPaymentsCheckedGiftCode(code).To_Payments_CheckedGiftCode(), nil
}

func (c *ApiFullCore) PaymentsApplyGiftCode(in *mtproto.TLPaymentsApplyGiftCode) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || strings.TrimSpace(in.GetSlug()) == "" {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if _, _, _, err = c.configuredPaymentProvider(); err != nil {
		return nil, err
	}
	if err = domain.CheckPremiumGrantOutbox(c.secretContext()); err != nil {
		return nil, mtproto.ErrPaymentUnsupported
	}
	slug := strings.TrimSpace(in.GetSlug())
	digest := sha256.Sum256([]byte("gift-code\x00" + slug))
	fingerprint := hex.EncodeToString(digest[:])
	requestKey := fmt.Sprintf("gift-code:%d:%s", uid, fingerprint)
	lockWait := 10 * time.Second
	if c.svcCtx != nil && c.svcCtx.Config.PaymentProviderTimeoutSeconds > 0 {
		lockWait = time.Duration(c.svcCtx.Config.PaymentProviderTimeoutSeconds+5) * time.Second
	}
	release, err := domain.LockPaymentRequest(c.secretContext(), uid, requestKey, lockWait)
	if err != nil {
		return nil, mtproto.ErrPaymentUnsupported
	}
	defer release()
	request, err := domain.BeginPaymentRequest(uid, requestKey, "gift_code", fingerprint, "", 0, 0, 0)
	if err != nil {
		return nil, err
	}
	if request.State == domain.PaymentStateSettled {
		receipt, found, receiptErr := domain.LoadPaymentReceiptByRequest(uid, requestKey)
		if receiptErr != nil {
			return nil, receiptErr
		}
		if !found {
			return nil, mtproto.ErrPaymentProviderInvalid
		}
		if err = c.grantPremiumGiftCode(c.secretContext(), uid, request, receipt); err != nil {
			return nil, err
		}
		return mtproto.MakeEmptyUpdates(), nil
	}
	if request.State != domain.PaymentStatePending {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	client, endpoint, providerKey, err := c.configuredPaymentProvider()
	if err != nil {
		return nil, err
	}
	payload := struct {
		Operation   string `json:"operation"`
		UserID      int64  `json:"user_id"`
		Slug        string `json:"slug"`
		RequestKey  string `json:"request_key"`
		Fingerprint string `json:"fingerprint"`
	}{"apply_gift_code", uid, slug, requestKey, fingerprint}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	req, err := http.NewRequestWithContext(c.secretContext(), http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	c.setPaymentProviderRequestSignature(req, body)
	req.Header.Set("Idempotency-Key", paymentProviderIdempotencyKey(uid, requestKey))
	if providerKey != "" {
		req.Header.Set("Authorization", "Bearer "+providerKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, mtproto.ErrPaymentUnsupported
	}
	defer resp.Body.Close()
	responseBody, readErr := readPaymentProviderBody(resp)
	if readErr != nil {
		return nil, readErr
	}
	if !c.paymentProviderResponseAuthenticated(resp, responseBody) {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	var result paymentProviderResponse
	if err = json.Unmarshal(responseBody, &result); err != nil || !validPremiumGiftCodeProviderResult(result, uid, requestKey, fingerprint) {
		_, _ = domain.RejectPaymentRequest(uid, requestKey, fingerprint, "provider rejected gift code")
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	storedReceipt, err := json.Marshal(paymentProviderReceiptEnvelope{Signature: resp.Header.Get("X-Teamgram-Payment-Signature"), ResponseBody: append([]byte(nil), responseBody...)})
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	settled, receipt, err := domain.SettlePremiumPaymentRequest(uid, requestKey, fingerprint, result.TransactionID, result.Currency, result.Title, result.Amount, 0, 0, storedReceipt, result.PremiumMonths)
	if err != nil {
		return nil, err
	}
	if err = c.grantPremiumGiftCode(c.secretContext(), uid, settled, receipt); err != nil {
		return nil, err
	}
	return mtproto.MakeEmptyUpdates(), nil
}

func readPaymentProviderBody(resp *http.Response) ([]byte, error) {
	if resp == nil || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, mtproto.ErrPaymentUnsupported
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		return nil, mtproto.ErrPaymentUnsupported
	}
	return body, nil
}

func validPremiumGiftCodeProviderResult(result paymentProviderResponse, uid int64, requestKey, fingerprint string) bool {
	return result.Verified && result.UserID == uid && result.BeneficiaryUserID == uid && result.RequestKey == requestKey && result.Fingerprint == fingerprint &&
		(result.ProductType == "premium_gift_code" || result.ProductType == paymentProductPremiumSubscription) && result.PremiumMonths >= 1 && result.PremiumMonths <= 36 &&
		strings.TrimSpace(result.TransactionID) != "" && len(bytes.TrimSpace(result.Receipt)) > 0 && !bytes.Equal(bytes.TrimSpace(result.Receipt), []byte("null")) && result.Amount >= 0
}

func (c *ApiFullCore) grantPremiumGiftCode(ctx context.Context, uid int64, request domain.PaymentRequest, receipt domain.PaymentReceipt) error {
	if c == nil || c.svcCtx == nil {
		return mtproto.ErrPaymentUnsupported
	}
	var envelope paymentProviderReceiptEnvelope
	if json.Unmarshal(receipt.Receipt, &envelope) != nil || !verifyPaymentProviderSignature(c.svcCtx.Config.PaymentProviderSigningKey, envelope.Signature, envelope.ResponseBody) {
		return mtproto.ErrPaymentProviderInvalid
	}
	var result paymentProviderResponse
	if json.Unmarshal(envelope.ResponseBody, &result) != nil || !validPremiumGiftCodeProviderResult(result, uid, request.RequestKey, request.Fingerprint) || result.TransactionID != receipt.TransactionID {
		return mtproto.ErrPaymentProviderInvalid
	}
	if err := domain.EnsurePremiumGrant(request.ID, uid, receipt.Provider, receipt.TransactionID, result.PremiumMonths); err != nil {
		return err
	}
	if c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return mtproto.ErrPaymentUnsupported
	}
	grant, err := c.svcCtx.Dao.UserClient.UserUpdatePremium(ctx, &userpb.TLUserUpdatePremium{
		UserId: uid, Premium: mtproto.ToBool(true), Months: wrapperspb.Int32(result.PremiumMonths), Provider: receipt.Provider, TransactionId: receipt.TransactionID,
	})
	if err != nil {
		retryPremiumGrant(request.ID, err)
		return err
	}
	if grant == nil || !mtproto.FromBool(grant) {
		err = mtproto.ErrPaymentUnsupported
		retryPremiumGrant(request.ID, err)
		return err
	}
	return domain.CompletePremiumGrant(ctx, request.ID)
}
