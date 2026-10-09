package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestDeepLinkMethodsReturnTypedEmptyResults(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}

	if _, err := c.MessagesStartBot(&mtproto.TLMessagesStartBot{
		Bot:      mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 2, AccessHash: 3}).To_InputUser(),
		Peer:     mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer(),
		RandomId: 1,
	}); err != mtproto.ErrMethodNotImpl {
		t.Fatalf("MessagesStartBot() error = %v, want METHOD_NOT_IMPL", err)
	}
	info, err := c.HelpGetDeepLinkInfo(&mtproto.TLHelpGetDeepLinkInfo{Path: "share/example"})
	if err != nil {
		t.Fatalf("HelpGetDeepLinkInfo() error = %v", err)
	}
	if info.GetPredicateName() != mtproto.Predicate_help_deepLinkInfoEmpty {
		t.Fatalf("HelpGetDeepLinkInfo() predicate = %q", info.GetPredicateName())
	}
	if _, err := c.HelpGetDeepLinkInfo(nil); err != mtproto.ErrInputRequestInvalid {
		t.Fatalf("HelpGetDeepLinkInfo(nil) error = %v, want INPUT_REQUEST_INVALID", err)
	}
	urls, err := c.HelpGetRecentMeUrls(&mtproto.TLHelpGetRecentMeUrls{})
	if err != nil {
		t.Fatalf("HelpGetRecentMeUrls() error = %v", err)
	}
	if urls.GetPredicateName() != mtproto.Predicate_help_recentMeUrls || len(urls.GetUrls()) != 0 || len(urls.GetChats()) != 0 || len(urls.GetUsers()) != 0 {
		t.Fatalf("HelpGetRecentMeUrls() = %+v", urls)
	}
}
