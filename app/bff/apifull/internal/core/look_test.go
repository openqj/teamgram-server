package core

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestLookAccountUpdateColor(t *testing.T) {
	c, _ := newAccentColorCore(1)
	if _, err := c.AccountUpdateColor(&mtproto.TLAccountUpdateColor{}); err != nil {
		t.Fatal(err)
	}
}

func TestThemeMySQL(t *testing.T) {
	userID := time.Now().UnixNano()
	document := mtproto.MakeTLDocument(&mtproto.Document{Id: userID + 1, AccessHash: userID + 2, MimeType: "application/x-tgtheme"}).To_Document()
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err = persist.Default.Set(themeUploadKey(userID, document.GetId()), string(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err = (&ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}).AccountCreateTheme(&mtproto.TLAccountCreateTheme{
		Slug: "mysql-theme", Title: "mysql-theme",
		Document: mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: document.GetId(), AccessHash: document.GetAccessHash()}).To_InputDocument(),
	}); err != nil {
		t.Fatal(err)
	}
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}
	inTheme := mtproto.MakeTLInputThemeSlug(&mtproto.InputTheme{
		Slug: "mysql-theme",
	}).To_InputTheme()
	ok, err := c.AccountSaveTheme(&mtproto.TLAccountSaveTheme{
		Theme:  inTheme,
		Unsave: mtproto.BoolFalse,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !mtproto.FromBool(ok) {
		t.Fatalf("save: %+v", ok)
	}
	got, err := c.AccountGetTheme(&mtproto.TLAccountGetTheme{
		Format: "android",
		Theme:  inTheme,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetSlug() != "mysql-theme" || got.GetTitle() != "mysql-theme" {
		t.Fatalf("theme: %+v", got)
	}
}
