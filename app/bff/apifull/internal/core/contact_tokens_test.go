package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type contactTokenUserClient struct {
	user_client.UserClient
	users     map[int64]*mtproto.ImmutableUser
	getCalls  []*userpb.TLUserGetImmutableUserV2
	addCalls  []*userpb.TLUserAddContact
	addResult *mtproto.Bool
	addErr    error
}

func (c *contactTokenUserClient) UserGetImmutableUserV2(_ context.Context, in *userpb.TLUserGetImmutableUserV2) (*mtproto.ImmutableUser, error) {
	c.getCalls = append(c.getCalls, in)
	return c.users[in.GetId()], nil
}

func (c *contactTokenUserClient) UserAddContact(_ context.Context, in *userpb.TLUserAddContact) (*mtproto.Bool, error) {
	c.addCalls = append(c.addCalls, in)
	return c.addResult, c.addErr
}

func contactTokenCore(userID int64, client user_client.UserClient) *ApiFullCore {
	return &ApiFullCore{
		MD:     &metadata.RpcMetadata{UserId: userID},
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
	}
}

func TestContactsExportImportContactTokenRoundTrip(t *testing.T) {
	ownerID, importerID := int64(810180), int64(810181)
	owner := &mtproto.ImmutableUser{User: &mtproto.UserData{
		Id: ownerID, AccessHash: 991, FirstName: "Ada", LastName: "Lovelace", Username: "ada",
		Phone: "+12025550180",
	}}
	exportClient := &contactTokenUserClient{users: map[int64]*mtproto.ImmutableUser{ownerID: owner}}
	exported, err := contactTokenCore(ownerID, exportClient).ContactsExportContactToken(nil)
	if err != nil {
		t.Fatalf("export contact token: %v", err)
	}
	const urlPrefix = "https://t.me/+"
	if !strings.HasPrefix(exported.GetUrl(), urlPrefix) || exported.GetExpires() <= int32(time.Now().Unix()) {
		t.Fatalf("exported token: url=%q expires=%d", exported.GetUrl(), exported.GetExpires())
	}
	token := strings.TrimPrefix(exported.GetUrl(), urlPrefix)
	if token == "" || strings.Contains(token, "/") {
		t.Fatalf("invalid token URL: %q", exported.GetUrl())
	}
	raw, err := persist.Default.Get(contactTokenKey(token))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = persist.CompareAndDelete(contactTokenKey(token), raw) })
	var record contactTokenRecord
	if err = json.Unmarshal([]byte(raw), &record); err != nil || record.UserID != ownerID || record.Expires != int64(exported.GetExpires()) {
		t.Fatalf("stored record=%+v err=%v", record, err)
	}

	importClient := &contactTokenUserClient{
		users:     map[int64]*mtproto.ImmutableUser{ownerID: owner},
		addResult: mtproto.BoolTrue,
	}
	imported, err := contactTokenCore(importerID, importClient).ContactsImportContactToken(&mtproto.TLContactsImportContactToken{Token: token})
	if err != nil {
		t.Fatalf("import contact token: %v", err)
	}
	if imported.GetId() != ownerID || !imported.GetContact() || !imported.GetMutualContact() || imported.GetAccessHash().GetValue() != 991 || imported.GetFirstName().GetValue() != "Ada" || imported.GetPhone().GetValue() != "+12025550180" {
		t.Fatalf("imported user: %+v", imported)
	}
	if len(importClient.addCalls) != 1 {
		t.Fatalf("contact writes: %d", len(importClient.addCalls))
	}
	added := importClient.addCalls[0]
	if added.GetUserId() != importerID || added.GetId() != ownerID || added.GetFirstName() != "Ada" || added.GetLastName() != "Lovelace" || added.GetPhone() != "+12025550180" {
		t.Fatalf("contact write: %+v", added)
	}
	if len(importClient.getCalls) != 1 || !importClient.getCalls[0].GetPrivacy() || !importClient.getCalls[0].GetHasTo() || len(importClient.getCalls[0].GetTo()) != 1 || importClient.getCalls[0].GetTo()[0] != importerID {
		t.Fatalf("owner lookup: %+v", importClient.getCalls)
	}
}

func TestContactsImportContactTokenRejectsInvalidAndExpired(t *testing.T) {
	expired := `{"user_id":810182,"expires":1}`
	if err := persist.Default.Set(contactTokenKey("expired"), expired); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = persist.CompareAndDelete(contactTokenKey("expired"), expired) })
	client := &contactTokenUserClient{users: make(map[int64]*mtproto.ImmutableUser)}
	c := contactTokenCore(810183, client)
	for _, token := range []string{"", "missing", "expired"} {
		if _, err := c.ContactsImportContactToken(&mtproto.TLContactsImportContactToken{Token: token}); err != mtproto.ErrTokenInvalid {
			t.Errorf("token %q: got %v, want TOKEN_INVALID", token, err)
		}
	}
	if len(client.getCalls) != 0 || len(client.addCalls) != 0 {
		t.Fatalf("invalid token caused service calls: lookups=%d writes=%d", len(client.getCalls), len(client.addCalls))
	}
}

func TestContactsImportContactTokenPropagatesContactWriteFailure(t *testing.T) {
	ownerID, importerID := int64(810184), int64(810185)
	token := "persist-failure"
	record, err := json.Marshal(contactTokenRecord{UserID: ownerID, Expires: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if err = persist.Default.Set(contactTokenKey(token), string(record)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = persist.CompareAndDelete(contactTokenKey(token), string(record)) })
	wantErr := mtproto.ErrInternalServerError
	client := &contactTokenUserClient{
		users:  map[int64]*mtproto.ImmutableUser{ownerID: {User: &mtproto.UserData{Id: ownerID}}},
		addErr: wantErr,
	}
	if got, err := contactTokenCore(importerID, client).ContactsImportContactToken(&mtproto.TLContactsImportContactToken{Token: token}); err != wantErr || got != nil {
		t.Fatalf("result=%+v err=%v, want nil and underlying contact write error", got, err)
	}
}
