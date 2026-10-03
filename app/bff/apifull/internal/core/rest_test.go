package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestDeepLinkMethodsReturnMethodNotImpl(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}

	if _, err := c.MessagesStartBot(&mtproto.TLMessagesStartBot{
		Bot:      &mtproto.InputUser{UserId: 2},
		Peer:     &mtproto.InputPeer{UserId: 1},
		RandomId: 1,
	}); err != mtproto.ErrMethodNotImpl {
		t.Fatalf("MessagesStartBot() error = %v, want METHOD_NOT_IMPL", err)
	}
	if _, err := c.HelpGetDeepLinkInfo(&mtproto.TLHelpGetDeepLinkInfo{Path: "share/example"}); err != mtproto.ErrMethodNotImpl {
		t.Fatalf("HelpGetDeepLinkInfo() error = %v, want METHOD_NOT_IMPL", err)
	}
	if _, err := c.HelpGetDeepLinkInfo(nil); err != mtproto.ErrInputRequestInvalid {
		t.Fatalf("HelpGetDeepLinkInfo(nil) error = %v, want INPUT_REQUEST_INVALID", err)
	}
	if _, err := c.HelpGetRecentMeUrls(&mtproto.TLHelpGetRecentMeUrls{}); err != mtproto.ErrMethodNotImpl {
		t.Fatalf("HelpGetRecentMeUrls() error = %v, want METHOD_NOT_IMPL", err)
	}
}
