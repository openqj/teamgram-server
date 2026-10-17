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
	"net/http"
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// starsSubscriptionsProviderResponse is the signed provider contract for the
// recurring Stars subscription methods. Subscription state is owned by the
// payment provider; APIFull only validates and forwards the signed result.
type starsSubscriptionsProviderResponse struct {
	Verified       bool                         `json:"verified"`
	UserID         int64                        `json:"user_id"`
	Balance        *int64                       `json:"balance,omitempty"`
	Subscriptions  []*mtproto.StarsSubscription `json:"subscriptions"`
	NextOffset     *wrapperspb.StringValue      `json:"next_offset,omitempty"`
	MissingBalance *wrapperspb.Int64Value       `json:"missing_balance,omitempty"`
}

type starsSubscriptionMutationProviderResponse struct {
	Verified       bool          `json:"verified"`
	CallerUserID   int64         `json:"caller_user_id"`
	UserID         int64         `json:"user_id,omitempty"`
	SubscriptionID string        `json:"subscription_id"`
	Result         *mtproto.Bool `json:"result"`
}

// validRequiredStarsPeer matches the mandatory InputPeer fields on the
// subscription methods. validStarsPeer is shared with methods whose peer is
// optional, so it intentionally accepts nil; subscription RPCs must reject
// nil and cannot accept an inputPeerUser without its required access hash.
func validRequiredStarsPeer(selfID int64, peer *mtproto.InputPeer) bool {
	if peer == nil || !validStarsPeer(selfID, peer) {
		return false
	}
	if peer.GetPredicateName() == mtproto.Predicate_inputPeerUser {
		return peer.GetAccessHash() != 0
	}
	return true
}

// callStarsSubscriptionMutationProvider gives each authoritative mutation a
// stable idempotency key. The provider must deduplicate this key before it
// changes a recurring charge, and every response remains HMAC authenticated.
func (c *ApiFullCore) callStarsSubscriptionMutationProvider(ctx context.Context, payload any, idempotencyKey string, response any) error {
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
	c.setPaymentProviderRequestSignature(req, body)
	req.Header.Set("Idempotency-Key", paymentProviderIdempotencyKey(c.MD.GetUserId(), idempotencyKey))
	if providerKey != "" {
		req.Header.Set("Authorization", "Bearer "+providerKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return mtproto.ErrPaymentUnsupported
	}
	defer resp.Body.Close()
	responseBody, err := readPaymentProviderBody(resp)
	if err != nil {
		return err
	}
	if !c.paymentProviderResponseAuthenticated(resp, responseBody) {
		return mtproto.ErrPaymentProviderInvalid
	}
	if err = json.Unmarshal(responseBody, response); err != nil {
		return mtproto.ErrPaymentProviderInvalid
	}
	return nil
}

func normalizeStarsSubscription(subscription *mtproto.StarsSubscription) bool {
	if subscription == nil || strings.TrimSpace(subscription.GetId()) == "" || subscription.GetUntilDate() < 0 {
		return false
	}
	if subscription.GetPredicateName() == "" {
		mtproto.MakeTLStarsSubscription(subscription).To_StarsSubscription()
	}
	if subscription.GetPredicateName() != mtproto.Predicate_starsSubscription {
		return false
	}
	pricing := subscription.GetPricing()
	if pricing == nil || pricing.GetPeriod() <= 0 || pricing.GetAmount() < 0 {
		return false
	}
	if pricing.GetPredicateName() == "" {
		mtproto.MakeTLStarsSubscriptionPricing(pricing).To_StarsSubscriptionPricing()
	}
	if pricing.GetPredicateName() != mtproto.Predicate_starsSubscriptionPricing {
		return false
	}
	if peer := subscription.GetPeer(); peer != nil {
		if peer.GetUserId() <= 0 && peer.GetChatId() <= 0 && peer.GetChannelId() <= 0 {
			return false
		}
	}
	if title := subscription.GetTitle(); title != nil && strings.TrimSpace(title.GetValue()) == "" {
		return false
	}
	if slug := subscription.GetInvoiceSlug(); slug != nil && strings.TrimSpace(slug.GetValue()) == "" {
		return false
	}
	return true
}

func (c *ApiFullCore) starsSubscriptionStatus(uid int64, in *mtproto.TLPaymentsGetStarsSubscriptions) (*mtproto.Payments_StarsStatus, error) {
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if !validRequiredStarsPeer(uid, in.GetPeer()) {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if _, err := parseStarsOffset(in.GetOffset()); err != nil {
		return nil, err
	}
	var response starsSubscriptionsProviderResponse
	if err := c.callStarsRevenueProvider(c.secretContext(), struct {
		Operation      string             `json:"operation"`
		UserID         int64              `json:"user_id"`
		MissingBalance bool               `json:"missing_balance"`
		Peer           *mtproto.InputPeer `json:"peer,omitempty"`
		Offset         string             `json:"offset,omitempty"`
	}{"get_stars_subscriptions", uid, in.GetMissingBalance(), in.GetPeer(), in.GetOffset()}, &response); err != nil {
		return nil, err
	}
	if !response.Verified || response.UserID != uid || response.Balance == nil || *response.Balance < 0 {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	seen := make(map[string]struct{}, len(response.Subscriptions))
	for _, subscription := range response.Subscriptions {
		if !normalizeStarsSubscription(subscription) {
			return nil, mtproto.ErrPaymentProviderInvalid
		}
		if _, ok := seen[subscription.GetId()]; ok {
			return nil, mtproto.ErrPaymentProviderInvalid
		}
		seen[subscription.GetId()] = struct{}{}
	}
	if response.NextOffset != nil && strings.TrimSpace(response.NextOffset.GetValue()) == "" {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	if response.MissingBalance != nil && response.MissingBalance.GetValue() < 0 {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	status := makeStarsStatus(*response.Balance, nil, nil)
	status.Subscriptions = response.Subscriptions
	status.SubscriptionsNextOffset = response.NextOffset
	status.SubscriptionsMissingBalance = response.MissingBalance
	return status, nil
}

func (c *ApiFullCore) starSubscriptionMutation(uid int64, operation, subscriptionID string, peer *mtproto.InputPeer, canceled *mtproto.Bool) (*mtproto.Bool, error) {
	if strings.TrimSpace(subscriptionID) == "" {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if !validRequiredStarsPeer(uid, peer) {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if canceled != nil && canceled.GetPredicateName() != mtproto.Predicate_boolTrue && canceled.GetPredicateName() != mtproto.Predicate_boolFalse {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	var response starsSubscriptionMutationProviderResponse
	requestKey := "stars-subscription:" + operation + ":" + strings.TrimSpace(subscriptionID)
	if err := c.callStarsSubscriptionMutationProvider(c.secretContext(), struct {
		Operation      string             `json:"operation"`
		CallerUserID   int64              `json:"caller_user_id"`
		UserID         int64              `json:"user_id"`
		SubscriptionID string             `json:"subscription_id"`
		Peer           *mtproto.InputPeer `json:"peer,omitempty"`
		Canceled       *mtproto.Bool      `json:"canceled,omitempty"`
	}{operation, uid, uid, strings.TrimSpace(subscriptionID), peer, canceled}, requestKey, &response); err != nil {
		return nil, err
	}
	if !response.Verified || response.CallerUserID != uid || (response.UserID != 0 && response.UserID != uid) || response.SubscriptionID != strings.TrimSpace(subscriptionID) || response.Result == nil {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	if response.Result.GetPredicateName() != mtproto.Predicate_boolTrue && response.Result.GetPredicateName() != mtproto.Predicate_boolFalse {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	return response.Result, nil
}

func (c *ApiFullCore) PaymentsGetStarsSubscriptions(in *mtproto.TLPaymentsGetStarsSubscriptions) (*mtproto.Payments_StarsStatus, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	return c.starsSubscriptionStatus(uid, in)
}

func (c *ApiFullCore) PaymentsChangeStarsSubscription(in *mtproto.TLPaymentsChangeStarsSubscription) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetCanceled() == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	return c.starSubscriptionMutation(uid, "change_stars_subscription", in.GetSubscriptionId(), in.GetPeer(), in.GetCanceled())
}

func (c *ApiFullCore) PaymentsFulfillStarsSubscription(in *mtproto.TLPaymentsFulfillStarsSubscription) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	return c.starSubscriptionMutation(uid, "fulfill_stars_subscription", in.GetSubscriptionId(), in.GetPeer(), nil)
}

func (c *ApiFullCore) PaymentsBotCancelStarsSubscription(in *mtproto.TLPaymentsBotCancelStarsSubscription) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetUserId() == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	target := in.GetUserId()
	if target.GetUserId() <= 0 || strings.TrimSpace(in.GetChargeId_STRING()) == "" && (in.GetChargeId_FLAGSTRING() == nil || strings.TrimSpace(in.GetChargeId_FLAGSTRING().GetValue()) == "") {
		return nil, mtproto.ErrInputUserDeactivated
	}
	chargeID := strings.TrimSpace(in.GetChargeId_STRING())
	if chargeID == "" {
		chargeID = strings.TrimSpace(in.GetChargeId_FLAGSTRING().GetValue())
	}
	var response starsSubscriptionMutationProviderResponse
	requestKey := "stars-subscription:bot-cancel:" + strconv.FormatInt(target.GetUserId(), 10) + ":" + chargeID
	if err := c.callStarsSubscriptionMutationProvider(c.secretContext(), struct {
		Operation    string                  `json:"operation"`
		CallerUserID int64                   `json:"caller_user_id"`
		UserID       int64                   `json:"user_id"`
		Restore      bool                    `json:"restore"`
		TargetUser   *mtproto.InputUser      `json:"target_user"`
		ChargeID     string                  `json:"charge_id"`
		InvoiceSlug  *wrapperspb.StringValue `json:"invoice_slug,omitempty"`
	}{"bot_cancel_stars_subscription", uid, target.GetUserId(), in.GetRestore(), target, chargeID, in.GetInvoiceSlug()}, requestKey, &response); err != nil {
		return nil, err
	}
	if !response.Verified || response.CallerUserID != uid || response.UserID != target.GetUserId() || response.SubscriptionID != chargeID || response.Result == nil {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	if response.Result.GetPredicateName() != mtproto.Predicate_boolTrue && response.Result.GetPredicateName() != mtproto.Predicate_boolFalse {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	return response.Result, nil
}
