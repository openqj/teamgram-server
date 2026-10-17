package core

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/pkg/rpc/businessctx"
)

type localStarsInvoice struct {
	kind          string
	payerID       int64
	beneficiaryID int64
	recipientID   int64
	stars         int64
	months        int32
	giftSlug      string
	connectionID  string
	title         string
	description   string
}

func (c *ApiFullCore) localStarsInvoice(in *mtproto.InputInvoice) (localStarsInvoice, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return localStarsInvoice{}, err
	}
	if in == nil {
		return localStarsInvoice{}, mtproto.ErrInvoicePayloadInvalid
	}
	result := localStarsInvoice{payerID: uid}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputInvoiceStarGift:
		gift, found, findErr := domain.FindGiftByID(in.GetGiftId())
		if findErr != nil {
			return localStarsInvoice{}, starsDomainError(findErr)
		}
		if !found || gift.Stars <= 0 || strings.TrimSpace(gift.Slug) == "" {
			return localStarsInvoice{}, mtproto.ErrPaymentUnsupported
		}
		result.kind, result.stars, result.giftSlug = domain.StarsSpendGift, gift.Stars, gift.Slug
		result.title, result.description = "Star Gift", gift.Slug
		if user := in.GetUserId(); user != nil {
			peer := mtproto.FromInputUser(uid, user)
			if peer == nil || (peer.PeerType != mtproto.PEER_USER && peer.PeerType != mtproto.PEER_SELF) || peer.PeerId <= 0 {
				return localStarsInvoice{}, mtproto.ErrUserIdInvalid
			}
			result.recipientID = peer.PeerId
		} else if peer := in.GetPeer(); peer != nil {
			if peer.GetPredicateName() != mtproto.Predicate_inputPeerUser && peer.GetPredicateName() != mtproto.Predicate_inputPeerSelf {
				return localStarsInvoice{}, mtproto.ErrMethodNotImpl
			}
			resolved := mtproto.FromInputPeer(peer)
			if resolved == nil || resolved.PeerId <= 0 {
				return localStarsInvoice{}, mtproto.ErrPeerIdInvalid
			}
			result.recipientID = resolved.PeerId
		}
		if result.recipientID <= 0 {
			return localStarsInvoice{}, mtproto.ErrUserIdInvalid
		}
	case mtproto.Predicate_inputInvoicePremiumGiftStars:
		user := in.GetUserId()
		if user == nil {
			return localStarsInvoice{}, mtproto.ErrUserIdInvalid
		}
		peer := mtproto.FromInputUser(uid, user)
		if peer == nil || (peer.PeerType != mtproto.PEER_USER && peer.PeerType != mtproto.PEER_SELF) || peer.PeerId <= 0 {
			return localStarsInvoice{}, mtproto.ErrUserIdInvalid
		}
		result.kind, result.recipientID, result.months = domain.StarsSpendPremiumGift, peer.PeerId, in.GetMonths()
		switch result.months {
		case 3:
			result.stars = 1000
		case 6:
			result.stars = 1500
		case 12:
			result.stars = 2500
		default:
			return localStarsInvoice{}, mtproto.ErrInputRequestInvalid
		}
		result.title, result.description = "Premium Subscription Gift", "Telegram Premium"
	case mtproto.Predicate_inputInvoiceBusinessBotTransferStars:
		scope, ok := businessctx.ScopeFromContext(callContext(c))
		if !ok || scope.OwnerID <= 0 {
			return localStarsInvoice{}, mtproto.ErrMethodNotImpl
		}
		if !scope.CanTransferStars {
			return localStarsInvoice{}, mtproto.ErrMethodNotImpl
		}
		bot := in.GetBot()
		if bot == nil {
			return localStarsInvoice{}, mtproto.ErrUserIdInvalid
		}
		botPeer := mtproto.FromInputUser(uid, bot)
		if botPeer == nil || (botPeer.PeerType != mtproto.PEER_SELF && botPeer.PeerType != mtproto.PEER_USER) || botPeer.PeerId != uid {
			return localStarsInvoice{}, mtproto.ErrUserIdInvalid
		}
		if in.GetStars() < 1 || in.GetStars() > 10000 {
			return localStarsInvoice{}, mtproto.ErrInputRequestInvalid
		}
		result.kind, result.payerID, result.beneficiaryID, result.stars = domain.StarsSpendBusinessTransfer, scope.OwnerID, uid, in.GetStars()
		result.connectionID = scope.ConnectionID
		result.title, result.description = "Business Account Stars Transfer", scope.ConnectionID
	default:
		return localStarsInvoice{}, mtproto.ErrPaymentUnsupported
	}
	return result, nil
}

func localStarsInvoiceFingerprint(in *mtproto.InputInvoice, actor localStarsInvoice) string {
	body, err := json.Marshal(struct {
		Invoice *mtproto.InputInvoice `json:"invoice"`
		Kind    string                `json:"kind"`
		Payer   int64                 `json:"payer"`
		Benef   int64                 `json:"beneficiary"`
		Target  int64                 `json:"target"`
		Stars   int64                 `json:"stars"`
		Months  int32                 `json:"months"`
		Slug    string                `json:"slug"`
		Conn    string                `json:"connection_id,omitempty"`
	}{in, actor.kind, actor.payerID, actor.beneficiaryID, actor.recipientID, actor.stars, actor.months, actor.giftSlug, actor.connectionID})
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func localStarsFormID(fingerprint string) int64 {
	if fingerprint == "" {
		return 0
	}
	decoded, err := hex.DecodeString(fingerprint)
	if err != nil || len(decoded) < 8 {
		return 0
	}
	formID := int64(binary.BigEndian.Uint64(decoded[:8]) & 0x7fffffffffffffff)
	if formID == 0 {
		return 1
	}
	return formID
}

func localStarsPaymentForm(in *mtproto.InputInvoice, actor localStarsInvoice, formID int64) (*mtproto.Payments_PaymentForm, error) {
	if in == nil || formID <= 0 || actor.stars <= 0 {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	invoice := mtproto.MakeTLInvoice(&mtproto.Invoice{
		Currency: "XTR",
		Prices:   []*mtproto.LabeledPrice{mtproto.MakeTLLabeledPrice(&mtproto.LabeledPrice{Label: actor.title, Amount: actor.stars}).To_LabeledPrice()},
	}).To_Invoice()
	botID := actor.payerID
	if actor.kind == domain.StarsSpendBusinessTransfer {
		botID = actor.beneficiaryID
	}
	form := &mtproto.Payments_PaymentForm{FormId: formID, BotId: botID, Title: actor.title, Description: actor.description, Invoice: invoice, Users: []*mtproto.User{}}
	if actor.kind == domain.StarsSpendGift {
		return mtproto.MakeTLPaymentsPaymentFormStarGift(&mtproto.Payments_PaymentForm{FormId: formID, Invoice: invoice}).To_Payments_PaymentForm(), nil
	}
	return mtproto.MakeTLPaymentsPaymentFormStars(form).To_Payments_PaymentForm(), nil
}

func (c *ApiFullCore) localStarsPaymentForm(in *mtproto.InputInvoice) (*mtproto.Payments_PaymentForm, error) {
	actor, err := c.localStarsInvoice(in)
	if err != nil {
		return nil, err
	}
	fingerprint := localStarsInvoiceFingerprint(in, actor)
	formID := localStarsFormID(fingerprint)
	if formID <= 0 {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	return localStarsPaymentForm(in, actor, formID)
}

func (c *ApiFullCore) settleLocalStarsForm(in *mtproto.TLPaymentsSendStarsForm) (*mtproto.Payments_PaymentResult, error) {
	if in == nil || in.GetFormId() <= 0 || in.GetInvoice() == nil {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	actor, err := c.localStarsInvoice(in.GetInvoice())
	if err != nil {
		return nil, err
	}
	fingerprint := localStarsInvoiceFingerprint(in.GetInvoice(), actor)
	if localStarsFormID(fingerprint) != in.GetFormId() {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	if !domain.Ready() {
		return nil, mtproto.ErrPaymentUnsupported
	}
	requestKey := "local:" + actor.kind + ":" + fingerprint
	release, err := domain.LockPaymentRequest(c.secretContext(), actor.payerID, requestKey, 10*time.Second)
	if err != nil {
		return nil, mtproto.ErrPaymentUnsupported
	}
	defer release()
	request, err := domain.BeginPaymentRequest(actor.payerID, requestKey, "local", fingerprint, "XTR", actor.stars, 0, 0)
	if err != nil {
		return nil, err
	}
	transactionID := "local:" + fingerprint
	receipt, err := json.Marshal(struct {
		Kind          string `json:"kind"`
		Payer         int64  `json:"payer"`
		Beneficiary   int64  `json:"beneficiary,omitempty"`
		Recipient     int64  `json:"recipient,omitempty"`
		Stars         int64  `json:"stars"`
		Months        int32  `json:"months,omitempty"`
		Gift          string `json:"gift,omitempty"`
		ConnectionID  string `json:"connection_id,omitempty"`
		TransactionID string `json:"transaction_id"`
	}{actor.kind, actor.payerID, actor.beneficiaryID, actor.recipientID, actor.stars, actor.months, actor.giftSlug, actor.connectionID, transactionID})
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	settled, _, err := domain.SettleStarsSpendRequest(actor.payerID, actor.beneficiaryID, actor.recipientID, request.RequestKey, fingerprint, transactionID, "XTR", actor.title, actor.stars, actor.stars, receipt, actor.kind, actor.giftSlug, actor.months)
	if err != nil {
		return nil, starsSpendDomainError(err)
	}
	if settled.State != domain.PaymentStateSettled {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	return mtproto.MakeTLPaymentsPaymentResult(&mtproto.Payments_PaymentResult{Updates: mtproto.MakeEmptyUpdates()}).To_Payments_PaymentResult(), nil
}

func starsSpendDomainError(err error) error {
	if err == nil {
		return nil
	}
	if !domain.Ready() {
		return mtproto.ErrMethodNotImpl
	}
	if errors.Is(err, domain.ErrStarsBalanceExceeded) {
		return mtproto.ErrPaymentUnsupported
	}
	return err
}
