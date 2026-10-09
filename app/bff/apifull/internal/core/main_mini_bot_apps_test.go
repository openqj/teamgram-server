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
	"os"
	"strconv"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestMainMiniBotAppsLangRoundTrip(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81008}}
	const want = "en"
	if _, err := c.BotsAddPreviewMedia(&mtproto.TLBotsAddPreviewMedia{LangCode: want}); err != nil {
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
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	media := mtproto.MakeTLInputMediaWebPage(&mtproto.InputMedia{Url: "https://preview.example.test/old"}).To_InputMedia()
	updated := mtproto.MakeTLInputMediaWebPage(&mtproto.InputMedia{Url: "https://preview.example.test/new"}).To_InputMedia()
	t.Cleanup(func() {
		_ = persist.Default.Set(previewMediaKey(uid), "")
		_ = persist.Default.Set("b8:"+strconv.FormatInt(uid, 10)+":", "")
	})
	if _, err := c.BotsAddPreviewMedia(&mtproto.TLBotsAddPreviewMedia{LangCode: "en", Media: media}); err != nil {
		t.Fatal(err)
	}
	got, err := c.BotsGetPreviewInfo(&mtproto.TLBotsGetPreviewInfo{LangCode: "en"})
	if err != nil || len(got.GetMedia()) != 1 || got.GetMedia()[0].GetMedia().GetWebpage().GetUrl_STRING() != "https://preview.example.test/old" {
		t.Fatalf("added preview = %#v, err=%v", got, err)
	}
	if _, err = c.BotsEditPreviewMedia(&mtproto.TLBotsEditPreviewMedia{LangCode: "en", Media: media, NewMedia: updated}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.BotsReorderPreviewMedias(&mtproto.TLBotsReorderPreviewMedias{LangCode: "en", Order: []*mtproto.InputMedia{updated}}); err != nil {
		t.Fatal(err)
	}
	all, err := c.BotsGetPreviewMedias(&mtproto.TLBotsGetPreviewMedias{})
	if err != nil || len(all.GetDatas()) != 1 {
		t.Fatalf("reordered previews = %#v, err=%v", all, err)
	}
	if _, err = c.BotsDeletePreviewMedia(&mtproto.TLBotsDeletePreviewMedia{LangCode: "en", Media: []*mtproto.InputMedia{updated}}); err != nil {
		t.Fatal(err)
	}
	got, err = c.BotsGetPreviewInfo(&mtproto.TLBotsGetPreviewInfo{LangCode: "en"})
	if err != nil || len(got.GetMedia()) != 0 {
		t.Fatalf("deleted previews = %#v, err=%v", got, err)
	}
}
