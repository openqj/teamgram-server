package core

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
)

func miniAppPeerValidationCore(callerID int64, users map[int64]*mtproto.ImmutableUser) *ApiFullCore {
	return &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: &miniBotAppsUserClient{users: users}}},
		MD:     &metadata.RpcMetadata{UserId: callerID},
	}
}

func TestMiniAppPeerValidationAcceptsSelfAndMessageDerivedUser(t *testing.T) {
	const (
		callerID = int64(992100)
		userID   = int64(992101)
		hash     = int64(882101)
	)
	c := miniAppPeerValidationCore(callerID, map[int64]*mtproto.ImmutableUser{
		userID: miniBotProfile(userID, hash, false),
	})
	cases := map[string]*mtproto.InputPeer{
		"self": mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		"explicit user": mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{
			UserId: userID, AccessHash: hash,
		}).To_InputPeer(),
		"user from message": mtproto.MakeTLInputPeerUserFromMessage(&mtproto.InputPeer{
			Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(), MsgId: 17, UserId: userID,
		}).To_InputPeer(),
	}
	for name, peer := range cases {
		t.Run(name, func(t *testing.T) {
			if err := c.validateMiniAppPeer(callerID, peer); err != nil {
				t.Fatalf("validate peer = %v, want nil", err)
			}
		})
	}
}

func TestMiniAppPeerValidationRejectsForgedOrMalformedUser(t *testing.T) {
	const (
		callerID = int64(992110)
		userID   = int64(992111)
		hash     = int64(882111)
	)
	c := miniAppPeerValidationCore(callerID, map[int64]*mtproto.ImmutableUser{
		userID: miniBotProfile(userID, hash, false),
	})
	cases := []struct {
		name string
		peer *mtproto.InputPeer
		want error
	}{
		{
			name: "stale access hash",
			peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: userID, AccessHash: hash + 1}).To_InputPeer(),
			want: mtproto.ErrUserIdInvalid,
		},
		{
			name: "missing access hash",
			peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: userID}).To_InputPeer(),
			want: mtproto.ErrUserIdInvalid,
		},
		{
			name: "unknown user",
			peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: userID + 1, AccessHash: hash}).To_InputPeer(),
			want: mtproto.ErrUserIdInvalid,
		},
		{
			name: "malformed from message",
			peer: mtproto.MakeTLInputPeerUserFromMessage(&mtproto.InputPeer{UserId: userID, MsgId: 17}).To_InputPeer(),
			want: mtproto.ErrPeerIdInvalid,
		},
		{
			name: "unknown constructor",
			peer: &mtproto.InputPeer{PredicateName: "inputPeerUnknown", UserId: userID, AccessHash: hash},
			want: mtproto.ErrInputConstructorInvalid,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := c.validateMiniAppPeer(callerID, tc.peer); !errors.Is(err, tc.want) {
				t.Fatalf("validate peer = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestMessagesRequestAppWebViewRejectsStalePeerBeforeControlPlaneLookup(t *testing.T) {
	if !persist.PostgresEnabled() {
		t.Skip("APIFULL PostgreSQL is not configured")
	}
	const (
		callerID = int64(992120)
		userID   = int64(992121)
		userHash = int64(882121)
		botID    = int64(992122)
		botHash  = int64(882122)
		appID    = int64(992123)
		appHash  = int64(882123)
	)
	c := miniAppPeerValidationCore(callerID, map[int64]*mtproto.ImmutableUser{
		userID: miniBotProfile(userID, userHash, false),
		botID:  miniBotProfile(botID, botHash, true),
	})
	app := mtproto.MakeTLInputBotAppID(&mtproto.InputBotApp{
		Id: appID, AccessHash: appHash, BotId: miniBotInput(botID, botHash),
	}).To_InputBotApp()
	canonicalPeer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{
		UserId: userID, AccessHash: userHash,
	}).To_InputPeer()
	stalePeer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{
		UserId: userID, AccessHash: userHash + 1,
	}).To_InputPeer()
	appKey := "bot:992122:id:992123"
	scopeKey := miniAppWebViewScopeKey(appKey, callerID, canonicalPeer, nil, "ios")
	resultJSON, err := json.Marshal(map[string]any{
		"predicate_name": "webViewResultUrl",
		"query_id_INT64": int64(992124),
		"url":            "https://mini.example/canonical",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = persist.PutBusinessControlRecord(persist.BusinessControlRecord{
		Operation: "messages.requestAppWebView", ScopeKey: scopeKey, CallerUserID: callerID, Result: resultJSON,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = persist.PutBusinessControlRecord(persist.BusinessControlRecord{
			Operation: "messages.requestAppWebView", ScopeKey: scopeKey, CallerUserID: callerID, State: "revoked", Result: resultJSON,
		})
	})

	request := &mtproto.TLMessagesRequestAppWebView53618BCE{Peer: stalePeer, App: app, Platform: "ios"}
	if result, err := c.MessagesRequestAppWebView53618BCE(request); result != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("stale current peer = (%#v, %v), want USER_ID_INVALID before local result", result, err)
	}
	legacy := &mtproto.TLMessagesRequestAppWebView8C5A3B3C{Peer: stalePeer, App: app, Platform: "ios"}
	if result, err := c.MessagesRequestAppWebView8C5A3B3C(legacy); result != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("stale legacy peer = (%#v, %v), want USER_ID_INVALID before local result", result, err)
	}
}
