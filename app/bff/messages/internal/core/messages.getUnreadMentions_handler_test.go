package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
)

func TestMessagesGetUnreadMentionsReportsUnsupportedChannelStorage(t *testing.T) {
	core := &MessagesCore{
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
	request := &mtproto.TLMessagesGetUnreadMentions{
		Peer: mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
			ChannelId:  10,
			AccessHash: 11,
		}).To_InputPeer(),
	}

	got, err := core.MessagesGetUnreadMentions(request)
	if got != nil {
		t.Fatalf("MessagesGetUnreadMentions(channel) result = %+v, want nil", got)
	}
	if err == nil || status.Code(err) != mtproto.ErrBadRequest || status.Convert(err).Message() != "CHANNEL_UNREAD_MENTIONS_UNSUPPORTED" {
		t.Fatalf("MessagesGetUnreadMentions(channel) error = %v, want CHANNEL_UNREAD_MENTIONS_UNSUPPORTED", err)
	}
}

func TestMessagesGetUnreadMentionsRejectsNilCoreAndMissingDependencies(t *testing.T) {
	request := &mtproto.TLMessagesGetUnreadMentions{
		Peer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 77}).To_InputPeer(),
	}
	var nilCore *MessagesCore
	if got, err := nilCore.MessagesGetUnreadMentions(request); got != nil || err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("nil core = (%v, %v), want AUTH_KEY_UNREGISTERED", got, err)
	}

	core := &MessagesCore{MD: &metadata.RpcMetadata{UserId: 42}}
	if got, err := core.MessagesGetUnreadMentions(request); got != nil || err != mtproto.ErrInternalServerError {
		t.Fatalf("missing dependencies = (%v, %v), want INTERNAL_SERVER_ERROR", got, err)
	}
}
