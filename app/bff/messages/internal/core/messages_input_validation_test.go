package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestMessagesMethodsRejectUnauthenticatedRequests(t *testing.T) {
	var nilCore *MessagesCore
	cases := []struct {
		name string
		call func() error
	}{
		{name: "delete history", call: func() error { _, err := nilCore.MessagesDeleteHistory(nil); return err }},
		{name: "edit message", call: func() error { _, err := nilCore.MessagesEditMessage(nil); return err }},
		{name: "edit data", call: func() error { _, err := nilCore.MessagesGetMessageEditData(nil); return err }},
		{name: "extended media", call: func() error { _, err := nilCore.MessagesGetExtendedMedia(nil); return err }},
		{name: "send message", call: func() error { _, err := nilCore.MessagesSendMessage(nil); return err }},
		{name: "toggle no forwards", call: func() error { _, err := nilCore.MessagesToggleNoForwards(nil); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
				t.Fatalf("error = %v, want AUTH_KEY_UNREGISTERED", err)
			}
		})
	}
}

func TestMessagesMethodsRejectMissingPeer(t *testing.T) {
	core := &MessagesCore{MD: &metadata.RpcMetadata{UserId: 1}}
	cases := []struct {
		name string
		call func() error
	}{
		{name: "delete history", call: func() error {
			_, err := core.MessagesDeleteHistory(&mtproto.TLMessagesDeleteHistory{})
			return err
		}},
		{name: "edit message", call: func() error {
			_, err := core.MessagesEditMessage(&mtproto.TLMessagesEditMessage{})
			return err
		}},
		{name: "edit data", call: func() error {
			_, err := core.MessagesGetMessageEditData(&mtproto.TLMessagesGetMessageEditData{})
			return err
		}},
		{name: "extended media", call: func() error {
			_, err := core.MessagesGetExtendedMedia(&mtproto.TLMessagesGetExtendedMedia{})
			return err
		}},
		{name: "send message", call: func() error {
			_, err := core.MessagesSendMessage(&mtproto.TLMessagesSendMessage{})
			return err
		}},
		{name: "toggle no forwards", call: func() error {
			_, err := core.MessagesToggleNoForwards(&mtproto.TLMessagesToggleNoForwards{})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); !errors.Is(err, mtproto.ErrPeerIdInvalid) {
				t.Fatalf("error = %v, want PEER_ID_INVALID", err)
			}
		})
	}
}

func TestMessagesSendMethodsFailClosedWithoutMessageProvider(t *testing.T) {
	core := &MessagesCore{MD: &metadata.RpcMetadata{UserId: 1}}
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "send message",
			call: func() error {
				_, err := core.MessagesSendMessage(&mtproto.TLMessagesSendMessage{
					Peer:    mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
					Message: "hello",
				})
				return err
			},
		},
		{
			name: "send media",
			call: func() error {
				_, err := core.MessagesSendMedia(&mtproto.TLMessagesSendMedia{
					Peer:  mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
					Media: mtproto.MakeTLInputMediaEmpty(nil).To_InputMedia(),
				})
				return err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); !errors.Is(err, mtproto.ErrInternalServerError) {
				t.Fatalf("error = %v, want INTERNAL_SERVER_ERROR", err)
			}
		})
	}
}
