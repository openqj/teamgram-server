package core

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/svc"
	contactsplugin "github.com/teamgram/teamgram-server/app/bff/contacts/plugin"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	chatclient "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type contactsSearchUserClientStub struct {
	userclient.UserClient
	contactIDs        *userpb.Vector_Long
	contactErr        error
	usernameResults   *userpb.Vector_UsernameData
	usernameErr       error
	userResults       *userpb.UsersFound
	userSearchErr     error
	mutableUsers      *userpb.Vector_ImmutableUser
	mutableUsersErr   error
	usernameRequest   *userpb.TLUserSearchUsername
	userSearchRequest *userpb.TLUserSearch
	mutableRequest    *userpb.TLUserGetMutableUsers
}

func (s *contactsSearchUserClientStub) UserGetContactIdList(_ context.Context, _ *userpb.TLUserGetContactIdList) (*userpb.Vector_Long, error) {
	if s.contactErr != nil {
		return nil, s.contactErr
	}
	if s.contactIDs == nil {
		return &userpb.Vector_Long{}, nil
	}
	return s.contactIDs, nil
}

func (s *contactsSearchUserClientStub) UserSearchUsername(_ context.Context, in *userpb.TLUserSearchUsername) (*userpb.Vector_UsernameData, error) {
	s.usernameRequest = in
	if s.usernameErr != nil {
		return nil, s.usernameErr
	}
	if s.usernameResults == nil {
		return &userpb.Vector_UsernameData{}, nil
	}
	return s.usernameResults, nil
}

func (s *contactsSearchUserClientStub) UserSearch(_ context.Context, in *userpb.TLUserSearch) (*userpb.UsersFound, error) {
	s.userSearchRequest = in
	if s.userSearchErr != nil {
		return nil, s.userSearchErr
	}
	if s.userResults == nil {
		return &userpb.UsersFound{PredicateName: userpb.Predicate_usersIdFound}, nil
	}
	return s.userResults, nil
}

func (s *contactsSearchUserClientStub) UserGetMutableUsers(_ context.Context, in *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	s.mutableRequest = in
	if s.mutableUsersErr != nil {
		return nil, s.mutableUsersErr
	}
	if s.mutableUsers == nil {
		return &userpb.Vector_ImmutableUser{}, nil
	}
	return s.mutableUsers, nil
}

type contactsSearchChatClientStub struct {
	chatclient.ChatClient
	searchCalls int
}

func (s *contactsSearchChatClientStub) ChatSearch(_ context.Context, _ *chatpb.TLChatSearch) (*chatpb.Vector_MutableChat, error) {
	s.searchCalls++
	return &chatpb.Vector_MutableChat{}, nil
}

type contactsSearchPluginStub struct {
	contactsplugin.ContactsPlugin
	chats  []*mtproto.Chat
	selfID int64
	ids    []int64
	calls  int
}

func (s *contactsSearchPluginStub) GetChannelListByIdList(_ context.Context, selfID int64, ids ...int64) []*mtproto.Chat {
	s.calls++
	s.selfID = selfID
	s.ids = append([]int64{}, ids...)
	return s.chats
}

func newContactsSearchTestCore(userClient *contactsSearchUserClientStub, chatClient *contactsSearchChatClientStub, plugin contactsplugin.ContactsPlugin) *ContactsCore {
	ctx := context.Background()
	return &ContactsCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{
			Dao: &dao.Dao{
				UserClient: userClient,
				ChatClient: chatClient,
			},
			Plugin: plugin,
		},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
}

func contactsSearchImmutableUser(id int64, contacts ...int64) *mtproto.ImmutableUser {
	userContacts := make([]*mtproto.ContactData, 0, len(contacts))
	for _, contactID := range contacts {
		userContacts = append(userContacts, &mtproto.ContactData{ContactUserId: contactID})
	}
	return mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User:     &mtproto.UserData{Id: id, AccessHash: id + 1000, FirstName: "User"},
		Contacts: userContacts,
	}).To_ImmutableUser()
}

func contactsSearchUsernameData(peer *mtproto.Peer) *userpb.UsernameData {
	return userpb.MakeTLUsernameData(&userpb.UsernameData{Username: "result", Peer: peer}).To_UsernameData()
}

func contactsSearchChannel(id int64) *mtproto.Chat {
	return mtproto.MakeTLChannel(&mtproto.Chat{
		Id:                   id,
		Title:                "Real channel",
		AccessHash_FLAGINT64: mtproto.MakeFlagsInt64(id + 100),
	}).To_Chat()
}

func TestContactsSearchHydratesUserAndChannelResults(t *testing.T) {
	userClient := &contactsSearchUserClientStub{
		contactIDs: &userpb.Vector_Long{Datas: []int64{84, 77}},
		usernameResults: &userpb.Vector_UsernameData{Datas: []*userpb.UsernameData{
			contactsSearchUsernameData(mtproto.MakePeerUser(85)),
			contactsSearchUsernameData(mtproto.MakePeerChannel(90)),
		}},
		// The current user.search implementation returns ID-only matches without
		// filtering the ExcludedContacts list, so contact 84 can appear here.
		userResults: &userpb.UsersFound{PredicateName: userpb.Predicate_usersIdFound, IdList: []int64{84, 85}},
		mutableUsers: &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{
			contactsSearchImmutableUser(42),
			contactsSearchImmutableUser(84),
			contactsSearchImmutableUser(85),
		}},
	}
	chatClient := &contactsSearchChatClientStub{}
	plugin := &contactsSearchPluginStub{chats: []*mtproto.Chat{contactsSearchChannel(90)}}
	core := newContactsSearchTestCore(userClient, chatClient, plugin)

	got, err := core.ContactsSearch(&mtproto.TLContactsSearch{Q: "@ana", Limit: 100})
	if err != nil {
		t.Fatalf("ContactsSearch() error = %v", err)
	}
	if got == nil {
		t.Fatal("ContactsSearch() returned nil result")
	}
	if userClient.usernameRequest == nil || userClient.usernameRequest.Q != "ana" || userClient.usernameRequest.Limit != 50 || !reflect.DeepEqual(userClient.usernameRequest.ExcludedContacts, []int64{84, 77, 42}) {
		t.Fatalf("user.searchUsername request = %+v", userClient.usernameRequest)
	}
	if userClient.userSearchRequest == nil || userClient.userSearchRequest.Q != "ana" || !reflect.DeepEqual(userClient.userSearchRequest.ExcludedContacts, []int64{84, 77, 42}) {
		t.Fatalf("user.search request = %+v", userClient.userSearchRequest)
	}
	if userClient.mutableRequest == nil || !reflect.DeepEqual(userClient.mutableRequest.GetId(), []int64{42, 85, 84}) {
		t.Fatalf("user.getMutableUsers request = %+v", userClient.mutableRequest)
	}
	if len(got.GetMyResults()) != 1 || got.GetMyResults()[0].GetUserId() != 84 {
		t.Fatalf("my_results = %v, want contact user 84", got.GetMyResults())
	}
	if len(got.GetResults()) != 2 || got.GetResults()[0].GetUserId() != 85 || got.GetResults()[1].GetChannelId() != 90 {
		t.Fatalf("results = %v, want user 85 then channel 90", got.GetResults())
	}
	if len(got.GetUsers()) != 2 || got.GetUsers()[0].GetId() != 85 || got.GetUsers()[1].GetId() != 84 {
		t.Fatalf("users = %v, want hydrated search users 85 and 84 without self", got.GetUsers())
	}
	if len(got.GetChats()) != 1 || got.GetChats()[0].GetPredicateName() != mtproto.Predicate_channel || got.GetChats()[0].GetId() != 90 {
		t.Fatalf("chats = %v, want channel 90 from resolver", got.GetChats())
	}
	if plugin.calls != 1 || plugin.selfID != 42 || !reflect.DeepEqual(plugin.ids, []int64{90}) {
		t.Fatalf("channel resolver call = calls:%d self:%d ids:%v", plugin.calls, plugin.selfID, plugin.ids)
	}
	if chatClient.searchCalls != 0 {
		t.Fatalf("unsafe chat.search called %d times", chatClient.searchCalls)
	}
}

func TestContactsSearchPropagatesQueryAndHydrationErrors(t *testing.T) {
	wantErr := errors.New("service failure")
	tests := []struct {
		name   string
		client *contactsSearchUserClientStub
	}{
		{name: "contact lookup", client: &contactsSearchUserClientStub{contactErr: wantErr}},
		{name: "username search", client: &contactsSearchUserClientStub{usernameErr: wantErr}},
		{name: "name search", client: &contactsSearchUserClientStub{userSearchErr: wantErr}},
		{name: "user hydration", client: &contactsSearchUserClientStub{
			usernameResults: &userpb.Vector_UsernameData{Datas: []*userpb.UsernameData{contactsSearchUsernameData(mtproto.MakePeerUser(84))}},
			mutableUsersErr: wantErr,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newContactsSearchTestCore(tt.client, &contactsSearchChatClientStub{}, nil).ContactsSearch(&mtproto.TLContactsSearch{Q: "alice"})
			if !errors.Is(err, wantErr) || got != nil {
				t.Fatalf("ContactsSearch() = (%v, %v), want nil response and propagated error", got, err)
			}
		})
	}
}

func TestContactsSearchFailsClosedWhenChannelResolverIsUnavailableOrIncomplete(t *testing.T) {
	baseClient := func() *contactsSearchUserClientStub {
		return &contactsSearchUserClientStub{
			usernameResults: &userpb.Vector_UsernameData{Datas: []*userpb.UsernameData{contactsSearchUsernameData(mtproto.MakePeerChannel(90))}},
		}
	}
	tests := []struct {
		name   string
		plugin contactsplugin.ContactsPlugin
		want   string
	}{
		{name: "resolver missing", want: "contacts server currently wires Plugin=nil"},
		{name: "result missing", plugin: &contactsSearchPluginStub{}, want: "did not return search result channel 90"},
		{name: "wrong entity id", plugin: &contactsSearchPluginStub{chats: []*mtproto.Chat{contactsSearchChannel(91)}}, want: "unrequested channel 91"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newContactsSearchTestCore(baseClient(), &contactsSearchChatClientStub{}, tt.plugin).ContactsSearch(&mtproto.TLContactsSearch{Q: "alice"})
			if err == nil || got != nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ContactsSearch() = (%v, %v), want fail-closed error containing %q", got, err, tt.want)
			}
		})
	}
}

func TestContactsSearchFailsClosedForUnscopedChatSearchResults(t *testing.T) {
	userClient := &contactsSearchUserClientStub{
		usernameResults: &userpb.Vector_UsernameData{Datas: []*userpb.UsernameData{contactsSearchUsernameData(mtproto.MakeTLPeerChat(&mtproto.Peer{ChatId: 17}).To_Peer())}},
	}
	chatClient := &contactsSearchChatClientStub{}
	got, err := newContactsSearchTestCore(userClient, chatClient, nil).ContactsSearch(&mtproto.TLContactsSearch{Q: "group"})
	if err == nil || got != nil || !strings.Contains(err.Error(), "chat search has no user-scoped search contract") {
		t.Fatalf("ContactsSearch() = (%v, %v), want fail-closed chat search error", got, err)
	}
	if chatClient.searchCalls != 0 {
		t.Fatalf("unsafe chat.search called %d times", chatClient.searchCalls)
	}
}
