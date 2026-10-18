package core

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
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
	})
	const (
		requesterID = int64(992001)
		botID       = int64(992002)
		requestHash = int64(992003)
	)
	client := &inlineBotUserClient{users: map[int64]*mtproto.ImmutableUser{
		botID: inlineBotProfile(botID, requestHash, true),
	}}
	sender := &inlineBotSender{}
	ctx := &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client, ScheduledMessageSender: sender}}
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
		Results: []*mtproto.InputBotInlineResult{{Id: "r1", Type: "article", SendMessage: mtproto.MakeTLInputBotInlineMessageText(&mtproto.InputBotInlineMessage{Message: "inline text"}).To_InputBotInlineMessage()}},
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
	sent, err := requester.MessagesSendInlineBotResult(&mtproto.TLMessagesSendInlineBotResult{
		Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(), QueryId: first.GetQueryId(), Id: "r1", RandomId: 991001,
	})
	if err != nil || sent == nil || sender.request == nil || len(sender.request.GetMessage()) != 1 || sender.request.GetMessage()[0].GetMessage().GetMessage() != "inline text" {
		t.Fatalf("send inline result = (%#v, %v), request=%#v", sent, err, sender.request)
	}
}

func TestSetBotCallbackAnswerUsesCallerScopedControlPlaneBeforeNativePath(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	previous := persist.Default
	if err := persist.OpenPostgresReadOnly(dsn); err != nil {
		t.Skipf("PostgreSQL control-plane schema unavailable: %v", err)
	}
	db, err := persist.OpenPostgresDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	const userID int64 = 992005
	const queryID int64 = 880004
	const operation = "messages.setBotCallbackAnswer"
	scopeKey := businessControlQueryKey(queryID)
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM apifull_business_control_plane WHERE operation=$1 AND scope_key=$2 AND caller_user_id=$3`, operation, scopeKey, userID)
		_ = db.Close()
		_ = persist.ClosePostgres()
		persist.Default = previous
	})
	if _, err = db.Exec(`DELETE FROM apifull_business_control_plane WHERE operation=$1 AND scope_key=$2 AND caller_user_id=$3`, operation, scopeKey, userID); err != nil {
		t.Fatal(err)
	}
	result, err := json.Marshal(map[string]any{"value": true})
	if err != nil {
		t.Fatal(err)
	}
	if err = persist.PutBusinessControlRecord(persist.BusinessControlRecord{
		Operation: operation, ScopeKey: scopeKey, CallerUserID: userID, Result: result,
	}); err != nil {
		t.Fatal(err)
	}

	userClient := &inlineBotUserClient{users: map[int64]*mtproto.ImmutableUser{
		userID: inlineBotProfile(userID, 992006, true),
	}}
	core := &ApiFullCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{
			UserClient:        userClient,
			PollMessageReader: &callbackAnswerPollReader{},
			SyncClient:        &callbackAnswerSyncClient{},
		}},
		MD: &metadata.RpcMetadata{UserId: userID},
	}
	got, err := core.MessagesSetBotCallbackAnswer(&mtproto.TLMessagesSetBotCallbackAnswer{QueryId: queryID})
	if err != nil || got != mtproto.BoolTrue {
		t.Fatalf("callback answer = (%v, %v), want true from caller-scoped fixture", got, err)
	}
}

type callbackAnswerPollReader struct{}

func (*callbackAnswerPollReader) MessageGetUserMessage(context.Context, *messagepb.TLMessageGetUserMessage) (*mtproto.MessageBox, error) {
	panic("native callback path should not be reached when a control-plane result is published")
}

type callbackAnswerSyncClient struct {
	sync_client.SyncClient
}

type inlineBotSender struct {
	request *msgpb.TLMsgSendMessageV2
}

func (s *inlineBotSender) MsgSendMessageV2(_ context.Context, in *msgpb.TLMsgSendMessageV2) (*mtproto.Updates, error) {
	s.request = in
	return mtproto.MakeEmptyUpdates(), nil
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
