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
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type starsRefundProviderResponse struct {
	Verified            bool            `json:"verified"`
	CallerUserID        int64           `json:"caller_user_id"`
	UserID              int64           `json:"user_id"`
	ChargeID            string          `json:"charge_id"`
	Provider            string          `json:"provider"`
	Stars               int64           `json:"stars"`
	RefundTransactionID string          `json:"refund_transaction_id"`
	Receipt             json.RawMessage `json:"receipt"`
}

type starsRevenueStatsProviderResponse struct {
	Verified bool                                `json:"verified"`
	UserID   int64                               `json:"user_id"`
	Stats    *mtproto.Payments_StarsRevenueStats `json:"stats"`
}

type starsRevenueURLProviderResponse struct {
	Verified bool   `json:"verified"`
	UserID   int64  `json:"user_id"`
	URL      string `json:"url"`
}

// callStarsRevenueProvider sends a signed, read-only revenue request. Revenue
// is provider-owned, but the caller and peer are always included in the
// signed request so the provider can enforce channel ownership and scope.
func (c *ApiFullCore) callStarsRevenueProvider(ctx context.Context, payload any, response any) error {
	client, endpoint, providerKey, err := c.configuredPaymentProvider()
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return mtproto.ErrInternalServerError
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return mtproto.ErrPaymentProviderInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	if providerKey != "" {
		req.Header.Set("Authorization", "Bearer "+providerKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return mtproto.ErrPaymentUnsupported
	}
	defer resp.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if readErr != nil || len(responseBody) > 1<<20 || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return mtproto.ErrPaymentUnsupported
	}
	if !c.paymentProviderResponseAuthenticated(resp, responseBody) {
		return mtproto.ErrPaymentProviderInvalid
	}
	if err = json.Unmarshal(responseBody, response); err != nil {
		return mtproto.ErrPaymentProviderInvalid
	}
	return nil
}

func validRevenuePeer(uid int64, peer *mtproto.InputPeer) bool {
	return peer != nil && validThreadInputPeer(uid, peer)
}

func validRevenueURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && parsed.Host != "" && strings.EqualFold(parsed.Scheme, "https")
}

func normalizeRevenueGraph(graph *mtproto.StatsGraph) bool {
	if graph == nil {
		return false
	}
	if graph.GetPredicateName() == "" {
		switch {
		case graph.GetJson() != nil:
			graph.To_StatsGraph()
		case strings.TrimSpace(graph.GetError()) != "":
			graph.To_StatsGraphError()
		case strings.TrimSpace(graph.GetToken()) != "":
			graph.To_StatsGraphAsync()
		default:
			return false
		}
	}
	switch graph.GetPredicateName() {
	case mtproto.Predicate_statsGraph:
		if graph.GetJson() == nil || strings.TrimSpace(graph.GetJson().GetData()) == "" {
			return false
		}
		if graph.GetJson().GetPredicateName() == "" {
			graph.GetJson().To_DataJSON()
		}
		return graph.GetJson().GetPredicateName() == mtproto.Predicate_dataJSON
	case mtproto.Predicate_statsGraphError:
		return strings.TrimSpace(graph.GetError()) != ""
	case mtproto.Predicate_statsGraphAsync:
		return strings.TrimSpace(graph.GetToken()) != ""
	default:
		return false
	}
}

func normalizeRevenueStats(stats *mtproto.Payments_StarsRevenueStats) bool {
	if stats == nil || stats.GetStatus() == nil || stats.GetRevenueGraph() == nil ||
		!normalizeRevenueGraph(stats.GetRevenueGraph()) || math.IsNaN(stats.GetUsdRate()) || math.IsInf(stats.GetUsdRate(), 0) || stats.GetUsdRate() < 0 {
		return false
	}
	if stats.GetTopHoursGraph() != nil && !normalizeRevenueGraph(stats.GetTopHoursGraph()) {
		return false
	}
	status := stats.GetStatus()
	for _, amount := range []*mtproto.StarsAmount{
		status.GetCurrentBalance_STARSAMOUNT(),
		status.GetAvailableBalance_STARSAMOUNT(),
		status.GetOverallRevenue_STARSAMOUNT(),
	} {
		if amount == nil {
			continue
		}
		if amount.GetPredicateName() == "" {
			mtproto.MakeTLStarsAmount(amount).To_StarsAmount()
		}
		if amount.GetPredicateName() != mtproto.Predicate_starsAmount {
			return false
		}
		if amount.GetAmount() < 0 || amount.GetNanos() < 0 {
			return false
		}
	}
	if status.GetCurrentBalance_INT64() == 0 && status.GetCurrentBalance_STARSAMOUNT() != nil {
		status.CurrentBalance_INT64 = status.GetCurrentBalance_STARSAMOUNT().GetAmount()
	}
	if status.GetAvailableBalance_INT64() == 0 && status.GetAvailableBalance_STARSAMOUNT() != nil {
		status.AvailableBalance_INT64 = status.GetAvailableBalance_STARSAMOUNT().GetAmount()
	}
	if status.GetOverallRevenue_INT64() == 0 && status.GetOverallRevenue_STARSAMOUNT() != nil {
		status.OverallRevenue_INT64 = status.GetOverallRevenue_STARSAMOUNT().GetAmount()
	}
	if status.GetCurrentBalance_INT64() < 0 || status.GetAvailableBalance_INT64() < 0 || status.GetOverallRevenue_INT64() < 0 {
		return false
	}
	if status.GetPredicateName() == "" {
		status.To_StarsRevenueStatus()
	}
	if status.GetPredicateName() != mtproto.Predicate_starsRevenueStatus {
		return false
	}
	stats.To_PaymentsStarsRevenueStats()
	return stats.GetPredicateName() == mtproto.Predicate_payments_starsRevenueStats
}

// RPCStarsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) starsUnavailable() error {
	if _, err := c.requireUserId(); err != nil {
		return err
	}
	return mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) PaymentsGetStarsTopupOptions(in *mtproto.TLPaymentsGetStarsTopupOptions) (*mtproto.Vector_StarsTopupOption, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		// Preserve the normal unauthenticated/unsupported distinction: a nil
		// request cannot be served without a configured catalog.
		return nil, mtproto.ErrMethodNotImpl
	}
	offers, err := domain.ListStarsOffers("topup")
	if err != nil {
		return nil, starsDomainError(err)
	}
	if len(offers) == 0 {
		return nil, mtproto.ErrMethodNotImpl
	}
	result := make([]*mtproto.StarsTopupOption, 0, len(offers))
	for _, offer := range offers {
		result = append(result, mtproto.MakeTLStarsTopupOption(&mtproto.StarsTopupOption{
			Extended:     offer.Extended,
			Stars:        offer.Stars,
			StoreProduct: optionalString(offer.StoreProduct),
			Currency:     offer.Currency,
			Amount:       offer.Amount,
		}).To_StarsTopupOption())
	}
	return &mtproto.Vector_StarsTopupOption{Datas: result}, nil
}

func (c *ApiFullCore) PaymentsGetStarsStatus(in *mtproto.TLPaymentsGetStarsStatus) (*mtproto.Payments_StarsStatus, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if in.GetTon() {
		return nil, mtproto.ErrPaymentUnsupported
	}
	if peer := in.GetPeer(); peer != nil {
		switch peer.GetPredicateName() {
		case mtproto.Predicate_inputPeerSelf:
		case mtproto.Predicate_inputPeerUser:
			if peer.GetUserId() != uid {
				return nil, mtproto.ErrPeerIdInvalid
			}
		default:
			return nil, mtproto.ErrPeerIdInvalid
		}
	}
	balance, err := domain.StarsBalance(uid)
	if err != nil {
		return nil, starsDomainError(err)
	}
	// A zero balance without any durable transaction is indistinguishable from
	// an unprovisioned Stars account. Do not advertise a synthetic account.
	history, err := domain.ListStarsTransactions(uid, false, false, false, 0, 1)
	if err != nil {
		return nil, starsDomainError(err)
	}
	if balance == 0 && len(history) == 0 {
		return nil, mtproto.ErrMethodNotImpl
	}
	return makeStarsStatus(balance, nil, nil), nil
}

func (c *ApiFullCore) PaymentsGetStarsTransactions(in *mtproto.TLPaymentsGetStarsTransactions) (*mtproto.Payments_StarsStatus, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if in.GetTon() {
		return nil, mtproto.ErrPaymentUnsupported
	}
	if !validStarsPeer(uid, in.GetPeer()) {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if _, err := parseStarsOffset(in.GetOffset()); err != nil {
		return nil, err
	}
	if in.GetLimit() < 0 {
		return nil, mtproto.ErrLimitInvalid
	}
	if in.GetSubscriptionId() != nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	start, limit, err := starsPage(in.GetOffset(), in.GetLimit())
	if err != nil {
		return nil, err
	}
	transactions, err := domain.ListStarsTransactions(uid, in.GetInbound(), in.GetOutbound(), in.GetAscending(), start, limit)
	if err != nil {
		return nil, starsDomainError(err)
	}
	balance, err := domain.StarsBalance(uid)
	if err != nil {
		return nil, starsDomainError(err)
	}
	nextOffset := (*wrapperspb.StringValue)(nil)
	if len(transactions) == limit {
		nextOffset = wrapperspb.String(strconv.Itoa(start + len(transactions)))
	}
	history := make([]*mtproto.StarsTransaction, 0, len(transactions))
	for _, item := range transactions {
		history = append(history, makeStarsTransaction(item))
	}
	return makeStarsStatus(balance, history, nextOffset), nil
}

func (c *ApiFullCore) PaymentsSendStarsForm(in *mtproto.TLPaymentsSendStarsForm) (*mtproto.Payments_PaymentResult, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetFormId() <= 0 || in.GetInvoice() == nil {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	offer, found, err := starsTopupOfferFromInvoice(in.GetInvoice())
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, mtproto.ErrPaymentUnsupported
	}
	if _, _, _, err = c.configuredPaymentProvider(); err != nil {
		return nil, err
	}
	fingerprint := starsPaymentFingerprint(in.GetFormId(), in.GetInvoice())
	requestKey := starsPaymentRequestKey(uid, in.GetFormId(), fingerprint)
	if fingerprint == "" || requestKey == "" {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	lockWait := 10 * time.Second
	if c.svcCtx.Config.PaymentProviderTimeoutSeconds > 0 {
		lockWait = time.Duration(c.svcCtx.Config.PaymentProviderTimeoutSeconds+5) * time.Second
	}
	release, err := domain.LockPaymentRequest(c.secretContext(), uid, requestKey, lockWait)
	if err != nil {
		return nil, mtproto.ErrPaymentUnsupported
	}
	defer release()
	request, err := domain.BeginPaymentRequest(uid, requestKey, "external", fingerprint, offer.Currency, offer.Amount, 0, 0)
	if err != nil {
		return nil, err
	}
	if request.State == domain.PaymentStateSettled {
		receipt, found, receiptErr := domain.LoadPaymentReceiptByRequest(uid, requestKey)
		if receiptErr != nil {
			return nil, receiptErr
		}
		if !found || c.svcCtx == nil {
			return nil, mtproto.ErrPaymentProviderInvalid
		}
		var envelope paymentProviderReceiptEnvelope
		if json.Unmarshal(receipt.Receipt, &envelope) != nil || !verifyPaymentProviderSignature(c.svcCtx.Config.PaymentProviderSigningKey, envelope.Signature, envelope.ResponseBody) {
			return nil, mtproto.ErrPaymentProviderInvalid
		}
		var result paymentProviderResponse
		if json.Unmarshal(envelope.ResponseBody, &result) != nil || !validStarsTopupProviderResult(result, uid, in.GetFormId(), requestKey, fingerprint, offer) ||
			result.TransactionID != receipt.TransactionID || result.Currency != receipt.Currency || result.Amount != receipt.Amount {
			return nil, mtproto.ErrPaymentProviderInvalid
		}
		return mtproto.MakeTLPaymentsPaymentResult(&mtproto.Payments_PaymentResult{
			Updates: mtproto.MakeEmptyUpdates(),
		}).To_Payments_PaymentResult(), nil
	}
	if request.State != domain.PaymentStatePending {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	settled, _, err := c.settleStarsWithPaymentProvider(c.secretContext(), uid, requestKey, fingerprint, in, offer)
	if err != nil {
		return nil, err
	}
	if settled.State != domain.PaymentStateSettled {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	return mtproto.MakeTLPaymentsPaymentResult(&mtproto.Payments_PaymentResult{
		Updates: mtproto.MakeEmptyUpdates(),
	}).To_Payments_PaymentResult(), nil
}

func (c *ApiFullCore) PaymentsRefundStarsCharge(in *mtproto.TLPaymentsRefundStarsCharge) (*mtproto.Updates, error) {
	callerID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetUserId() == nil || strings.TrimSpace(in.GetChargeId()) == "" {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	target := in.GetUserId()
	var userID int64
	switch target.GetPredicateName() {
	case mtproto.Predicate_inputUserSelf:
		userID = callerID
	case mtproto.Predicate_inputUser:
		if target.GetUserId() <= 0 || target.GetAccessHash() == 0 {
			return nil, mtproto.ErrUserIdInvalid
		}
		userID = target.GetUserId()
	default:
		return nil, mtproto.ErrUserIdInvalid
	}
	chargeID := strings.TrimSpace(in.GetChargeId())
	charge, found, err := domain.LoadStarsCharge(userID, chargeID)
	if err != nil {
		if errors.Is(err, domain.ErrStarsChargeInvalid) {
			return nil, mtproto.ErrPaymentChargeInvalid
		}
		return nil, starsDomainError(err)
	}
	if !found {
		return nil, mtproto.ErrPaymentChargeInvalid
	}
	// A committed refund is idempotent and does not call the provider again.
	if refund, refundFound, refundErr := domain.LoadStarsRefund(userID, chargeID); refundErr != nil {
		return nil, starsDomainError(refundErr)
	} else if refundFound {
		if refund.State != domain.StarsRefundStateComplete || refund.Provider != charge.Provider || refund.Stars != charge.Stars {
			return nil, mtproto.ErrPaymentChargeInvalid
		}
		return mtproto.MakeEmptyUpdates(), nil
	}

	client, endpoint, providerKey, err := c.configuredPaymentProvider()
	if err != nil {
		return nil, err
	}
	payload := struct {
		Operation      string `json:"operation"`
		CallerUserID   int64  `json:"caller_user_id"`
		UserID         int64  `json:"user_id"`
		UserAccessHash int64  `json:"user_access_hash,omitempty"`
		ChargeID       string `json:"charge_id"`
		Provider       string `json:"provider"`
		TransactionID  string `json:"transaction_id"`
		Stars          int64  `json:"stars"`
	}{"refund_stars_charge", callerID, userID, target.GetAccessHash(), chargeID, charge.Provider, charge.TransactionID, charge.Stars}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	req, err := http.NewRequestWithContext(c.secretContext(), http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", paymentProviderIdempotencyKey(callerID, "refund:"+charge.Provider+":"+chargeID))
	if providerKey != "" {
		req.Header.Set("Authorization", "Bearer "+providerKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, mtproto.ErrPaymentUnsupported
	}
	defer resp.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if readErr != nil || len(responseBody) > 1<<20 || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, mtproto.ErrPaymentUnsupported
	}
	if !c.paymentProviderResponseAuthenticated(resp, responseBody) {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	var result starsRefundProviderResponse
	if err = json.Unmarshal(responseBody, &result); err != nil || !result.Verified || result.CallerUserID != callerID || result.UserID != userID ||
		result.ChargeID != chargeID || result.Provider != charge.Provider || result.Stars != charge.Stars || strings.TrimSpace(result.RefundTransactionID) == "" ||
		len(bytes.TrimSpace(result.Receipt)) == 0 || bytes.Equal(bytes.TrimSpace(result.Receipt), []byte("null")) {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	if _, err = domain.CommitStarsRefund(userID, chargeID, result.Provider, result.RefundTransactionID, result.Stars, responseBody); err != nil {
		if errors.Is(err, domain.ErrStarsChargeNotFound) || errors.Is(err, domain.ErrStarsChargeInvalid) || errors.Is(err, domain.ErrStarsRefundConflict) {
			return nil, mtproto.ErrPaymentChargeInvalid
		}
		return nil, starsDomainError(err)
	}
	return mtproto.MakeEmptyUpdates(), nil
}

func (c *ApiFullCore) PaymentsGetStarsRevenueStats(in *mtproto.TLPaymentsGetStarsRevenueStats) (*mtproto.Payments_StarsRevenueStats, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if in.GetTon() {
		return nil, mtproto.ErrPaymentUnsupported
	}
	if !validRevenuePeer(uid, in.GetPeer()) {
		return nil, mtproto.ErrPeerIdInvalid
	}
	var response starsRevenueStatsProviderResponse
	err = c.callStarsRevenueProvider(c.secretContext(), struct {
		Operation string             `json:"operation"`
		UserID    int64              `json:"user_id"`
		Peer      *mtproto.InputPeer `json:"peer"`
		Dark      bool               `json:"dark"`
		Ton       bool               `json:"ton"`
	}{"get_stars_revenue_stats", uid, in.GetPeer(), in.GetDark(), in.GetTon()}, &response)
	if err != nil {
		return nil, err
	}
	if !response.Verified || response.UserID != uid || !normalizeRevenueStats(response.Stats) {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	return mtproto.MakeTLPaymentsStarsRevenueStats(response.Stats).To_Payments_StarsRevenueStats(), nil
}

func (c *ApiFullCore) PaymentsGetStarsRevenueWithdrawalUrl(in *mtproto.TLPaymentsGetStarsRevenueWithdrawalUrl) (*mtproto.Payments_StarsRevenueWithdrawalUrl, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if in.GetTon() {
		return nil, mtproto.ErrPaymentUnsupported
	}
	if !validRevenuePeer(uid, in.GetPeer()) {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.GetAmount() != nil && in.GetAmount().GetValue() <= 0 {
		return nil, mtproto.ErrPaymentUnsupported
	}
	if in.GetPassword() == nil {
		return nil, mtproto.ErrPasswordHashInvalid
	}
	var response starsRevenueURLProviderResponse
	err = c.callStarsRevenueProvider(c.secretContext(), struct {
		Operation string                         `json:"operation"`
		UserID    int64                          `json:"user_id"`
		Peer      *mtproto.InputPeer             `json:"peer"`
		Amount    *wrapperspb.Int64Value         `json:"amount,omitempty"`
		Password  *mtproto.InputCheckPasswordSRP `json:"password"`
		Ton       bool                           `json:"ton"`
	}{"get_stars_revenue_withdrawal_url", uid, in.GetPeer(), in.GetAmount(), in.GetPassword(), in.GetTon()}, &response)
	if err != nil {
		return nil, err
	}
	if !response.Verified || response.UserID != uid || !validRevenueURL(response.URL) {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	return mtproto.MakeTLPaymentsStarsRevenueWithdrawalUrl(&mtproto.Payments_StarsRevenueWithdrawalUrl{Url: strings.TrimSpace(response.URL)}).To_Payments_StarsRevenueWithdrawalUrl(), nil
}

func (c *ApiFullCore) PaymentsGetStarsRevenueAdsAccountUrl(in *mtproto.TLPaymentsGetStarsRevenueAdsAccountUrl) (*mtproto.Payments_StarsRevenueAdsAccountUrl, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if !validRevenuePeer(uid, in.GetPeer()) {
		return nil, mtproto.ErrPeerIdInvalid
	}
	var response starsRevenueURLProviderResponse
	err = c.callStarsRevenueProvider(c.secretContext(), struct {
		Operation string             `json:"operation"`
		UserID    int64              `json:"user_id"`
		Peer      *mtproto.InputPeer `json:"peer"`
	}{"get_stars_revenue_ads_account_url", uid, in.GetPeer()}, &response)
	if err != nil {
		return nil, err
	}
	if !response.Verified || response.UserID != uid || !validRevenueURL(response.URL) {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	return mtproto.MakeTLPaymentsStarsRevenueAdsAccountUrl(&mtproto.Payments_StarsRevenueAdsAccountUrl{Url: strings.TrimSpace(response.URL)}).To_Payments_StarsRevenueAdsAccountUrl(), nil
}

func (c *ApiFullCore) PaymentsGetStarsTransactionsByID(in *mtproto.TLPaymentsGetStarsTransactionsByID) (*mtproto.Payments_StarsStatus, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in != nil && in.GetTon() {
		return nil, mtproto.ErrPaymentUnsupported
	}
	if in != nil && !validStarsPeer(uid, in.GetPeer()) {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in == nil || len(in.GetId()) == 0 {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	for _, input := range in.GetId() {
		if input == nil || input.GetId() == "" {
			return nil, mtproto.ErrInputConstructorInvalid
		}
	}
	balance, err := domain.StarsBalance(uid)
	if err != nil {
		return nil, starsDomainError(err)
	}
	history := make([]*mtproto.StarsTransaction, 0, len(in.GetId()))
	for _, input := range in.GetId() {
		item, ok, getErr := domain.GetStarsTransaction(uid, input.GetId())
		if getErr != nil {
			return nil, starsDomainError(getErr)
		}
		if ok {
			history = append(history, makeStarsTransaction(item))
		}
	}
	return makeStarsStatus(balance, history, nil), nil
}

func (c *ApiFullCore) PaymentsGetStarsGiftOptions(in *mtproto.TLPaymentsGetStarsGiftOptions) (*mtproto.Vector_StarsGiftOption, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetUserId() == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	// The recipient is part of the offer request, but no recipient inventory
	// is exposed. Accept a well-formed self/user reference so gifting another
	// account does not require reading that account's private ledger.
	inputUser := in.GetUserId()
	if inputUser.GetPredicateName() == mtproto.Predicate_inputUser && inputUser.GetAccessHash() == 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	peer := mtproto.FromInputUser(uid, inputUser)
	if peer == nil || (peer.PeerType != mtproto.PEER_SELF && peer.PeerType != mtproto.PEER_USER) || peer.PeerId <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	offers, err := domain.ListStarsOffers("gift")
	if err != nil {
		return nil, starsDomainError(err)
	}
	if len(offers) == 0 {
		return nil, mtproto.ErrMethodNotImpl
	}
	result := make([]*mtproto.StarsGiftOption, 0, len(offers))
	for _, offer := range offers {
		result = append(result, mtproto.MakeTLStarsGiftOption(&mtproto.StarsGiftOption{
			Extended:     offer.Extended,
			Stars:        offer.Stars,
			StoreProduct: optionalString(offer.StoreProduct),
			Currency:     offer.Currency,
			Amount:       offer.Amount,
		}).To_StarsGiftOption())
	}
	return &mtproto.Vector_StarsGiftOption{Datas: result}, nil
}

func optionalString(value string) *wrapperspb.StringValue {
	if value == "" {
		return nil
	}
	return wrapperspb.String(value)
}

func validStarsPeer(selfID int64, peer *mtproto.InputPeer) bool {
	if peer == nil {
		return true
	}
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		return true
	case mtproto.Predicate_inputPeerUser:
		return peer.GetUserId() == selfID
	default:
		return false
	}
}

func parseStarsOffset(offset string) (int, error) {
	if offset == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(offset)
	if err != nil || n < 0 {
		return 0, mtproto.ErrOffsetInvalid
	}
	return n, nil
}

func makeStarsTransaction(item domain.StarsTransaction) *mtproto.StarsTransaction {
	return mtproto.MakeTLStarsTransaction(&mtproto.StarsTransaction{
		Id:          item.Idem,
		Amount:      mtproto.MakeTLStarsAmount(&mtproto.StarsAmount{Amount: item.Amount}).To_StarsAmount(),
		Stars_INT64: item.Amount,
	}).To_StarsTransaction()
}

func starsDomainError(err error) error {
	if err != nil && !domain.Ready() {
		return mtproto.ErrMethodNotImpl
	}
	return err
}

func starsPage(offset string, limit int32) (int, int, error) {
	start, err := parseStarsOffset(offset)
	if err != nil {
		return 0, 0, err
	}
	if limit < 0 {
		return 0, 0, mtproto.ErrLimitInvalid
	}
	if limit == 0 {
		limit = 100
	}
	return start, int(limit), nil
}

func makeStarsStatus(balance int64, history []*mtproto.StarsTransaction, nextOffset *wrapperspb.StringValue) *mtproto.Payments_StarsStatus {
	return mtproto.MakeTLPaymentsStarsStatus(&mtproto.Payments_StarsStatus{
		Balance_STARSAMOUNT: mtproto.MakeTLStarsAmount(&mtproto.StarsAmount{Amount: balance}).To_StarsAmount(),
		Balance_INT64:       balance,
		Subscriptions:       []*mtproto.StarsSubscription{},
		History:             history,
		NextOffset:          nextOffset,
		Chats:               []*mtproto.Chat{},
		Users:               []*mtproto.User{},
	}).To_Payments_StarsStatus()
}
