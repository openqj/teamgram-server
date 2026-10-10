package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestBuildMessageReadParticipantDates(t *testing.T) {
	got, err := buildMessageReadParticipantDates(nil)
	if err != nil || got == nil || len(got.GetDatas()) != 0 {
		t.Fatalf("empty read-participant result = (%+v, %v), want empty vector and nil error", got, err)
	}

	got, err = buildMessageReadParticipantDates([]int64{42})
	if got != nil || err != mtproto.ErrMethodNotImpl {
		t.Fatalf("read-participant result = (%+v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
	}
}

func TestGetMessageReadParticipants31RejectsNilRequestOrPeer(t *testing.T) {
	c := &ChatsCore{}
	for name, in := range map[string]*mtproto.TLMessagesGetMessageReadParticipants31C1C44F{
		"nil request": nil,
		"nil peer":    {},
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := c.MessagesGetMessageReadParticipants31C1C44F(in); got != nil || err != mtproto.ErrInputRequestInvalid {
				t.Fatalf("result=(%+v, %v), want (nil, INPUT_REQUEST_INVALID)", got, err)
			}
		})
	}
}

func TestGetMessageReadParticipants31RejectsMissingAuthentication(t *testing.T) {
	in := &mtproto.TLMessagesGetMessageReadParticipants31C1C44F{
		Peer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 1}).To_InputPeer(),
	}
	if got, err := (&ChatsCore{}).MessagesGetMessageReadParticipants31C1C44F(in); got != nil || err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("result=(%+v, %v), want (nil, AUTH_KEY_UNREGISTERED)", got, err)
	}
}

func TestValidateReadParticipantMessageRequiresRequestedChatAndOwner(t *testing.T) {
	box := &mtproto.MessageBox{
		UserId:    100,
		MessageId: 7,
		PeerType:  mtproto.PEER_CHAT,
		PeerId:    200,
		Message:   &mtproto.Message{},
	}
	if err := validateReadParticipantMessage(100, 200, 7, box); err != nil {
		t.Fatalf("valid message = %v, want nil", err)
	}
	for _, tc := range []struct {
		name      string
		userID    int64
		chatID    int64
		messageID int32
		box       *mtproto.MessageBox
	}{
		{name: "wrong user", userID: 101, chatID: 200, messageID: 7, box: box},
		{name: "wrong peer", userID: 100, chatID: 201, messageID: 7, box: box},
		{name: "wrong message", userID: 100, chatID: 200, messageID: 8, box: box},
		{name: "nil message", userID: 100, chatID: 200, messageID: 7, box: &mtproto.MessageBox{UserId: 100, MessageId: 7, PeerType: mtproto.PEER_CHAT, PeerId: 200}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateReadParticipantMessage(tc.userID, tc.chatID, tc.messageID, tc.box); err != mtproto.ErrPeerIdInvalid {
				t.Fatalf("validateReadParticipantMessage() = %v, want PEER_ID_INVALID", err)
			}
		})
	}
}
