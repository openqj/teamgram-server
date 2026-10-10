package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
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

func TestMessagesStartBotDirectUserDispatchesValidatedStartMessage(t *testing.T) {
	const (
		callerID   int64 = 11001
		botID      int64 = 11002
		accessHash int64 = 771199
		randomID   int64 = 9911002
	)
	sender := &scheduledSenderStub{}
	users := &miniBotAppsUserClient{users: map[int64]*mtproto.ImmutableUser{
		botID: miniBotProfile(botID, accessHash, true),
	}}
	c := &ApiFullCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{
			UserClient:             users,
			ScheduledMessageSender: sender,
		}},
		MD: &metadata.RpcMetadata{UserId: callerID, PermAuthKeyId: 91},
	}
	updates, err := c.MessagesStartBot(&mtproto.TLMessagesStartBot{
		Bot:        mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: botID, AccessHash: accessHash}).To_InputUser(),
		Peer:       mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: botID, AccessHash: accessHash}).To_InputPeer(),
		RandomId:   randomID,
		StartParam: "launch_1",
	})
	if err != nil || updates == nil {
		t.Fatalf("direct start bot = (%+v, %v), want typed updates", updates, err)
	}
	if sender.request == nil || sender.request.GetUserId() != callerID || sender.request.GetAuthKeyId() != 91 || sender.request.GetPeerType() != mtproto.PEER_USER || sender.request.GetPeerId() != botID {
		t.Fatalf("dispatch request = %+v", sender.request)
	}
	if len(sender.request.GetMessage()) != 1 {
		t.Fatalf("dispatch message count = %d, want 1", len(sender.request.GetMessage()))
	}
	outbox := sender.request.GetMessage()[0]
	if outbox.GetRandomId() != randomID || outbox.GetMessage().GetMessage() != "/start launch_1" || !outbox.GetNoWebpage() {
		t.Fatalf("dispatch outbox = %+v", outbox)
	}
}
