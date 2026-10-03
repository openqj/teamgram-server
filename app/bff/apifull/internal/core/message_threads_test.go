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
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

type threadMessageReaderStub struct {
	boxes []*mtproto.MessageBox
}

func (s *threadMessageReaderStub) MessageGetUserMessage(_ context.Context, in *messagepb.TLMessageGetUserMessage) (*mtproto.MessageBox, error) {
	for _, box := range s.boxes {
		if box != nil && box.GetMessageId() == in.GetId() {
			return box, nil
		}
	}
	return nil, mtproto.ErrMessageIdInvalid
}

func (s *threadMessageReaderStub) MessageGetHistoryMessages(_ context.Context, _ *messagepb.TLMessageGetHistoryMessages) (*messagepb.Vector_MessageBox, error) {
	return &messagepb.Vector_MessageBox{Datas: s.boxes}, nil
}

func threadBox(id int32, peerID int64, replyTo int32) *mtproto.MessageBox {
	return &mtproto.MessageBox{
		MessageId:    id,
		PeerType:     mtproto.PEER_USER,
		PeerId:       peerID,
		ReplyToMsgId: replyTo,
		Message: mtproto.MakeTLMessage(&mtproto.Message{
			Id:      id,
			PeerId:  mtproto.MakePeerUser(peerID),
			Message: "thread message",
		}).To_Message(),
	}
}

func TestMessagesReadDiscussionPersistsMonotonicCursor(t *testing.T) {
	const userID int64 = 81015
	peer := &mtproto.InputPeer{PredicateName: mtproto.Predicate_inputPeerChat, ChatId: 7001}
	key := discussionReadKey(userID, peer, 2)
	if err := persist.Default.Set(key, ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = persist.Default.Set(key, "") })

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}
	if result, err := c.MessagesReadDiscussion(&mtproto.TLMessagesReadDiscussion{
		Peer: peer, MsgId: 2, ReadMaxId: 9,
	}); err != nil || result != mtproto.BoolTrue {
		t.Fatalf("first read: result=%v err=%v", result, err)
	}
	if got, err := persist.Default.Get(key); err != nil || got != "9" {
		t.Fatalf("stored read cursor = %q, err=%v; want 9", got, err)
	}

	if result, err := c.MessagesReadDiscussion(&mtproto.TLMessagesReadDiscussion{
		Peer: peer, MsgId: 2, ReadMaxId: 4,
	}); err != nil || result != mtproto.BoolTrue {
		t.Fatalf("backward read: result=%v err=%v", result, err)
	}
	if got, err := persist.Default.Get(key); err != nil || got != "9" {
		t.Fatalf("backward read changed cursor = %q, err=%v; want 9", got, err)
	}

	other := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID + 1}}
	if _, err := other.MessagesReadDiscussion(&mtproto.TLMessagesReadDiscussion{
		Peer: peer, MsgId: 2, ReadMaxId: 3,
	}); err != nil {
		t.Fatalf("other user read: %v", err)
	}
	otherKey := discussionReadKey(userID+1, peer, 2)
	if got, err := persist.Default.Get(otherKey); err != nil || got != "3" {
		t.Fatalf("other user cursor = %q, err=%v; want 3", got, err)
	}
	t.Cleanup(func() { _ = persist.Default.Set(otherKey, "") })
}

func TestMessagesReadDiscussionRejectsUnresolvablePeer(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81015}}
	_, err := c.MessagesReadDiscussion(&mtproto.TLMessagesReadDiscussion{
		Peer:  &mtproto.InputPeer{PredicateName: mtproto.Predicate_inputPeerUsername, Username: "discussion"},
		MsgId: 2,
	})
	if !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("unresolvable peer error = %v, want PEER_ID_INVALID", err)
	}
}

func TestMessagesGetRepliesAndDiscussionMessageUseStoredHistory(t *testing.T) {
	const userID int64 = 81015
	reply := threadBox(2, userID+1, 1)
	reply.ReplyToMsgId = 0
	reply.Message.ReplyTo = mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
		ReplyToMsgId:           1,
		ReplyToMsgId_INT32:     1,
		ReplyToMsgId_FLAGINT32: mtproto.MakeFlagsInt32(1),
	}).To_MessageReplyHeader()
	reader := &threadMessageReaderStub{boxes: []*mtproto.MessageBox{
		threadBox(1, userID+1, 0),
		reply,
		threadBox(3, userID+1, 0),
	}}
	core := &ApiFullCore{
		MD: &metadata.RpcMetadata{UserId: userID},
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			PollMessageReader:    reader,
			MessageHistoryReader: reader,
		}},
	}
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: userID + 1}).To_InputPeer()
	replies, err := core.MessagesGetReplies(&mtproto.TLMessagesGetReplies{Peer: peer, MsgId: 1, Limit: 10})
	if err != nil {
		t.Fatalf("MessagesGetReplies() error = %v", err)
	}
	if replies == nil || len(replies.GetMessages()) != 1 || replies.GetMessages()[0].GetId() != 2 {
		t.Fatalf("MessagesGetReplies() = %#v, want only reply id 2", replies)
	}
	discussion, err := core.MessagesGetDiscussionMessage(&mtproto.TLMessagesGetDiscussionMessage{Peer: peer, MsgId: 1})
	if err != nil {
		t.Fatalf("MessagesGetDiscussionMessage() error = %v", err)
	}
	if discussion == nil || len(discussion.GetMessages()) != 1 || discussion.GetMessages()[0].GetId() != 1 {
		t.Fatalf("MessagesGetDiscussionMessage() = %#v, want root id 1", discussion)
	}
}
