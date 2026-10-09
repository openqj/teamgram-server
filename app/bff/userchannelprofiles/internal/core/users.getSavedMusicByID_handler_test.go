package core

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	mediaclient "github.com/teamgram/teamgram-server/app/service/media/client"
	media "github.com/teamgram/teamgram-server/app/service/media/media"
	"github.com/zeromicro/go-zero/core/logx"
)

type savedMusicByIDUserClient struct {
	userclient.UserClient
	users          map[int64]*mtproto.ImmutableUser
	privacy        []*mtproto.PrivacyRule
	savedIDs       []int64
	immutableCalls []*userpb.TLUserGetImmutableUser
	savedRequest   *userpb.TLUserGetSavedMusicIdList
	savedCalls     int
	privacyCalls   int
	privacyErr     error
	nilSaved       bool
	member         bool
	nilPrivacy     bool
	privacyRequest *userpb.TLUserCheckPrivacy
}

func (c *savedMusicByIDUserClient) UserGetImmutableUser(_ context.Context, in *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	c.immutableCalls = append(c.immutableCalls, in)
	return c.users[in.GetId()], nil
}

func (c *savedMusicByIDUserClient) UserCheckPrivacy(_ context.Context, in *userpb.TLUserCheckPrivacy) (*mtproto.Bool, error) {
	c.privacyCalls++
	c.privacyRequest = in
	if c.privacyErr != nil {
		return nil, c.privacyErr
	}
	if c.nilPrivacy {
		return nil, nil
	}
	ctx := mtproto.PrivacyRuleContext{UserKnown: true, ContactKnown: true, ChatParticipant: func([]int64) (bool, error) { return c.member, nil }}
	if owner := c.users[in.GetUserId()]; owner != nil {
		if contact := owner.GetContactData(in.GetPeerId()); contact != nil {
			ctx.Contact, ctx.CloseFriend = true, contact.GetCloseFriend()
		}
	}
	if viewer := c.users[in.GetPeerId()]; viewer != nil {
		ctx.Bot, ctx.Premium = viewer.IsBot(), viewer.Premium()
	}
	allowed, err := mtproto.CheckPrivacyRules(c.privacy, in.GetPeerId(), ctx)
	return mtproto.ToBool(allowed), err
}

func (c *savedMusicByIDUserClient) UserGetSavedMusicIdList(_ context.Context, in *userpb.TLUserGetSavedMusicIdList) (*userpb.Vector_Long, error) {
	c.savedCalls++
	c.savedRequest = in
	if c.nilSaved {
		return nil, nil
	}
	return &userpb.Vector_Long{Datas: c.savedIDs}, nil
}

type savedMusicByIDMediaClient struct {
	mediaclient.MediaClient
	documents []*mtproto.Document
	ids       []int64
	err       error
	nilResult bool
}

func (c *savedMusicByIDMediaClient) MediaGetDocumentList(_ context.Context, in *media.TLMediaGetDocumentList) (*media.Vector_Document, error) {
	c.ids = append([]int64(nil), in.GetIdList()...)
	if c.err != nil {
		return nil, c.err
	}
	if c.nilResult {
		return nil, nil
	}
	return &media.Vector_Document{Datas: c.documents}, nil
}

func TestUsersGetSavedMusicByIDReturnsOnlyAuthorizedSavedDocuments(t *testing.T) {
	users := newSavedMusicByIDUserClient()
	users.savedIDs = []int64{10}
	mediaClient := &savedMusicByIDMediaClient{documents: []*mtproto.Document{
		{Id: 10, AccessHash: 1000},
		{Id: 20, AccessHash: 2000},
	}}
	core := newSavedMusicByIDCore(users, mediaClient)
	in := savedMusicByIDRequest(7, 700, savedMusicInputDocument(10, 1000), savedMusicInputDocument(20, 2000))

	got, err := core.UsersGetSavedMusicByID(in)
	if err != nil {
		t.Fatalf("UsersGetSavedMusicByID() error = %v", err)
	}
	if got.GetCount() != 1 || len(got.GetDocuments()) != 1 || got.GetDocuments()[0].GetId() != 10 {
		t.Fatalf("UsersGetSavedMusicByID() = %v, want only saved document 10", got)
	}
	if !reflect.DeepEqual(mediaClient.ids, []int64{10}) {
		t.Fatalf("media lookup ids = %v, want [10]", mediaClient.ids)
	}
	if users.savedRequest.GetUserId() != 7 {
		t.Fatalf("saved-music owner id = %d, want 7", users.savedRequest.GetUserId())
	}
	if users.immutableCalls[0].GetContacts() == nil || !reflect.DeepEqual(users.immutableCalls[0].GetContacts(), []int64{42}) {
		t.Fatalf("target contact lookup ids = %v, want [42]", users.immutableCalls[0].GetContacts())
	}
}

func TestUsersGetSavedMusicByIDChecksAccessHashBeforeReadingSavedMusic(t *testing.T) {
	users := newSavedMusicByIDUserClient()
	mediaClient := &savedMusicByIDMediaClient{}
	core := newSavedMusicByIDCore(users, mediaClient)

	got, err := core.UsersGetSavedMusicByID(savedMusicByIDRequest(7, 701, savedMusicInputDocument(10, 1000)))
	if got != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("UsersGetSavedMusicByID() = (%v, %v), want USER_ID_INVALID", got, err)
	}
	if users.savedCalls != 0 || len(mediaClient.ids) != 0 {
		t.Fatalf("reads after access hash mismatch: saved=%d media=%v", users.savedCalls, mediaClient.ids)
	}
}

func TestUsersGetSavedMusicByIDDenyPrivacyBeforeReadingSavedMusic(t *testing.T) {
	users := newSavedMusicByIDUserClient()
	users.privacy = []*mtproto.PrivacyRule{mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule()}
	mediaClient := &savedMusicByIDMediaClient{}
	core := newSavedMusicByIDCore(users, mediaClient)

	got, err := core.UsersGetSavedMusicByID(savedMusicByIDRequest(7, 700, savedMusicInputDocument(10, 1000)))
	if got != nil || !errors.Is(err, mtproto.ErrUserPrivacyRestricted) {
		t.Fatalf("UsersGetSavedMusicByID() = (%v, %v), want USER_PRIVACY_RESTRICTED", got, err)
	}
	if users.savedCalls != 0 || len(mediaClient.ids) != 0 {
		t.Fatalf("reads after privacy denial: saved=%d media=%v", users.savedCalls, mediaClient.ids)
	}
}

func TestUsersGetSavedMusicByIDHonorsContactPrivacy(t *testing.T) {
	users := newSavedMusicByIDUserClient()
	users.privacy = []*mtproto.PrivacyRule{mtproto.MakeTLPrivacyValueAllowContacts(nil).To_PrivacyRule()}
	users.users[7] = mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User:     mtproto.MakeTLUserData(&mtproto.UserData{Id: 7, AccessHash: 700}).To_UserData(),
		Contacts: []*mtproto.ContactData{{ContactUserId: 42}},
	}).To_ImmutableUser()
	users.savedIDs = []int64{10}
	mediaClient := &savedMusicByIDMediaClient{documents: []*mtproto.Document{{Id: 10, AccessHash: 1000}}}
	core := newSavedMusicByIDCore(users, mediaClient)

	got, err := core.UsersGetSavedMusicByID(savedMusicByIDRequest(7, 700, savedMusicInputDocument(10, 1000)))
	if err != nil {
		t.Fatalf("UsersGetSavedMusicByID() error = %v", err)
	}
	if got.GetCount() != 1 || got.GetDocuments()[0].GetId() != 10 {
		t.Fatalf("UsersGetSavedMusicByID() = %v, want saved document 10", got)
	}
}

func TestUsersGetSavedMusicByIDFailsClosedForChatParticipantPrivacy(t *testing.T) {
	users := newSavedMusicByIDUserClient()
	users.member = true
	users.privacy = []*mtproto.PrivacyRule{
		mtproto.MakeTLPrivacyValueAllowAll(nil).To_PrivacyRule(),
		mtproto.MakeTLPrivacyValueDisallowChatParticipants(&mtproto.PrivacyRule{Chats: []int64{900}}).To_PrivacyRule(),
	}
	mediaClient := &savedMusicByIDMediaClient{}
	core := newSavedMusicByIDCore(users, mediaClient)

	got, err := core.UsersGetSavedMusicByID(savedMusicByIDRequest(7, 700, savedMusicInputDocument(10, 1000)))
	if got != nil || !errors.Is(err, mtproto.ErrUserPrivacyRestricted) {
		t.Fatalf("UsersGetSavedMusicByID() = (%v, %v), want USER_PRIVACY_RESTRICTED", got, err)
	}
	if users.savedCalls != 0 || len(mediaClient.ids) != 0 {
		t.Fatalf("reads after unsupported privacy rule: saved=%d media=%v", users.savedCalls, mediaClient.ids)
	}
}

func TestUsersGetSavedMusicByIDSelfReadsOnlyOwnSavedMusic(t *testing.T) {
	users := newSavedMusicByIDUserClient()
	users.users[42] = mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User: mtproto.MakeTLUserData(&mtproto.UserData{Id: 42, AccessHash: 4200}).To_UserData(),
	}).To_ImmutableUser()
	users.savedIDs = []int64{10}
	mediaClient := &savedMusicByIDMediaClient{documents: []*mtproto.Document{{Id: 10, AccessHash: 1000}}}
	core := newSavedMusicByIDCore(users, mediaClient)
	in := &mtproto.TLUsersGetSavedMusicByID{
		Id:        mtproto.MakeTLInputUserSelf(nil).To_InputUser(),
		Documents: []*mtproto.InputDocument{savedMusicInputDocument(10, 1000)},
	}

	got, err := core.UsersGetSavedMusicByID(in)
	if err != nil {
		t.Fatalf("UsersGetSavedMusicByID(self) error = %v", err)
	}
	if users.privacyCalls != 0 || users.savedRequest.GetUserId() != 42 || got.GetCount() != 1 {
		t.Fatalf("self result/privacy/saved owner = (%v, %d, %v), want own list without privacy lookup", got, users.savedRequest.GetUserId(), users.privacyCalls)
	}
}

func TestUsersGetSavedMusicByIDRejectsDocumentAccessHashMismatch(t *testing.T) {
	users := newSavedMusicByIDUserClient()
	users.savedIDs = []int64{10}
	mediaClient := &savedMusicByIDMediaClient{documents: []*mtproto.Document{{Id: 10, AccessHash: 1001}}}
	core := newSavedMusicByIDCore(users, mediaClient)

	got, err := core.UsersGetSavedMusicByID(savedMusicByIDRequest(7, 700, savedMusicInputDocument(10, 1000)))
	if got != nil || !errors.Is(err, mtproto.ErrDocumentInvalid) {
		t.Fatalf("UsersGetSavedMusicByID() = (%v, %v), want DOCUMENT_INVALID", got, err)
	}
}

func TestUsersGetSavedMusicByIDRejectsInputUserWithoutAccessHash(t *testing.T) {
	users := newSavedMusicByIDUserClient()
	core := newSavedMusicByIDCore(users, &savedMusicByIDMediaClient{})
	in := savedMusicByIDRequest(7, 0, savedMusicInputDocument(10, 1000))

	got, err := core.UsersGetSavedMusicByID(in)
	if got != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("UsersGetSavedMusicByID() = (%v, %v), want USER_ID_INVALID", got, err)
	}
}

func TestUsersGetSavedMusicByIDFailsClosedOnNilSavedMusicList(t *testing.T) {
	users := newSavedMusicByIDUserClient()
	users.nilSaved = true
	mediaClient := &savedMusicByIDMediaClient{}
	core := newSavedMusicByIDCore(users, mediaClient)

	got, err := core.UsersGetSavedMusicByID(savedMusicByIDRequest(7, 700, savedMusicInputDocument(10, 1000)))
	if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("UsersGetSavedMusicByID() = (%v, %v), want INTERNAL_SERVER_ERROR for nil saved list", got, err)
	}
	if len(mediaClient.ids) != 0 {
		t.Fatalf("media reads after nil saved list = %v, want none", mediaClient.ids)
	}
}

func TestUsersGetSavedMusicByIDFailsClosedOnNilMediaList(t *testing.T) {
	users := newSavedMusicByIDUserClient()
	users.savedIDs = []int64{10}
	mediaClient := &savedMusicByIDMediaClient{nilResult: true}
	core := newSavedMusicByIDCore(users, mediaClient)

	got, err := core.UsersGetSavedMusicByID(savedMusicByIDRequest(7, 700, savedMusicInputDocument(10, 1000)))
	if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("UsersGetSavedMusicByID() = (%v, %v), want INTERNAL_SERVER_ERROR for nil media list", got, err)
	}
}

func TestUsersGetSavedMusicByIDFiltersMissingSavedDocument(t *testing.T) {
	users := newSavedMusicByIDUserClient()
	users.savedIDs = []int64{10}
	mediaClient := &savedMusicByIDMediaClient{documents: []*mtproto.Document{}}
	core := newSavedMusicByIDCore(users, mediaClient)

	got, err := core.UsersGetSavedMusicByID(savedMusicByIDRequest(7, 700, savedMusicInputDocument(10, 1000)))
	if err != nil {
		t.Fatalf("UsersGetSavedMusicByID() error = %v, want stale missing document filtered", err)
	}
	if got.GetCount() != 0 || len(got.GetDocuments()) != 0 {
		t.Fatalf("UsersGetSavedMusicByID() = %v, want empty result for missing stale document", got)
	}
}

func newSavedMusicByIDUserClient() *savedMusicByIDUserClient {
	return &savedMusicByIDUserClient{
		users: map[int64]*mtproto.ImmutableUser{
			7: mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
				User: mtproto.MakeTLUserData(&mtproto.UserData{Id: 7, AccessHash: 700}).To_UserData(),
			}).To_ImmutableUser(),
		},
		privacy: []*mtproto.PrivacyRule{mtproto.MakeTLPrivacyValueAllowAll(nil).To_PrivacyRule()},
	}
}

func newSavedMusicByIDCore(users *savedMusicByIDUserClient, mediaClient *savedMusicByIDMediaClient) *UserChannelProfilesCore {
	ctx := context.Background()
	return &UserChannelProfilesCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient:  users,
			MediaClient: mediaClient,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
}

func savedMusicByIDRequest(userID, accessHash int64, documents ...*mtproto.InputDocument) *mtproto.TLUsersGetSavedMusicByID {
	return &mtproto.TLUsersGetSavedMusicByID{
		Id:        mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: userID, AccessHash: accessHash}).To_InputUser(),
		Documents: documents,
	}
}

func savedMusicInputDocument(id, accessHash int64) *mtproto.InputDocument {
	return mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: id, AccessHash: accessHash}).To_InputDocument()
}

func TestUsersSavedMusicAuthoritativePrivacy(t *testing.T) {
	wantErr := errors.New("postgres membership unavailable")
	for _, method := range []string{"by ID", "list"} {
		for _, tc := range []struct {
			name         string
			member       bool
			err, wantErr error
			nilPrivacy   bool
		}{
			{name: "participant allowed", member: true},
			{name: "nonparticipant denied", wantErr: mtproto.ErrUserPrivacyRestricted},
			{name: "membership failure", err: wantErr, wantErr: wantErr},
			{name: "nil privacy", nilPrivacy: true, wantErr: mtproto.ErrInternalServerError},
		} {
			t.Run(method+"/"+tc.name, func(t *testing.T) {
				users := newSavedMusicByIDUserClient()
				users.privacy = []*mtproto.PrivacyRule{
					mtproto.MakeTLPrivacyValueAllowChatParticipants(&mtproto.PrivacyRule{Chats: []int64{900}}).To_PrivacyRule(),
					mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule(),
				}
				users.member, users.privacyErr, users.nilPrivacy = tc.member, tc.err, tc.nilPrivacy
				users.savedIDs = []int64{10}
				mediaClient := &savedMusicByIDMediaClient{documents: []*mtproto.Document{{Id: 10, AccessHash: 1000}}}
				core := newSavedMusicByIDCore(users, mediaClient)
				var got *mtproto.Users_SavedMusic
				var err error
				if method == "by ID" {
					got, err = core.UsersGetSavedMusicByID(savedMusicByIDRequest(7, 700, savedMusicInputDocument(10, 1000)))
				} else {
					got, err = core.UsersGetSavedMusic(&mtproto.TLUsersGetSavedMusic{Id: savedMusicByIDRequest(7, 700).GetId(), Limit: 10})
				}
				if !errors.Is(err, tc.wantErr) || tc.wantErr != nil && got != nil {
					t.Fatalf("saved music = (%v, %v), want error %v", got, err, tc.wantErr)
				}
				if tc.wantErr == nil && (got.GetCount() != 1 || len(got.GetDocuments()) != 1) {
					t.Fatalf("authorized saved music = %v, want one document", got)
				}
				if tc.wantErr != nil && (users.savedCalls != 0 || len(mediaClient.ids) != 0) {
					t.Fatalf("reads after privacy failure: saved=%d media=%v", users.savedCalls, mediaClient.ids)
				}
				if in := users.privacyRequest; in.GetUserId() != 7 || in.GetPeerId() != 42 || in.GetKeyType() != mtproto.SAVED_MUSIC {
					t.Fatalf("saved music privacy request = %v", in)
				}
			})
		}
	}
}
