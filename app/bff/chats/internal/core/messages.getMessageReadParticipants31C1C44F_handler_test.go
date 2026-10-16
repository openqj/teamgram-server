package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
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

func TestValidateReadParticipantDatesRejectsMalformedRows(t *testing.T) {
	valid := mtproto.MakeTLReadParticipantDate(&mtproto.ReadParticipantDate{UserId: 42, Date: 123}).To_ReadParticipantDate()
	if err := validateReadParticipantDates([]*mtproto.ReadParticipantDate{valid}); err != nil {
		t.Fatalf("valid read-participant date = %v, want nil", err)
	}
	for name, dates := range map[string][]*mtproto.ReadParticipantDate{
		"nil row":       {nil},
		"zero user":     {{UserId: 0, Date: 123}},
		"zero date":     {{UserId: 42, Date: 0}},
		"negative date": {{UserId: 42, Date: -1}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateReadParticipantDates(dates); err != mtproto.ErrInternalServerError {
				t.Fatalf("validateReadParticipantDates() = %v, want INTERNAL_SERVER_ERROR", err)
			}
		})
	}
}

func TestBuildMessageReadParticipantDatesFromStoredCursorFailsClosed(t *testing.T) {
	got, err := buildMessageReadParticipantDatesFromStoredCursor(nil)
	if err != nil || got == nil || len(got.GetDatas()) != 0 {
		t.Fatalf("empty stored cursor result = (%+v, %v), want empty vector and nil error", got, err)
	}

	date := mtproto.MakeTLReadParticipantDate(&mtproto.ReadParticipantDate{UserId: 42, Date: 123}).To_ReadParticipantDate()
	if got, err := buildMessageReadParticipantDatesFromStoredCursor([]*mtproto.ReadParticipantDate{date}); got != nil || err != mtproto.ErrMethodNotImpl {
		t.Fatalf("stored cursor result = (%+v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
	}
}

func TestValidateMessageReadParticipantIDsRejectsInvalidRows(t *testing.T) {
	if err := validateMessageReadParticipantIDs([]int64{42}, 7); err != nil {
		t.Fatalf("valid participant IDs = %v, want nil", err)
	}
	for name, ids := range map[string][]int64{
		"zero ID":     {0},
		"negative ID": {-1},
		"caller ID":   {7},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateMessageReadParticipantIDs(ids, 7); err != mtproto.ErrInternalServerError {
				t.Fatalf("validateMessageReadParticipantIDs() = %v, want INTERNAL_SERVER_ERROR", err)
			}
		})
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

func TestGetMessageReadParticipants31RejectsMissingDependencies(t *testing.T) {
	in := &mtproto.TLMessagesGetMessageReadParticipants31C1C44F{
		Peer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 1}).To_InputPeer(),
	}
	c := &ChatsCore{MD: &metadata.RpcMetadata{UserId: 1}}
	if got, err := c.MessagesGetMessageReadParticipants31C1C44F(in); got != nil || err != mtproto.ErrInternalServerError {
		t.Fatalf("result=(%+v, %v), want (nil, INTERNAL_SERVER_ERROR)", got, err)
	}
}

func TestGetMessageReadParticipants31RejectsZeroChannelAccessHash(t *testing.T) {
	in := &mtproto.TLMessagesGetMessageReadParticipants31C1C44F{
		Peer: mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 1}).To_InputPeer(),
	}
	c := &ChatsCore{MD: &metadata.RpcMetadata{UserId: 1}}
	if got, err := c.MessagesGetMessageReadParticipants31C1C44F(in); got != nil || err != mtproto.ErrChannelInvalid {
		t.Fatalf("result=(%+v, %v), want (nil, CHANNEL_INVALID)", got, err)
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

func TestValidateReadParticipantChannelMessageRequiresRequestedChannelAndOwner(t *testing.T) {
	box := &mtproto.MessageBox{
		UserId:    100,
		MessageId: 7,
		PeerType:  mtproto.PEER_CHANNEL,
		PeerId:    200,
		Message:   &mtproto.Message{},
	}
	if err := validateReadParticipantChannelMessage(100, 200, 7, box); err != nil {
		t.Fatalf("valid message = %v, want nil", err)
	}
	if err := validateReadParticipantChannelMessage(100, 201, 7, box); err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("wrong channel = %v, want PEER_ID_INVALID", err)
	}
	if err := validateReadParticipantChannelMessage(101, 200, 7, box); err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("wrong owner = %v, want PEER_ID_INVALID", err)
	}
}

func TestValidateReadParticipantCallerDialogRequiresRequestedChannel(t *testing.T) {
	dlg := &dialog.DialogExt{Dialog: mtproto.MakeTLDialog(&mtproto.Dialog{Peer: mtproto.MakeTLPeerChannel(&mtproto.Peer{ChannelId: 200}).To_Peer()}).To_Dialog()}
	if err := validateReadParticipantCallerDialog(dlg, 200); err != nil {
		t.Fatalf("valid caller dialog = %v, want nil", err)
	}
	if err := validateReadParticipantCallerDialog(dlg, 201); err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("wrong channel = %v, want PEER_ID_INVALID", err)
	}
}
