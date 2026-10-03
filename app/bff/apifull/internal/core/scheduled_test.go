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
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	"github.com/teamgram/teamgram-server/app/bff/apifull/schedstore"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/zeromicro/go-zero/core/logx"
)

type scheduledSenderStub struct {
	request *msgpb.TLMsgSendMessageV2
}

func (s *scheduledSenderStub) MsgSendMessageV2(_ context.Context, in *msgpb.TLMsgSendMessageV2) (*mtproto.Updates, error) {
	s.request = in
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: []*mtproto.Update{},
		Users:   []*mtproto.User{},
		Chats:   []*mtproto.Chat{},
	}).To_Updates(), nil
}

func scheduledTestCore(userID int64, sender dao.ScheduledMessageSender) *ApiFullCore {
	ctx := context.Background()
	return &ApiFullCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			ScheduledMessageSender: sender,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: userID, PermAuthKeyId: 99},
	}
}

func TestScheduledMessagesLifecycleScopesPeerAndConsumesSentRecords(t *testing.T) {
	const (
		userID   int64 = 979101
		receiver int64 = 979102
		other    int64 = 979103
	)
	if err := persist.Default.Set("sched:"+strconv.FormatInt(userID, 10), "[]"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := persist.Default.Set("sched:"+strconv.FormatInt(userID, 10), "[]"); err != nil {
			t.Errorf("clear scheduled store: %v", err)
		}
	})

	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: receiver}).To_InputPeer()
	otherPeer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: other}).To_InputPeer()
	if _, err := schedstore.Append(userID, mtproto.MakePeerUser(receiver), "send now", int32(time.Now().Add(time.Hour).Unix())); err != nil {
		t.Fatal("append scheduled message:", err)
	}
	pending, err := schedstore.List(userID)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending schedule: messages=%+v err=%v", pending, err)
	}
	id := pending[0].GetId()

	sender := &scheduledSenderStub{}
	c := scheduledTestCore(userID, sender)
	history, err := c.MessagesGetScheduledHistory(&mtproto.TLMessagesGetScheduledHistory{Peer: peer})
	if err != nil || len(history.GetMessages()) != 1 || history.GetMessages()[0].GetMessage() != "send now" {
		t.Fatalf("scheduled history: result=%+v err=%v", history, err)
	}
	byID, err := c.MessagesGetScheduledMessages(&mtproto.TLMessagesGetScheduledMessages{Peer: peer, Id: []int32{id}})
	if err != nil || len(byID.GetMessages()) != 1 || byID.GetMessages()[0].GetId() != id {
		t.Fatalf("scheduled lookup: result=%+v err=%v", byID, err)
	}
	if _, err = c.MessagesGetScheduledMessages(&mtproto.TLMessagesGetScheduledMessages{Peer: otherPeer, Id: []int32{id}}); !errors.Is(err, mtproto.ErrMessageIdInvalid) {
		t.Fatalf("cross-peer lookup: got %v, want MESSAGE_ID_INVALID", err)
	}

	updates, err := c.MessagesSendScheduledMessages(&mtproto.TLMessagesSendScheduledMessages{Peer: peer, Id: []int32{id}})
	if err != nil {
		t.Fatal("send scheduled:", err)
	}
	if sender.request == nil || sender.request.GetPeerType() != mtproto.PEER_USER || sender.request.GetPeerId() != receiver || len(sender.request.GetMessage()) != 1 {
		t.Fatalf("delivery request: %+v", sender.request)
	}
	outbox := sender.request.GetMessage()[0]
	if outbox.GetScheduleDate() != nil || outbox.GetMessage().GetMessage() != "send now" || outbox.GetMessage().GetFromScheduled() {
		t.Fatalf("delivery outbox: %+v", outbox)
	}
	if len(updates.GetUpdates()) != 1 || updates.GetUpdates()[0].GetPredicateName() != mtproto.Predicate_updateDeleteScheduledMessages {
		t.Fatalf("send scheduled updates: %+v", updates)
	}
	history, err = c.MessagesGetScheduledHistory(&mtproto.TLMessagesGetScheduledHistory{Peer: peer})
	if err != nil || len(history.GetMessages()) != 0 {
		t.Fatalf("history after send: result=%+v err=%v", history, err)
	}

	if _, err = schedstore.Append(userID, mtproto.MakePeerUser(receiver), "cancel me", int32(time.Now().Add(2*time.Hour).Unix())); err != nil {
		t.Fatal("append cancellation fixture:", err)
	}
	pending, err = schedstore.List(userID)
	if err != nil || len(pending) != 1 {
		t.Fatalf("cancellation fixture: messages=%+v err=%v", pending, err)
	}
	if pending[0].GetId() == id {
		t.Fatalf("scheduled ids must remain unique after a sent record: got %d twice", id)
	}
	updates, err = c.MessagesDeleteScheduledMessages(&mtproto.TLMessagesDeleteScheduledMessages{Peer: peer, Id: []int32{pending[0].GetId()}})
	if err != nil || len(updates.GetUpdates()) != 1 || updates.GetUpdates()[0].GetPredicateName() != mtproto.Predicate_updateDeleteScheduledMessages {
		t.Fatalf("delete scheduled: result=%+v err=%v", updates, err)
	}
	history, err = c.MessagesGetScheduledHistory(&mtproto.TLMessagesGetScheduledHistory{Peer: peer})
	if err != nil || len(history.GetMessages()) != 0 {
		t.Fatalf("history after delete: result=%+v err=%v", history, err)
	}

	if _, err = c.MessagesGetScheduledHistory(&mtproto.TLMessagesGetScheduledHistory{
		Peer: mtproto.MakeTLInputPeerEmpty(nil).To_InputPeer(),
	}); !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("invalid peer history: got %v, want PEER_ID_INVALID", err)
	}
	if _, err = (&ApiFullCore{}).MessagesGetScheduledHistory(&mtproto.TLMessagesGetScheduledHistory{Peer: peer}); !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("anonymous history: got %v, want AUTH_KEY_UNREGISTERED", err)
	}
}

func TestScheduledMessagesRejectNilRequests(t *testing.T) {
	c := scheduledTestCore(979104, &scheduledSenderStub{})
	checks := []struct {
		name string
		call func() error
	}{
		{"history", func() error {
			_, err := c.MessagesGetScheduledHistory(nil)
			return err
		}},
		{"messages", func() error {
			_, err := c.MessagesGetScheduledMessages(nil)
			return err
		}},
		{"send", func() error {
			_, err := c.MessagesSendScheduledMessages(nil)
			return err
		}},
		{"delete", func() error {
			_, err := c.MessagesDeleteScheduledMessages(nil)
			return err
		}},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.call(); !errors.Is(err, mtproto.ErrInputRequestInvalid) {
				t.Fatalf("nil request: got %v, want INPUT_REQUEST_INVALID", err)
			}
		})
	}
}
