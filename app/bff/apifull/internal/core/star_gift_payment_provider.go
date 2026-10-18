package core

import (
	"bytes"
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
)

const (
	starGiftUpgradePaymentProduct  = "star_gift_upgrade"
	starGiftTransferPaymentProduct = "star_gift_transfer"
)

type starGiftPaymentInvoice struct {
	CallerID     int64
	PayerID      int64
	GiftID       int64
	GiftSlug     string
	TargetID     int64
	Stars        int64
	Product      string
	KeepOriginal bool
	Invoice      *mtproto.InputInvoice
}

func (c *ApiFullCore) starGiftPaymentInvoice(in *mtproto.InputInvoice) (starGiftPaymentInvoice, error) {
	callerID, err := c.requireUserId()
	if err != nil {
		return starGiftPaymentInvoice{}, err
	}
	if in == nil {
		return starGiftPaymentInvoice{}, mtproto.ErrInvoicePayloadInvalid
	}
	product := ""
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputInvoiceStarGiftUpgrade:
		product = starGiftUpgradePaymentProduct
	case mtproto.Predicate_inputInvoiceStarGiftTransfer:
		product = starGiftTransferPaymentProduct
	default:
		return starGiftPaymentInvoice{}, mtproto.ErrPaymentUnsupported
	}
	if !domain.Ready() || in.GetStargift() == nil {
		return starGiftPaymentInvoice{}, mtproto.ErrPaymentUnsupported
	}
	ownerID, err := c.giftActorID()
	if err != nil {
		return starGiftPaymentInvoice{}, err
	}
	giftID, giftSlug, err := localSavedGiftReference(ownerID, in.GetStargift())
	if err != nil {
		return starGiftPaymentInvoice{}, err
	}
	var gift domain.Gift
	var found bool
	if giftID > 0 {
		gift, found, err = domain.FindGiftByID(giftID)
	} else {
		gift, found, err = domain.FindGiftBySlug(giftSlug)
	}
	if err != nil {
		return starGiftPaymentInvoice{}, starsDomainError(err)
	}
	// The local row binds the provider quote to an issued gift owned by the
	// business owner. The business provider remains authoritative for the
	// eventual upgrade/transfer mutation.
	if !found || gift.To != ownerID || gift.Stars <= 0 || strings.TrimSpace(gift.Slug) == "" {
		return starGiftPaymentInvoice{}, mtproto.ErrPaymentUnsupported
	}
	result := starGiftPaymentInvoice{
		CallerID:     callerID,
		PayerID:      ownerID,
		GiftID:       gift.ID,
		GiftSlug:     gift.Slug,
		Stars:        gift.Stars,
		Product:      product,
		KeepOriginal: in.GetKeepOriginalDetails(),
		Invoice:      in,
	}
	if product == starGiftTransferPaymentProduct {
		result.TargetID, err = giftTransferTarget(ownerID, &mtproto.TLPaymentsTransferStarGift{
			Stargift:       in.GetStargift(),
			ToId_INPUTPEER: in.GetToId_INPUTPEER(),
		})
		if err != nil {
			return starGiftPaymentInvoice{}, err
		}
	}
	return result, nil
}

func starGiftPaymentFingerprint(invoice starGiftPaymentInvoice, formID int64) string {
	body, err := json.Marshal(struct {
		Invoice      *mtproto.InputInvoice `json:"invoice"`
		CallerID     int64                 `json:"caller_id"`
		PayerID      int64                 `json:"payer_id"`
		GiftID       int64                 `json:"gift_id"`
		GiftSlug     string                `json:"gift_slug"`
		TargetID     int64                 `json:"target_id,omitempty"`
		Stars        int64                 `json:"stars"`
		Product      string                `json:"product"`
		KeepOriginal bool                  `json:"keep_original"`
		FormID       int64                 `json:"form_id"`
	}{invoice.Invoice, invoice.CallerID, invoice.PayerID, invoice.GiftID, invoice.GiftSlug, invoice.TargetID, invoice.Stars, invoice.Product, invoice.KeepOriginal, formID})
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func makeStarGiftPaymentRequestKey(invoice starGiftPaymentInvoice, formID int64, fingerprint string) string {
	if invoice.CallerID <= 0 || invoice.PayerID <= 0 || formID <= 0 || fingerprint == "" {
		return ""
	}
	return "gift:" + invoice.Product + ":" +
		strconvInt64(invoice.CallerID) + ":" + strconvInt64(invoice.PayerID) + ":" +
		strconvInt64(invoice.GiftID) + ":" + strconvInt64(formID) + ":" + fingerprint
}

func strconvInt64(value int64) string {
	return fmt.Sprintf("%d", value)
}

func validStarGiftPaymentForm(result paymentFormProviderResponse, invoice starGiftPaymentInvoice) bool {
	if !result.Verified || result.UserID != invoice.CallerID || result.BeneficiaryUserID != invoice.PayerID ||
		result.ProductType != invoice.Product || result.Stars != invoice.Stars || result.Currency != "XTR" || result.Amount != invoice.Stars ||
		result.Form == nil || result.Form.GetPredicateName() != mtproto.Predicate_payments_paymentFormStars || result.Form.GetFormId() <= 0 ||
		result.Form.GetInvoice() == nil || result.Form.GetInvoice().GetCurrency() != "XTR" {
		return false
	}
	var amount int64
	for _, price := range result.Form.GetInvoice().GetPrices() {
		if price == nil || price.GetAmount() < 0 || amount > int64(^uint64(0)>>1)-price.GetAmount() {
			return false
		}
		amount += price.GetAmount()
	}
	return amount == invoice.Stars
}

func (c *ApiFullCore) starGiftPaymentForm(in *mtproto.InputInvoice) (*mtproto.Payments_PaymentForm, error) {
	quote, err := c.starGiftPaymentInvoice(in)
	if err != nil {
		return nil, err
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
		GiftID            int64                 `json:"gift_id"`
		GiftSlug          string                `json:"gift_slug"`
		TargetID          int64                 `json:"target_id,omitempty"`
		Stars             int64                 `json:"stars"`
		KeepOriginal      bool                  `json:"keep_original_details,omitempty"`
	}{"get_gift_form", quote.CallerID, quote.PayerID, quote.Product, quote.Invoice, quote.GiftID, quote.GiftSlug, quote.TargetID, quote.Stars, quote.KeepOriginal}
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
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, mtproto.ErrPaymentUnsupported
	}
	if !c.paymentProviderResponseAuthenticated(resp, responseBody) {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	var result paymentFormProviderResponse
	if err = json.Unmarshal(responseBody, &result); err != nil || !validStarGiftPaymentForm(result, quote) {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	return mtproto.MakeTLPaymentsPaymentFormStars(result.Form).To_Payments_PaymentForm(), nil
}

func validStarGiftPaymentResult(result paymentProviderResponse, invoice starGiftPaymentInvoice, requestKey, fingerprint string, formID int64) bool {
	return result.Verified && result.UserID == invoice.CallerID && result.BeneficiaryUserID == invoice.PayerID &&
		result.RequestKey == requestKey && result.Fingerprint == fingerprint && result.FormID == formID &&
		result.ProductType == invoice.Product && result.Stars == invoice.Stars && result.Currency == "XTR" && result.Amount == invoice.Stars &&
		strings.TrimSpace(result.TransactionID) != "" && len(bytes.TrimSpace(result.Receipt)) > 0 && !bytes.Equal(bytes.TrimSpace(result.Receipt), []byte("null"))
}

func (c *ApiFullCore) settleStarGiftWithPaymentProvider(in *mtproto.TLPaymentsSendStarsForm) (*mtproto.Payments_PaymentResult, error) {
	if in == nil || in.GetFormId() <= 0 || in.GetInvoice() == nil {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	quote, err := c.starGiftPaymentInvoice(in.GetInvoice())
	if err != nil {
		return nil, err
	}
	fingerprint := starGiftPaymentFingerprint(quote, in.GetFormId())
	requestKey := makeStarGiftPaymentRequestKey(quote, in.GetFormId(), fingerprint)
	if fingerprint == "" || requestKey == "" {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if _, _, _, err = c.configuredPaymentProvider(); err != nil {
		return nil, err
	}
	lockWait := 10 * time.Second
	if c.svcCtx.Config.PaymentProviderTimeoutSeconds > 0 {
		lockWait = time.Duration(c.svcCtx.Config.PaymentProviderTimeoutSeconds+5) * time.Second
	}
	release, err := domain.LockPaymentRequest(c.secretContext(), quote.PayerID, requestKey, lockWait)
	if err != nil {
		return nil, mtproto.ErrPaymentUnsupported
	}
	defer release()
	client, endpoint, providerKey, err := c.configuredPaymentProvider()
	if err != nil {
		return nil, err
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
		GiftID            int64                 `json:"gift_id"`
		GiftSlug          string                `json:"gift_slug"`
		TargetID          int64                 `json:"target_id,omitempty"`
		Stars             int64                 `json:"stars"`
		KeepOriginal      bool                  `json:"keep_original_details,omitempty"`
	}{"settle_gift", quote.CallerID, quote.PayerID, quote.Product, requestKey, fingerprint, in.GetFormId(), quote.Invoice, quote.GiftID, quote.GiftSlug, quote.TargetID, quote.Stars, quote.KeepOriginal}
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
	req.Header.Set("Idempotency-Key", paymentProviderIdempotencyKey(quote.CallerID, requestKey))
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
	var result paymentProviderResponse
	if err = json.Unmarshal(responseBody, &result); err != nil || !validStarGiftPaymentResult(result, quote, requestKey, fingerprint, in.GetFormId()) {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	return mtproto.MakeTLPaymentsPaymentResult(&mtproto.Payments_PaymentResult{Updates: mtproto.MakeEmptyUpdates()}).To_Payments_PaymentResult(), nil
}

func isBusinessGiftPaymentInvoice(invoice *mtproto.InputInvoice) bool {
	if invoice == nil {
		return false
	}
	return invoice.GetPredicateName() == mtproto.Predicate_inputInvoiceStarGiftUpgrade || invoice.GetPredicateName() == mtproto.Predicate_inputInvoiceStarGiftTransfer
}
