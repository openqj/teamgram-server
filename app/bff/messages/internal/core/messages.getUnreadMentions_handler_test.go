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
