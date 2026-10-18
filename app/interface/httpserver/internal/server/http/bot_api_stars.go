// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func getMyStarBalance(ctx *svc.ServiceContext, requestCtx context.Context, botID int64) (any, int, int, string) {
	if ctx == nil || ctx.StarsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: payment service is not configured"
	}
	status, err := ctx.StarsClient.PaymentsGetStarsStatus(botRPCContext(requestCtx, botID), &mtproto.TLPaymentsGetStarsStatus{
		Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
	})
	if err != nil {
		// A bot with no Stars ledger has a valid zero balance. The MTProto
		// implementation deliberately does not expose an empty account, while
		// the Bot API must return a StarAmount for every authenticated bot.
		if errors.Is(err, mtproto.ErrMethodNotImpl) {
			return botapi.StarAmount{}, http.StatusOK, 0, ""
		}
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if status == nil {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: Stars balance was not returned"
	}
	return botapi.StarAmount{
		Amount:         status.GetBalance_INT64(),
		NanostarAmount: starsNanostarAmount(status.GetBalance_STARSAMOUNT()),
	}, http.StatusOK, 0, ""
}

func getStarTransactions(ctx *svc.ServiceContext, requestCtx context.Context, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.StarsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: payment service is not configured"
	}
	offset := request.string("offset")
	limit, err := request.int("limit")
	if err != nil || (limit != 0 && (limit < 1 || limit > 100)) {
		return nil, http.StatusBadRequest, 400, "Bad Request: limit is invalid"
	}
	status, err := ctx.StarsClient.PaymentsGetStarsTransactions(botRPCContext(requestCtx, botID), &mtproto.TLPaymentsGetStarsTransactions{
		Peer:   mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Offset: offset,
		Limit:  int32(limit),
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if status == nil {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: Stars transactions were not returned"
	}
	transactions := make([]botapi.StarTransaction, 0, len(status.GetHistory()))
	for _, transaction := range status.GetHistory() {
		if transaction == nil {
			continue
		}
		transactions = append(transactions, botAPIStarTransaction(transaction, status.GetUsers(), status.GetChats()))
	}
	result := botapi.StarTransactions{Transactions: transactions}
	if status.GetNextOffset() != nil {
		result.NextOffset = status.GetNextOffset().GetValue()
	}
	return result, http.StatusOK, 0, ""
}

func refundStarPayment(ctx *svc.ServiceContext, requestCtx context.Context, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.StarsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: payment service is not configured"
	}
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	chargeID := strings.TrimSpace(request.string("telegram_payment_charge_id"))
	if chargeID == "" || len(chargeID) > 512 {
		return nil, http.StatusBadRequest, 400, "Bad Request: telegram_payment_charge_id is invalid"
	}
	inputUser, err := botAPIStarsInputUser(ctx, requestCtx, botID, userID)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if _, err = ctx.StarsClient.PaymentsRefundStarsCharge(botRPCContext(requestCtx, botID), &mtproto.TLPaymentsRefundStarsCharge{
		UserId:   inputUser,
		ChargeId: chargeID,
	}); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func editUserStarSubscription(ctx *svc.ServiceContext, requestCtx context.Context, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.StarSubscriptionsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: payment service is not configured"
	}
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	chargeID := strings.TrimSpace(request.string("telegram_payment_charge_id"))
	if chargeID == "" || len(chargeID) > 512 {
		return nil, http.StatusBadRequest, 400, "Bad Request: telegram_payment_charge_id is invalid"
	}
	if len(request.raw("is_canceled")) == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: is_canceled is required"
	}
	isCanceled, err := request.bool("is_canceled")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: is_canceled is invalid"
	}
	inputUser, err := botAPIStarsInputUser(ctx, requestCtx, botID, userID)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	result, err := ctx.StarSubscriptionsClient.PaymentsBotCancelStarsSubscription(botRPCContext(requestCtx, botID), &mtproto.TLPaymentsBotCancelStarsSubscription{
		Restore:         !isCanceled,
		UserId:          inputUser,
		ChargeId_STRING: chargeID,
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if result == nil {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: subscription state was not returned"
	}
	return mtproto.FromBool(result), http.StatusOK, 0, ""
}

func botAPIStarsInputUser(ctx *svc.ServiceContext, requestCtx context.Context, botID, userID int64) (*mtproto.InputUser, error) {
	if userID == botID {
		return mtproto.MakeTLInputUserSelf(nil).To_InputUser(), nil
	}
	if ctx == nil || ctx.UserClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	profile, err := ctx.UserClient.UserGetImmutableUser(botRPCContext(requestCtx, botID), &userpb.TLUserGetImmutableUser{Id: userID})
	if err != nil {
		return nil, err
	}
	if profile == nil || profile.GetUser() == nil || profile.GetUser().GetId() != userID || profile.GetUser().GetDeleted() || profile.GetUser().GetAccessHash() == 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	return mtproto.MakeTLInputUser(&mtproto.InputUser{
		UserId:     userID,
		AccessHash: profile.GetUser().GetAccessHash(),
	}).To_InputUser(), nil
}

func starsNanostarAmount(amount *mtproto.StarsAmount) int {
	if amount == nil {
		return 0
	}
	return int(amount.GetNanos())
}

func botAPIStarTransaction(transaction *mtproto.StarsTransaction, users []*mtproto.User, chats []*mtproto.Chat) botapi.StarTransaction {
	amount := transaction.GetStars_INT64()
	nanostars := starsNanostarAmount(transaction.GetAmount())
	if amount == 0 && transaction.GetAmount() != nil {
		amount = transaction.GetAmount().GetAmount()
	}
	result := botapi.StarTransaction{
		ID:             transaction.GetId(),
		Amount:         amount,
		NanostarAmount: nanostars,
		Date:           int64(transaction.GetDate()),
	}
	if transaction.GetPeer() != nil && transaction.GetPeer().GetPeer() != nil {
		partner := botAPITransactionPartner(transaction.GetPeer().GetPeer(), users, chats)
		if amount >= 0 {
			result.Source = partner
		} else {
			result.Receiver = partner
		}
	}
	return result
}

func botAPITransactionPartner(peer *mtproto.Peer, users []*mtproto.User, chats []*mtproto.Chat) *botapi.TransactionPartner {
	if peer == nil {
		return nil
	}
	if peer.GetUserId() > 0 {
		for _, user := range users {
			if user != nil && user.GetId() == peer.GetUserId() {
				return &botapi.TransactionPartner{Type: "user", User: botapi.UserFromMTProto(user)}
			}
		}
		return &botapi.TransactionPartner{Type: "user"}
	}
	if peer.GetChatId() > 0 || peer.GetChannelId() > 0 {
		return &botapi.TransactionPartner{Type: "chat", Chat: botapi.ChatFromPeer(peer, users, chats)}
	}
	return nil
}
