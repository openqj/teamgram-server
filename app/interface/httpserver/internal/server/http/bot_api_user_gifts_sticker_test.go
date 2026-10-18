// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/grpc"
)

type botAPIUserGiftsStub struct {
	mtproto.RPCGiftsClient
	request *mtproto.TLPaymentsGetUserStarGifts
	result  *mtproto.Payments_UserStarGifts
}

func (s *botAPIUserGiftsStub) PaymentsGetUserStarGifts(_ context.Context, request *mtproto.TLPaymentsGetUserStarGifts, _ ...grpc.CallOption) (*mtproto.Payments_UserStarGifts, error) {
	s.request = request
	return s.result, nil
}

type botAPIUserGiftsUserStub struct {
	userclient.UserClient
	user *mtproto.ImmutableUser
}

func (s *botAPIUserGiftsUserStub) UserGetImmutableUser(context.Context, *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	return s.user, nil
}

type botAPIStickerReplaceStub struct {
	mtproto.RPCStickersClient
}

func (botAPIStickerReplaceStub) StickersReplaceSticker(context.Context, *mtproto.TLStickersReplaceSticker, ...grpc.CallOption) (*mtproto.Messages_StickerSet, error) {
	return &mtproto.Messages_StickerSet{}, nil
}

func TestGetUserGiftsUsesRequestedUserGiftRPC(t *testing.T) {
	gifts := &botAPIUserGiftsStub{
		result: mtproto.MakeTLPaymentsUserStarGifts(&mtproto.Payments_UserStarGifts{Count: 0}).To_Payments_UserStarGifts(),
	}
	service := &svc.ServiceContext{
		GiftsClient: gifts,
		UserClient: &botAPIUserGiftsUserStub{user: &mtproto.ImmutableUser{
			User: &mtproto.UserData{Id: 7, AccessHash: 99},
		}},
	}
	request := &botAPIRequest{values: map[string]json.RawMessage{
		"user_id": json.RawMessage("7"),
		"offset":  json.RawMessage(`"cursor"`),
		"limit":   json.RawMessage("2"),
	}}
	result, status, code, description := getUserGifts(service, httptest.NewRequest("POST", "/bot/token/getUserGifts", nil), 42, request)
	if status != 200 || code != 0 || description != "" {
		t.Fatalf("getUserGifts = (%#v, %d, %d, %q), want success", result, status, code, description)
	}
	if _, ok := result.(*botapi.OwnedGifts); !ok {
		t.Fatalf("getUserGifts result = %T, want *botapi.OwnedGifts", result)
	}
	if gifts.request == nil || gifts.request.GetOffset() != "cursor" || gifts.request.GetLimit() != 2 {
		t.Fatalf("user gift request = %#v, want offset cursor and limit 2", gifts.request)
	}
	if gifts.request.GetUserId() == nil || gifts.request.GetUserId().GetUserId() != 7 || gifts.request.GetUserId().GetAccessHash() != 99 {
		t.Fatalf("user gift input user = %#v, want user 7 access hash 99", gifts.request.GetUserId())
	}
}

func TestGetUserGiftsRejectsUnsupportedSavedGiftFilters(t *testing.T) {
	for _, filter := range []string{"exclude_unsaved", "exclude_saved"} {
		t.Run(filter, func(t *testing.T) {
			gifts := &botAPIUserGiftsStub{}
			service := &svc.ServiceContext{
				GiftsClient: gifts,
				UserClient: &botAPIUserGiftsUserStub{user: &mtproto.ImmutableUser{
					User: &mtproto.UserData{Id: 7, AccessHash: 99},
				}},
			}
			request := &botAPIRequest{values: map[string]json.RawMessage{
				"user_id": json.RawMessage("7"),
				filter:    json.RawMessage("true"),
			}}
			result, status, code, description := getUserGifts(service, httptest.NewRequest("POST", "/bot/token/getUserGifts", nil), 42, request)
			if result != nil || status != http.StatusNotImplemented || code != 501 || description != "Not Implemented: exclude_unsaved and exclude_saved are not supported by the Layer 229 user gift adapter" {
				t.Fatalf("getUserGifts %s = (%#v, %d, %d, %q), want explicit unsupported response", filter, result, status, code, description)
			}
			if gifts.request != nil {
				t.Fatalf("unsupported %s reached the gifts RPC: %#v", filter, gifts.request)
			}
		})
	}
}

func TestReplaceStickerInSetRequiresName(t *testing.T) {
	request := &botAPIRequest{values: map[string]json.RawMessage{
		"user_id":     json.RawMessage("42"),
		"old_sticker": json.RawMessage(`"document_1_2"`),
		"sticker":     json.RawMessage(`{"sticker":"document_3_4","format":"static","emoji_list":["😀"]}`),
	}}
	result, status, code, description := replaceStickerInSet(
		&svc.ServiceContext{StickersClient: botAPIStickerReplaceStub{}},
		httptest.NewRequest("POST", "/bot/token/replaceStickerInSet", nil),
		42,
		request,
	)
	if result != nil || status != 400 || code != 400 || description != "Bad Request: name is invalid" {
		t.Fatalf("replaceStickerInSet missing name = (%#v, %d, %d, %q), want required name error", result, status, code, description)
	}
}
