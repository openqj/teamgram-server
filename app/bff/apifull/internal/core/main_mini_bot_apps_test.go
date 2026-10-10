// Copyright 2026 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type previewBotUserClient struct {
	user_client.UserClient
	profile *mtproto.ImmutableUser
}

func (c *previewBotUserClient) UserGetImmutableUser(_ context.Context, _ *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	return c.profile, nil
}

func previewCore(uid, accessHash int64) *ApiFullCore {
	return &ApiFullCore{
		MD: &metadata.RpcMetadata{UserId: uid},
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: &previewBotUserClient{
			profile: mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{
				Id: uid, AccessHash: accessHash,
				Bot: mtproto.MakeTLBotData(&mtproto.BotData{Id: uid}).To_BotData(),
			}}).To_ImmutableUser(),
		}}},
	}
}

func previewBotInput(uid, accessHash int64) *mtproto.InputUser {
	return mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: uid, AccessHash: accessHash}).To_InputUser()
}

func TestPreviewMethodsRequireBotAndMedia(t *testing.T) {
	const uid = int64(81018)
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	if _, err := c.BotsAddPreviewMedia(&mtproto.TLBotsAddPreviewMedia{}); !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("missing bot = %v, want USER_ID_INVALID", err)
	}
	c = previewCore(uid, uid+1)
	if _, err := c.BotsAddPreviewMedia(&mtproto.TLBotsAddPreviewMedia{Bot: previewBotInput(uid, uid+1)}); !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("missing media = %v, want INPUT_REQUEST_INVALID", err)
	}
	if _, err := c.BotsAddPreviewMedia(&mtproto.TLBotsAddPreviewMedia{
		Bot: previewBotInput(uid, uid+1), Media: mtproto.MakeTLInputMediaWebPage(&mtproto.InputMedia{Url: "http://insecure.example"}).To_InputMedia(),
	}); !errors.Is(err, mtproto.ErrWebdocumentUrlInvalid) {
		t.Fatalf("insecure media = %v, want WEBDOCUMENT_URL_INVALID", err)
	}
	photo := mtproto.MakeTLInputMediaPhoto(&mtproto.InputMedia{Id_INPUTPHOTO: mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{Id: 7, AccessHash: 8}).To_InputPhoto()}).To_InputMedia()
	if _, err := c.BotsAddPreviewMedia(&mtproto.TLBotsAddPreviewMedia{Bot: previewBotInput(uid, uid+1), Media: photo}); !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("unhydrated photo = %v, want METHOD_NOT_IMPL", err)
	}
}

func TestMainMiniBotAppsLangRoundTrip(t *testing.T) {
	const uid, accessHash = int64(81008), int64(81009)
	c := previewCore(uid, accessHash)
	t.Cleanup(func() {
		_ = persist.Default.Set(previewMediaKey(uid), "")
		_ = persist.Default.Set("b8:"+strconv.FormatInt(uid, 10)+":", "")
	})
	const want = "en"
	if _, err := c.BotsAddPreviewMedia(&mtproto.TLBotsAddPreviewMedia{Bot: previewBotInput(uid, accessHash), LangCode: want, Media: mtproto.MakeTLInputMediaWebPage(&mtproto.InputMedia{Url: "https://preview.example.test/lang"}).To_InputMedia()}); err != nil {
		t.Fatal(err)
	}
	got, err := c.BotsGetPreviewInfo(&mtproto.TLBotsGetPreviewInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GetLangCodes()) != 1 || got.GetLangCodes()[0] != want {
		t.Fatalf("lang %v", got.GetLangCodes())
	}
}

func TestBotPreviewMediaPostgresRoundTrip(t *testing.T) {
	if os.Getenv("APIFULL_POSTGRES_DSN") == "" || !persist.PostgresEnabled() {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	uid := int64(930000000 + os.Getpid()%100000)
	accessHash := uid + 100
	c := previewCore(uid, accessHash)
	bot := previewBotInput(uid, accessHash)
	media := mtproto.MakeTLInputMediaWebPage(&mtproto.InputMedia{Url: "https://preview.example.test/old"}).To_InputMedia()
	updated := mtproto.MakeTLInputMediaWebPage(&mtproto.InputMedia{Url: "https://preview.example.test/new"}).To_InputMedia()
	t.Cleanup(func() {
		_ = persist.Default.Set(previewMediaKey(uid), "")
		_ = persist.Default.Set("b8:"+strconv.FormatInt(uid, 10)+":", "")
	})
	if _, err := c.BotsAddPreviewMedia(&mtproto.TLBotsAddPreviewMedia{Bot: bot, LangCode: "en", Media: media}); err != nil {
		t.Fatal(err)
	}
	got, err := c.BotsGetPreviewInfo(&mtproto.TLBotsGetPreviewInfo{LangCode: "en"})
	if err != nil || len(got.GetMedia()) != 1 || got.GetMedia()[0].GetMedia().GetWebpage().GetUrl_STRING() != "https://preview.example.test/old" {
		t.Fatalf("added preview = %#v, err=%v", got, err)
	}
	if _, err = c.BotsEditPreviewMedia(&mtproto.TLBotsEditPreviewMedia{Bot: bot, LangCode: "en", Media: media, NewMedia: updated}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.BotsReorderPreviewMedias(&mtproto.TLBotsReorderPreviewMedias{Bot: bot, LangCode: "en", Order: []*mtproto.InputMedia{updated}}); err != nil {
		t.Fatal(err)
	}
	all, err := c.BotsGetPreviewMedias(&mtproto.TLBotsGetPreviewMedias{})
	if err != nil || len(all.GetDatas()) != 1 {
		t.Fatalf("reordered previews = %#v, err=%v", all, err)
	}
	if _, err = c.BotsDeletePreviewMedia(&mtproto.TLBotsDeletePreviewMedia{Bot: bot, LangCode: "en", Media: []*mtproto.InputMedia{updated}}); err != nil {
		t.Fatal(err)
	}
	got, err = c.BotsGetPreviewInfo(&mtproto.TLBotsGetPreviewInfo{LangCode: "en"})
	if err != nil || len(got.GetMedia()) != 0 {
		t.Fatalf("deleted previews = %#v, err=%v", got, err)
	}
}
