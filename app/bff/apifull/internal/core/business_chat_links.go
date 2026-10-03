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
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RPCBusinessChatLinksServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

type chatLinkJSON struct {
	Slug     string                   `json:"slug"`
	Message  string                   `json:"message"`
	Title    string                   `json:"title"`
	Entities []*mtproto.MessageEntity `json:"entities,omitempty"`
}

type chatLinkSlugJSON struct {
	UserId   int64                    `json:"user_id"`
	Message  string                   `json:"message"`
	Title    string                   `json:"title"`
	Entities []*mtproto.MessageEntity `json:"entities,omitempty"`
}

func chatLinksKey(userId int64) string {
	return fmt.Sprintf("blink:%d", userId)
}

func loadChatLinks(userId int64) ([]chatLinkJSON, error) {
	raw, err := persist.Default.Get(chatLinksKey(userId))
	if err != nil || raw == "" {
		return nil, err
	}
	var list []chatLinkJSON
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	return list, nil
}

func saveChatLinks(userId int64, list []chatLinkJSON) error {
	if list == nil {
		list = []chatLinkJSON{}
	}
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return persist.Default.Set(chatLinksKey(userId), string(b))
}

func chatLinkFromInput(slug string, in *mtproto.InputBusinessChatLink) chatLinkJSON {
	out := chatLinkJSON{Slug: slug}
	if in == nil {
		return out
	}
	out.Message = in.GetMessage()
	if t := in.GetTitle(); t != nil {
		out.Title = t.GetValue()
	}
	if len(in.GetEntities()) > 0 {
		out.Entities = in.GetEntities()
	}
	return out
}

func businessChatLink(row chatLinkJSON) *mtproto.BusinessChatLink {
	ents := row.Entities
	if ents == nil {
		ents = []*mtproto.MessageEntity{}
	}
	out := &mtproto.BusinessChatLink{
		Link:     row.Slug,
		Message:  row.Message,
		Entities: ents,
	}
	if row.Title != "" {
		out.Title = wrapperspb.String(row.Title)
	}
	return mtproto.MakeTLBusinessChatLink(out).To_BusinessChatLink()
}

func chatLinkSlugKey(slug string) string {
	return "blink:slug:" + slug
}

func indexChatLink(userId int64, row chatLinkJSON) error {
	b, err := json.Marshal(chatLinkSlugJSON{
		UserId:   userId,
		Message:  row.Message,
		Title:    row.Title,
		Entities: row.Entities,
	})
	if err != nil {
		return err
	}
	return persist.Default.Set(chatLinkSlugKey(row.Slug), string(b))
}

func clearChatLinkSlug(slug string) error {
	if slug == "" {
		return nil
	}
	return persist.Default.Set(chatLinkSlugKey(slug), "")
}

func (c *ApiFullCore) AccountCreateBusinessChatLink(in *mtproto.TLAccountCreateBusinessChatLink) (*mtproto.BusinessChatLink, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	list, err := loadChatLinks(c.MD.UserId)
	if err != nil {
		return nil, err
	}
	var link *mtproto.InputBusinessChatLink
	if in != nil {
		link = in.GetLink()
	}
	row := chatLinkFromInput(fmt.Sprintf("%d-%d", c.MD.UserId, len(list)+1), link)
	list = append(list, row)
	if err := saveChatLinks(c.MD.UserId, list); err != nil {
		return nil, err
	}
	if err := indexChatLink(c.MD.UserId, row); err != nil {
		return nil, err
	}
	return businessChatLink(row), nil
}

func (c *ApiFullCore) AccountEditBusinessChatLink(in *mtproto.TLAccountEditBusinessChatLink) (*mtproto.BusinessChatLink, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	list, err := loadChatLinks(c.MD.UserId)
	if err != nil {
		return nil, err
	}
	slug := ""
	var link *mtproto.InputBusinessChatLink
	if in != nil {
		slug = in.GetSlug()
		link = in.GetLink()
	}
	row := chatLinkFromInput(slug, link)
	found := false
	for i := range list {
		if list[i].Slug == slug {
			list[i] = row
			found = true
			break
		}
	}
	if !found {
		list = append(list, row)
	}
	if err := saveChatLinks(c.MD.UserId, list); err != nil {
		return nil, err
	}
	if err := indexChatLink(c.MD.UserId, row); err != nil {
		return nil, err
	}
	return businessChatLink(row), nil
}

func (c *ApiFullCore) AccountDeleteBusinessChatLink(in *mtproto.TLAccountDeleteBusinessChatLink) (*mtproto.Bool, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	list, err := loadChatLinks(c.MD.UserId)
	if err != nil {
		return nil, err
	}
	slug := ""
	if in != nil {
		slug = in.GetSlug()
	}
	next := list[:0]
	removed := false
	for _, row := range list {
		if row.Slug == slug {
			removed = true
			continue
		}
		next = append(next, row)
	}
	if removed {
		if err := clearChatLinkSlug(slug); err != nil {
			return nil, err
		}
	}
	if err := saveChatLinks(c.MD.UserId, next); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) AccountGetBusinessChatLinks(in *mtproto.TLAccountGetBusinessChatLinks) (*mtproto.Account_BusinessChatLinks, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	_ = in
	list, err := loadChatLinks(c.MD.UserId)
	if err != nil {
		return nil, err
	}
	links := make([]*mtproto.BusinessChatLink, 0, len(list))
	for _, row := range list {
		links = append(links, businessChatLink(row))
	}
	return mtproto.MakeTLAccountBusinessChatLinks(&mtproto.Account_BusinessChatLinks{
		Links: links,
		Chats: []*mtproto.Chat{},
		Users: []*mtproto.User{},
	}).To_Account_BusinessChatLinks(), nil
}

func (c *ApiFullCore) AccountResolveBusinessChatLink(in *mtproto.TLAccountResolveBusinessChatLink) (*mtproto.Account_ResolvedBusinessChatLinks, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	slug := ""
	if in != nil {
		slug = in.GetSlug()
	}
	raw, err := persist.Default.Get(chatLinkSlugKey(slug))
	if err != nil {
		return nil, err
	}
	empty := mtproto.MakeTLAccountResolvedBusinessChatLinks(&mtproto.Account_ResolvedBusinessChatLinks{
		Entities: []*mtproto.MessageEntity{},
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}).To_Account_ResolvedBusinessChatLinks()
	if raw == "" {
		return empty, nil
	}
	var row chatLinkSlugJSON
	if err := json.Unmarshal([]byte(raw), &row); err != nil {
		return nil, err
	}
	ents := row.Entities
	if ents == nil {
		ents = []*mtproto.MessageEntity{}
	}
	return mtproto.MakeTLAccountResolvedBusinessChatLinks(&mtproto.Account_ResolvedBusinessChatLinks{
		Peer:     mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: row.UserId}).To_Peer(),
		Message:  row.Message,
		Entities: ents,
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}).To_Account_ResolvedBusinessChatLinks(), nil
}
