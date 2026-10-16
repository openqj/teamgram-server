package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
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

func TestGetMessageReadParticipants2RejectsMissingDependencies(t *testing.T) {
	in := &mtproto.TLMessagesGetMessageReadParticipants2C6F97B7{
		Peer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 1}).To_InputPeer(),
	}
	c := &ChatsCore{MD: &metadata.RpcMetadata{UserId: 1}}
	if got, err := c.MessagesGetMessageReadParticipants2C6F97B7(in); got != nil || err != mtproto.ErrInternalServerError {
		t.Fatalf("result=(%+v, %v), want (nil, INTERNAL_SERVER_ERROR)", got, err)
	}
}

func TestGetMessageReadParticipants2RejectsZeroChannelAccessHash(t *testing.T) {
	in := &mtproto.TLMessagesGetMessageReadParticipants2C6F97B7{
		Peer: mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 1}).To_InputPeer(),
	}
	c := &ChatsCore{MD: &metadata.RpcMetadata{UserId: 1}}
	if got, err := c.MessagesGetMessageReadParticipants2C6F97B7(in); got != nil || err != mtproto.ErrChannelInvalid {
		t.Fatalf("result=(%+v, %v), want (nil, CHANNEL_INVALID)", got, err)
	}
}
