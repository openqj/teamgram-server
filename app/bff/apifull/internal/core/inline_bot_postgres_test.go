package core

import (
	"context"
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func TestInlineBotResultsPostgresRoundTrip(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	if err := persist.OpenPostgresReadOnly(dsn); err != nil {
		t.Skipf("PostgreSQL schema unavailable: %v", err)
	}
	// Remove a prior failed-run record before creating the query. The test uses
	// a stable bot identity so the cleanup remains deterministic across retries.
	if db, err := persist.OpenPostgresDB(dsn); err == nil {
		_, _ = db.Exec(`DELETE FROM apifull_kv WHERE k = $1`, "inline-bot-query:992002")
		_ = db.Close()
	}
	t.Cleanup(func() {
		if db, err := persist.OpenPostgresDB(dsn); err == nil {
			_, _ = db.Exec(`DELETE FROM apifull_kv WHERE k = $1`, "inline-bot-query:992002")
			_ = db.Close()
		}
		_ = persist.ClosePostgres()
	})
	const (
		requesterID = int64(992001)
		botID       = int64(992002)
		requestHash = int64(992003)
	)
	client := &inlineBotUserClient{users: map[int64]*mtproto.ImmutableUser{
		botID: inlineBotProfile(botID, requestHash, true),
	}}
	ctx := &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}}
	requester := &ApiFullCore{ctx: context.Background(), svcCtx: ctx, MD: &metadata.RpcMetadata{UserId: requesterID}}
	bot := &mtproto.InputUser{PredicateName: mtproto.Predicate_inputUser, UserId: botID, AccessHash: requestHash}
	first, err := requester.MessagesGetInlineBotResults(&mtproto.TLMessagesGetInlineBotResults{
		Bot: bot, Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(), Query: "postgres", Offset: "",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first == nil || first.GetQueryId() <= 0 || len(first.GetResults()) != 0 {
		t.Fatalf("pending inline result = %+v", first)
	}
	botCore := &ApiFullCore{ctx: context.Background(), svcCtx: ctx, MD: &metadata.RpcMetadata{UserId: botID}}
	if _, err = botCore.MessagesSetInlineBotResults(&mtproto.TLMessagesSetInlineBotResults{
		QueryId: first.GetQueryId(), CacheTime: 30,
		Results: []*mtproto.InputBotInlineResult{{Id: "r1", Type: "article"}},
	}); err != nil {
		t.Fatal(err)
	}
	second, err := requester.MessagesGetInlineBotResults(&mtproto.TLMessagesGetInlineBotResults{
		Bot: bot, Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(), Query: "postgres", Offset: "",
	})
	if err != nil {
		t.Fatal(err)
	}
	if second == nil || second.GetQueryId() != first.GetQueryId() || len(second.GetResults()) != 1 || second.GetResults()[0].GetId() != "r1" {
		t.Fatalf("published inline result = %+v", second)
	}
}

type inlineBotUserClient struct {
	user_client.UserClient
	users map[int64]*mtproto.ImmutableUser
}

func (c *inlineBotUserClient) UserGetImmutableUser(_ context.Context, in *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	if user := c.users[in.GetId()]; user != nil {
		return user, nil
	}
	return nil, mtproto.ErrUserIdInvalid
}

func inlineBotProfile(id, accessHash int64, bot bool) *mtproto.ImmutableUser {
	data := &mtproto.UserData{Id: id, AccessHash: accessHash}
	if bot {
		data.Bot = &mtproto.BotData{}
	}
	return &mtproto.ImmutableUser{User: data}
}
