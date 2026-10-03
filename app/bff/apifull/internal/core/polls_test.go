// Copyright 2026 Teamgram Authors
// All rights reserved.
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
// Author: teamgramio (teamgramio@gmail.com)

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
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	chatclient "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

type pollTestStore struct {
	values map[string]string
}

func (s *pollTestStore) Get(key string) (string, error) {
	return s.values[key], nil
}

func (s *pollTestStore) Set(key, value string) error {
	if s.values == nil {
		s.values = make(map[string]string)
	}
	s.values[key] = value
	return nil
}

func usePollTestStore(t *testing.T) {
	t.Helper()
	old := persist.Default
	persist.Use(&pollTestStore{})
	t.Cleanup(func() { persist.Use(old) })
}

type pollMessageReaderStub struct {
	boxes   map[int64]*mtproto.MessageBox
	calls   int
	request *messagepb.TLMessageGetUserMessage
}

func (s *pollMessageReaderStub) MessageGetUserMessage(_ context.Context, in *messagepb.TLMessageGetUserMessage) (*mtproto.MessageBox, error) {
	s.calls++
	s.request = in
	box := s.boxes[in.GetUserId()]
	if box == nil || box.GetMessageId() != in.GetId() {
		return nil, mtproto.ErrMessageIdInvalid
	}
	return box, nil
}

type pollChatClientStub struct {
	chatclient.ChatClient
	chat  *mtproto.MutableChat
	calls int
}

func (s *pollChatClientStub) ChatGetMutableChat(_ context.Context, in *chatpb.TLChatGetMutableChat) (*mtproto.MutableChat, error) {
	s.calls++
	if s.chat == nil || s.chat.GetChat() == nil || s.chat.GetChat().GetId() != in.GetChatId() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	return s.chat, nil
}

func pollTestCore(userID int64, messages dao.PollMessageReader, chats chatclient.ChatClient) *ApiFullCore {
	return &ApiFullCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			PollMessageReader: messages,
			ChatClient:        chats,
		}},
		MD: &metadata.RpcMetadata{UserId: userID},
	}
}

func pollTestChat(id int64, members ...int64) *mtproto.MutableChat {
	participants := make([]*mtproto.ImmutableChatParticipant, 0, len(members))
	for _, memberID := range members {
		participants = append(participants, &mtproto.ImmutableChatParticipant{
			UserId:          memberID,
			ParticipantType: mtproto.ChatMemberNormal,
			State:           mtproto.ChatMemberStateNormal,
		})
	}
	return &mtproto.MutableChat{
		Chat:             &mtproto.ImmutableChat{Id: id},
		ChatParticipants: participants,
	}
}

func pollTestInputPeer(chatID int64) *mtproto.InputPeer {
	return mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: chatID}).To_InputPeer()
}

func pollTestBox(chatID int64, msgID int32, poll *mtproto.Poll) *mtproto.MessageBox {
	return mtproto.MakeTLMessageBox(&mtproto.MessageBox{
		MessageId: msgID,
		PeerType:  mtproto.PEER_CHAT,
		PeerId:    chatID,
		Message: mtproto.MakeTLMessage(&mtproto.Message{
			Id:     msgID,
			PeerId: mtproto.MakePeerChat(chatID),
			Media: mtproto.MakeTLMessageMediaPoll(&mtproto.MessageMedia{
				Poll: poll,
			}).To_MessageMedia(),
		}).To_Message(),
	}).To_MessageBox()
}

func pollTestPoll(id int64, public, multiple, closed bool) *mtproto.Poll {
	return mtproto.MakeTLPoll(&mtproto.Poll{
		Id:             id,
		Hash:           id + 1,
		PublicVoters:   public,
		MultipleChoice: multiple,
		Closed:         closed,
		Question_TEXTWITHENTITIES: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
			Text: "Which option?",
		}).To_TextWithEntities(),
		Answers: []*mtproto.PollAnswer{
			mtproto.MakeTLPollAnswer(&mtproto.PollAnswer{
				Text_TEXTWITHENTITIES: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{Text: "One"}).To_TextWithEntities(),
				Option:                []byte("one"),
			}).To_PollAnswer(),
			mtproto.MakeTLPollAnswer(&mtproto.PollAnswer{
				Text_TEXTWITHENTITIES: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{Text: "Two"}).To_TextWithEntities(),
				Option:                []byte("two"),
			}).To_PollAnswer(),
		},
	}).To_Poll()
}

func pollResultsFromUpdates(t *testing.T, updates *mtproto.Updates, pollID int64) *mtproto.PollResults {
	t.Helper()
	if updates == nil || len(updates.GetUpdates()) != 1 {
		t.Fatalf("updates = %+v, want one updateMessagePoll", updates)
	}
	update := updates.GetUpdates()[0]
	if update.GetPollId() != pollID || update.GetPoll() == nil || update.GetResults() == nil {
		t.Fatalf("update = %+v, want poll %d with results", update, pollID)
	}
	return update.GetResults()
}

func pollResult(t *testing.T, results *mtproto.PollResults, option string) *mtproto.PollAnswerVoters {
	t.Helper()
	for _, result := range results.GetResults() {
		if string(result.GetOption()) == option {
			return result
		}
	}
	t.Fatalf("missing option %q in %+v", option, results)
	return nil
}

func pollVoterCount(result *mtproto.PollAnswerVoters) int32 {
	if result.GetVoters_FLAGINT32() != nil {
		return result.GetVoters_FLAGINT32().GetValue()
	}
	return result.GetVoters_INT32()
}

func TestPollsUnauthed(t *testing.T) {
	cores := []*ApiFullCore{
		{},
		{MD: &metadata.RpcMetadata{}},
	}
	for _, c := range cores {
		if _, err := c.MessagesSendVote(nil); !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
			t.Fatalf("sendVote: %v", err)
		}
		if _, err := c.MessagesGetPollResults(nil); !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
			t.Fatalf("getPollResults: %v", err)
		}
		if _, err := c.MessagesGetPollVotes(nil); !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
			t.Fatalf("getPollVotes: %v", err)
		}
	}
}

func TestPollVoteRoundtripUsesStoredPoll(t *testing.T) {
	usePollTestStore(t)
	const (
		chatID int64 = 701
		msgID  int32 = 19
		pollID int64 = 90210
	)
	poll := pollTestPoll(pollID, true, false, false)
	reader := &pollMessageReaderStub{boxes: map[int64]*mtproto.MessageBox{
		1: pollTestBox(chatID, msgID, poll),
		2: pollTestBox(chatID, msgID, poll),
	}}
	chats := &pollChatClientStub{chat: pollTestChat(chatID, 1, 2)}
	owner := pollTestCore(1, reader, chats)
	member := pollTestCore(2, reader, chats)
	peer := pollTestInputPeer(chatID)

	first, err := owner.MessagesSendVote(&mtproto.TLMessagesSendVote{
		Peer: peer, MsgId: msgID, Options: [][]byte{[]byte("one")},
	})
	if err != nil {
		t.Fatal(err)
	}
	firstResults := pollResultsFromUpdates(t, first, pollID)
	if firstResults.GetTotalVoters().GetValue() != 1 || !pollResult(t, firstResults, "one").GetChosen() || pollVoterCount(pollResult(t, firstResults, "two")) != 0 {
		t.Fatalf("owner vote results = %+v", firstResults)
	}

	second, err := member.MessagesSendVote(&mtproto.TLMessagesSendVote{
		Peer: peer, MsgId: msgID, Options: [][]byte{[]byte("two")},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondResults := pollResultsFromUpdates(t, second, pollID)
	if secondResults.GetTotalVoters().GetValue() != 2 || !pollResult(t, secondResults, "two").GetChosen() || pollResult(t, secondResults, "one").GetChosen() {
		t.Fatalf("member vote results = %+v", secondResults)
	}

	read, err := owner.MessagesGetPollResults(&mtproto.TLMessagesGetPollResults{Peer: peer, MsgId: msgID})
	if err != nil {
		t.Fatal(err)
	}
	readResults := pollResultsFromUpdates(t, read, pollID)
	if readResults.GetTotalVoters().GetValue() != 2 || pollVoterCount(pollResult(t, readResults, "one")) != 1 || pollVoterCount(pollResult(t, readResults, "two")) != 1 || !pollResult(t, readResults, "one").GetChosen() {
		t.Fatalf("read results = %+v", readResults)
	}

	votes, err := owner.MessagesGetPollVotes(&mtproto.TLMessagesGetPollVotes{Peer: peer, Id: msgID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if votes.GetCount() != 2 || len(votes.GetVotes_VECTORMESSAGEPEERVOTE()) != 2 || len(votes.GetVotes_VECTORMESSAGEUSERVOTE()) != 0 {
		t.Fatalf("votes = %+v", votes)
	}
	ids := map[int64]bool{}
	for _, vote := range votes.GetVotes_VECTORMESSAGEPEERVOTE() {
		ids[vote.GetPeer().GetUserId()] = true
	}
	if !ids[1] || !ids[2] {
		t.Fatalf("voter IDs = %+v, want owner and member", ids)
	}
	if reader.request == nil || reader.request.GetUserId() != 1 || reader.request.GetId() != msgID {
		t.Fatalf("message lookup = %+v", reader.request)
	}
}

func TestPollVoteRejectsInvalidOptionsAndClosedPoll(t *testing.T) {
	usePollTestStore(t)
	const (
		chatID int64 = 702
		msgID  int32 = 20
	)
	reader := &pollMessageReaderStub{boxes: map[int64]*mtproto.MessageBox{
		1: pollTestBox(chatID, msgID, pollTestPoll(90211, true, false, false)),
	}}
	core := pollTestCore(1, reader, &pollChatClientStub{chat: pollTestChat(chatID, 1)})
	peer := pollTestInputPeer(chatID)

	tests := []struct {
		name    string
		options [][]byte
		want    error
	}{
		{name: "duplicate option", options: [][]byte{[]byte("one"), []byte("one")}, want: mtproto.ErrPollOptionDuplicate},
		{name: "unknown option", options: [][]byte{[]byte("three")}, want: mtproto.ErrPollOptionInvalid},
		{name: "multiple answers in a single-choice poll", options: [][]byte{[]byte("one"), []byte("two")}, want: mtproto.ErrPollAnswerInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := core.MessagesSendVote(&mtproto.TLMessagesSendVote{Peer: peer, MsgId: msgID, Options: tt.options})
			if !errors.Is(err, tt.want) {
				t.Fatalf("MessagesSendVote() error = %v, want %v", err, tt.want)
			}
		})
	}

	reader.boxes[1] = pollTestBox(chatID, msgID, pollTestPoll(90212, true, false, true))
	if _, err := core.MessagesSendVote(&mtproto.TLMessagesSendVote{Peer: peer, MsgId: msgID, Options: [][]byte{[]byte("one")}}); !errors.Is(err, mtproto.ErrMessagePollClosed) {
		t.Fatalf("closed poll error = %v", err)
	}
}

func TestPollRejectsPeerMismatchAndNonMembers(t *testing.T) {
	usePollTestStore(t)
	const (
		chatID int64 = 703
		msgID  int32 = 21
	)
	peer := pollTestInputPeer(chatID)
	mismatchReader := &pollMessageReaderStub{boxes: map[int64]*mtproto.MessageBox{
		1: pollTestBox(chatID+1, msgID, pollTestPoll(90213, true, false, false)),
	}}
	mismatchCore := pollTestCore(1, mismatchReader, &pollChatClientStub{chat: pollTestChat(chatID, 1, 2)})
	if _, err := mismatchCore.MessagesGetPollResults(&mtproto.TLMessagesGetPollResults{Peer: peer, MsgId: msgID}); !errors.Is(err, mtproto.ErrMessageIdInvalid) {
		t.Fatalf("peer mismatch error = %v", err)
	}

	outsiderReader := &pollMessageReaderStub{boxes: map[int64]*mtproto.MessageBox{
		3: pollTestBox(chatID, msgID, pollTestPoll(90213, true, false, false)),
	}}
	outsider := pollTestCore(3, outsiderReader, &pollChatClientStub{chat: pollTestChat(chatID, 1, 2)})
	if _, err := outsider.MessagesSendVote(&mtproto.TLMessagesSendVote{Peer: peer, MsgId: msgID, Options: [][]byte{[]byte("one")}}); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("outsider sendVote error = %v", err)
	}
	if _, err := outsider.MessagesGetPollResults(&mtproto.TLMessagesGetPollResults{Peer: peer, MsgId: msgID}); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("outsider getPollResults error = %v", err)
	}
	if outsiderReader.calls != 0 {
		t.Fatalf("outsider reached message reader %d times", outsiderReader.calls)
	}
}

func TestPollVotesRequirePublicVoters(t *testing.T) {
	usePollTestStore(t)
	const (
		chatID int64 = 704
		msgID  int32 = 22
	)
	reader := &pollMessageReaderStub{boxes: map[int64]*mtproto.MessageBox{
		1: pollTestBox(chatID, msgID, pollTestPoll(90214, false, false, false)),
	}}
	core := pollTestCore(1, reader, &pollChatClientStub{chat: pollTestChat(chatID, 1)})
	if _, err := core.MessagesGetPollVotes(&mtproto.TLMessagesGetPollVotes{Peer: pollTestInputPeer(chatID), Id: msgID, Limit: 10}); !errors.Is(err, mtproto.ErrPollVoteRequired) {
		t.Fatalf("private poll votes error = %v", err)
	}
}

func TestPollUpdateEncodingIncludesPeerMessageIDPair(t *testing.T) {
	const (
		chatID int64 = 705
		msgID  int32 = 23
	)
	poll := pollTestPoll(90215, true, false, false)
	updates := pollUpdatesFor(&resolvedPoll{
		poll:  poll,
		peer:  pollTestInputPeer(chatID),
		msgID: msgID,
	}, []pollBallot{{userId: 1, options: [][]byte{[]byte("one")}}}, 1)
	buf := mtproto.NewEncodeBuf(1024)
	if err := updates.Encode(buf, 229); err != nil {
		t.Fatal(err)
	}
	decoded := mtproto.NewDecodeBuf(buf.GetBuf())
	object := decoded.Object()
	if err := decoded.GetError(); err != nil {
		t.Fatal(err)
	}
	if decoded.GetOffset() != decoded.GetSize() {
		t.Fatalf("poll update left %d bytes", decoded.GetSize()-decoded.GetOffset())
	}
	result, ok := object.(*mtproto.TLUpdates)
	if !ok || len(result.GetUpdates()) != 1 {
		t.Fatalf("decoded updates = %#v", object)
	}
	update := result.GetUpdates()[0]
	if update.GetPeer_PEER().GetChatId() != chatID || update.GetMsgId_FLAGINT32().GetValue() != msgID {
		t.Fatalf("decoded poll update = %+v, want peer %d and message %d", update, chatID, msgID)
	}
	if got := update.GetResults().GetResults()[0].GetVoters_FLAGINT32(); got == nil || got.GetValue() != 1 {
		t.Fatalf("decoded poll voter count = %v, want 1", got)
	}
	recent := update.GetResults().GetResults()[0].GetRecentVoters()
	if len(recent) != 1 || recent[0].GetUserId() != 1 {
		t.Fatalf("decoded poll recent voters = %+v, want user 1", recent)
	}
	if update.GetResults().GetResults()[1].GetVoters_FLAGINT32() != nil {
		t.Fatalf("zero-vote answer must omit optional voters field: %+v", update.GetResults().GetResults()[1])
	}
}
