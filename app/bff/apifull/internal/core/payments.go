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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
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

// paymentValidationProviderResponse is the provider's verified result for
// invoice contact/shipping validation. Both returned fields are optional in
// the MTProto result; APIFull only validates them when the provider supplies
// them and never infers an ID or prices.
type paymentValidationProviderResponse struct {
	Verified          bool                      `json:"verified"`
	UserID            int64                     `json:"user_id"`
	BeneficiaryUserID int64                     `json:"beneficiary_user_id"`
	ProductType       string                    `json:"product_type"`
	ID                string                    `json:"id"`
	ShippingOptions   []*mtproto.ShippingOption `json:"shipping_options"`
}

// paymentBankCardProviderResponse is the signed, provider-owned BIN/card
// metadata used by payments.getBankCardData. APIFull never stores the number
// or any card data; it only validates and forwards the signed response.
type paymentBankCardProviderResponse struct {
	Verified bool                          `json:"verified"`
	UserID   int64                         `json:"user_id"`
	Number   string                        `json:"number"`
	Data     *mtproto.Payments_BankCardData `json:"data"`
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
		for _, price := range option.GetPrices() {
			if price == nil || price.GetAmount() < 0 {
				return false
			}
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
	if err != nil || parsed.Host == "" || parsed.User != nil || !securePaymentProviderURL(parsed) {
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

func securePaymentProviderURL(parsed *url.URL) bool {
	if parsed == nil {
		return false
	}
	if strings.EqualFold(parsed.Scheme, "https") {
		return true
	}
	if !strings.EqualFold(parsed.Scheme, "http") {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func verifyPaymentProviderSignature(key, signature string, body []byte) bool {
	key = strings.TrimSpace(key)
	signature = strings.TrimSpace(signature)
	signature = strings.TrimPrefix(signature, "sha256=")
	provided, err := hex.DecodeString(signature)
	if err != nil || key == "" || len(provided) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
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

func loadSavedPayInfo(uid int64) (savedPayInfo, bool, error) {
	if persist.PostgresEnabled() {
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

func (c *ApiFullCore) AccountGetTmpPassword(in *mtproto.TLAccountGetTmpPassword) (*mtproto.Account_TmpPassword, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	// APIFull has no temporary payment-password issuer or consumer.
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) MessagesSetBotShippingResults(in *mtproto.TLMessagesSetBotShippingResults) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetQueryId() == 0 {
		return nil, mtproto.ErrQueryIdEmpty
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) MessagesSetBotPrecheckoutResults(in *mtproto.TLMessagesSetBotPrecheckoutResults) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetQueryId() == 0 {
		return nil, mtproto.ErrQueryIdEmpty
	}
	return nil, mtproto.ErrMethodNotImpl
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
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetInvoiceMedia() == nil {
		return nil, mtproto.ErrInvoicePayloadInvalid
	}
	return nil, mtproto.ErrPaymentUnsupported
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
