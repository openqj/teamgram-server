package core

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestThemeSaveGetRoundtrip(t *testing.T) {
	userID := time.Now().UnixNano()
	document := mtproto.MakeTLDocument(&mtproto.Document{
		Id:         userID + 1,
		AccessHash: userID + 2,
		MimeType:   "application/x-tgtheme",
	}).To_Document()
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err = persist.Default.Set(themeUploadKey(userID, document.GetId()), string(raw)); err != nil {
		t.Fatal(err)
	}
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}
	inTheme := mtproto.MakeTLInputTheme(&mtproto.InputTheme{
		Id:         document.GetId(),
		AccessHash: document.GetAccessHash(),
	}).To_InputTheme()
	if _, err = c.AccountCreateTheme(&mtproto.TLAccountCreateTheme{
		Slug:     "roundtrip-theme",
		Title:    "Roundtrip theme",
		Document: mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: document.GetId(), AccessHash: document.GetAccessHash()}).To_InputDocument(),
	}); err != nil {
		t.Fatal(err)
	}
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
	if got.GetId() != document.GetId() || got.GetAccessHash() != document.GetAccessHash() || got.GetDocument() == nil {
		t.Fatalf("theme: %+v", got)
	}
	chatThemes, err := c.AccountGetChatThemes(&mtproto.TLAccountGetChatThemes{})
	if err != nil || len(chatThemes.GetThemes()) != 1 || chatThemes.GetThemes()[0].GetId() != document.GetId() {
		t.Fatalf("chat themes: result=%+v err=%v", chatThemes, err)
	}
	unchanged, err := c.AccountGetChatThemes(&mtproto.TLAccountGetChatThemes{Hash: chatThemes.GetHash()})
	if err != nil || unchanged == nil || unchanged.GetPredicateName() != mtproto.Predicate_account_themesNotModified {
		t.Fatalf("chat themes not modified: result=%+v err=%v", unchanged, err)
	}
}

func TestThemeSaveAndInstallRejectUnownedInput(t *testing.T) {
	userID := time.Now().UnixNano()
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}
	inTheme := mtproto.MakeTLInputTheme(&mtproto.InputTheme{Id: userID + 1, AccessHash: userID + 2}).To_InputTheme()
	if result, err := c.AccountSaveTheme(&mtproto.TLAccountSaveTheme{Theme: inTheme}); result != nil || err != mtproto.ErrThemeInvalid {
		t.Fatalf("save result=%v err=%v, want THEME_INVALID", result, err)
	}
	if result, err := c.AccountInstallTheme(&mtproto.TLAccountInstallTheme{Theme: inTheme}); result != nil || err != mtproto.ErrThemeInvalid {
		t.Fatalf("install result=%v err=%v, want THEME_INVALID", result, err)
	}
	if result, err := c.AccountGetThemes(&mtproto.TLAccountGetThemes{}); err != nil || len(result.GetThemes()) != 0 {
		t.Fatalf("themes result=%v err=%v, want empty persisted list", result, err)
	}
}

func TestThemeCreateUpdateUsesOwnedUploadedDocument(t *testing.T) {
	userID := time.Now().UnixNano()
	document := mtproto.MakeTLDocument(&mtproto.Document{
		Id:         userID + 1,
		AccessHash: userID + 2,
		MimeType:   "application/x-tgtheme",
	}).To_Document()
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err = persist.Default.Set(themeUploadKey(userID, document.GetId()), string(raw)); err != nil {
		t.Fatal(err)
	}

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}
	inputDocument := mtproto.MakeTLInputDocument(&mtproto.InputDocument{
		Id:         document.GetId(),
		AccessHash: document.GetAccessHash(),
	}).To_InputDocument()
	created, err := c.AccountCreateTheme(&mtproto.TLAccountCreateTheme{
		Slug:     "owned-theme",
		Title:    "Owned theme",
		Document: inputDocument,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.GetId() != document.GetId() || created.GetDocument().GetAccessHash() != document.GetAccessHash() {
		t.Fatalf("created theme did not retain the uploaded document: %+v", created)
	}

	updated, err := c.AccountUpdateTheme(&mtproto.TLAccountUpdateTheme{
		Theme: mtproto.MakeTLInputTheme(&mtproto.InputTheme{
			Id:         created.GetId(),
			AccessHash: created.GetAccessHash(),
		}).To_InputTheme(),
		Title: wrapperspb.String("Renamed theme"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.GetTitle() != "Renamed theme" || updated.GetDocument().GetId() != document.GetId() {
		t.Fatalf("updated theme lost persisted data: %+v", updated)
	}

	if _, err = c.AccountCreateTheme(&mtproto.TLAccountCreateTheme{
		Slug:     "unowned-theme",
		Title:    "Unowned theme",
		Document: mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: document.GetId() + 99, AccessHash: document.GetAccessHash()}).To_InputDocument(),
	}); err != mtproto.ErrDocumentInvalid {
		t.Fatalf("unowned document error = %v, want DOCUMENT_INVALID", err)
	}
}
