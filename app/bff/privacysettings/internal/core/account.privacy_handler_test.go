package core

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/privacysettings/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/privacysettings/internal/svc"
	syncclient "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	chatclient "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type privacyUserClientStub struct {
	userclient.UserClient
	rules        *userpb.Vector_PrivacyRule
	rulesErr     error
	users        *userpb.Vector_ImmutableUser
	usersErr     error
	setErr       error
	setResult    *mtproto.Bool
	nilSetResult bool
	setCalls     int
	requestedID  []int64
	events       *[]string
}

func (s *privacyUserClientStub) UserGetPrivacy(context.Context, *userpb.TLUserGetPrivacy) (*userpb.Vector_PrivacyRule, error) {
	return s.rules, s.rulesErr
}

func (s *privacyUserClientStub) UserGetMutableUsers(_ context.Context, in *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	s.requestedID = append([]int64(nil), in.GetId()...)
	if s.events != nil {
		*s.events = append(*s.events, "load_users")
	}
	return s.users, s.usersErr
}

func (s *privacyUserClientStub) UserSetPrivacy(context.Context, *userpb.TLUserSetPrivacy) (*mtproto.Bool, error) {
	s.setCalls++
	if s.events != nil {
		*s.events = append(*s.events, "save_rules")
	}
	if s.nilSetResult {
		return nil, s.setErr
	}
	if s.setResult != nil {
		return s.setResult, s.setErr
	}
	return mtproto.BoolTrue, s.setErr
}

type privacyChatClientStub struct {
	chatclient.ChatClient
	chats    *chatpb.Vector_MutableChat
	chatsErr error
}

func (s *privacyChatClientStub) ChatGetChatListByIdList(context.Context, *chatpb.TLChatGetChatListByIdList) (*chatpb.Vector_MutableChat, error) {
	return s.chats, s.chatsErr
}

type privacySyncClientStub struct {
	syncclient.SyncClient
	err      error
	reply    *mtproto.Void
	nilReply bool
	events   *[]string
}

func (s *privacySyncClientStub) SyncUpdatesNotMe(context.Context, *sync.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	if s.events != nil {
		*s.events = append(*s.events, "sync")
	}
	if s.nilReply {
		return nil, s.err
	}
	if s.reply != nil {
		return s.reply, s.err
	}
	return mtproto.EmptyVoid, s.err
}

func newPrivacyTestCore(userStub *privacyUserClientStub, chatStub *privacyChatClientStub, syncStub *privacySyncClientStub) *PrivacySettingsCore {
	c := New(context.Background(), &svc.ServiceContext{Dao: &dao.Dao{
		UserClient: userStub,
		ChatClient: chatStub,
		SyncClient: syncStub,
	}})
	c.MD = &metadata.RpcMetadata{UserId: 1, PermAuthKeyId: 10}
	return c
}

func privacyRules(rules ...*mtproto.PrivacyRule) *userpb.Vector_PrivacyRule {
	return &userpb.Vector_PrivacyRule{Datas: rules}
}

func immutablePrivacyUser(id int64) *mtproto.ImmutableUser {
	return &mtproto.ImmutableUser{User: &mtproto.UserData{Id: id}}
}

func privacyGetRequest() *mtproto.TLAccountGetPrivacy {
	return &mtproto.TLAccountGetPrivacy{Key: mtproto.MakeTLInputPrivacyKeyStatusTimestamp(nil).To_InputPrivacyKey()}
}

func privacySetRequest(rule *mtproto.InputPrivacyRule) *mtproto.TLAccountSetPrivacy {
	return &mtproto.TLAccountSetPrivacy{
		Key:   mtproto.MakeTLInputPrivacyKeyStatusTimestamp(nil).To_InputPrivacyKey(),
		Rules: []*mtproto.InputPrivacyRule{rule},
	}
}

func TestAccountGetPrivacyPropagatesRuleLoadError(t *testing.T) {
	wantErr := errors.New("user service unavailable")
	c := newPrivacyTestCore(&privacyUserClientStub{rulesErr: wantErr}, &privacyChatClientStub{}, nil)

	got, err := c.AccountGetPrivacy(privacyGetRequest())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped rule load error %v, got %v", wantErr, err)
	}
	if got != nil {
		t.Fatalf("expected no partial result, got %v", got)
	}
}

func TestAccountGetPrivacyPropagatesUserHydrationError(t *testing.T) {
	wantErr := errors.New("mutable users unavailable")
	userStub := &privacyUserClientStub{
		rules:    privacyRules(mtproto.MakeTLPrivacyValueAllowUsers(&mtproto.PrivacyRule{Users: []int64{2}}).To_PrivacyRule()),
		usersErr: wantErr,
	}
	c := newPrivacyTestCore(userStub, &privacyChatClientStub{}, nil)

	got, err := c.AccountGetPrivacy(privacyGetRequest())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped user hydration error %v, got %v", wantErr, err)
	}
	if got != nil {
		t.Fatalf("expected no partial result, got %v", got)
	}
}

func TestAccountGetPrivacyPropagatesChatHydrationError(t *testing.T) {
	wantErr := errors.New("chat service unavailable")
	chatStub := &privacyChatClientStub{chatsErr: wantErr}
	userStub := &privacyUserClientStub{
		rules: privacyRules(mtproto.MakeTLPrivacyValueAllowChatParticipants(&mtproto.PrivacyRule{Chats: []int64{42}}).To_PrivacyRule()),
	}
	c := newPrivacyTestCore(userStub, chatStub, nil)

	got, err := c.AccountGetPrivacy(privacyGetRequest())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped chat hydration error %v, got %v", wantErr, err)
	}
	if got != nil {
		t.Fatalf("expected no partial result, got %v", got)
	}
}

func TestAccountGetPrivacyFailsClosedForChannelRules(t *testing.T) {
	userStub := &privacyUserClientStub{
		rules: privacyRules(mtproto.MakeTLPrivacyValueAllowChatParticipants(&mtproto.PrivacyRule{Chats: []int64{mtproto.MinNebulaChatChannelID}}).To_PrivacyRule()),
	}
	c := newPrivacyTestCore(userStub, &privacyChatClientStub{}, nil)

	got, err := c.AccountGetPrivacy(privacyGetRequest())
	if err == nil {
		t.Fatal("expected unavailable channel hydration error")
	}
	if got != nil {
		t.Fatalf("expected no partial result, got %v", got)
	}
}

func TestAccountGetPrivacyBackfillsReferencedUsers(t *testing.T) {
	userStub := &privacyUserClientStub{
		rules: privacyRules(mtproto.MakeTLPrivacyValueAllowUsers(&mtproto.PrivacyRule{Users: []int64{2}}).To_PrivacyRule()),
		users: &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{immutablePrivacyUser(1), immutablePrivacyUser(2)}},
	}
	c := newPrivacyTestCore(userStub, &privacyChatClientStub{}, nil)

	got, err := c.AccountGetPrivacy(privacyGetRequest())
	if err != nil {
		t.Fatalf("AccountGetPrivacy returned error: %v", err)
	}
	if !reflect.DeepEqual(userStub.requestedID, []int64{1, 2}) {
		t.Fatalf("requested user IDs = %v, want [1 2]", userStub.requestedID)
	}
	if len(got.GetUsers()) != 1 || got.GetUsers()[0].GetId() != 2 {
		t.Fatalf("referenced users = %v, want only user 2", got.GetUsers())
	}
}

func TestAccountGetPrivacyBackfillsReferencedBasicGroups(t *testing.T) {
	userStub := &privacyUserClientStub{
		rules: privacyRules(mtproto.MakeTLPrivacyValueAllowChatParticipants(&mtproto.PrivacyRule{Chats: []int64{42}}).To_PrivacyRule()),
	}
	chatStub := &privacyChatClientStub{
		chats: &chatpb.Vector_MutableChat{Datas: []*mtproto.MutableChat{{Chat: &mtproto.ImmutableChat{Id: 42, Title: "group"}}}},
	}
	c := newPrivacyTestCore(userStub, chatStub, nil)

	got, err := c.AccountGetPrivacy(privacyGetRequest())
	if err != nil {
		t.Fatalf("AccountGetPrivacy returned error: %v", err)
	}
	if len(got.GetChats()) != 1 || got.GetChats()[0].GetId() != 42 {
		t.Fatalf("referenced chats = %v, want group 42", got.GetChats())
	}
}

func TestAccountSetPrivacyDoesNotWriteWhenHydrationFails(t *testing.T) {
	wantErr := errors.New("mutable users unavailable")
	userStub := &privacyUserClientStub{
		usersErr: wantErr,
	}
	allowUsers := mtproto.MakeTLInputPrivacyValueAllowUsers(nil)
	allowUsers.SetUsers([]*mtproto.InputUser{mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 2}).To_InputUser()})
	c := newPrivacyTestCore(userStub, &privacyChatClientStub{}, nil)

	got, err := c.AccountSetPrivacy(privacySetRequest(allowUsers.To_InputPrivacyRule()))
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped hydration error %v, got %v", wantErr, err)
	}
	if got != nil {
		t.Fatalf("expected no partial result, got %v", got)
	}
	if userStub.setCalls != 0 {
		t.Fatalf("privacy rules saved %d times after hydration failure", userStub.setCalls)
	}
}

func TestAccountSetPrivacyFailsClosedForChannelRulesBeforeWrite(t *testing.T) {
	allowChats := mtproto.MakeTLInputPrivacyValueAllowChatParticipants(nil)
	allowChats.SetChats([]int64{mtproto.MinNebulaChatChannelID})
	userStub := &privacyUserClientStub{}
	c := newPrivacyTestCore(userStub, &privacyChatClientStub{}, nil)

	got, err := c.AccountSetPrivacy(privacySetRequest(allowChats.To_InputPrivacyRule()))
	if err == nil {
		t.Fatal("expected unavailable channel hydration error")
	}
	if got != nil {
		t.Fatalf("expected no partial result, got %v", got)
	}
	if userStub.setCalls != 0 {
		t.Fatalf("privacy rules saved %d times after channel hydration failure", userStub.setCalls)
	}
}

func TestAccountSetPrivacyPropagatesSyncErrorAfterSave(t *testing.T) {
	wantErr := errors.New("sync broker unavailable")
	events := []string{}
	userStub := &privacyUserClientStub{
		users:  &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{immutablePrivacyUser(1), immutablePrivacyUser(2)}},
		events: &events,
	}
	syncStub := &privacySyncClientStub{err: wantErr, events: &events}
	allowUsers := mtproto.MakeTLInputPrivacyValueAllowUsers(nil)
	allowUsers.SetUsers([]*mtproto.InputUser{mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 2}).To_InputUser()})
	c := newPrivacyTestCore(userStub, &privacyChatClientStub{}, syncStub)

	got, err := c.AccountSetPrivacy(privacySetRequest(allowUsers.To_InputPrivacyRule()))
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped sync error %v, got %v", wantErr, err)
	}
	if got != nil {
		t.Fatalf("expected no reply after sync failure, got %v", got)
	}
	if userStub.setCalls != 1 {
		t.Fatalf("privacy rules saved %d times, want once before sync", userStub.setCalls)
	}
	if !reflect.DeepEqual(events, []string{"load_users", "save_rules", "sync"}) {
		t.Fatalf("operation order = %v, want [load_users save_rules sync]", events)
	}
}

func TestAccountSetPrivacyDoesNotSyncWhenUserServiceRejectsWrite(t *testing.T) {
	events := []string{}
	userStub := &privacyUserClientStub{
		users:     &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{immutablePrivacyUser(1), immutablePrivacyUser(2)}},
		setResult: mtproto.BoolFalse,
		events:    &events,
	}
	syncStub := &privacySyncClientStub{events: &events}
	allowUsers := mtproto.MakeTLInputPrivacyValueAllowUsers(nil)
	allowUsers.SetUsers([]*mtproto.InputUser{mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 2}).To_InputUser()})
	c := newPrivacyTestCore(userStub, &privacyChatClientStub{}, syncStub)

	got, err := c.AccountSetPrivacy(privacySetRequest(allowUsers.To_InputPrivacyRule()))
	if err == nil {
		t.Fatal("expected false persistence response to fail")
	}
	if got != nil {
		t.Fatalf("expected no reply after persistence rejection, got %v", got)
	}
	if !reflect.DeepEqual(events, []string{"load_users", "save_rules"}) {
		t.Fatalf("operation order = %v, want [load_users save_rules]", events)
	}
}

func TestAccountSetPrivacyPropagatesEmptySyncResponseAfterSave(t *testing.T) {
	events := []string{}
	userStub := &privacyUserClientStub{
		users:  &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{immutablePrivacyUser(1), immutablePrivacyUser(2)}},
		events: &events,
	}
	syncStub := &privacySyncClientStub{nilReply: true, events: &events}
	allowUsers := mtproto.MakeTLInputPrivacyValueAllowUsers(nil)
	allowUsers.SetUsers([]*mtproto.InputUser{mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 2}).To_InputUser()})
	c := newPrivacyTestCore(userStub, &privacyChatClientStub{}, syncStub)

	got, err := c.AccountSetPrivacy(privacySetRequest(allowUsers.To_InputPrivacyRule()))
	if err == nil {
		t.Fatal("expected empty sync response to fail")
	}
	if got != nil {
		t.Fatalf("expected no reply after empty sync response, got %v", got)
	}
	if userStub.setCalls != 1 {
		t.Fatalf("privacy rules saved %d times, want once before sync", userStub.setCalls)
	}
}
