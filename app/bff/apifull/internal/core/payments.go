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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RPCPaymentsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.
// pay: is this server's own JSON ledger, not a charge against an external processor.

const payLedgerPrefix = "pay:"

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

// paymentProviderResponse is the small, signed-by-transport contract between
// APIFull and an external payment processor. The processor must explicitly
// mark the transaction verified; client credentials or a receipt alone never
// settle the durable ledger.
type paymentProviderResponse struct {
	Verified      bool            `json:"verified"`
	TransactionID string          `json:"transaction_id"`
	Currency      string          `json:"currency"`
	Amount        int64           `json:"amount"`
	Title         string          `json:"title"`
	Receipt       json.RawMessage `json:"receipt"`
	Verification  string          `json:"verification_url,omitempty"`
}

// paymentFormProviderResponse is returned by the payment provider for the
// form discovery operation. The provider is authoritative for the form
// metadata; APIFull only forwards a verified, structurally encodable form.
type paymentFormProviderResponse struct {
	Verified bool                          `json:"verified"`
	Form     *mtproto.Payments_PaymentForm `json:"form"`
}

func (c *ApiFullCore) configuredPaymentProvider() (*http.Client, string, string, error) {
	if c == nil || c.svcCtx == nil {
		return nil, "", "", mtproto.ErrPaymentUnsupported
	}
	endpoint := strings.TrimSpace(c.svcCtx.Config.PaymentProviderEndpoint)
	if endpoint == "" {
		return nil, "", "", mtproto.ErrPaymentUnsupported
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
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

func (c *ApiFullCore) settleWithPaymentProvider(ctx context.Context, uid int64, requestKey, fingerprint string, in *mtproto.TLPaymentsSendPaymentForm, currency string, amount, peerID int64, msgID int32) (domain.PaymentRequest, domain.PaymentReceipt, error) {
	client, endpoint, providerKey, err := c.configuredPaymentProvider()
	if err != nil {
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, err
	}
	payload := struct {
		UserID      int64                            `json:"user_id"`
		RequestKey  string                           `json:"request_key"`
		Fingerprint string                           `json:"fingerprint"`
		Invoice     *mtproto.InputInvoice            `json:"invoice"`
		Credentials *mtproto.InputPaymentCredentials `json:"credentials"`
		Currency    string                           `json:"currency"`
		Amount      int64                            `json:"amount"`
		PeerID      int64                            `json:"peer_id"`
		MsgID       int32                            `json:"msg_id"`
	}{uid, requestKey, fingerprint, in.GetInvoice(), in.GetCredentials(), currency, amount, peerID, msgID}
	body, err := json.Marshal(payload)
	if err != nil {
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrInternalServerError
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrPaymentProviderInvalid
	}
	req.Header.Set("Content-Type", "application/json")
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
	var result paymentProviderResponse
	if err = json.Unmarshal(responseBody, &result); err != nil {
		if _, rejectErr := domain.RejectPaymentRequest(uid, requestKey, fingerprint, "provider returned invalid payment response"); rejectErr != nil {
			return domain.PaymentRequest{}, domain.PaymentReceipt{}, rejectErr
		}
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrPaymentProviderInvalid
	}
	receipt := bytes.TrimSpace(result.Receipt)
	if !result.Verified || strings.TrimSpace(result.TransactionID) == "" || len(receipt) == 0 || bytes.Equal(receipt, []byte("null")) {
		if _, rejectErr := domain.RejectPaymentRequest(uid, requestKey, fingerprint, "provider rejected payment"); rejectErr != nil {
			return domain.PaymentRequest{}, domain.PaymentReceipt{}, rejectErr
		}
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrPaymentProviderInvalid
	}
	// Currency and amount are part of the signed provider result. Omitting
	// either field must never downgrade to the locally requested value.
	if result.Currency != currency || result.Amount != amount {
		if _, rejectErr := domain.RejectPaymentRequest(uid, requestKey, fingerprint, "provider response does not match invoice"); rejectErr != nil {
			return domain.PaymentRequest{}, domain.PaymentReceipt{}, rejectErr
		}
		return domain.PaymentRequest{}, domain.PaymentReceipt{}, mtproto.ErrPaymentProviderInvalid
	}
	return domain.SettlePaymentRequest(uid, requestKey, fingerprint, result.TransactionID, result.Currency, result.Title, result.Amount, peerID, msgID, append([]byte(nil), receipt...), true)
}

func (c *ApiFullCore) paymentFormFromProvider(ctx context.Context, uid int64, in *mtproto.TLPaymentsGetPaymentForm) (*mtproto.Payments_PaymentForm, error) {
	client, endpoint, providerKey, err := c.configuredPaymentProvider()
	if err != nil {
		return nil, err
	}
	payload := struct {
		Operation   string                `json:"operation"`
		UserID      int64                 `json:"user_id"`
		Invoice     *mtproto.InputInvoice `json:"invoice"`
		Peer        *mtproto.InputPeer    `json:"peer,omitempty"`
		MsgID       int32                 `json:"msg_id,omitempty"`
		ThemeParams *mtproto.DataJSON     `json:"theme_params,omitempty"`
	}{"get_form", uid, in.GetInvoice(), in.GetPeer(), in.GetMsgId(), in.GetThemeParams()}
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
	var result paymentFormProviderResponse
	if err = json.Unmarshal(responseBody, &result); err != nil || !result.Verified || result.Form == nil {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	// Encode() dereferences Invoice and clients need a stable non-zero form ID
	// to submit the returned form. Reject malformed provider data before it can
	// reach the MTProto encoder.
	if result.Form.GetInvoice() == nil || result.Form.GetFormId() <= 0 {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	switch result.Form.GetPredicateName() {
	case mtproto.Predicate_payments_paymentFormStars:
		return mtproto.MakeTLPaymentsPaymentFormStars(result.Form).To_Payments_PaymentForm(), nil
	case mtproto.Predicate_payments_paymentFormStarGift:
		return mtproto.MakeTLPaymentsPaymentFormStarGift(result.Form).To_Payments_PaymentForm(), nil
	default:
		return mtproto.MakeTLPaymentsPaymentForm(result.Form).To_Payments_PaymentForm(), nil
	}
}

func loadSavedPayInfo(uid int64) (savedPayInfo, bool, error) {
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
	raw, err := persist.Default.Get(payCredKey(uid))
	if err != nil || raw == "" {
		return false, err
	}
	return raw == "1", nil
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

func (c *ApiFullCore) PaymentsGetPaymentForm(in *mtproto.TLPaymentsGetPaymentForm) (*mtproto.Payments_PaymentForm, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetInvoice() == nil {
		return nil, mtproto.ErrInvoicePayloadInvalid
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
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if in.GetInvoice() == nil {
		return nil, mtproto.ErrInvoicePayloadInvalid
	}
	return nil, mtproto.ErrMethodNotImpl
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
	// Validate the provider before opening a durable request. An absent or
	// malformed provider must not leave a pending local payment attempt.
	if _, _, _, err = c.configuredPaymentProvider(); err != nil {
		return nil, err
	}
	fingerprint := paymentFingerprint(in)
	if fingerprint == "" {
		return nil, mtproto.ErrInvoicePayloadInvalid
	}
	requestKey := paymentRequestKey(uid, in)
	if requestKey == "" {
		return nil, mtproto.ErrInputConstructorInvalid
	}
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
	if _, err = domain.BeginPaymentRequest(uid, requestKey, "external", fingerprint, currency, amount, peerID, msgID); err != nil {
		return nil, err
	}
	settled, _, settleErr := c.settleWithPaymentProvider(c.secretContext(), uid, requestKey, fingerprint, in, currency, amount, peerID, msgID)
	if settleErr != nil {
		return nil, settleErr
	}
	if settled.State != domain.PaymentStateSettled {
		return nil, mtproto.ErrPaymentProviderInvalid
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
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) PaymentsGetBankCardData(in *mtproto.TLPaymentsGetBankCardData) (*mtproto.Payments_BankCardData, error) {
	_ = in
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
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
