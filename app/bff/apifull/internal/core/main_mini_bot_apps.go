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
	"encoding/json"
	"strconv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// RPCMainMiniBotAppsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) MessagesRequestMainWebView(in *mtproto.TLMessagesRequestMainWebView) (*mtproto.WebViewResult, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in == nil || in.GetBot() == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	botID, err := miniBotID(in.GetBot())
	if err != nil {
		return nil, err
	}
	botInfo, err := c.svcCtx.Dao.UserGetBotInfoV2(c.ctx, &userpb.TLUserGetBotInfoV2{BotId: botID})
	if err != nil {
		return nil, err
	}
	if botInfo == nil || botInfo.GetMainAppUrl() == nil || botInfo.GetMainAppUrl().GetValue() == "" {
		return nil, mtproto.ErrMethodNotImpl
	}
	return c.requestMiniWebView(uid, in.GetBot(), botInfo.GetMainAppUrl().GetValue(), "requestMainWebView", in.GetPlatform(), !in.GetCompact(), in.GetFullscreen(), in)
}

func (c *ApiFullCore) BotsGetPopularAppBots(in *mtproto.TLBotsGetPopularAppBots) (*mtproto.Bots_PopularAppBots, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) BotsAddPreviewMedia(in *mtproto.TLBotsAddPreviewMedia) (*mtproto.BotPreviewMedia, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err = miniAppPut(uid, "bots.addPreviewMedia", in); err != nil {
		return nil, err
	}
	if in != nil && in.GetLangCode() != "" {
		if err = persist.Default.Set("b8:"+strconv.FormatInt(uid, 10)+":", in.GetLangCode()); err != nil {
			return nil, err
		}
	}
	rec := previewRec{}
	if in != nil && in.GetMedia() != nil {
		rec = previewFromInput(in.GetLangCode(), in.GetMedia())
		items, err := loadPreviewMedias(uid)
		if err != nil {
			return nil, err
		}
		if err = savePreviewMedias(uid, append(items, rec)); err != nil {
			return nil, err
		}
	}
	return previewBotMedia(rec), nil
}

func (c *ApiFullCore) BotsEditPreviewMedia(in *mtproto.TLBotsEditPreviewMedia) (*mtproto.BotPreviewMedia, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err = miniAppPut(uid, "bots.editPreviewMedia", in); err != nil {
		return nil, err
	}
	rec := previewRec{}
	if in != nil {
		lang := in.GetLangCode()
		rec = previewFromInput(lang, in.GetNewMedia())
		items, err := loadPreviewMedias(uid)
		if err != nil {
			return nil, err
		}
		oldID := previewFromInput(lang, in.GetMedia()).id()
		replaced := false
		if oldID != "" {
			for i := range items {
				if lang != "" && items[i].Lang != lang {
					continue
				}
				if items[i].id() == oldID {
					if rec.Lang == "" {
						rec.Lang = items[i].Lang
					}
					items[i] = rec
					replaced = true
					break
				}
			}
		}
		if !replaced && in.GetNewMedia() != nil {
			items = append(items, rec)
		}
		if err = savePreviewMedias(uid, items); err != nil {
			return nil, err
		}
	}
	return previewBotMedia(rec), nil
}

func (c *ApiFullCore) BotsDeletePreviewMedia(in *mtproto.TLBotsDeletePreviewMedia) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err = miniAppPut(uid, "bots.deletePreviewMedia", in); err != nil {
		return nil, err
	}
	if in != nil && len(in.GetMedia()) > 0 {
		lang := in.GetLangCode()
		drop := map[string]bool{}
		for _, media := range in.GetMedia() {
			if id := previewFromInput(lang, media).id(); id != "" {
				drop[id] = true
			}
		}
		if len(drop) > 0 {
			items, err := loadPreviewMedias(uid)
			if err != nil {
				return nil, err
			}
			kept := make([]previewRec, 0, len(items))
			for _, item := range items {
				if (lang == "" || item.Lang == lang) && drop[item.id()] {
					continue
				}
				kept = append(kept, item)
			}
			if err = savePreviewMedias(uid, kept); err != nil {
				return nil, err
			}
		}
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) BotsReorderPreviewMedias(in *mtproto.TLBotsReorderPreviewMedias) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err = miniAppPut(uid, "bots.reorderPreviewMedias", in); err != nil {
		return nil, err
	}
	if in != nil && len(in.GetOrder()) > 0 {
		lang := in.GetLangCode()
		order := make([]string, 0, len(in.GetOrder()))
		for _, media := range in.GetOrder() {
			order = append(order, previewFromInput(lang, media).id())
		}
		items, err := loadPreviewMedias(uid)
		if err != nil {
			return nil, err
		}
		if err = savePreviewMedias(uid, reorderPreviewMedias(items, lang, order)); err != nil {
			return nil, err
		}
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) BotsGetPreviewInfo(in *mtproto.TLBotsGetPreviewInfo) (*mtproto.Bots_PreviewInfo, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	lang, err := persist.Default.Get("b8:" + strconv.FormatInt(uid, 10) + ":")
	if err != nil {
		return nil, err
	}
	codes := []string{}
	if lang != "" {
		codes = []string{lang}
	}
	items, err := loadPreviewMedias(uid)
	if err != nil {
		return nil, err
	}
	want := ""
	if in != nil {
		want = in.GetLangCode()
	}
	return mtproto.MakeTLBotsPreviewInfo(&mtproto.Bots_PreviewInfo{
		Media:     previewBotMedias(items, want),
		LangCodes: codes,
	}).To_Bots_PreviewInfo(), nil
}

func (c *ApiFullCore) BotsGetPreviewMedias(in *mtproto.TLBotsGetPreviewMedias) (*mtproto.Vector_BotPreviewMedia, error) {
	_ = in
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	items, err := loadPreviewMedias(uid)
	if err != nil {
		return nil, err
	}
	return &mtproto.Vector_BotPreviewMedia{Datas: previewBotMedias(items, "")}, nil
}

type previewRec struct {
	Lang  string `json:"lang,omitempty"`
	URL   string `json:"url,omitempty"`
	Title string `json:"title,omitempty"`
	Photo int64  `json:"photo,omitempty"`
	Doc   int64  `json:"doc,omitempty"`
}

func previewMediaKey(uid int64) string {
	return "b8:" + strconv.FormatInt(uid, 10) + ":medias"
}

func (r previewRec) id() string {
	switch {
	case r.URL != "":
		return "u:" + r.URL
	case r.Title != "":
		return "t:" + r.Title
	case r.Photo != 0:
		return "p:" + strconv.FormatInt(r.Photo, 10)
	case r.Doc != 0:
		return "d:" + strconv.FormatInt(r.Doc, 10)
	default:
		return ""
	}
}

func loadPreviewMedias(uid int64) ([]previewRec, error) {
	raw, err := persist.Default.Get(previewMediaKey(uid))
	if err != nil || raw == "" || raw == "null" {
		return []previewRec{}, err
	}
	var items []previewRec
	if err = json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, err
	}
	if items == nil {
		items = []previewRec{}
	}
	return items, nil
}

func savePreviewMedias(uid int64, items []previewRec) error {
	if items == nil {
		items = []previewRec{}
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return err
	}
	return persist.Default.Set(previewMediaKey(uid), string(raw))
}

func previewFromInput(lang string, media *mtproto.InputMedia) previewRec {
	rec := previewRec{Lang: lang}
	if media == nil {
		return rec
	}
	rec.URL = media.GetUrl()
	rec.Title = media.GetTitle()
	if photo := media.GetId_INPUTPHOTO(); photo != nil {
		rec.Photo = photo.GetId()
	}
	if doc := media.GetId_INPUTDOCUMENT(); doc != nil {
		rec.Doc = doc.GetId()
	}
	return rec
}

func previewBotMedia(rec previewRec) *mtproto.BotPreviewMedia {
	media := &mtproto.MessageMedia{Title: rec.Title}
	if rec.URL != "" {
		media.Webpage = mtproto.MakeTLWebPage(&mtproto.WebPage{Url_STRING: rec.URL}).To_WebPage()
	}
	if rec.Photo != 0 {
		media.Photo_FLAGPHOTO = mtproto.MakeTLPhoto(&mtproto.Photo{Id: rec.Photo}).To_Photo()
	}
	if rec.Doc != 0 {
		media.Document = mtproto.MakeTLDocument(&mtproto.Document{Id: rec.Doc}).To_Document()
	}
	var out *mtproto.MessageMedia
	switch {
	case rec.Photo != 0:
		out = mtproto.MakeTLMessageMediaPhoto(media).To_MessageMedia()
	case rec.Doc != 0:
		out = mtproto.MakeTLMessageMediaDocument(media).To_MessageMedia()
	case rec.URL != "" || rec.Title != "":
		out = mtproto.MakeTLMessageMediaWebPage(media).To_MessageMedia()
	default:
		out = mtproto.MakeTLMessageMediaEmpty(&mtproto.MessageMedia{}).To_MessageMedia()
	}
	return mtproto.MakeTLBotPreviewMedia(&mtproto.BotPreviewMedia{Media: out}).To_BotPreviewMedia()
}

func previewBotMedias(items []previewRec, lang string) []*mtproto.BotPreviewMedia {
	out := []*mtproto.BotPreviewMedia{}
	for _, item := range items {
		if lang != "" && item.Lang != lang {
			continue
		}
		if item.id() == "" {
			continue
		}
		out = append(out, previewBotMedia(item))
	}
	return out
}

func reorderPreviewMedias(items []previewRec, lang string, order []string) []previewRec {
	subset := make([]int, 0, len(items))
	for i, item := range items {
		if lang != "" && item.Lang != lang {
			continue
		}
		subset = append(subset, i)
	}
	picked := map[int]bool{}
	ordered := make([]previewRec, 0, len(subset))
	for _, id := range order {
		if id == "" {
			continue
		}
		for _, idx := range subset {
			if picked[idx] || items[idx].id() != id {
				continue
			}
			ordered = append(ordered, items[idx])
			picked[idx] = true
			break
		}
	}
	for _, idx := range subset {
		if !picked[idx] {
			ordered = append(ordered, items[idx])
		}
	}
	out := append([]previewRec(nil), items...)
	for n, idx := range subset {
		out[idx] = ordered[n]
	}
	return out
}
