package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
)

func sendGift(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.PaymentsClient == nil || ctx.StarsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: payment service is not configured"
	}
	giftIDValue := strings.TrimSpace(request.string("gift_id"))
	giftID, err := strconv.ParseInt(giftIDValue, 10, 64)
	if giftIDValue == "" || err != nil || giftID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: gift_id is invalid"
	}
	if request.has("user_id") == request.has("chat_id") {
		return nil, http.StatusBadRequest, 400, "Bad Request: exactly one of user_id or chat_id is required"
	}
	var target *mtproto.InputUser
	if request.has("user_id") {
		userID, parseErr := request.int64("user_id")
		if parseErr != nil || userID <= 0 {
			return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
		}
		target, err = botAPIStarsInputUser(ctx, r.Context(), botID, userID)
	} else {
		_, peerType, peerID, resolveErr := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
		if resolveErr != nil || peerID <= 0 {
			return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
		}
		if peerType != mtproto.PEER_USER {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: gift settlement for chats and channels is not represented by the local gift ledger"
		}
		target, err = botAPIStarsInputUser(ctx, r.Context(), botID, peerID)
	}
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	payForUpgrade, err := request.bool("pay_for_upgrade")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: pay_for_upgrade is invalid"
	}
	text, entities, err := botAPITextEntities(request, "text", "text_entities", "text_parse_mode")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	if err := validateBotAPITextLength(text, 128, "text"); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	message := (*mtproto.TextWithEntities)(nil)
	if request.has("text") {
		message = mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{Text: text, Entities: botapi.MessageEntitiesToMTProto(entities)}).To_TextWithEntities()
	}
	invoice := mtproto.MakeTLInputInvoiceStarGift(&mtproto.InputInvoice{
		UserId:         target,
		GiftId:         giftID,
		IncludeUpgrade: payForUpgrade,
		Message:        message,
	}).To_InputInvoice()
	if err = settleBotAPILayer229StarsInvoice(ctx.PaymentsClient, ctx.StarsClient, botRPCContext(r.Context(), botID), invoice, mtproto.Predicate_payments_paymentFormStarGift); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func giftPremiumSubscription(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.PaymentsClient == nil || ctx.StarsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: payment service is not configured"
	}
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	months, err := request.int("month_count")
	if err != nil || (months != 3 && months != 6 && months != 12) {
		return nil, http.StatusBadRequest, 400, "Bad Request: month_count must be 3, 6, or 12"
	}
	target, err := botAPIStarsInputUser(ctx, r.Context(), botID, userID)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if !request.has("star_count") {
		return nil, http.StatusBadRequest, 400, "Bad Request: star_count is required"
	}
	starCount, err := request.int64("star_count")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: star_count is invalid"
	}
	expectedStars := int64(0)
	switch months {
	case 3:
		expectedStars = 1000
	case 6:
		expectedStars = 1500
	case 12:
		expectedStars = 2500
	}
	if starCount != expectedStars {
		return nil, http.StatusBadRequest, 400, "Bad Request: star_count does not match month_count"
	}
	text, entities, err := botAPITextEntities(request, "text", "text_entities", "text_parse_mode")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	message := (*mtproto.TextWithEntities)(nil)
	if request.has("text") {
		message = mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{Text: text, Entities: botapi.MessageEntitiesToMTProto(entities)}).To_TextWithEntities()
	}
	invoice := mtproto.MakeTLInputInvoicePremiumGiftStars(&mtproto.InputInvoice{
		UserId:  target,
		Months:  int32(months),
		Message: message,
	}).To_InputInvoice()
	if err = settleBotAPILayer229StarsInvoice(ctx.PaymentsClient, ctx.StarsClient, botRPCContext(r.Context(), botID), invoice, mtproto.Predicate_payments_paymentFormStars); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func transferBusinessAccountStars(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.PaymentsClient == nil || ctx.StarsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: payment service is not configured"
	}
	connectionID := strings.TrimSpace(request.string("business_connection_id"))
	if connectionID == "" || len(connectionID) > 128 {
		return nil, http.StatusBadRequest, 400, "Bad Request: business_connection_id is invalid"
	}
	scope, err := resolveBotAPIBusinessScope(ctx, r.Context(), botID, connectionID)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if !scope.CanTransferStars {
		return nil, http.StatusBadRequest, 400, "Bad Request: business connection does not allow Stars transfers"
	}
	stars, err := request.int64("star_count")
	if err != nil || stars < 1 || stars > 10000 {
		return nil, http.StatusBadRequest, 400, "Bad Request: star_count must be between 1 and 10000"
	}
	invoice := mtproto.MakeTLInputInvoiceBusinessBotTransferStars(&mtproto.InputInvoice{
		Bot:   mtproto.MakeTLInputUserSelf(nil).To_InputUser(),
		Stars: stars,
	}).To_InputInvoice()
	callCtx := businessBotCallContext(r.Context(), botID, scope)
	if err = settleBotAPILayer229StarsInvoice(ctx.PaymentsClient, ctx.StarsClient, callCtx, invoice, mtproto.Predicate_payments_paymentFormStars); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func settleBotAPILayer229StarsInvoice(paymentClient mtproto.RPCPaymentsClient, starsClient mtproto.RPCStarsClient, callCtx context.Context, invoice *mtproto.InputInvoice, expectedForm string) error {
	if paymentClient == nil || starsClient == nil || invoice == nil {
		return mtproto.ErrPaymentUnsupported
	}
	form, err := paymentClient.PaymentsGetPaymentForm(callCtx, &mtproto.TLPaymentsGetPaymentForm{Invoice: invoice})
	if err != nil {
		return err
	}
	if form == nil || form.GetFormId() <= 0 || form.GetPredicateName() != expectedForm {
		return errors.New("payment form did not match the requested Stars invoice")
	}
	result, err := starsClient.PaymentsSendStarsForm(callCtx, &mtproto.TLPaymentsSendStarsForm{FormId: form.GetFormId(), Invoice: invoice})
	if err != nil {
		return err
	}
	if result == nil || result.GetPredicateName() != mtproto.Predicate_payments_paymentResult || result.GetUpdates() == nil {
		return errors.New("Stars payment result was not returned")
	}
	return nil
}
