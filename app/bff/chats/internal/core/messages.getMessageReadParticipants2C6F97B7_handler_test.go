package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestGetMessageReadParticipants2RejectsNilRequestOrPeer(t *testing.T) {
	c := &ChatsCore{}
	for name, in := range map[string]*mtproto.TLMessagesGetMessageReadParticipants2C6F97B7{
		"nil request": nil,
		"nil peer":    {},
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := c.MessagesGetMessageReadParticipants2C6F97B7(in); got != nil || err != mtproto.ErrInputRequestInvalid {
				t.Fatalf("result=(%+v, %v), want (nil, INPUT_REQUEST_INVALID)", got, err)
			}
		})
	}
}

func TestGetMessageReadParticipants2RejectsMissingAuthentication(t *testing.T) {
	in := &mtproto.TLMessagesGetMessageReadParticipants2C6F97B7{
		Peer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 1}).To_InputPeer(),
	}
	if got, err := (&ChatsCore{}).MessagesGetMessageReadParticipants2C6F97B7(in); got != nil || err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("result=(%+v, %v), want (nil, AUTH_KEY_UNREGISTERED)", got, err)
	}
}
