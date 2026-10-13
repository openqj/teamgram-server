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
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/paymentinvoice"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/providercontract"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RPCPaymentsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.
// pay: is this server's own JSON ledger, not a charge against an external processor.

const payLedgerPrefix = "pay:"
const paymentProductPremiumSubscription = "premium_subscription"

type payLedgerEvent struct {
	Op  string `json:"op"`
	UID int64  `json:"uid"`
}

func recordPay(uid int64, op string) error {
	key := fmt.Sprintf("%s%d", payLedgerPrefix, uid)
	raw, err := persist.Default.Get(key)
	if err != nil {
		return err
	}
	var events []payLedgerEvent
	if raw != "" {
		if err = json.Unmarshal([]byte(raw), &events); err != nil {
			events = nil
		}
	}
	events = append(events, payLedgerEvent{Op: op, UID: uid})
	b, err := json.Marshal(events)
	if err != nil {
		return err
	}
	return persist.Default.Set(key, string(b))
}

func loadPayNote(uid int64) (string, error) {
	raw, err := persist.Default.Get(fmt.Sprintf("%s%d", payLedgerPrefix, uid))
	if err != nil || raw == "" || raw[0] == '[' || raw[0] == '{' {
		return "", err
	}
	return raw, nil
}

func payInfoKey(uid int64) string    { return fmt.Sprintf("pay:info:%d", uid) }
func payCredKey(uid int64) string    { return fmt.Sprintf("pay:cred:%d", uid) }
func payReceiptKey(uid int64) string { return fmt.Sprintf("pay:receipt:%d", uid) }

type savedPayInfo struct {
	Name  string `json:"name,omitempty"`
	Phone string `json:"phone,omitempty"`
	Email string `json:"email,omitempty"`
}

type payReceipt struct {
	Title    string `json:"title,omitempty"`
	Currency string `json:"currency,omitempty"`
	MsgID    int32  `json:"msg_id,omitempty"`
	FormID   int64  `json:"form_id,omitempty"`
	Peer     int64  `json:"peer,omitempty"`
	Date     int32  `json:"date,omitempty"`
}

// Payment provider responses are authenticated with an HMAC over the exact
// response body, carried in X-Teamgram-Payment-Signature as a lowercase hex
// SHA-256 digest. The processor must also mark transactions verified.
type paymentProviderResponse struct {
	Verified          bool            `json:"verified"`
	UserID            int64           `json:"user_id"`
	BeneficiaryUserID int64           `json:"beneficiary_user_id"`
	BotUserID         int64           `json:"bot_user_id,omitempty"`
	RequestKey        string          `json:"request_key"`
	Fingerprint       string          `json:"fingerprint"`
	FormID            int64           `json:"form_id"`
	ProductType       string          `json:"product_type"`
	PremiumMonths     int32           `json:"premium_months"`
	OfferID           int64           `json:"offer_id,omitempty"`
	Stars             int64           `json:"stars,omitempty"`
	StoreProduct      string          `json:"store_product,omitempty"`
	TransactionID     string          `json:"transaction_id"`
	Currency          string          `json:"currency"`
	Amount            int64           `json:"amount"`
	Title             string          `json:"title"`
	Receipt           json.RawMessage `json:"receipt"`
	Verification      string          `json:"verification_url,omitempty"`
}

type paymentProviderReceiptEnvelope struct {
	Signature    string `json:"signature"`
	ResponseBody []byte `json:"response_body"`
}

// paymentFormProviderResponse is returned by the payment provider for the
// form discovery operation. The provider is authoritative for the form
// metadata; APIFull only forwards a verified, structurally encodable form.
type paymentFormProviderResponse struct {
	Verified          bool                          `json:"verified"`
	UserID            int64                         `json:"user_id"`
	BeneficiaryUserID int64                         `json:"beneficiary_user_id"`
	ProductType       string                        `json:"product_type"`
	PremiumMonths     int32                         `json:"premium_months"`
	OfferID           int64                         `json:"offer_id,omitempty"`
	Stars             int64                         `json:"stars,omitempty"`
	StoreProduct      string                        `json:"store_product,omitempty"`
	Currency          string                        `json:"currency,omitempty"`
	Amount            int64                         `json:"amount,omitempty"`
	Form              *mtproto.Payments_PaymentForm `json:"form"`
}

type paymentBotInvoiceFormProviderResponse struct {
	Verified          bool                          `json:"verified"`
	UserID            int64                         `json:"user_id"`
	BeneficiaryUserID int64                         `json:"beneficiary_user_id"`
	BotUserID         int64                         `json:"bot_user_id"`
	ProductType       string                        `json:"product_type"`
	Form              *mtproto.Payments_PaymentForm `json:"form"`
}

type paymentInvoiceMessage struct {
	Peer      *mtproto.InputPeer
	MsgID     int32
	BotUserID int64
	Metadata  paymentinvoice.Metadata
	Invoice   *mtproto.Invoice
	Media     *mtproto.MessageMedia
}

// botPaymentContext is the canonical, durable context for one invoice
// validation. Bot callbacks arrive on a different RPC and may be delivered
// after APIFull restarts, so the update payload cannot depend on request
// memory. InvoicePayload remains the exact bytes supplied by the bot; the
// remaining fields are the signed invoice/request metadata needed to build
// updateBotPrecheckoutQuery and to settle the provider charge.
type botPaymentContext struct {
	Version          int                           `json:"version"`
	QueryID          int64                         `json:"query_id"`
	BuyerUserID      int64                         `json:"buyer_user_id"`
	BotUserID        int64                         `json:"bot_user_id"`
	Peer             *mtproto.InputPeer            `json:"peer"`
	MsgID            int32                         `json:"msg_id"`
	InvoicePayload   []byte                        `json:"invoice_payload"`
	Provider         string                        `json:"provider"`
	ProviderData     string                        `json:"provider_data"`
	ProviderInfoID   string                        `json:"provider_info_id,omitempty"`
	Invoice          *mtproto.Invoice              `json:"invoice"`
	Info             *mtproto.PaymentRequestedInfo `json:"info,omitempty"`
	Currency         string                        `json:"currency"`
	Amount           int64                         `json:"amount"`
	ShippingOptionID string                        `json:"shipping_option_id,omitempty"`
	TipAmount        int64                         `json:"tip_amount,omitempty"`
}

const botPaymentContextVersion = 1

func encodeBotPaymentContext(value botPaymentContext) ([]byte, error) {
	if value.Version == 0 {
		value.Version = botPaymentContextVersion
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var compact bytes.Buffer
	if err = json.Compact(&compact, encoded); err != nil {
		return nil, err
	}
	return compact.Bytes(), nil
}

func decodeBotPaymentContext(raw []byte) (botPaymentContext, error) {
	var value botPaymentContext
	if len(bytes.TrimSpace(raw)) == 0 || json.Unmarshal(raw, &value) != nil ||
		value.Version != botPaymentContextVersion || value.QueryID <= 0 ||
		value.BuyerUserID <= 0 || value.BotUserID <= 0 || value.MsgID <= 0 ||
		value.Peer == nil || value.Invoice == nil || len(value.InvoicePayload) == 0 ||
		strings.TrimSpace(value.Currency) == "" || value.Amount < 0 || value.TipAmount < 0 {
		return botPaymentContext{}, mtproto.ErrInvoicePayloadInvalid
	}
	if value.Info != nil {
		if value.Info.GetPredicateName() == "" {
			mtproto.MakeTLPaymentRequestedInfo(value.Info).To_PaymentRequestedInfo()
		}
		if value.Info.GetShippingAddress() != nil && value.Info.GetShippingAddress().GetPredicateName() == "" {
			mtproto.MakeTLPostAddress(value.Info.GetShippingAddress()).To_PostAddress()
		}
	}
	if value.Invoice.GetPredicateName() == "" {
		mtproto.MakeTLInvoice(value.Invoice).To_Invoice()
	}
	return value, nil
}

func botInvoiceTotal(invoice *mtproto.Invoice) (int64, error) {
	if invoice == nil || strings.TrimSpace(invoice.GetCurrency()) == "" {
		return 0, mtproto.ErrInvoicePayloadInvalid
	}
	var total int64
	for _, price := range invoice.GetPrices() {
		if price == nil || price.GetAmount() < 0 || total > int64(^uint64(0)>>1)-price.GetAmount() {
			return 0, mtproto.ErrInvoicePayloadInvalid
		}
		total += price.GetAmount()
	}
	return total, nil
}

func botPaymentQueryID(uid, botID int64, peer *mtproto.InputPeer, msgID int32, providerInfoID string, info *mtproto.PaymentRequestedInfo, payload []byte) int64 {
	if info == nil {
		info = &mtproto.PaymentRequestedInfo{}
		mtproto.MakeTLPaymentRequestedInfo(info).To_PaymentRequestedInfo()
	}
	seed, err := json.Marshal(struct {
		UID, BotID int64
		Peer       *mtproto.InputPeer
		MsgID      int32
		ProviderID string
		Info       *mtproto.PaymentRequestedInfo
		Payload    []byte
	}{uid, botID, peer, msgID, strings.TrimSpace(providerInfoID), info, payload})
	if err != nil {
		seed = []byte(fmt.Sprintf("%d:%d:%d:%s", uid, botID, msgID, providerInfoID))
	}
	digest := sha256.Sum256(seed)
	queryID := int64(binary.BigEndian.Uint64(digest[:8]) & uint64(^uint64(0)>>1))
	if queryID == 0 {
		queryID = 1
	}
	return queryID
}

// paymentValidationProviderResponse is the provider's verified result for
// invoice contact/shipping validation. Both returned fields are optional in
// the MTProto result; APIFull only validates them when the provider supplies
// them and never infers an ID or prices.
type paymentValidationProviderResponse struct {
	Verified          bool                      `json:"verified"`
	UserID            int64                     `json:"user_id"`
	BeneficiaryUserID int64                     `json:"beneficiary_user_id"`
	BotUserID         int64                     `json:"bot_user_id,omitempty"`
	ProductType       string                    `json:"product_type"`
	ID                string                    `json:"id"`
	ShippingOptions   []*mtproto.ShippingOption `json:"shipping_options"`
}

// paymentBankCardProviderResponse is the signed, provider-owned BIN/card
// metadata used by payments.getBankCardData. APIFull never stores the number
// or any card data; it only validates and forwards the signed response.
type paymentBankCardProviderResponse struct {
	Verified bool                           `json:"verified"`
	UserID   int64                          `json:"user_id"`
	Number   string                         `json:"number"`
	Data     *mtproto.Payments_BankCardData `json:"data"`
}

type paymentExportInvoiceProviderResponse struct {
	Verified bool   `json:"verified"`
	UserID   int64  `json:"user_id"`
	URL      string `json:"url"`
}

func normalizeBankCardData(data *mtproto.Payments_BankCardData) bool {
	if data == nil {
		return false
	}
	if data.GetPredicateName() == "" {
		data.To_PaymentsBankCardData()
	}
	if data.GetPredicateName() != mtproto.Predicate_payments_bankCardData {
		return false
	}
	for _, openURL := range data.GetOpenUrls() {
		if openURL == nil || strings.TrimSpace(openURL.GetUrl()) == "" {
			return false
		}
		parsed, err := url.Parse(strings.TrimSpace(openURL.GetUrl()))
		if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
			return false
		}
		if openURL.GetPredicateName() == "" {
			mtproto.MakeTLBankCardOpenUrl(openURL).To_BankCardOpenUrl()
		}
		if openURL.GetPredicateName() != mtproto.Predicate_bankCardOpenUrl {
			return false
		}
	}
	return true
}

func validPaymentShippingOptions(options []*mtproto.ShippingOption) bool {
	seen := make(map[string]struct{}, len(options))
	for _, option := range options {
		if option == nil || strings.TrimSpace(option.GetId()) == "" {
			return false
		}
		if _, ok := seen[option.GetId()]; ok {
			return false
		}
		seen[option.GetId()] = struct{}{}
		var optionTotal int64
		for _, price := range option.GetPrices() {
			if price == nil || price.GetAmount() < 0 || optionTotal > int64(^uint64(0)>>1)-price.GetAmount() {
				return false
			}
			optionTotal += price.GetAmount()
		}
	}
	return true
}

// normalizePaymentInvoice fills the generated TL union markers omitted by a
// plain JSON provider response. The markers are required by the MTProto
// encoder; they do not change any provider-supplied values.
func normalizePaymentInvoice(invoice *mtproto.Invoice) bool {
	if invoice == nil {
		return false
	}
	if invoice.GetPredicateName() == "" {
		mtproto.MakeTLInvoice(invoice).To_Invoice()
	}
	for _, price := range invoice.GetPrices() {
		if price == nil {
			return false
		}
		if price.GetPredicateName() == "" {
			mtproto.MakeTLLabeledPrice(price).To_LabeledPrice()
		}
	}
	return true
}

func normalizePaymentShippingOptions(options []*mtproto.ShippingOption) bool {
	for _, option := range options {
		if option == nil {
			return false
		}
		if option.GetPredicateName() == "" {
			mtproto.MakeTLShippingOption(option).To_ShippingOption()
		}
		for _, price := range option.GetPrices() {
			if price == nil {
				return false
			}
			if price.GetPredicateName() == "" {
				mtproto.MakeTLLabeledPrice(price).To_LabeledPrice()
			}
		}
	}
	return true
}

// paymentFormMatchesInvoice prevents a provider from swapping the product
// after the client asked for a concrete store-payment purpose. Slug-based
// invoices do not carry an amount locally, so those are checked for structure
// only and remain authoritative at the provider.
func paymentFormMatchesInvoice(request *mtproto.InputInvoice, form *mtproto.Payments_PaymentForm) bool {
	if request == nil || form == nil || form.GetInvoice() == nil {
		return false
	}
	purpose := request.GetPurpose()
	if purpose == nil {
		return true
	}
	invoice := form.GetInvoice()
	if currency := strings.TrimSpace(purpose.GetCurrency()); currency != "" && invoice.GetCurrency() != currency {
		return false
	}
	if purpose.GetAmount() < 0 {
		return false
	}
	var total int64
	for _, price := range invoice.GetPrices() {
		if price == nil {
			return false
		}
		amount := price.GetAmount()
		if amount < 0 || total > int64(^uint64(0)>>1)-amount {
			return false
		}
		total += amount
	}
	if purpose.GetAmount() == 0 {
		return true
	}
	return total == purpose.GetAmount()
}

func sameBotPaymentPeer(left, right *mtproto.InputPeer) bool {
	if left == nil || right == nil {
		return left == right
	}
	leftPredicate, rightPredicate := left.GetPredicateName(), right.GetPredicateName()
	if leftPredicate == "" {
		leftPredicate = fmt.Sprintf("constructor:%d", left.GetConstructor())
	}
	if rightPredicate == "" {
		rightPredicate = fmt.Sprintf("constructor:%d", right.GetConstructor())
	}
	return leftPredicate == rightPredicate && left.GetChatId() == right.GetChatId() &&
		left.GetUserId() == right.GetUserId() && left.GetChannelId() == right.GetChannelId() &&
		left.GetAccessHash() == right.GetAccessHash() && left.GetUsername() == right.GetUsername()
}

func sameBotInvoice(left, right *mtproto.Invoice) bool {
	if left == nil || right == nil || left.GetCurrency() != right.GetCurrency() ||
		left.GetTest() != right.GetTest() || left.GetShippingAddressRequested() != right.GetShippingAddressRequested() ||
		len(left.GetPrices()) != len(right.GetPrices()) {
		return false
	}
	for i, price := range left.GetPrices() {
		other := right.GetPrices()[i]
		if price == nil || other == nil || price.GetLabel() != other.GetLabel() || price.GetAmount() != other.GetAmount() {
			return false
		}
	}
	return true
}

func botPaymentRequestID(in *mtproto.TLPaymentsSendPaymentForm) (int64, error) {
	if in == nil || in.GetRequestedInfoId() == nil || strings.TrimSpace(in.GetRequestedInfoId().GetValue()) == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(strings.TrimSpace(in.GetRequestedInfoId().GetValue()), 10, 64)
	if err != nil || value <= 0 {
		return 0, mtproto.ErrInvoicePayloadInvalid
	}
	return value, nil
}

func botPaymentFingerprint(in *mtproto.TLPaymentsSendPaymentForm, queryID int64, value botPaymentContext) string {
	if in == nil || in.GetInvoice() == nil || value.QueryID <= 0 {
		return ""
	}
	body, err := json.Marshal(struct {
		FormID            int64
		Invoice           *mtproto.InputInvoice
		QueryID           int64
		ProviderInfoID    string
		ShippingOptionID  string
		TipAmount         int64
		Provider          string
		ProviderData      string
		InvoicePayload    []byte
		RequestedInfoHash string
	}{
		FormID: in.GetFormId(), Invoice: in.GetInvoice(), QueryID: queryID,
		ProviderInfoID: value.ProviderInfoID, ShippingOptionID: value.ShippingOptionID,
		TipAmount: value.TipAmount, Provider: value.Provider, ProviderData: value.ProviderData,
		InvoicePayload:    value.InvoicePayload,
		RequestedInfoHash: fmt.Sprintf("%d:%d:%d", value.BuyerUserID, value.BotUserID, value.MsgID),
	})
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func botPaymentRequestKey(uid, queryID int64, fingerprint string) string {
	if uid <= 0 || queryID <= 0 || strings.TrimSpace(fingerprint) == "" {
		return ""
	}
	return fmt.Sprintf("bot:%d:%d:%s", uid, queryID, fingerprint)
}

func botPaymentProviderName(value botPaymentContext) (string, bool) {
	provider := strings.TrimSpace(value.Provider)
	return provider, provider != "" && len(provider) <= 32
}

func shippingOptionTotal(raw []byte, optionID string) (int64, bool, error) {
	if strings.TrimSpace(optionID) == "" {
		return 0, false, nil
	}
	var options []*mtproto.ShippingOption
	if len(raw) == 0 || json.Unmarshal(raw, &options) != nil || !validPaymentShippingOptions(options) {
		return 0, false, mtproto.ErrPaymentProviderInvalid
	}
	for _, option := range options {
		if option.GetId() != optionID {
			continue
		}
		var total int64
		for _, price := range option.GetPrices() {
			if total > int64(^uint64(0)>>1)-price.GetAmount() {
				return 0, false, mtproto.ErrPaymentProviderInvalid
			}
			total += price.GetAmount()
		}
		return total, true, nil
	}
	return 0, false, nil
}

func botPaymentExpectedAmount(value botPaymentContext, options []byte) (int64, error) {
	base, err := botInvoiceTotal(value.Invoice)
	if err != nil {
		return 0, err
	}
	if value.Amount < base || value.TipAmount < 0 {
		return 0, mtproto.ErrPaymentProviderInvalid
	}
	shipping := value.Amount - base - value.TipAmount
	if shipping < 0 {
		return 0, mtproto.ErrPaymentProviderInvalid
	}
	if strings.TrimSpace(value.ShippingOptionID) == "" {
		if shipping != 0 {
			return 0, mtproto.ErrPaymentProviderInvalid
		}
		return base + value.TipAmount, nil
	}
	optionTotal, found, err := shippingOptionTotal(options, value.ShippingOptionID)
	if err != nil || !found || shipping != optionTotal || base > int64(^uint64(0)>>1)-optionTotal || base+optionTotal > int64(^uint64(0)>>1)-value.TipAmount {
		return 0, mtproto.ErrPaymentProviderInvalid
	}
	return base + optionTotal + value.TipAmount, nil
}

func (c *ApiFullCore) configuredPaymentProvider() (*http.Client, string, string, error) {
	if c == nil || c.svcCtx == nil {
		return nil, "", "", mtproto.ErrPaymentUnsupported
	}
	endpoint := strings.TrimSpace(c.svcCtx.Config.PaymentProviderEndpoint)
	if endpoint == "" {
		return nil, "", "", mtproto.ErrPaymentUnsupported
	}
	if strings.TrimSpace(c.svcCtx.Config.PaymentProviderSigningKey) == "" {
		return nil, "", "", mtproto.ErrPaymentProviderInvalid
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !securePaymentProviderURL(parsed, c.svcCtx.Config.AllowInsecureProviderHTTP) {
		return nil, "", "", mtproto.ErrPaymentProviderInvalid
	}
	if c.svcCtx.Config.PaymentProviderTimeoutSeconds < 0 || c.svcCtx.Config.PaymentProviderTimeoutSeconds > 300 {
		return nil, "", "", mtproto.ErrPaymentProviderInvalid
	}
	timeout := 5 * time.Second
	if c.svcCtx.Config.PaymentProviderTimeoutSeconds > 0 {
		timeout = time.Duration(c.svcCtx.Config.PaymentProviderTimeoutSeconds) * time.Second
	}
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, endpoint, c.svcCtx.Config.PaymentProviderKey, nil
}

func securePaymentProviderURL(parsed *url.URL, allowInsecureHTTP bool) bool {
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

func verifyPaymentProviderSignature(key, signature string, body []byte) bool {
	return providercontract.VerifySignature(key, signature, body)
}

func paymentProviderRequestSignature(key string, body []byte) string {
	return providercontract.Signature(key, body)
}

func (c *ApiFullCore) setPaymentProviderRequestSignature(req *http.Request, body []byte) {
	if c == nil || c.svcCtx == nil || req == nil {
		return
	}
	if key := strings.TrimSpace(c.svcCtx.Config.PaymentProviderSigningKey); key != "" {
		providercontract.SetRequestHeaders(req, body, "", "", "X-Teamgram-Payment-Provider-Request-Signature", key)
	}
}

func (c *ApiFullCore) paymentProviderResponseAuthenticated(resp *http.Response, body []byte) bool {
	return c != nil && c.svcCtx != nil && resp != nil && verifyPaymentProviderSignature(
		c.svcCtx.Config.PaymentProviderSigningKey,
		resp.Header.Get("X-Teamgram-Payment-Signature"),
		body,
	)
}

func selfPremiumSubscriptionInvoice(invoice *mtproto.InputInvoice) bool {
	if invoice == nil || invoice.GetPredicateName() != mtproto.Predicate_inputInvoiceSlug {
		return false
	}
	purpose := invoice.GetPurpose()
	// Layer 229 inputInvoiceSlug carries only the slug. The signed provider
	// response establishes the product and self-beneficiary for wire requests.
	return purpose == nil ||
		purpose.GetPredicateName() == mtproto.Predicate_inputStorePaymentPremiumSubscription &&
			!purpose.GetRestore() && !purpose.GetUpgrade()
}

func validPremiumProviderProduct(product string, userID, beneficiaryUserID int64, months int32, expectedUserID int64) bool {
	return product == paymentProductPremiumSubscription && userID == expectedUserID &&
		beneficiaryUserID == expectedUserID && months >= 1 && months <= 36
}

func (c *ApiFullCore) settleWithPaymentProvider(ctx context.Context, uid int64, requestKey, fingerprint string, in *mtproto.TLPaymentsSendPaymentForm, currency string, amount, peerID int64, msgID int32) (domain.PaymentRequest, domain.PaymentReceipt, error) {
	client, endpoint, providerKey, err := c.configuredPaymentProvider()
	if err != nil {
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, err
	}
	payload := struct {
		UserID            int64                            `json:"user_id"`
		BeneficiaryUserID int64                            `json:"beneficiary_user_id"`
		RequiredProduct   string                           `json:"required_product"`
		RequestKey        string                           `json:"request_key"`
		Fingerprint       string                           `json:"fingerprint"`
		FormID            int64                            `json:"form_id"`
		Invoice           *mtproto.InputInvoice            `json:"invoice"`
		Credentials       *mtproto.InputPaymentCredentials `json:"credentials"`
		Currency          string                           `json:"currency"`
		Amount            int64                            `json:"amount"`
		PeerID            int64                            `json:"peer_id"`
		MsgID             int32                            `json:"msg_id"`
	}{
		UserID:            uid,
		BeneficiaryUserID: uid,
		RequiredProduct:   paymentProductPremiumSubscription,
		RequestKey:        requestKey,
		Fingerprint:       fingerprint,
		FormID:            in.GetFormId(),
		Invoice:           in.GetInvoice(),
		Credentials:       in.GetCredentials(),
		Currency:          currency,
		Amount:            amount,
		PeerID:            peerID,
		MsgID:             msgID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrInternalServerError
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrPaymentProviderInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	c.setPaymentProviderRequestSignature(req, body)
	// The settlement bridge must use this stable key to deduplicate concurrent
	// retries before it contacts the external processor.
	req.Header.Set("Idempotency-Key", paymentProviderIdempotencyKey(uid, requestKey))
	if providerKey != "" {
		req.Header.Set("Authorization", "Bearer "+providerKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrPaymentUnsupported
	}
	defer resp.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrPaymentUnsupported
	}
	if !c.paymentProviderResponseAuthenticated(resp, responseBody) {
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrPaymentProviderInvalid
	}
	var result paymentProviderResponse
	if err = json.Unmarshal(responseBody, &result); err != nil {
		if _, rejectErr := domain.RejectPaymentRequest(uid, requestKey, fingerprint, "provider returned invalid payment response"); rejectErr != nil {
			return domain.PaymentRequest{}, domain.PaymentReceipt{}, rejectErr
		}
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrPaymentProviderInvalid
	}
	receipt := bytes.TrimSpace(result.Receipt)
	if !result.Verified || result.UserID != uid || result.BeneficiaryUserID != uid || result.RequestKey != requestKey ||
		result.Fingerprint != fingerprint || result.FormID != in.GetFormId() ||
		!validPremiumProviderProduct(result.ProductType, result.UserID, result.BeneficiaryUserID, result.PremiumMonths, uid) ||
		strings.TrimSpace(result.TransactionID) == "" || len(receipt) == 0 || bytes.Equal(receipt, []byte("null")) {
		if _, rejectErr := domain.RejectPaymentRequest(uid, requestKey, fingerprint, "provider rejected payment"); rejectErr != nil {
			return domain.PaymentRequest{}, domain.PaymentReceipt{}, rejectErr
		}
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrPaymentProviderInvalid
	}
	// Currency and amount are part of the signed provider result. When an
	// invoice carries a purpose, both terms must match it exactly. Slug-based
	// invoices intentionally omit those terms; in that case the provider owns
	// the product quote and must supply non-empty, non-negative values.
	if strings.TrimSpace(result.Currency) == "" || result.Amount < 0 ||
		(currency != "" && (result.Currency != currency || result.Amount != amount)) {
		if _, rejectErr := domain.RejectPaymentRequest(uid, requestKey, fingerprint, "provider response does not match invoice"); rejectErr != nil {
			return domain.PaymentRequest{}, domain.PaymentReceipt{}, rejectErr
		}
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrPaymentProviderInvalid
	}
	storedReceipt, err := json.Marshal(paymentProviderReceiptEnvelope{
		Signature:    resp.Header.Get("X-Teamgram-Payment-Signature"),
		ResponseBody: append([]byte(nil), responseBody...),
	})
	if err != nil {
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrInternalServerError
	}
	return domain.SettlePremiumPaymentRequest(uid, requestKey, fingerprint, result.TransactionID, result.Currency, result.Title, result.Amount, peerID, msgID, storedReceipt, result.PremiumMonths)
}

func starsTopupOfferFromInvoice(invoice *mtproto.InputInvoice) (domain.StarsOffer, bool, error) {
	if invoice == nil || invoice.GetPredicateName() != mtproto.Predicate_inputInvoiceStars {
		return domain.StarsOffer{}, false, mtproto.ErrPaymentUnsupported
	}
	option := invoice.GetOption_STARSTOPUPOPTION()
	purpose := invoice.GetPurpose()
	if option != nil && purpose != nil {
		return domain.StarsOffer{}, false, mtproto.ErrPaymentProviderInvalid
	}
	if option != nil {
		storeProduct := ""
		if option.GetStoreProduct() != nil {
			storeProduct = option.GetStoreProduct().GetValue()
		}
		extended := option.GetExtended()
		return domain.FindActiveStarsTopupOffer(option.GetStars(), storeProduct, option.GetCurrency(), option.GetAmount(), &extended)
	}
	if purpose != nil && purpose.GetPredicateName() == mtproto.Predicate_inputStorePaymentStarsTopup {
		return domain.FindActiveStarsTopupOffer(purpose.GetStars(), "", purpose.GetCurrency(), purpose.GetAmount(), nil)
	}
	return domain.StarsOffer{}, false, mtproto.ErrPaymentUnsupported
}

func starsPaymentFingerprint(formID int64, invoice *mtproto.InputInvoice) string {
	if formID <= 0 || invoice == nil {
		return ""
	}
	body, err := json.Marshal(struct {
		FormID  int64                 `json:"form_id"`
		Invoice *mtproto.InputInvoice `json:"invoice"`
	}{formID, invoice})
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func starsPaymentRequestKey(uid, formID int64, fingerprint string) string {
	if uid <= 0 || formID <= 0 || fingerprint == "" {
		return ""
	}
	return fmt.Sprintf("stars:%d:%d:%s", uid, formID, fingerprint)
}

func validStarsTopupProviderResult(result paymentProviderResponse, uid, formID int64, requestKey, fingerprint string, offer domain.StarsOffer) bool {
	return result.Verified && result.UserID == uid && result.BeneficiaryUserID == uid &&
		result.RequestKey == requestKey && result.Fingerprint == fingerprint && result.FormID == formID &&
		result.ProductType == "stars_topup" && result.OfferID == offer.ID && result.Stars == offer.Stars &&
		(result.StoreProduct == offer.StoreProduct) && (offer.Currency == "" || result.Currency == offer.Currency) &&
		(offer.Amount == 0 || result.Amount == offer.Amount) && strings.TrimSpace(result.Currency) != "" &&
		result.Amount >= 0 && strings.TrimSpace(result.TransactionID) != "" &&
		len(bytes.TrimSpace(result.Receipt)) > 0 && !bytes.Equal(bytes.TrimSpace(result.Receipt), []byte("null"))
}

func (c *ApiFullCore) settleStarsWithPaymentProvider(ctx context.Context, uid int64, requestKey, fingerprint string, in *mtproto.TLPaymentsSendStarsForm, offer domain.StarsOffer) (domain.PaymentRequest, domain.PaymentReceipt, error) {
	client, endpoint, providerKey, err := c.configuredPaymentProvider()
	if err != nil {
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, err
	}
	payload := struct {
		Operation         string                `json:"operation"`
		UserID            int64                 `json:"user_id"`
		BeneficiaryUserID int64                 `json:"beneficiary_user_id"`
		RequiredProduct   string                `json:"required_product"`
		RequestKey        string                `json:"request_key"`
		Fingerprint       string                `json:"fingerprint"`
		FormID            int64                 `json:"form_id"`
		Invoice           *mtproto.InputInvoice `json:"invoice"`
		OfferID           int64                 `json:"offer_id"`
		Offer             domain.StarsOffer     `json:"offer"`
	}{"settle_stars", uid, uid, "stars_topup", requestKey, fingerprint, in.GetFormId(), in.GetInvoice(), offer.ID, offer}
	body, err := json.Marshal(payload)
	if err != nil {
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrInternalServerError
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrPaymentProviderInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	c.setPaymentProviderRequestSignature(req, body)
	req.Header.Set("Idempotency-Key", paymentProviderIdempotencyKey(uid, requestKey))
	if providerKey != "" {
		req.Header.Set("Authorization", "Bearer "+providerKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrPaymentUnsupported
	}
	defer resp.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if readErr != nil || len(responseBody) > 1<<20 || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrPaymentUnsupported
	}
	if !c.paymentProviderResponseAuthenticated(resp, responseBody) {
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrPaymentProviderInvalid
	}
	var result paymentProviderResponse
	if err = json.Unmarshal(responseBody, &result); err != nil {
		if _, rejectErr := domain.RejectPaymentRequest(uid, requestKey, fingerprint, "provider returned invalid Stars payment response"); rejectErr != nil {
			return domain.PaymentRequest{}, domain.PaymentReceipt{}, rejectErr
		}
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrPaymentProviderInvalid
	}
	if !validStarsTopupProviderResult(result, uid, in.GetFormId(), requestKey, fingerprint, offer) {
		if _, rejectErr := domain.RejectPaymentRequest(uid, requestKey, fingerprint, "provider rejected Stars top-up"); rejectErr != nil {
			return domain.PaymentRequest{}, domain.PaymentReceipt{}, rejectErr
		}
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrPaymentProviderInvalid
	}
	storedReceipt, err := json.Marshal(paymentProviderReceiptEnvelope{
		Signature:    resp.Header.Get("X-Teamgram-Payment-Signature"),
		ResponseBody: append([]byte(nil), responseBody...),
	})
	if err != nil {
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrInternalServerError
	}
	return domain.SettleStarsPaymentRequest(uid, requestKey, fingerprint, result.TransactionID, result.Currency, result.Title, result.Amount, result.Stars, storedReceipt)
}

func (c *ApiFullCore) paymentFormFromProvider(ctx context.Context, uid int64, in *mtproto.TLPaymentsGetPaymentForm) (*mtproto.Payments_PaymentForm, error) {
	if in == nil || !selfPremiumSubscriptionInvoice(in.GetInvoice()) {
		return nil, mtproto.ErrPaymentUnsupported
	}
	client, endpoint, providerKey, err := c.configuredPaymentProvider()
	if err != nil {
		return nil, err
	}
	payload := struct {
		Operation         string                `json:"operation"`
		UserID            int64                 `json:"user_id"`
		BeneficiaryUserID int64                 `json:"beneficiary_user_id"`
		RequiredProduct   string                `json:"required_product"`
		Invoice           *mtproto.InputInvoice `json:"invoice"`
		Peer              *mtproto.InputPeer    `json:"peer,omitempty"`
		MsgID             int32                 `json:"msg_id,omitempty"`
		ThemeParams       *mtproto.DataJSON     `json:"theme_params,omitempty"`
	}{"get_form", uid, uid, paymentProductPremiumSubscription, in.GetInvoice(), in.GetPeer(), in.GetMsgId(), in.GetThemeParams()}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	c.setPaymentProviderRequestSignature(req, body)
	if providerKey != "" {
		req.Header.Set("Authorization", "Bearer "+providerKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, mtproto.ErrPaymentUnsupported
	}
	defer resp.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, mtproto.ErrPaymentUnsupported
	}
	if !c.paymentProviderResponseAuthenticated(resp, responseBody) {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	var result paymentFormProviderResponse
	if err = json.Unmarshal(responseBody, &result); err != nil || !result.Verified || result.Form == nil ||
		!validPremiumProviderProduct(result.ProductType, result.UserID, result.BeneficiaryUserID, result.PremiumMonths, uid) {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	// Encode() dereferences Invoice and clients need a stable non-zero form ID
	// to submit the returned form. Reject malformed provider data before it can
	// reach the MTProto encoder.
	if result.Form.GetInvoice() == nil || result.Form.GetFormId() <= 0 || !paymentFormMatchesInvoice(in.GetInvoice(), result.Form) || !normalizePaymentInvoice(result.Form.GetInvoice()) {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	if result.Form.GetPredicateName() != mtproto.Predicate_payments_paymentForm {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	return mtproto.MakeTLPaymentsPaymentForm(result.Form).To_Payments_PaymentForm(), nil
}

func paymentFormMatchesStarsOffer(offer domain.StarsOffer, form *mtproto.Payments_PaymentForm, providerCurrency string, providerAmount int64) bool {
	if form == nil || form.GetInvoice() == nil || !normalizePaymentInvoice(form.GetInvoice()) {
		return false
	}
	expectedCurrency := offer.Currency
	if expectedCurrency == "" {
		expectedCurrency = providerCurrency
	}
	if expectedCurrency != "" && form.GetInvoice().GetCurrency() != expectedCurrency {
		return false
	}
	var total int64
	for _, price := range form.GetInvoice().GetPrices() {
		if price == nil || price.GetAmount() < 0 || total > int64(^uint64(0)>>1)-price.GetAmount() {
			return false
		}
		total += price.GetAmount()
	}
	expectedAmount := offer.Amount
	if expectedAmount == 0 {
		expectedAmount = providerAmount
	}
	return expectedAmount == 0 || total == expectedAmount
}

func (c *ApiFullCore) paymentStarsFormFromProvider(ctx context.Context, uid int64, in *mtproto.TLPaymentsGetPaymentForm, offer domain.StarsOffer) (*mtproto.Payments_PaymentForm, error) {
	client, endpoint, providerKey, err := c.configuredPaymentProvider()
	if err != nil {
		return nil, err
	}
	payload := struct {
		Operation         string                `json:"operation"`
		UserID            int64                 `json:"user_id"`
		BeneficiaryUserID int64                 `json:"beneficiary_user_id"`
		RequiredProduct   string                `json:"required_product"`
		Invoice           *mtproto.InputInvoice `json:"invoice"`
		OfferID           int64                 `json:"offer_id"`
		Offer             domain.StarsOffer     `json:"offer"`
	}{"get_stars_form", uid, uid, "stars_topup", in.GetInvoice(), offer.ID, offer}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	c.setPaymentProviderRequestSignature(req, body)
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
	var result paymentFormProviderResponse
	if json.Unmarshal(responseBody, &result) != nil || !result.Verified || result.Form == nil ||
		result.UserID != uid || result.BeneficiaryUserID != uid || result.ProductType != "stars_topup" ||
		result.OfferID != offer.ID || result.Stars != offer.Stars || result.StoreProduct != offer.StoreProduct ||
		(offer.Currency != "" && result.Currency != offer.Currency) || (offer.Amount > 0 && result.Amount != offer.Amount) || result.Amount < 0 ||
		!paymentFormMatchesStarsOffer(offer, result.Form, result.Currency, result.Amount) || result.Form.GetFormId() <= 0 ||
		result.Form.GetPredicateName() != mtproto.Predicate_payments_paymentFormStars {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	return mtproto.MakeTLPaymentsPaymentFormStars(result.Form).To_Payments_PaymentForm(), nil
}

func (c *ApiFullCore) loadPaymentInvoiceMessage(uid int64, invoice *mtproto.InputInvoice) (*paymentInvoiceMessage, error) {
	if invoice == nil || invoice.GetPredicateName() != mtproto.Predicate_inputInvoiceMessage ||
		invoice.GetPeer() == nil || invoice.GetMsgId() <= 0 || !validThreadInputPeer(uid, invoice.GetPeer()) {
		return nil, mtproto.ErrInvoicePayloadInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.PollMessageReader == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrPaymentUnsupported
	}
	box, err := c.svcCtx.Dao.MessageGetUserMessage(c.secretContext(), &messagepb.TLMessageGetUserMessage{
		UserId: uid,
		Id:     invoice.GetMsgId(),
	})
	if err != nil {
		return nil, err
	}
	if box == nil || box.GetMessage() == nil || box.GetMessage().GetMedia() == nil || box.GetSenderUserId() <= 0 {
		return nil, mtproto.ErrInvoicePayloadInvalid
	}
	requestedType, requestedID := apifullPeerTypeID(uid, invoice.GetPeer())
	if requestedType == mtproto.PEER_SELF {
		requestedType = mtproto.PEER_USER
		requestedID = uid
	}
	if requestedType == 0 || requestedID <= 0 || box.GetPeerId() != requestedID || box.GetPeerType() != requestedType {
		return nil, mtproto.ErrPeerIdInvalid
	}
	isBot, err := c.svcCtx.Dao.UserIsBot(c.secretContext(), &userpb.TLUserIsBot{Id: box.GetSenderUserId()})
	if err != nil {
		return nil, err
	}
	if isBot == nil || !mtproto.FromBool(isBot) {
		return nil, mtproto.ErrBotInvalid
	}
	media := box.GetMessage().GetMedia()
	metadata, storedInvoice, err := paymentinvoice.ParseMetadata(media)
	if err != nil {
		return nil, mtproto.ErrInvoicePayloadInvalid
	}
	return &paymentInvoiceMessage{
		Peer:      invoice.GetPeer(),
		MsgID:     invoice.GetMsgId(),
		BotUserID: box.GetSenderUserId(),
		Metadata:  metadata,
		Invoice:   storedInvoice,
		Media:     media,
	}, nil
}

func (c *ApiFullCore) paymentBotInvoiceFormFromProvider(ctx context.Context, uid int64, in *mtproto.TLPaymentsGetPaymentForm, message *paymentInvoiceMessage) (*mtproto.Payments_PaymentForm, error) {
	if in == nil || message == nil || message.Invoice == nil {
		return nil, mtproto.ErrInvoicePayloadInvalid
	}
	client, endpoint, providerKey, err := c.configuredPaymentProvider()
	if err != nil {
		return nil, err
	}
	payload := struct {
		Operation         string             `json:"operation"`
		UserID            int64              `json:"user_id"`
		BeneficiaryUserID int64              `json:"beneficiary_user_id"`
		BotUserID         int64              `json:"bot_user_id"`
		RequiredProduct   string             `json:"required_product"`
		Peer              *mtproto.InputPeer `json:"peer"`
		MsgID             int32              `json:"msg_id"`
		Payload           []byte             `json:"payload"`
		Provider          string             `json:"provider"`
		ProviderData      string             `json:"provider_data"`
		Title             string             `json:"title"`
		Description       string             `json:"description"`
		Invoice           *mtproto.Invoice   `json:"invoice"`
	}{
		"get_bot_invoice_form", uid, message.BotUserID, message.BotUserID, "bot_invoice", message.Peer,
		message.MsgID, message.Metadata.Payload, message.Metadata.Provider, message.Metadata.ProviderData,
		message.Metadata.Title, message.Metadata.Description, message.Invoice,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	c.setPaymentProviderRequestSignature(req, body)
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
	var result paymentBotInvoiceFormProviderResponse
	if err = json.Unmarshal(responseBody, &result); err != nil || !result.Verified || result.Form == nil ||
		result.ProductType != "bot_invoice" || result.UserID != uid ||
		(result.BotUserID != 0 && result.BotUserID != message.BotUserID) || result.BeneficiaryUserID != message.BotUserID ||
		result.Form.GetBotId() != message.BotUserID || result.Form.GetFormId() <= 0 || result.Form.GetInvoice() == nil {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	if !normalizePaymentInvoice(result.Form.GetInvoice()) || !paymentFormMatchesBotInvoice(message, result.Form) {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	if result.Form.GetPredicateName() != mtproto.Predicate_payments_paymentForm {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	return mtproto.MakeTLPaymentsPaymentForm(result.Form).To_Payments_PaymentForm(), nil
}

func paymentFormMatchesBotInvoice(message *paymentInvoiceMessage, form *mtproto.Payments_PaymentForm) bool {
	if message == nil || message.Media == nil || message.Invoice == nil || form == nil || form.GetInvoice() == nil {
		return false
	}
	if form.GetTitle() != message.Media.GetTitle() || form.GetDescription() != message.Media.GetDescription() ||
		form.GetInvoice().GetCurrency() != message.Invoice.GetCurrency() {
		return false
	}
	var total int64
	for _, price := range form.GetInvoice().GetPrices() {
		if price == nil || price.GetAmount() < 0 || total > int64(^uint64(0)>>1)-price.GetAmount() {
			return false
		}
		total += price.GetAmount()
	}
	return total == message.Media.GetTotalAmount()
}

func (c *ApiFullCore) validateRequestedInfoWithProvider(ctx context.Context, uid int64, in *mtproto.TLPaymentsValidateRequestedInfo) (*mtproto.Payments_ValidatedRequestedInfo, error) {
	if in == nil || !selfPremiumSubscriptionInvoice(in.GetInvoice()) {
		return nil, mtproto.ErrPaymentUnsupported
	}
	client, endpoint, providerKey, err := c.configuredPaymentProvider()
	if err != nil {
		return nil, err
	}
	payload := struct {
		Operation         string                        `json:"operation"`
		UserID            int64                         `json:"user_id"`
		BeneficiaryUserID int64                         `json:"beneficiary_user_id"`
		RequiredProduct   string                        `json:"required_product"`
		Invoice           *mtproto.InputInvoice         `json:"invoice"`
		Info              *mtproto.PaymentRequestedInfo `json:"info"`
		Save              bool                          `json:"save"`
		Peer              *mtproto.InputPeer            `json:"peer,omitempty"`
		MsgID             int32                         `json:"msg_id,omitempty"`
	}{"validate_requested_info", uid, uid, paymentProductPremiumSubscription, in.GetInvoice(), in.GetInfo(), in.GetSave(), in.GetPeer(), in.GetMsgId()}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	c.setPaymentProviderRequestSignature(req, body)
	if providerKey != "" {
		req.Header.Set("Authorization", "Bearer "+providerKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, mtproto.ErrPaymentUnsupported
	}
	defer resp.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, mtproto.ErrPaymentUnsupported
	}
	if !c.paymentProviderResponseAuthenticated(resp, responseBody) {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	var result paymentValidationProviderResponse
	if err = json.Unmarshal(responseBody, &result); err != nil || !result.Verified ||
		!validPremiumProviderProduct(result.ProductType, result.UserID, result.BeneficiaryUserID, 1, uid) ||
		!validPaymentShippingOptions(result.ShippingOptions) || !normalizePaymentShippingOptions(result.ShippingOptions) {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	return mtproto.MakeTLPaymentsValidatedRequestedInfo(&mtproto.Payments_ValidatedRequestedInfo{
		Id:              optionalString(result.ID),
		ShippingOptions: result.ShippingOptions,
	}).To_Payments_ValidatedRequestedInfo(), nil
}

func (c *ApiFullCore) validateBotInvoiceRequestedInfoWithProvider(ctx context.Context, uid int64, in *mtproto.TLPaymentsValidateRequestedInfo, message *paymentInvoiceMessage) (*mtproto.Payments_ValidatedRequestedInfo, error) {
	if in == nil || message == nil || message.Invoice == nil {
		return nil, mtproto.ErrInvoicePayloadInvalid
	}
	client, endpoint, providerKey, err := c.configuredPaymentProvider()
	if err != nil {
		return nil, err
	}
	payload := struct {
		Operation         string                        `json:"operation"`
		UserID            int64                         `json:"user_id"`
		BeneficiaryUserID int64                         `json:"beneficiary_user_id"`
		BotUserID         int64                         `json:"bot_user_id"`
		RequiredProduct   string                        `json:"required_product"`
		Peer              *mtproto.InputPeer            `json:"peer"`
		MsgID             int32                         `json:"msg_id"`
		Payload           []byte                        `json:"payload"`
		Provider          string                        `json:"provider"`
		ProviderData      string                        `json:"provider_data"`
		Invoice           *mtproto.Invoice              `json:"invoice"`
		Info              *mtproto.PaymentRequestedInfo `json:"info"`
		Save              bool                          `json:"save"`
	}{
		"validate_bot_invoice_info", uid, message.BotUserID, message.BotUserID, "bot_invoice", message.Peer,
		message.MsgID, message.Metadata.Payload, message.Metadata.Provider, message.Metadata.ProviderData,
		message.Invoice, in.GetInfo(), in.GetSave(),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	c.setPaymentProviderRequestSignature(req, body)
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
	var result paymentValidationProviderResponse
	if err = json.Unmarshal(responseBody, &result); err != nil || !result.Verified || result.ProductType != "bot_invoice" ||
		result.UserID != uid || result.BeneficiaryUserID != message.BotUserID ||
		(result.BotUserID != 0 && result.BotUserID != message.BotUserID) ||
		!validPaymentShippingOptions(result.ShippingOptions) || !normalizePaymentShippingOptions(result.ShippingOptions) {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	return mtproto.MakeTLPaymentsValidatedRequestedInfo(&mtproto.Payments_ValidatedRequestedInfo{
		Id:              optionalString(result.ID),
		ShippingOptions: result.ShippingOptions,
	}).To_Payments_ValidatedRequestedInfo(), nil
}

func loadSavedPayInfo(uid int64) (savedPayInfo, bool, error) {
	if persist.PostgresEnabled() {
		if !domain.PostgresEnabled() {
			return savedPayInfo{}, false, mtproto.ErrPaymentUnsupported
		}
		info, found, err := domain.LoadSavedPaymentInfo(uid)
		if err != nil {
			return savedPayInfo{}, false, err
		}
		return savedPayInfo{Name: info.Name, Phone: info.Phone, Email: info.Email}, found && (info.Name != "" || info.Phone != "" || info.Email != ""), nil
	}
	raw, err := persist.Default.Get(payInfoKey(uid))
	if err != nil || raw == "" {
		return savedPayInfo{}, false, err
	}
	var info savedPayInfo
	if json.Unmarshal([]byte(raw), &info) != nil || (info.Name == "" && info.Phone == "" && info.Email == "") {
		return savedPayInfo{}, false, nil
	}
	return info, true, nil
}

func requestedFromSaved(info savedPayInfo) *mtproto.PaymentRequestedInfo {
	if info.Name == "" && info.Phone == "" && info.Email == "" {
		return nil
	}
	out := &mtproto.PaymentRequestedInfo{}
	if info.Name != "" {
		out.Name = wrapperspb.String(info.Name)
	}
	if info.Phone != "" {
		out.Phone = wrapperspb.String(info.Phone)
	}
	if info.Email != "" {
		out.Email = wrapperspb.String(info.Email)
	}
	return out
}

func hasSavedCredentials(uid int64) (bool, error) {
	if persist.PostgresEnabled() {
		if !domain.PostgresEnabled() {
			return false, mtproto.ErrPaymentUnsupported
		}
		info, _, err := domain.LoadSavedPaymentInfo(uid)
		if err != nil {
			return false, err
		}
		return info.HasSavedCredentials, nil
	}
	raw, err := persist.Default.Get(payCredKey(uid))
	if err != nil || raw == "" {
		return false, err
	}
	return raw == "1", nil
}

// savePaymentCredentials records only the provider-issued reusable-credential
// marker. Credential bytes never enter APIFull persistence. The marker is
// written only after the provider settlement and entitlement grant have
// succeeded, so a failed checkout cannot make an unsaved credential appear
// reusable.
func savePaymentCredentials(uid int64) error {
	if persist.PostgresEnabled() {
		if !domain.PostgresEnabled() {
			return mtproto.ErrPaymentUnsupported
		}
		return domain.SetSavedPaymentCredentials(uid, true)
	}
	return persist.Default.Set(payCredKey(uid), "1")
}

func savePayReceipt(uid int64, rec payReceipt) error {
	if rec.Title == "" && rec.MsgID == 0 && rec.FormID == 0 && rec.Peer == 0 {
		return nil
	}
	if rec.Date == 0 {
		rec.Date = int32(time.Now().Unix())
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return persist.Default.Set(payReceiptKey(uid), string(b))
}

func loadPayReceipt(uid int64) (payReceipt, bool, error) {
	raw, err := persist.Default.Get(payReceiptKey(uid))
	if err != nil || raw == "" {
		return payReceipt{}, false, err
	}
	var rec payReceipt
	if json.Unmarshal([]byte(raw), &rec) != nil {
		return payReceipt{}, false, nil
	}
	return rec, true, nil
}

func payPeerID(peer *mtproto.InputPeer) int64 {
	if peer == nil {
		return 0
	}
	if id := peer.GetUserId(); id != 0 {
		return id
	}
	return peer.GetChannelId()
}

func payPeerIDForUser(uid int64, peer *mtproto.InputPeer) int64 {
	if peer != nil && peer.GetPredicateName() == mtproto.Predicate_inputPeerSelf {
		return uid
	}
	return payPeerID(peer)
}

func paymentRequestKey(uid int64, in *mtproto.TLPaymentsSendPaymentForm) string {
	if in == nil {
		return ""
	}
	peer, msgID := in.GetPeer(), in.GetMsgId()
	if in.GetInvoice() != nil {
		if peer == nil {
			peer = in.GetInvoice().GetPeer()
		}
		if msgID == 0 {
			msgID = in.GetInvoice().GetMsgId()
		}
		// Slug invoices are provider quoted and carry no message peer. A
		// provider may legitimately reuse one form ID for different slugs, so
		// the invoice identity must remain part of the idempotency key.
		if in.GetInvoice().GetPredicateName() == mtproto.Predicate_inputInvoiceSlug &&
			payPeerIDForUser(uid, peer) == 0 && msgID == 0 {
			return "invoice:" + paymentFingerprint(in)
		}
	}
	if in.GetFormId() == 0 && payPeerIDForUser(uid, peer) == 0 && msgID == 0 {
		return "invoice:" + paymentFingerprint(in)
	}
	return fmt.Sprintf("form:%d:%d:%d", in.GetFormId(), payPeerIDForUser(uid, peer), msgID)
}

func paymentProviderIdempotencyKey(uid int64, requestKey string) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", uid, requestKey)))
	return hex.EncodeToString(digest[:])
}

func paymentFingerprint(in *mtproto.TLPaymentsSendPaymentForm) string {
	if in == nil || in.GetInvoice() == nil {
		return ""
	}
	b, _ := json.Marshal(in.GetInvoice())
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func paymentCredentialsPredicate(credentials *mtproto.InputPaymentCredentials) string {
	if credentials == nil {
		return ""
	}
	// Unit callers and older generated wrappers may carry only the TL
	// constructor. Preserve the wire union semantics before inspecting fields.
	predicate := credentials.GetPredicateName()
	constructorPredicate := ""
	switch credentials.GetConstructor() {
	case mtproto.TLConstructor_CRC32_inputPaymentCredentialsSaved:
		constructorPredicate = mtproto.Predicate_inputPaymentCredentialsSaved
	case mtproto.TLConstructor_CRC32_inputPaymentCredentials:
		constructorPredicate = mtproto.Predicate_inputPaymentCredentials
	case mtproto.TLConstructor_CRC32_inputPaymentCredentialsApplePay:
		constructorPredicate = mtproto.Predicate_inputPaymentCredentialsApplePay
	case mtproto.TLConstructor_CRC32_inputPaymentCredentialsGooglePay:
		constructorPredicate = mtproto.Predicate_inputPaymentCredentialsGooglePay
	}
	if constructorPredicate != "" && predicate != "" && predicate != constructorPredicate {
		return ""
	}
	if constructorPredicate != "" {
		return constructorPredicate
	}
	return predicate
}

func (c *ApiFullCore) validateTmpPaymentPassword(userID int64, credentials *mtproto.InputPaymentCredentials) error {
	if paymentCredentialsPredicate(credentials) != mtproto.Predicate_inputPaymentCredentialsSaved {
		return nil
	}
	if len(credentials.GetTmpPassword()) == 0 {
		return mtproto.ErrTmpPasswordDisabled
	}
	if !domain.PostgresEnabled() {
		return mtproto.ErrPaymentUnsupported
	}
	ok, err := domain.ValidateTmpPassword(c.secretContext(), userID, credentials.GetTmpPassword())
	if err != nil {
		return err
	}
	if !ok {
		return mtproto.ErrTmpPasswordDisabled
	}
	return nil
}

func (c *ApiFullCore) AccountGetTmpPassword(in *mtproto.TLAccountGetTmpPassword) (*mtproto.Account_TmpPassword, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPassword() == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	period := in.GetPeriod()
	if period < 60 || period > 86400 {
		return nil, mtproto.ErrTmpPasswordDisabled
	}
	if !domain.PostgresEnabled() {
		return nil, mtproto.ErrTmpPasswordDisabled
	}
	state, err := loadAcctPassword(userID)
	if err != nil {
		return nil, err
	}
	if len(state.Secret) == 0 {
		return nil, mtproto.ErrTmpPasswordDisabled
	}
	if err = checkStoredPassword(userID, state, in.GetPassword()); err != nil {
		return nil, err
	}
	token := make([]byte, 32)
	if _, err = rand.Read(token); err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	validUntil := time.Now().Unix() + int64(period)
	if err = domain.StoreTmpPassword(c.secretContext(), userID, token, validUntil); err != nil {
		return nil, err
	}
	return mtproto.MakeTLAccountTmpPassword(&mtproto.Account_TmpPassword{
		TmpPassword: token,
		ValidUntil:  int32(validUntil),
	}).To_Account_TmpPassword(), nil
}

func (c *ApiFullCore) MessagesSetBotShippingResults(in *mtproto.TLMessagesSetBotShippingResults) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetQueryId() == 0 {
		return nil, mtproto.ErrQueryIdEmpty
	}
	// PostgreSQL is authoritative for deployed APIFull payment queries. All
	// stateful local outcomes fail closed when the caller-owned query is absent.
	if persist.PostgresEnabled() {
		options := in.GetShippingOptions()
		if !validPaymentShippingOptions(options) || !normalizePaymentShippingOptions(options) {
			return nil, mtproto.ErrInputRequestInvalid
		}
		if options == nil {
			options = []*mtproto.ShippingOption{}
		}
		encoded, marshalErr := json.Marshal(options)
		if marshalErr != nil {
			return nil, mtproto.ErrInternalServerError
		}
		errorText := ""
		if in.GetError() != nil {
			errorText = in.GetError().GetValue()
		}
		localErr := persist.SetBotShippingResults(uid, in.GetQueryId(), errorText, encoded)
		if localErr == nil {
			return mtproto.BoolTrue, nil
		}
		// PostgreSQL is authoritative for deployed Bot invoice queries. Do not
		// fall through to a provider for an unknown query_id: accepting a
		// provider response here would detach the callback from the caller-owned
		// query and violate Telegram's QUERY_ID_INVALID semantics.
		return nil, botPaymentQueryError(localErr)
	}
	var result mtproto.Bool
	if err = c.callBusinessProvider("messages.setBotShippingResults", map[string]any{
		"user_id": uid, "query_id": in.GetQueryId(), "error": in.GetError(), "shipping_options": in.GetShippingOptions(),
	}, &result); err != nil {
		return nil, err
	}
	if result.GetPredicateName() == "" || (result.GetPredicateName() != mtproto.Predicate_boolTrue && result.GetPredicateName() != mtproto.Predicate_boolFalse) {
		return nil, mtproto.ErrInternalServerError
	}
	return &result, nil
}

func (c *ApiFullCore) MessagesSetBotPrecheckoutResults(in *mtproto.TLMessagesSetBotPrecheckoutResults) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetQueryId() == 0 {
		return nil, mtproto.ErrQueryIdEmpty
	}
	if persist.PostgresEnabled() {
		errorText := ""
		if in.GetError() != nil {
			errorText = in.GetError().GetValue()
		}
		localErr := persist.SetBotPrecheckoutResults(uid, in.GetQueryId(), in.GetSuccess(), errorText)
		if localErr == nil {
			return mtproto.BoolTrue, nil
		}
		// See MessagesSetBotShippingResults: once PostgreSQL is enabled, the
		// durable query state is the sole callback authority.
		return nil, botPaymentQueryError(localErr)
	}
	var result mtproto.Bool
	if err = c.callBusinessProvider("messages.setBotPrecheckoutResults", map[string]any{
		"user_id": uid, "query_id": in.GetQueryId(), "success": in.GetSuccess(), "error": in.GetError(),
	}, &result); err != nil {
		return nil, err
	}
	if result.GetPredicateName() == "" || (result.GetPredicateName() != mtproto.Predicate_boolTrue && result.GetPredicateName() != mtproto.Predicate_boolFalse) {
		return nil, mtproto.ErrInternalServerError
	}
	return &result, nil
}

func botPaymentQueryError(err error) error {
	switch {
	case errors.Is(err, persist.ErrBotPaymentQueryNotFound),
		errors.Is(err, persist.ErrBotPaymentQueryExpired),
		errors.Is(err, persist.ErrBotPaymentQueryOwner),
		errors.Is(err, persist.ErrBotPaymentQueryState),
		errors.Is(err, persist.ErrBotPaymentQueryConflict):
		return mtproto.ErrQueryIdInvalid
	default:
		return mtproto.ErrMethodNotImpl
	}
}

func (c *ApiFullCore) grantPremiumForPayment(ctx context.Context, uid, formID int64, request domain.PaymentRequest, receipt domain.PaymentReceipt) error {
	if c.svcCtx == nil {
		return mtproto.ErrPaymentUnsupported
	}
	var envelope paymentProviderReceiptEnvelope
	if json.Unmarshal(receipt.Receipt, &envelope) != nil ||
		!verifyPaymentProviderSignature(c.svcCtx.Config.PaymentProviderSigningKey, envelope.Signature, envelope.ResponseBody) {
		return mtproto.ErrPaymentProviderInvalid
	}
	var providerResult paymentProviderResponse
	if json.Unmarshal(envelope.ResponseBody, &providerResult) != nil || !providerResult.Verified ||
		providerResult.UserID != uid || providerResult.BeneficiaryUserID != uid ||
		providerResult.RequestKey != request.RequestKey || providerResult.Fingerprint != request.Fingerprint ||
		providerResult.FormID != formID || providerResult.TransactionID != receipt.TransactionID ||
		providerResult.Currency != receipt.Currency || providerResult.Amount != receipt.Amount ||
		!validPremiumProviderProduct(providerResult.ProductType, providerResult.UserID, providerResult.BeneficiaryUserID, providerResult.PremiumMonths, uid) ||
		len(bytes.TrimSpace(providerResult.Receipt)) == 0 || bytes.Equal(bytes.TrimSpace(providerResult.Receipt), []byte("null")) {
		return mtproto.ErrPaymentProviderInvalid
	}
	if err := domain.EnsurePremiumGrant(request.ID, uid, receipt.Provider, receipt.TransactionID, providerResult.PremiumMonths); err != nil {
		return err
	}
	if c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return mtproto.ErrPaymentUnsupported
	}
	result, err := c.svcCtx.Dao.UserClient.UserUpdatePremium(ctx, &userpb.TLUserUpdatePremium{
		UserId:        uid,
		Premium:       mtproto.ToBool(true),
		Months:        wrapperspb.Int32(providerResult.PremiumMonths),
		Provider:      receipt.Provider,
		TransactionId: receipt.TransactionID,
	})
	if err != nil {
		retryPremiumGrant(request.ID, err)
		return err
	}
	if result == nil || !mtproto.FromBool(result) {
		err = mtproto.ErrPaymentUnsupported
		retryPremiumGrant(request.ID, err)
		return err
	}
	if err = domain.CompletePremiumGrant(ctx, request.ID); err != nil {
		return err
	}
	return nil
}

func retryPremiumGrant(requestID int64, cause error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = domain.RetryPremiumGrant(ctx, requestID, cause)
}

func (c *ApiFullCore) PaymentsGetPaymentForm(in *mtproto.TLPaymentsGetPaymentForm) (*mtproto.Payments_PaymentForm, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetInvoice() == nil {
		return nil, mtproto.ErrInvoicePayloadInvalid
	}
	if in.GetInvoice().GetPredicateName() == mtproto.Predicate_inputInvoiceMessage {
		message, err := c.loadPaymentInvoiceMessage(uid, in.GetInvoice())
		if err != nil {
			return nil, err
		}
		return c.paymentBotInvoiceFormFromProvider(c.secretContext(), uid, in, message)
	}
	if in.GetInvoice().GetPredicateName() == mtproto.Predicate_inputInvoiceStars {
		offer, found, offerErr := starsTopupOfferFromInvoice(in.GetInvoice())
		if offerErr != nil {
			return nil, offerErr
		}
		if !found {
			return nil, mtproto.ErrPaymentUnsupported
		}
		return c.paymentStarsFormFromProvider(c.secretContext(), uid, in, offer)
	}
	if !selfPremiumSubscriptionInvoice(in.GetInvoice()) {
		return nil, mtproto.ErrPaymentUnsupported
	}
	return c.paymentFormFromProvider(c.secretContext(), uid, in)
}

func (c *ApiFullCore) PaymentsGetPaymentReceipt(in *mtproto.TLPaymentsGetPaymentReceipt) (*mtproto.Payments_PaymentReceipt, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if !validThreadInputPeer(uid, in.GetPeer()) {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.GetMsgId() <= 0 {
		return nil, mtproto.ErrMsgIdInvalid
	}
	receipt, ok, err := domain.LoadPaymentReceiptByMessage(uid, payPeerIDForUser(uid, in.GetPeer()), in.GetMsgId())
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, mtproto.ErrPaymentUnsupported
	}
	return mtproto.MakeTLPaymentsPaymentReceipt(&mtproto.Payments_PaymentReceipt{
		Date:          int32(receipt.CreatedAt),
		Title:         receipt.Title,
		Currency:      receipt.Currency,
		TotalAmount:   receipt.Amount,
		TransactionId: receipt.TransactionID,
	}).To_Payments_PaymentReceipt(), nil
}

func (c *ApiFullCore) PaymentsValidateRequestedInfo(in *mtproto.TLPaymentsValidateRequestedInfo) (*mtproto.Payments_ValidatedRequestedInfo, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if in.GetInvoice() == nil {
		return nil, mtproto.ErrInvoicePayloadInvalid
	}
	if in.GetInvoice().GetPredicateName() == mtproto.Predicate_inputInvoiceMessage {
		message, err := c.loadPaymentInvoiceMessage(uid, in.GetInvoice())
		if err != nil {
			return nil, err
		}
		validated, err := c.validateBotInvoiceRequestedInfoWithProvider(c.secretContext(), uid, in, message)
		if err != nil {
			return nil, err
		}
		// Keep the validated context durable for both invoice shapes. Invoices
		// that request an address also emit a shipping query; the remaining
		// invoices start directly at the precheckout stage when submitted.
		info := in.GetInfo()
		if message.Invoice.GetShippingAddressRequested() && (info == nil || info.GetShippingAddress() == nil) {
			return nil, mtproto.ErrInvoicePayloadInvalid
		}
		if info == nil {
			info = &mtproto.PaymentRequestedInfo{}
		}
		if info != nil && info.GetPredicateName() == "" {
			mtproto.MakeTLPaymentRequestedInfo(info).To_PaymentRequestedInfo()
		}
		if info != nil && info.GetShippingAddress() != nil && info.GetShippingAddress().GetPredicateName() == "" {
			mtproto.MakeTLPostAddress(info.GetShippingAddress()).To_PostAddress()
		}
		amount, amountErr := botInvoiceTotal(message.Invoice)
		if amountErr != nil {
			return nil, amountErr
		}
		providerInfoID := ""
		if validated.GetId() != nil {
			providerInfoID = validated.GetId().GetValue()
		}
		queryID := botPaymentQueryID(uid, message.BotUserID, message.Peer, message.MsgID, providerInfoID, info, message.Metadata.Payload)
		contextPayload, encodeErr := encodeBotPaymentContext(botPaymentContext{
			Version:        botPaymentContextVersion,
			QueryID:        queryID,
			BuyerUserID:    uid,
			BotUserID:      message.BotUserID,
			Peer:           message.Peer,
			MsgID:          message.MsgID,
			InvoicePayload: append([]byte(nil), message.Metadata.Payload...),
			Provider:       message.Metadata.Provider,
			ProviderData:   message.Metadata.ProviderData,
			ProviderInfoID: providerInfoID,
			Invoice:        message.Invoice,
			Info:           info,
			Currency:       message.Invoice.GetCurrency(),
			Amount:         amount,
		})
		if encodeErr != nil {
			return nil, mtproto.ErrInternalServerError
		}
		query := persist.BotPaymentQuery{
			QueryID: queryID, BotUserID: message.BotUserID, BuyerUserID: uid,
			Stage: persist.BotPaymentStageShippingAnswered, Payload: contextPayload,
			Currency: message.Invoice.GetCurrency(), Amount: amount,
			ExpiresAt: time.Now().UTC().Add(5 * time.Minute).Unix(),
		}
		if message.Invoice.GetShippingAddressRequested() {
			query.Stage = persist.BotPaymentStageShippingPending
			if err = persist.CreateBotPaymentQueryAndDelivery(query); err != nil {
				return nil, err
			}
		} else if err = persist.CreateBotPaymentQuery(query); err != nil {
			return nil, err
		}
		// The requested-info id is an opaque server token. Returning the durable
		// query id lets the subsequent sendPaymentForm call recover the exact
		// callback state after a process restart; the provider id remains in the
		// signed context sent at settlement time.
		validated.Id = wrapperspb.String(fmt.Sprintf("%d", queryID))
		return validated, nil
	}
	if !selfPremiumSubscriptionInvoice(in.GetInvoice()) {
		return nil, mtproto.ErrPaymentUnsupported
	}
	validated, err := c.validateRequestedInfoWithProvider(c.secretContext(), uid, in)
	if err != nil {
		return nil, err
	}
	if in.GetSave() && in.GetInfo() != nil {
		info := savedPayInfo{
			Name:  in.GetInfo().GetName().GetValue(),
			Phone: in.GetInfo().GetPhone().GetValue(),
			Email: in.GetInfo().GetEmail().GetValue(),
		}
		if info.Name != "" || info.Phone != "" || info.Email != "" {
			if persist.PostgresEnabled() {
				if !domain.PostgresEnabled() {
					return nil, mtproto.ErrPaymentUnsupported
				}
				if err = domain.SaveSavedPaymentInfo(uid, info.Name, info.Phone, info.Email); err != nil {
					return nil, err
				}
			} else {
				encoded, marshalErr := json.Marshal(info)
				if marshalErr != nil {
					return nil, mtproto.ErrInternalServerError
				}
				if err = persist.Default.Set(payInfoKey(uid), string(encoded)); err != nil {
					return nil, err
				}
			}
		}
	}
	return validated, nil
}

func equalBotPaymentPeer(left, right *mtproto.InputPeer) bool {
	if left == nil || right == nil {
		return left == right
	}
	a, errA := json.Marshal(left)
	b, errB := json.Marshal(right)
	return errA == nil && errB == nil && bytes.Equal(a, b)
}

func addBotPaymentAmount(total, value int64) (int64, error) {
	if value < 0 || total < 0 || total > int64(^uint64(0)>>1)-value {
		return 0, mtproto.ErrInvoicePayloadInvalid
	}
	return total + value, nil
}

func botShippingOptionTotal(raw []byte, optionID string) (int64, error) {
	if len(raw) == 0 {
		raw = []byte("[]")
	}
	var options []*mtproto.ShippingOption
	if err := json.Unmarshal(raw, &options); err != nil || !validPaymentShippingOptions(options) || !normalizePaymentShippingOptions(options) {
		return 0, mtproto.ErrInvoicePayloadInvalid
	}
	optionID = strings.TrimSpace(optionID)
	if optionID == "" {
		if len(options) > 0 {
			return 0, mtproto.ErrInvoicePayloadInvalid
		}
		return 0, nil
	}
	for _, option := range options {
		if option.GetId() != optionID {
			continue
		}
		var total int64
		for _, price := range option.GetPrices() {
			var err error
			total, err = addBotPaymentAmount(total, price.GetAmount())
			if err != nil {
				return 0, err
			}
		}
		return total, nil
	}
	return 0, mtproto.ErrInvoicePayloadInvalid
}

func (c *ApiFullCore) loadBotPaymentContextForSend(uid int64, in *mtproto.TLPaymentsSendPaymentForm, message *paymentInvoiceMessage) (persist.BotPaymentQuery, botPaymentContext, error) {
	if !persist.PostgresEnabled() || !domain.PostgresEnabled() {
		return persist.BotPaymentQuery{}, botPaymentContext{}, mtproto.ErrPaymentUnsupported
	}
	baseAmount, err := botInvoiceTotal(message.Invoice)
	if err != nil {
		return persist.BotPaymentQuery{}, botPaymentContext{}, err
	}
	providerInfoID := ""
	if requestedInfoID := in.GetRequestedInfoId(); requestedInfoID != nil {
		providerInfoID = strings.TrimSpace(requestedInfoID.GetValue())
	}
	needsShipping := message.Invoice.GetShippingAddressRequested()
	queryID := int64(0)
	if providerInfoID != "" {
		queryID, err = strconv.ParseInt(providerInfoID, 10, 64)
		if err != nil || queryID <= 0 {
			if needsShipping {
				return persist.BotPaymentQuery{}, botPaymentContext{}, mtproto.ErrInvoicePayloadInvalid
			}
			queryID = botPaymentQueryID(uid, message.BotUserID, message.Peer, message.MsgID, providerInfoID, nil, message.Metadata.Payload)
		}
	} else {
		if needsShipping {
			return persist.BotPaymentQuery{}, botPaymentContext{}, mtproto.ErrInvoicePayloadInvalid
		}
		queryID = botPaymentQueryID(uid, message.BotUserID, message.Peer, message.MsgID, "", nil, message.Metadata.Payload)
	}
	query, found, err := persist.LoadBotPaymentQuery(queryID)
	if err != nil {
		return persist.BotPaymentQuery{}, botPaymentContext{}, err
	}
	if !found {
		if needsShipping {
			return persist.BotPaymentQuery{}, botPaymentContext{}, mtproto.ErrQueryIdInvalid
		}
		payload, encodeErr := encodeBotPaymentContext(botPaymentContext{
			Version:        botPaymentContextVersion,
			QueryID:        queryID,
			BuyerUserID:    uid,
			BotUserID:      message.BotUserID,
			Peer:           message.Peer,
			MsgID:          message.MsgID,
			InvoicePayload: append([]byte(nil), message.Metadata.Payload...),
			Provider:       message.Metadata.Provider,
			ProviderData:   message.Metadata.ProviderData,
			ProviderInfoID: providerInfoID,
			Invoice:        message.Invoice,
			Currency:       message.Invoice.GetCurrency(),
			Amount:         baseAmount,
		})
		if encodeErr != nil {
			return persist.BotPaymentQuery{}, botPaymentContext{}, mtproto.ErrInternalServerError
		}
		if err = persist.CreateBotPaymentQuery(persist.BotPaymentQuery{
			QueryID: queryID, BotUserID: message.BotUserID, BuyerUserID: uid,
			Stage: persist.BotPaymentStageShippingAnswered, Payload: payload,
			Currency: message.Invoice.GetCurrency(), Amount: baseAmount,
			ExpiresAt: time.Now().UTC().Add(5 * time.Minute).Unix(),
		}); err != nil {
			return persist.BotPaymentQuery{}, botPaymentContext{}, err
		}
		query, found, err = persist.LoadBotPaymentQuery(queryID)
		if err != nil {
			return persist.BotPaymentQuery{}, botPaymentContext{}, err
		}
		if !found {
			return persist.BotPaymentQuery{}, botPaymentContext{}, mtproto.ErrQueryIdInvalid
		}
	}
	if query.QueryID != queryID || query.BotUserID != message.BotUserID || query.BuyerUserID != uid {
		return persist.BotPaymentQuery{}, botPaymentContext{}, mtproto.ErrQueryIdInvalid
	}
	value, err := decodeBotPaymentContext(query.Payload)
	if err != nil || value.QueryID != queryID || value.BuyerUserID != uid || value.BotUserID != message.BotUserID ||
		value.MsgID != message.MsgID || !equalBotPaymentPeer(value.Peer, message.Peer) ||
		!bytes.Equal(value.InvoicePayload, message.Metadata.Payload) || value.Provider != message.Metadata.Provider ||
		value.ProviderData != message.Metadata.ProviderData || value.Invoice == nil ||
		value.Invoice.GetCurrency() != message.Invoice.GetCurrency() || value.Amount < baseAmount {
		return persist.BotPaymentQuery{}, botPaymentContext{}, mtproto.ErrInvoicePayloadInvalid
	}
	if value.Info == nil {
		value.Info = &mtproto.PaymentRequestedInfo{}
		mtproto.MakeTLPaymentRequestedInfo(value.Info).To_PaymentRequestedInfo()
	}
	return query, value, nil
}

func (c *ApiFullCore) waitForBotShipping(ctx context.Context, queryID, botID, buyerID int64) (persist.BotPaymentQuery, error) {
	timeout := 10 * time.Second
	if c != nil && c.svcCtx != nil && c.svcCtx.Config.PaymentProviderTimeoutSeconds > 0 {
		timeout = time.Duration(c.svcCtx.Config.PaymentProviderTimeoutSeconds+5) * time.Second
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		query, found, err := persist.LoadBotPaymentQuery(queryID)
		if err != nil {
			return persist.BotPaymentQuery{}, err
		}
		if !found || query.BotUserID != botID || query.BuyerUserID != buyerID {
			return persist.BotPaymentQuery{}, mtproto.ErrQueryIdInvalid
		}
		switch query.Stage {
		case persist.BotPaymentStageShippingPending:
		case persist.BotPaymentStageShippingAnswered,
			persist.BotPaymentStagePrecheckoutPending,
			persist.BotPaymentStagePrecheckoutAnswered:
			return query, nil
		default:
			return persist.BotPaymentQuery{}, mtproto.ErrQueryIdInvalid
		}
		select {
		case <-waitCtx.Done():
			return persist.BotPaymentQuery{}, mtproto.ErrBotResponseTimeout
		case <-ticker.C:
		}
	}
}

func (c *ApiFullCore) waitForBotPrecheckout(ctx context.Context, queryID, botID, buyerID int64) (persist.BotPaymentQuery, error) {
	timeout := 10 * time.Second
	if c != nil && c.svcCtx != nil && c.svcCtx.Config.PaymentProviderTimeoutSeconds > 0 {
		timeout = time.Duration(c.svcCtx.Config.PaymentProviderTimeoutSeconds+5) * time.Second
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		query, found, err := persist.LoadBotPaymentQuery(queryID)
		if err != nil {
			return persist.BotPaymentQuery{}, err
		}
		if !found || query.BotUserID != botID || query.BuyerUserID != buyerID {
			return persist.BotPaymentQuery{}, mtproto.ErrQueryIdInvalid
		}
		switch query.Stage {
		case persist.BotPaymentStagePrecheckoutAnswered:
			if query.PrecheckoutSuccess == nil {
				return persist.BotPaymentQuery{}, mtproto.ErrQueryIdInvalid
			}
			return query, nil
		case persist.BotPaymentStagePrecheckoutPending:
		default:
			return persist.BotPaymentQuery{}, mtproto.ErrQueryIdInvalid
		}
		select {
		case <-waitCtx.Done():
			return persist.BotPaymentQuery{}, mtproto.ErrBotResponseTimeout
		case <-ticker.C:
		}
	}
}

func (c *ApiFullCore) settleBotInvoiceWithPaymentProvider(ctx context.Context, uid int64, requestKey, fingerprint string, in *mtproto.TLPaymentsSendPaymentForm, message *paymentInvoiceMessage, paymentContext botPaymentContext, peerID int64, msgID int32) (domain.PaymentRequest, domain.PaymentReceipt, error) {
	client, endpoint, providerKey, err := c.configuredPaymentProvider()
	if err != nil {
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, err
	}
	provider, ok := botPaymentProviderName(paymentContext)
	if !ok || message == nil || message.Invoice == nil || in == nil {
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrPaymentProviderInvalid
	}
	payload := struct {
		Operation         string                           `json:"operation"`
		UserID            int64                            `json:"user_id"`
		BeneficiaryUserID int64                            `json:"beneficiary_user_id"`
		BotUserID         int64                            `json:"bot_user_id"`
		RequiredProduct   string                           `json:"required_product"`
		RequestKey        string                           `json:"request_key"`
		Fingerprint       string                           `json:"fingerprint"`
		FormID            int64                            `json:"form_id"`
		QueryID           int64                            `json:"query_id"`
		Peer              *mtproto.InputPeer               `json:"peer"`
		MsgID             int32                            `json:"msg_id"`
		InvoicePayload    []byte                           `json:"invoice_payload"`
		Provider          string                           `json:"provider"`
		ProviderData      string                           `json:"provider_data"`
		ProviderInfoID    string                           `json:"provider_info_id,omitempty"`
		Invoice           *mtproto.Invoice                 `json:"invoice"`
		Info              *mtproto.PaymentRequestedInfo    `json:"info,omitempty"`
		ShippingOptionID  string                           `json:"shipping_option_id,omitempty"`
		Credentials       *mtproto.InputPaymentCredentials `json:"credentials"`
		Currency          string                           `json:"currency"`
		Amount            int64                            `json:"amount"`
		TipAmount         int64                            `json:"tip_amount,omitempty"`
	}{
		"settle_bot_invoice", uid, message.BotUserID, message.BotUserID, "bot_invoice", requestKey, fingerprint,
		in.GetFormId(), paymentContext.QueryID, message.Peer, msgID, paymentContext.InvoicePayload,
		provider, paymentContext.ProviderData, paymentContext.ProviderInfoID, paymentContext.Invoice,
		paymentContext.Info, paymentContext.ShippingOptionID, in.GetCredentials(), paymentContext.Currency,
		paymentContext.Amount, paymentContext.TipAmount,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrInternalServerError
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrPaymentProviderInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	c.setPaymentProviderRequestSignature(req, body)
	req.Header.Set("Idempotency-Key", paymentProviderIdempotencyKey(uid, requestKey))
	if providerKey != "" {
		req.Header.Set("Authorization", "Bearer "+providerKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrPaymentUnsupported
	}
	defer resp.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if readErr != nil || len(responseBody) > 1<<20 || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrPaymentUnsupported
	}
	if !c.paymentProviderResponseAuthenticated(resp, responseBody) {
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrPaymentProviderInvalid
	}
	var result paymentProviderResponse
	if err = json.Unmarshal(responseBody, &result); err != nil {
		if _, rejectErr := domain.RejectPaymentRequest(uid, requestKey, fingerprint, "provider returned invalid bot payment response"); rejectErr != nil {
			return domain.PaymentRequest{}, domain.PaymentReceipt{}, rejectErr
		}
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrPaymentProviderInvalid
	}
	receipt := bytes.TrimSpace(result.Receipt)
	if !result.Verified || result.UserID != uid || result.BeneficiaryUserID != message.BotUserID ||
		(result.BotUserID != 0 && result.BotUserID != message.BotUserID) || result.RequestKey != requestKey ||
		result.Fingerprint != fingerprint || result.FormID != in.GetFormId() || result.ProductType != "bot_invoice" ||
		strings.TrimSpace(result.TransactionID) == "" || len(receipt) == 0 || bytes.Equal(receipt, []byte("null")) ||
		strings.TrimSpace(result.Currency) == "" || result.Currency != paymentContext.Currency || result.Amount != paymentContext.Amount {
		if _, rejectErr := domain.RejectPaymentRequest(uid, requestKey, fingerprint, "provider rejected bot payment"); rejectErr != nil {
			return domain.PaymentRequest{}, domain.PaymentReceipt{}, rejectErr
		}
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrPaymentProviderInvalid
	}
	storedReceipt, err := json.Marshal(paymentProviderReceiptEnvelope{
		Signature:    resp.Header.Get("X-Teamgram-Payment-Signature"),
		ResponseBody: append([]byte(nil), responseBody...),
	})
	if err != nil {
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrInternalServerError
	}
	return domain.SettlePaymentRequest(uid, requestKey, fingerprint, result.TransactionID, result.Currency, result.Title,
		result.Amount, peerID, msgID, storedReceipt, true)
}

func (c *ApiFullCore) sendBotInvoicePaymentForm(uid int64, in *mtproto.TLPaymentsSendPaymentForm) (*mtproto.Payments_PaymentResult, error) {
	if in.GetFormId() <= 0 {
		return nil, mtproto.ErrPaymentUnsupported
	}
	message, err := c.loadPaymentInvoiceMessage(uid, in.GetInvoice())
	if err != nil {
		return nil, err
	}
	if in.GetPeer() != nil && !equalBotPaymentPeer(in.GetPeer(), message.Peer) {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.GetMsgId() != 0 && in.GetMsgId() != message.MsgID {
		return nil, mtproto.ErrMsgIdInvalid
	}
	if !persist.PostgresEnabled() || !domain.PostgresEnabled() {
		return nil, mtproto.ErrPaymentUnsupported
	}
	if _, _, _, err = c.configuredPaymentProvider(); err != nil {
		return nil, err
	}
	query, paymentContext, err := c.loadBotPaymentContextForSend(uid, in, message)
	if err != nil {
		return nil, err
	}
	if query.Stage == persist.BotPaymentStageShippingPending {
		query, err = c.waitForBotShipping(c.secretContext(), query.QueryID, message.BotUserID, uid)
		if err != nil {
			return nil, err
		}
	}
	if query.ShippingError != "" {
		return nil, mtproto.ErrInvoicePayloadInvalid
	}
	shippingOptionID := ""
	if shippingOption := in.GetShippingOptionId(); shippingOption != nil {
		shippingOptionID = strings.TrimSpace(shippingOption.GetValue())
	}
	if query.Stage == persist.BotPaymentStagePrecheckoutPending || query.Stage == persist.BotPaymentStagePrecheckoutAnswered {
		if shippingOptionID == "" {
			shippingOptionID = paymentContext.ShippingOptionID
		} else if paymentContext.ShippingOptionID != "" && shippingOptionID != paymentContext.ShippingOptionID {
			return nil, mtproto.ErrInvoicePayloadInvalid
		}
	}
	if !message.Invoice.GetShippingAddressRequested() {
		shippingOptionID = ""
	}
	shippingAmount := int64(0)
	if message.Invoice.GetShippingAddressRequested() {
		shippingAmount, err = botShippingOptionTotal(query.ShippingOptions, shippingOptionID)
		if err != nil {
			return nil, err
		}
	}
	baseAmount, err := botInvoiceTotal(message.Invoice)
	if err != nil {
		return nil, err
	}
	tipAmount := paymentContext.TipAmount
	if in.GetTipAmount() != nil {
		tipAmount = in.GetTipAmount().GetValue()
		if tipAmount < 0 || (query.Stage != persist.BotPaymentStageShippingAnswered && tipAmount != paymentContext.TipAmount) {
			return nil, mtproto.ErrInvoicePayloadInvalid
		}
	}
	if maxTip := message.Invoice.GetMaxTipAmount(); maxTip != nil && (tipAmount < 0 || tipAmount > maxTip.GetValue()) {
		return nil, mtproto.ErrInvoicePayloadInvalid
	}
	total, err := addBotPaymentAmount(baseAmount, shippingAmount)
	if err != nil {
		return nil, err
	}
	total, err = addBotPaymentAmount(total, tipAmount)
	if err != nil {
		return nil, err
	}
	if query.Stage == persist.BotPaymentStagePrecheckoutPending || query.Stage == persist.BotPaymentStagePrecheckoutAnswered {
		if paymentContext.Amount != total {
			return nil, mtproto.ErrInvoicePayloadInvalid
		}
	}
	paymentContext.Amount = total
	paymentContext.TipAmount = tipAmount
	paymentContext.ShippingOptionID = shippingOptionID
	payload, err := encodeBotPaymentContext(paymentContext)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	if query.Stage == persist.BotPaymentStageShippingPending {
		return nil, mtproto.ErrBotResponseTimeout
	}
	if query.Stage == persist.BotPaymentStageShippingAnswered || query.Stage == persist.BotPaymentStagePrecheckoutPending || query.Stage == persist.BotPaymentStagePrecheckoutAnswered {
		if err = persist.AdvanceBotPaymentQueryAndDelivery(query.QueryID, message.BotUserID, payload, paymentContext.Currency, total); err != nil {
			if errors.Is(err, persist.ErrBotPaymentOutboxRequiresPostgres) {
				return nil, mtproto.ErrPaymentUnsupported
			}
			return nil, botPaymentQueryError(err)
		}
	} else {
		return nil, mtproto.ErrQueryIdInvalid
	}
	query, err = c.waitForBotPrecheckout(c.secretContext(), query.QueryID, message.BotUserID, uid)
	if err != nil {
		return nil, err
	}
	if query.PrecheckoutSuccess == nil || !*query.PrecheckoutSuccess {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	fingerprint := botPaymentFingerprint(in, query.QueryID, paymentContext)
	if fingerprint == "" {
		return nil, mtproto.ErrInvoicePayloadInvalid
	}
	requestKey := botPaymentRequestKey(uid, query.QueryID, fingerprint)
	if requestKey == "" {
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
	peerID := payPeerIDForUser(uid, message.Peer)
	provider, ok := botPaymentProviderName(paymentContext)
	if !ok {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	request, err := domain.BeginPaymentRequest(uid, requestKey, provider, fingerprint, paymentContext.Currency, paymentContext.Amount, peerID, message.MsgID)
	if err != nil {
		return nil, err
	}
	if request.State == domain.PaymentStateSettled {
		if _, found, receiptErr := domain.LoadPaymentReceiptByRequest(uid, requestKey); receiptErr != nil {
			return nil, receiptErr
		} else if !found {
			return nil, mtproto.ErrPaymentProviderInvalid
		}
		if in.GetCredentials().GetSave() {
			if err = savePaymentCredentials(uid); err != nil {
				return nil, err
			}
		}
		return mtproto.MakeTLPaymentsPaymentResult(&mtproto.Payments_PaymentResult{Updates: mtproto.MakeEmptyUpdates()}).To_Payments_PaymentResult(), nil
	}
	if request.State != domain.PaymentStatePending {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	settled, _, err := c.settleBotInvoiceWithPaymentProvider(c.secretContext(), uid, requestKey, fingerprint, in, message, paymentContext, peerID, message.MsgID)
	if err != nil {
		return nil, err
	}
	if settled.State != domain.PaymentStateSettled {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	if in.GetCredentials().GetSave() {
		if err = savePaymentCredentials(uid); err != nil {
			return nil, err
		}
	}
	return mtproto.MakeTLPaymentsPaymentResult(&mtproto.Payments_PaymentResult{Updates: mtproto.MakeEmptyUpdates()}).To_Payments_PaymentResult(), nil
}

func (c *ApiFullCore) PaymentsSendPaymentForm(in *mtproto.TLPaymentsSendPaymentForm) (*mtproto.Payments_PaymentResult, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetInvoice() == nil {
		return nil, mtproto.ErrInvoicePayloadInvalid
	}
	if in.GetCredentials() == nil {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	if paymentCredentialsPredicate(in.GetCredentials()) == "" {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	if err = c.validateTmpPaymentPassword(uid, in.GetCredentials()); err != nil {
		return nil, err
	}
	if in.GetInvoice().GetPredicateName() == mtproto.Predicate_inputInvoiceMessage {
		return c.sendBotInvoicePaymentForm(uid, in)
	}
	if !selfPremiumSubscriptionInvoice(in.GetInvoice()) || in.GetFormId() <= 0 {
		return nil, mtproto.ErrPaymentUnsupported
	}
	// Validate the provider before opening a durable request. An absent or
	// malformed provider must not leave a pending local payment attempt.
	if _, _, _, err = c.configuredPaymentProvider(); err != nil {
		return nil, err
	}
	if err = domain.CheckPremiumGrantOutbox(c.secretContext()); err != nil {
		return nil, mtproto.ErrPaymentUnsupported
	}
	fingerprint := paymentFingerprint(in)
	if fingerprint == "" {
		return nil, mtproto.ErrInvoicePayloadInvalid
	}
	requestKey := paymentRequestKey(uid, in)
	if requestKey == "" {
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
	purpose := in.GetInvoice().GetPurpose()
	currency, amount := "", int64(0)
	if purpose != nil {
		currency, amount = purpose.GetCurrency(), purpose.GetAmount()
	}
	peer, msgID := in.GetPeer(), in.GetMsgId()
	if in.GetInvoice().GetPeer() != nil && peer == nil {
		peer = in.GetInvoice().GetPeer()
	}
	if in.GetInvoice().GetMsgId() > 0 && msgID == 0 {
		msgID = in.GetInvoice().GetMsgId()
	}
	peerID := payPeerIDForUser(uid, peer)
	request, err := domain.BeginPaymentRequest(uid, requestKey, "external", fingerprint, currency, amount, peerID, msgID)
	if err != nil {
		return nil, err
	}
	// Retrying a settled request must not charge the external provider again.
	// Rejected requests are terminal as well; callers receive the same
	// provider-invalid result without reopening the attempt.
	if request.State == domain.PaymentStateSettled {
		receipt, found, receiptErr := domain.LoadPaymentReceiptByRequest(uid, requestKey)
		if receiptErr != nil {
			return nil, receiptErr
		}
		if !found {
			return nil, mtproto.ErrPaymentProviderInvalid
		}
		if err = c.grantPremiumForPayment(c.secretContext(), uid, in.GetFormId(), request, receipt); err != nil {
			return nil, err
		}
		if in.GetCredentials().GetSave() {
			if err = savePaymentCredentials(uid); err != nil {
				return nil, err
			}
		}
		return mtproto.MakeTLPaymentsPaymentResult(&mtproto.Payments_PaymentResult{
			Updates: mtproto.MakeEmptyUpdates(),
		}).To_Payments_PaymentResult(), nil
	}
	if request.State != domain.PaymentStatePending {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	settled, receipt, settleErr := c.settleWithPaymentProvider(c.secretContext(), uid, requestKey, fingerprint, in, currency, amount, peerID, msgID)
	if settleErr != nil {
		return nil, settleErr
	}
	if settled.State != domain.PaymentStateSettled {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	if err = c.grantPremiumForPayment(c.secretContext(), uid, in.GetFormId(), settled, receipt); err != nil {
		return nil, err
	}
	if in.GetCredentials().GetSave() {
		if err = savePaymentCredentials(uid); err != nil {
			return nil, err
		}
	}
	return mtproto.MakeTLPaymentsPaymentResult(&mtproto.Payments_PaymentResult{
		Updates: mtproto.MakeEmptyUpdates(),
	}).To_Payments_PaymentResult(), nil
}

func (c *ApiFullCore) PaymentsGetSavedInfo(in *mtproto.TLPaymentsGetSavedInfo) (*mtproto.Payments_SavedInfo, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	info, _, err := loadSavedPayInfo(uid)
	if err != nil {
		return nil, err
	}
	credentials, err := hasSavedCredentials(uid)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLPaymentsSavedInfo(&mtproto.Payments_SavedInfo{
		HasSavedCredentials: credentials,
		SavedInfo:           requestedFromSaved(info),
	}).To_Payments_SavedInfo(), nil
}

func (c *ApiFullCore) PaymentsClearSavedInfo(in *mtproto.TLPaymentsClearSavedInfo) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if persist.PostgresEnabled() {
		if !domain.PostgresEnabled() {
			return nil, mtproto.ErrPaymentUnsupported
		}
		if err = domain.ClearSavedPaymentInfo(uid, in.GetInfo(), in.GetCredentials()); err != nil {
			return nil, err
		}
	} else {
		if in.GetInfo() {
			if err = persist.Default.Set(payInfoKey(uid), ""); err != nil {
				return nil, err
			}
		}
		if in.GetCredentials() {
			if err = persist.Default.Set(payCredKey(uid), "0"); err != nil {
				return nil, err
			}
		}
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) PaymentsGetBankCardData(in *mtproto.TLPaymentsGetBankCardData) (*mtproto.Payments_BankCardData, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || strings.TrimSpace(in.GetNumber()) == "" {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	client, endpoint, providerKey, err := c.configuredPaymentProvider()
	if err != nil {
		return nil, err
	}
	payload := struct {
		Operation string `json:"operation"`
		UserID    int64  `json:"user_id"`
		Number    string `json:"number"`
	}{"get_bank_card_data", uid, strings.TrimSpace(in.GetNumber())}
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
	var result paymentBankCardProviderResponse
	if err = json.Unmarshal(responseBody, &result); err != nil || !result.Verified || result.UserID != uid ||
		result.Number != strings.TrimSpace(in.GetNumber()) || !normalizeBankCardData(result.Data) {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	return mtproto.MakeTLPaymentsBankCardData(result.Data).To_Payments_BankCardData(), nil
}

func (c *ApiFullCore) PaymentsExportInvoice(in *mtproto.TLPaymentsExportInvoice) (*mtproto.Payments_ExportedInvoice, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetInvoiceMedia() == nil {
		return nil, mtproto.ErrInvoicePayloadInvalid
	}
	var response paymentExportInvoiceProviderResponse
	if err = c.callStarsRevenueProvider(c.secretContext(), struct {
		Operation    string              `json:"operation"`
		UserID       int64               `json:"user_id"`
		InvoiceMedia *mtproto.InputMedia `json:"invoice_media"`
		TargetUserID *mtproto.InputUser  `json:"target_user_id,omitempty"`
	}{"export_invoice", uid, in.GetInvoiceMedia(), in.GetUserId()}, &response); err != nil {
		return nil, err
	}
	if !response.Verified || response.UserID != uid || !validRevenueURL(response.URL) {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	return mtproto.MakeTLPaymentsExportedInvoice(&mtproto.Payments_ExportedInvoice{Url: strings.TrimSpace(response.URL)}).To_Payments_ExportedInvoice(), nil
}

func (c *ApiFullCore) PaymentsRequestRecurringPayment(in *mtproto.TLPaymentsRequestRecurringPayment) (*mtproto.Updates, error) {
	_ = in
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrPaymentUnsupported
}

func (c *ApiFullCore) PaymentsRestorePlayMarketReceipt(in *mtproto.TLPaymentsRestorePlayMarketReceipt) (*mtproto.Updates, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	// A Play receipt is never verified here and never becomes premium or a balance credit.
	if in == nil || len(in.GetReceipt()) == 0 {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	return nil, mtproto.ErrPaymentUnsupported
}
