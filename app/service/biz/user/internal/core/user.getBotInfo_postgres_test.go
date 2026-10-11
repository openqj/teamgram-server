package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	userdao "github.com/teamgram/teamgram-server/app/service/biz/user/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	media_client "github.com/teamgram/teamgram-server/app/service/media/client"
	"github.com/teamgram/teamgram-server/app/service/media/media"
)

func TestUserGetBotInfoPostgresRoundTrip(t *testing.T) {
	pg := userPostgresTest(t, 2)
	ctx := context.Background()
	botID := time.Now().UnixNano()
	insertUserPostgresFixtures(t, pg, botID)
	if _, err := pg.Pool.Exec(ctx, `UPDATE users SET user_type=$1,is_bot=TRUE WHERE id=$2`, user.UserTypeBot, botID); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO bots(bot_id,description,has_menu_button,menu_button_text,menu_button_url,has_preview_medias) VALUES($1,$2,TRUE,$3,$4,TRUE)`, botID, "Bot description", "Launch", "https://example.com/start"); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO bot_commands(bot_id,command,description) VALUES($1,'start','Start'),($1,'help','Help')`, botID); err != nil {
		t.Fatal(err)
	}

	c := New(ctx, &svc.ServiceContext{Dao: &userdao.Dao{Postgres: pg}})
	got, err := c.UserGetBotInfo(&user.TLUserGetBotInfo{BotId: botID})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.GetUserId_INT64() != botID || got.GetDescription_STRING() != "Bot description" || !got.GetHasPreviewMedias() {
		t.Fatalf("bot info = %#v, want PostgreSQL bot fields", got)
	}
	if got.GetMenuButton() == nil || got.GetMenuButton().GetText() != "Launch" || got.GetMenuButton().GetUrl() != "https://example.com/start" {
		t.Fatalf("menu button = %#v, want persisted button", got.GetMenuButton())
	}
	commands := got.GetCommands()
	if len(commands) != 2 || commands[0].GetCommand() != "start" || commands[1].GetCommand() != "help" {
		t.Fatalf("commands = %#v, want stable PostgreSQL id order", commands)
	}

	if _, err := pg.Pool.Exec(ctx, `UPDATE bots SET description_photo_id=77 WHERE bot_id=$1`, botID); err != nil {
		t.Fatal(err)
	}
	if got, err := c.UserGetBotInfo(&user.TLUserGetBotInfo{BotId: botID}); got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("missing media provider = (%v, %v), want fail-closed METHOD_NOT_IMPL", got, err)
	}
	if got, err := c.UserGetBotInfoV2(&user.TLUserGetBotInfoV2{BotId: botID}); got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("missing media provider v2 = (%v, %v), want fail-closed METHOD_NOT_IMPL", got, err)
	}
	mediaErr := errors.New("media provider unavailable")
	c.svcCtx.Dao.MediaClient = &failingBotInfoMediaClient{err: mediaErr}
	if got, err := c.UserGetBotInfo(&user.TLUserGetBotInfo{BotId: botID}); got != nil || !errors.Is(err, mediaErr) {
		t.Fatalf("media provider error = (%v, %v), want propagated provider error", got, err)
	}
	if got, err := c.UserGetBotInfoV2(&user.TLUserGetBotInfoV2{BotId: botID}); got != nil || !errors.Is(err, mediaErr) {
		t.Fatalf("media provider error v2 = (%v, %v), want propagated provider error", got, err)
	}
	if _, err := pg.Pool.Exec(ctx, `UPDATE bots SET description_photo_id=0,description_document_id=88 WHERE bot_id=$1`, botID); err != nil {
		t.Fatal(err)
	}
	document := &mtproto.Document{Id: 88}
	c.svcCtx.Dao.MediaClient = &failingBotInfoMediaClient{document: document}
	if got, err := c.UserGetBotInfo(&user.TLUserGetBotInfo{BotId: botID}); err != nil || got == nil || got.GetDescriptionDocument() != document {
		t.Fatalf("document media = (%v, %v), want hydrated document", got, err)
	}
	if got, err := c.UserGetBotInfoV2(&user.TLUserGetBotInfoV2{BotId: botID}); err != nil || got == nil || got.GetBotInfo().GetDescriptionDocument() != document {
		t.Fatalf("document media v2 = (%v, %v), want hydrated document", got, err)
	}

	if _, err := c.UserGetBotInfo(&user.TLUserGetBotInfo{BotId: botID + 1}); err == nil {
		t.Fatal("missing bot unexpectedly returned info")
	}
}

type failingBotInfoMediaClient struct {
	media_client.MediaClient
	err      error
	document *mtproto.Document
}

func (c *failingBotInfoMediaClient) MediaGetPhoto(context.Context, *media.TLMediaGetPhoto) (*mtproto.Photo, error) {
	return nil, c.err
}

func (c *failingBotInfoMediaClient) MediaGetDocument(context.Context, *media.TLMediaGetDocument) (*mtproto.Document, error) {
	return c.document, c.err
}
