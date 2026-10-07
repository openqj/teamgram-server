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
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"github.com/zeromicro/go-zero/core/logx"
)

type quickReplyMessageReader struct {
	messageclient.MessageClient
	boxes map[int32]*mtproto.MessageBox
}

func (r *quickReplyMessageReader) MessageGetUserMessage(_ context.Context, in *messagepb.TLMessageGetUserMessage) (*mtproto.MessageBox, error) {
	if box := r.boxes[in.GetId()]; box != nil {
		return box, nil
	}
	return nil, mtproto.ErrMessageIdInvalid
}

type quickReplySender struct {
	request *msgpb.TLMsgSendMessageV2
	result  *mtproto.Updates
}

func (s *quickReplySender) MsgSendMessageV2(_ context.Context, in *msgpb.TLMsgSendMessageV2) (*mtproto.Updates, error) {
	s.request = in
	return s.result, nil
}

func quickReplyMessageBox(userID int64, id int32, text string) *mtproto.MessageBox {
	return mtproto.MakeTLMessageBox(&mtproto.MessageBox{
		UserId:       userID,
		MessageId:    id,
		SenderUserId: userID,
		PeerType:     mtproto.PEER_USER,
		PeerId:       userID,
		Message: mtproto.MakeTLMessage(&mtproto.Message{
			Id:      id,
			Out:     true,
			FromId:  mtproto.MakePeerUser(userID),
			PeerId:  mtproto.MakePeerUser(userID),
			Message: text,
		}).To_Message(),
	}).To_MessageBox()
}

func quickReplyDeliveryCore(userID int64, reader messageclient.MessageClient, sender dao.ScheduledMessageSender) *ApiFullCore {
	ctx := context.Background()
	return &ApiFullCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			PollMessageReader:      reader,
			ScheduledMessageSender: sender,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: userID, PermAuthKeyId: 99},
	}
}

func TestQuickReplyRoundtrip(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	ok, err := c.MessagesEditQuickReplyShortcut(&mtproto.TLMessagesEditQuickReplyShortcut{
		ShortcutId: 7,
		Shortcut:   "prod-qr",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !mtproto.FromBool(ok) {
		t.Fatal("expected boolTrue")
	}
	got, err := c.MessagesGetQuickReplies(&mtproto.TLMessagesGetQuickReplies{})
	if err != nil {
		t.Fatal(err)
	}
	var found *mtproto.QuickReply
	for _, q := range got.GetQuickReplies() {
		if q.GetShortcutId() == 7 {
			found = q
			break
		}
	}
	if found == nil || found.GetShortcut() != "prod-qr" {
		t.Fatalf("quick reply roundtrip: %+v", got.GetQuickReplies())
	}

	if _, err := (&ApiFullCore{}).MessagesGetQuickReplies(nil); err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("auth: %v", err)
	}
}

func TestQuickReplySendUsesStoredMessagesAndMsgService(t *testing.T) {
	const userID int64 = 981201
	if err := saveQuickReplyMsgs(userID, []qrMsgItem{{ShortcutId: 7, Ids: []int32{10}}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = saveQuickReplyMsgs(userID, nil) })
	reader := &quickReplyMessageReader{boxes: map[int32]*mtproto.MessageBox{10: quickReplyMessageBox(userID, 10, "saved reply")}}
	sender := &quickReplySender{result: callUpdates()}
	c := quickReplyDeliveryCore(userID, reader, sender)

	got, err := c.MessagesSendQuickReplyMessages(&mtproto.TLMessagesSendQuickReplyMessages{
		Peer:       mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		ShortcutId: 7,
		Id:         []int32{10},
		RandomId:   []int64{9010},
	})
	if err != nil || got == nil {
		t.Fatalf("send quick reply = (%v, %v)", got, err)
	}
	if sender.request == nil || sender.request.GetPeerType() != mtproto.PEER_USER || sender.request.GetPeerId() != userID || len(sender.request.GetMessage()) != 1 {
		t.Fatalf("send request = %+v", sender.request)
	}
	out := sender.request.GetMessage()[0]
	if out.GetRandomId() != 9010 || out.GetMessage().GetMessage() != "saved reply" || out.GetMessage().GetId() != 0 || out.GetMessage().GetPeerId().GetUserId() != userID {
		t.Fatalf("outbox = %+v", out)
	}
}

func TestQuickReplyDeleteRemovesOnlyRequestedStoredMessages(t *testing.T) {
	const userID int64 = 981202
	if err := saveQuickReplyMsgs(userID, []qrMsgItem{{ShortcutId: 8, Ids: []int32{11, 12}}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = saveQuickReplyMsgs(userID, nil) })
	c := quickReplyDeliveryCore(userID, nil, nil)
	if got, err := c.MessagesDeleteQuickReplyMessages(&mtproto.TLMessagesDeleteQuickReplyMessages{ShortcutId: 8, Id: []int32{11}}); err != nil || got == nil {
		t.Fatalf("delete quick reply = (%v, %v)", got, err)
	}
	items, err := loadQuickReplyMsgs(userID)
	if err != nil || len(items) != 1 || len(items[0].Ids) != 1 || items[0].Ids[0] != 12 {
		t.Fatalf("stored quick replies after delete = %+v (err=%v)", items, err)
	}
	if got, err := c.MessagesDeleteQuickReplyMessages(&mtproto.TLMessagesDeleteQuickReplyMessages{ShortcutId: 8, Id: []int32{99}}); got != nil || !errors.Is(err, mtproto.ErrMessageIdInvalid) {
		t.Fatalf("unknown quick reply delete = (%v, %v), want MESSAGE_ID_INVALID", got, err)
	}
}

func TestQuickReplyCheckAndReorderPersist(t *testing.T) {
	const userID int64 = 981203
	if err := saveQuickReplies(userID, []quickReplyStored{
		{ShortcutId: 1, Shortcut: "first"},
		{ShortcutId: 2, Shortcut: "second"},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = saveQuickReplies(userID, nil) })
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}

	if available, err := c.MessagesCheckQuickReplyShortcut(&mtproto.TLMessagesCheckQuickReplyShortcut{Shortcut: "first"}); err != nil || mtproto.FromBool(available) {
		t.Fatalf("existing shortcut check = (%v, %v), want false", available, err)
	}
	if available, err := c.MessagesCheckQuickReplyShortcut(&mtproto.TLMessagesCheckQuickReplyShortcut{Shortcut: "new"}); err != nil || !mtproto.FromBool(available) {
		t.Fatalf("new shortcut check = (%v, %v), want true", available, err)
	}
	if ok, err := c.MessagesReorderQuickReplies(&mtproto.TLMessagesReorderQuickReplies{Order: []int32{2, 2, 99}}); err != nil || !mtproto.FromBool(ok) {
		t.Fatalf("reorder shortcuts = (%v, %v)", ok, err)
	}
	list, err := loadQuickReplies(userID)
	if err != nil || len(list) != 2 || list[0].ShortcutId != 2 || list[1].ShortcutId != 1 {
		t.Fatalf("reordered shortcuts = %+v (err=%v)", list, err)
	}
}
