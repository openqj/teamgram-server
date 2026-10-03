// Copyright 2022 Teamgram Authors
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

package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/svc"
	syncclient "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	chatclient "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type setTypingSyncClientStub struct {
	syncclient.SyncClient
	response *mtproto.Void
	err      error
	requests []*syncpb.TLSyncPushUpdates
}

func (s *setTypingSyncClientStub) SyncPushUpdates(_ context.Context, in *syncpb.TLSyncPushUpdates) (*mtproto.Void, error) {
	s.requests = append(s.requests, in)
	return s.response, s.err
}

type setTypingChatClientStub struct {
	chatclient.ChatClient
	response *mtproto.MutableChat
	err      error
	request  *chatpb.TLChatGetMutableChat
}

type setTypingUserClientStub struct {
	userclient.UserClient
	response *userpb.Vector_ImmutableUser
	err      error
	request  *userpb.TLUserGetMutableUsers
}

func (s *setTypingUserClientStub) UserGetMutableUsers(_ context.Context, in *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	s.request = in
	return s.response, s.err
}

func (s *setTypingChatClientStub) ChatGetMutableChat(_ context.Context, in *chatpb.TLChatGetMutableChat) (*mtproto.MutableChat, error) {
	s.request = in
	return s.response, s.err
}

func newSetTypingTestCore(syncStub *setTypingSyncClientStub, chatStub *setTypingChatClientStub, userStubs ...*setTypingUserClientStub) *DialogsCore {
	ctx := context.Background()
	d := &dao.Dao{
		SyncClient: syncStub,
		ChatClient: chatStub,
	}
	if len(userStubs) > 0 {
		d.UserClient = userStubs[0]
	}
	return &DialogsCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{
			Dao: d,
		},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
}

func setTypingImmutableUser(userID, accessHash int64) *mtproto.ImmutableUser {
	return &mtproto.ImmutableUser{User: &mtproto.UserData{Id: userID, AccessHash: accessHash}}
}

func setTypingUsers(targetID, targetAccessHash int64) *setTypingUserClientStub {
	datas := []*mtproto.ImmutableUser{setTypingImmutableUser(targetID, targetAccessHash)}
	if targetID != 42 {
		datas = append(datas, setTypingImmutableUser(42, 420))
	}
	return &setTypingUserClientStub{response: &userpb.Vector_ImmutableUser{Datas: datas}}
}

func setTypingAction() *mtproto.SendMessageAction {
	return mtproto.MakeTLSendMessageTypingAction(nil).To_SendMessageAction()
}

func setTypingMutableChat(chatID int64, participants ...*mtproto.ImmutableChatParticipant) *mtproto.MutableChat {
	return &mtproto.MutableChat{
		Chat:             &mtproto.ImmutableChat{Id: chatID, Title: "group"},
		ChatParticipants: participants,
	}
}

func setTypingParticipant(userID int64, state int32) *mtproto.ImmutableChatParticipant {
	return &mtproto.ImmutableChatParticipant{
		UserId:          userID,
		ParticipantType: mtproto.ChatMemberNormal,
		State:           state,
	}
}

func TestMessagesSetTypingPushesUserTypingAndPropagatesSyncFailures(t *testing.T) {
	t.Run("user peer", func(t *testing.T) {
		syncStub := &setTypingSyncClientStub{response: mtproto.EmptyVoid}
		userStub := setTypingUsers(84, 840)
		core := newSetTypingTestCore(syncStub, &setTypingChatClientStub{}, userStub)
		got, err := core.MessagesSetTyping(&mtproto.TLMessagesSetTyping{
			Peer:   mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 84, AccessHash: 840}).To_InputPeer(),
			Action: setTypingAction(),
		})
		if err != nil || got != mtproto.BoolTrue {
			t.Fatalf("MessagesSetTyping() = (%v, %v), want BoolTrue", got, err)
		}
		if len(syncStub.requests) != 1 || syncStub.requests[0].GetUserId() != 84 {
			t.Fatalf("SyncPushUpdates requests = %v, want one push to user 84", syncStub.requests)
		}
		if userStub.request == nil || len(userStub.request.GetId()) != 2 || len(userStub.request.GetTo()) != 1 || userStub.request.GetTo()[0] != 42 {
			t.Fatalf("UserGetMutableUsers request = %v, want target and self in requester context", userStub.request)
		}
		update := syncStub.requests[0].GetUpdates().GetUpdate()
		if update == nil || update.GetPredicateName() != mtproto.Predicate_updateUserTyping || update.To_UpdateUserTyping().GetUserId() != 42 {
			t.Fatalf("typing update = %v, want current user 42", update)
		}
	})

	t.Run("self peer", func(t *testing.T) {
		syncStub := &setTypingSyncClientStub{response: mtproto.EmptyVoid}
		core := newSetTypingTestCore(syncStub, &setTypingChatClientStub{}, setTypingUsers(42, 420))
		got, err := core.MessagesSetTyping(&mtproto.TLMessagesSetTyping{
			Peer:   mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
			Action: setTypingAction(),
		})
		if err != nil || got != mtproto.BoolTrue || len(syncStub.requests) != 1 || syncStub.requests[0].GetUserId() != 42 {
			t.Fatalf("MessagesSetTyping(self) = (%v, %v), requests %v; want push to self", got, err, syncStub.requests)
		}
	})

	t.Run("sync error", func(t *testing.T) {
		wantErr := errors.New("sync unavailable")
		syncStub := &setTypingSyncClientStub{err: wantErr}
		core := newSetTypingTestCore(syncStub, &setTypingChatClientStub{}, setTypingUsers(84, 840))
		got, err := core.MessagesSetTyping(&mtproto.TLMessagesSetTyping{
			Peer:   mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 84, AccessHash: 840}).To_InputPeer(),
			Action: setTypingAction(),
		})
		if !errors.Is(err, wantErr) || got != nil {
			t.Fatalf("MessagesSetTyping() = (%v, %v), want propagated sync error", got, err)
		}
	})

	t.Run("nil sync response", func(t *testing.T) {
		syncStub := &setTypingSyncClientStub{}
		core := newSetTypingTestCore(syncStub, &setTypingChatClientStub{}, setTypingUsers(84, 840))
		got, err := core.MessagesSetTyping(&mtproto.TLMessagesSetTyping{
			Peer:   mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 84, AccessHash: 840}).To_InputPeer(),
			Action: setTypingAction(),
		})
		if err == nil || got != nil {
			t.Fatalf("MessagesSetTyping() = (%v, %v), want nil sync-response error", got, err)
		}
	})

	t.Run("user lookup error", func(t *testing.T) {
		wantErr := errors.New("user service unavailable")
		userStub := &setTypingUserClientStub{err: wantErr}
		core := newSetTypingTestCore(&setTypingSyncClientStub{response: mtproto.EmptyVoid}, &setTypingChatClientStub{}, userStub)
		got, err := core.MessagesSetTyping(&mtproto.TLMessagesSetTyping{
			Peer:   mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 84, AccessHash: 840}).To_InputPeer(),
			Action: setTypingAction(),
		})
		if !errors.Is(err, wantErr) || got != nil {
			t.Fatalf("MessagesSetTyping() = (%v, %v), want propagated user lookup error", got, err)
		}
	})

	t.Run("nil user lookup response", func(t *testing.T) {
		syncStub := &setTypingSyncClientStub{response: mtproto.EmptyVoid}
		core := newSetTypingTestCore(syncStub, &setTypingChatClientStub{}, &setTypingUserClientStub{})
		got, err := core.MessagesSetTyping(&mtproto.TLMessagesSetTyping{
			Peer:   mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 84, AccessHash: 840}).To_InputPeer(),
			Action: setTypingAction(),
		})
		if err == nil || got != nil || len(syncStub.requests) != 0 {
			t.Fatalf("MessagesSetTyping() = (%v, %v), pushes %d; want nil user-response error without push", got, err, len(syncStub.requests))
		}
	})

	t.Run("wrong access hash", func(t *testing.T) {
		syncStub := &setTypingSyncClientStub{response: mtproto.EmptyVoid}
		core := newSetTypingTestCore(syncStub, &setTypingChatClientStub{}, setTypingUsers(84, 841))
		got, err := core.MessagesSetTyping(&mtproto.TLMessagesSetTyping{
			Peer:   mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 84, AccessHash: 840}).To_InputPeer(),
			Action: setTypingAction(),
		})
		if !errors.Is(err, mtproto.ErrUserIdInvalid) || got != nil || len(syncStub.requests) != 0 {
			t.Fatalf("MessagesSetTyping() = (%v, %v), pushes %d; want USER_ID_INVALID without push", got, err, len(syncStub.requests))
		}
	})

}

func TestMessagesSetTypingRejectsMissingRequiredArguments(t *testing.T) {
	syncStub := &setTypingSyncClientStub{response: mtproto.EmptyVoid}
	userStub := setTypingUsers(84, 840)
	core := newSetTypingTestCore(syncStub, &setTypingChatClientStub{}, userStub)
	got, err := core.MessagesSetTyping(&mtproto.TLMessagesSetTyping{
		Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 84, AccessHash: 840}).To_InputPeer(),
	})
	if !errors.Is(err, mtproto.ErrSendMessageTypeInvalid) || got != nil || userStub.request != nil {
		t.Fatalf("MessagesSetTyping(nil action) = (%v, %v); user lookups %v, want SEND_MESSAGE_TYPE_INVALID without backend calls", got, err, userStub.request)
	}
}

func TestMessagesSetTypingRejectsMissingAuthentication(t *testing.T) {
	core := newSetTypingTestCore(&setTypingSyncClientStub{response: mtproto.EmptyVoid}, &setTypingChatClientStub{}, setTypingUsers(84, 840))
	core.MD = nil

	got, err := core.MessagesSetTyping(&mtproto.TLMessagesSetTyping{
		Peer:   mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 84, AccessHash: 840}).To_InputPeer(),
		Action: setTypingAction(),
	})
	if !errors.Is(err, mtproto.ErrAuthKeyUnregistered) || got != nil {
		t.Fatalf("MessagesSetTyping(missing auth) = (%v, %v), want AUTH_KEY_UNREGISTERED", got, err)
	}
}

func TestMessagesSetTypingBroadcastsOnlyToActiveChatMembers(t *testing.T) {
	syncStub := &setTypingSyncClientStub{response: mtproto.EmptyVoid}
	chatStub := &setTypingChatClientStub{response: setTypingMutableChat(17,
		setTypingParticipant(42, mtproto.ChatMemberStateNormal),
		setTypingParticipant(84, mtproto.ChatMemberStateNormal),
		setTypingParticipant(85, mtproto.ChatMemberStateNormal),
		setTypingParticipant(86, mtproto.ChatMemberStateLeft),
	)}
	core := newSetTypingTestCore(syncStub, chatStub)
	got, err := core.MessagesSetTyping(&mtproto.TLMessagesSetTyping{
		Peer:   mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 17}).To_InputPeer(),
		Action: setTypingAction(),
	})
	if err != nil || got != mtproto.BoolTrue {
		t.Fatalf("MessagesSetTyping() = (%v, %v), want BoolTrue", got, err)
	}
	if chatStub.request == nil || chatStub.request.GetChatId() != 17 {
		t.Fatalf("ChatGetMutableChat request = %v, want chat 17", chatStub.request)
	}
	if len(syncStub.requests) != 2 {
		t.Fatalf("SyncPushUpdates requests = %d, want active non-sender members 84 and 85", len(syncStub.requests))
	}
	gotRecipients := map[int64]bool{}
	for _, request := range syncStub.requests {
		if request.GetUpdates().GetUpdate().GetPredicateName() != mtproto.Predicate_updateChatUserTyping {
			t.Fatalf("typing update = %v, want chat typing", request.GetUpdates().GetUpdate())
		}
		gotRecipients[request.GetUserId()] = true
	}
	if len(gotRecipients) != 2 || !gotRecipients[84] || !gotRecipients[85] {
		t.Fatalf("typing recipients = %v, want {84, 85}", gotRecipients)
	}
}

func TestMessagesSetTypingFailsClosedForChatHydrationAndSyncFailures(t *testing.T) {
	t.Run("chat lookup error", func(t *testing.T) {
		wantErr := errors.New("chat unavailable")
		chatStub := &setTypingChatClientStub{err: wantErr}
		core := newSetTypingTestCore(&setTypingSyncClientStub{response: mtproto.EmptyVoid}, chatStub)
		got, err := core.MessagesSetTyping(&mtproto.TLMessagesSetTyping{
			Peer:   mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 17}).To_InputPeer(),
			Action: setTypingAction(),
		})
		if !errors.Is(err, wantErr) || got != nil {
			t.Fatalf("MessagesSetTyping() = (%v, %v), want propagated chat error", got, err)
		}
	})

	t.Run("nil chat response", func(t *testing.T) {
		core := newSetTypingTestCore(&setTypingSyncClientStub{response: mtproto.EmptyVoid}, &setTypingChatClientStub{})
		got, err := core.MessagesSetTyping(&mtproto.TLMessagesSetTyping{
			Peer:   mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 17}).To_InputPeer(),
			Action: setTypingAction(),
		})
		if err == nil || got != nil {
			t.Fatalf("MessagesSetTyping() = (%v, %v), want nil-chat error", got, err)
		}
	})

	t.Run("sync error", func(t *testing.T) {
		wantErr := errors.New("sync unavailable")
		syncStub := &setTypingSyncClientStub{err: wantErr}
		chatStub := &setTypingChatClientStub{response: setTypingMutableChat(17,
			setTypingParticipant(42, mtproto.ChatMemberStateNormal),
			setTypingParticipant(84, mtproto.ChatMemberStateNormal),
		)}
		core := newSetTypingTestCore(syncStub, chatStub)
		got, err := core.MessagesSetTyping(&mtproto.TLMessagesSetTyping{
			Peer:   mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 17}).To_InputPeer(),
			Action: setTypingAction(),
		})
		if !errors.Is(err, wantErr) || got != nil {
			t.Fatalf("MessagesSetTyping() = (%v, %v), want propagated sync error", got, err)
		}
	})

	t.Run("sender is not an active member", func(t *testing.T) {
		syncStub := &setTypingSyncClientStub{response: mtproto.EmptyVoid}
		chatStub := &setTypingChatClientStub{response: setTypingMutableChat(17,
			setTypingParticipant(42, mtproto.ChatMemberStateLeft),
			setTypingParticipant(84, mtproto.ChatMemberStateNormal),
		)}
		core := newSetTypingTestCore(syncStub, chatStub)
		got, err := core.MessagesSetTyping(&mtproto.TLMessagesSetTyping{
			Peer:   mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 17}).To_InputPeer(),
			Action: setTypingAction(),
		})
		if !errors.Is(err, mtproto.ErrUserNotParticipant) || got != nil || len(syncStub.requests) != 0 {
			t.Fatalf("MessagesSetTyping() = (%v, %v), pushes %d; want USER_NOT_PARTICIPANT without pushes", got, err, len(syncStub.requests))
		}
	})
}

func TestMessagesSetTypingChannelFailsClosedWithoutMemberBroadcastPath(t *testing.T) {
	syncStub := &setTypingSyncClientStub{response: mtproto.EmptyVoid}
	chatStub := &setTypingChatClientStub{}
	core := newSetTypingTestCore(syncStub, chatStub)
	got, err := core.MessagesSetTyping(&mtproto.TLMessagesSetTyping{
		Peer:   mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 90, AccessHash: 900}).To_InputPeer(),
		Action: setTypingAction(),
	})
	if !errors.Is(err, mtproto.ErrMethodNotImpl) || got != nil {
		t.Fatalf("MessagesSetTyping(channel) = (%v, %v), want METHOD_NOT_IMPL", got, err)
	}
	if len(syncStub.requests) != 0 || chatStub.request != nil {
		t.Fatalf("channel typing touched unrelated broadcast clients: sync=%v chat=%v", syncStub.requests, chatStub.request)
	}
}
