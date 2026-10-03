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

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// RPCTsfServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

type tsfEntity struct {
	Type     string `json:"t,omitempty"`
	Offset   int32  `json:"o"`
	Length   int32  `json:"l"`
	URL      string `json:"u,omitempty"`
	Language string `json:"g,omitempty"`
}

type tsfInfo struct {
	Message  string      `json:"message"`
	Entities []tsfEntity `json:"entities,omitempty"`
}

func tsfInfoUser(auth int64, user *mtproto.InputUser) int64 {
	if user != nil && user.GetUserId() != 0 {
		return user.GetUserId()
	}
	return auth
}

func loadTsfInfo(raw string) tsfInfo {
	var info tsfInfo
	if raw == "" {
		return info
	}
	if raw[0] != '{' {
		info.Message = raw
		return info
	}
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		info.Message = raw
		info.Entities = nil
	}
	return info
}

func tsfEntities(in []*mtproto.MessageEntity) []tsfEntity {
	if len(in) == 0 {
		return nil
	}
	out := make([]tsfEntity, 0, len(in))
	for _, e := range in {
		if e == nil {
			continue
		}
		out = append(out, tsfEntity{
			Type:     e.GetPredicateName(),
			Offset:   e.GetOffset(),
			Length:   e.GetLength(),
			URL:      e.GetUrl(),
			Language: e.GetLanguage(),
		})
	}
	return out
}

func tsfToProto(info tsfInfo) *mtproto.Help_UserInfo {
	ents := make([]*mtproto.MessageEntity, 0, len(info.Entities))
	for _, e := range info.Entities {
		ents = append(ents, &mtproto.MessageEntity{
			PredicateName: e.Type,
			Offset:        e.Offset,
			Length:        e.Length,
			Url:           e.URL,
			Language:      e.Language,
		})
	}
	return mtproto.MakeTLHelpUserInfo(&mtproto.Help_UserInfo{
		Message:  info.Message,
		Entities: ents,
	}).To_Help_UserInfo()
}

func (c *ApiFullCore) HelpGetUserInfo(in *mtproto.TLHelpGetUserInfo) (*mtproto.Help_UserInfo, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var user *mtproto.InputUser
	if in != nil {
		user = in.GetUserId()
	}
	raw, err := persist.Default.Get(b18Key(tsfInfoUser(uid, user), "info"))
	if err != nil {
		return nil, err
	}
	return tsfToProto(loadTsfInfo(raw)), nil
}

func (c *ApiFullCore) HelpEditUserInfo(in *mtproto.TLHelpEditUserInfo) (*mtproto.Help_UserInfo, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err = smsPut(uid, "editUserInfo", in); err != nil {
		return nil, err
	}
	info := tsfInfo{}
	var user *mtproto.InputUser
	if in != nil {
		info.Message = in.GetMessage()
		info.Entities = tsfEntities(in.GetEntities())
		user = in.GetUserId()
	}
	b, err := json.Marshal(info)
	if err != nil {
		return nil, err
	}
	if err = persist.Default.Set(b18Key(tsfInfoUser(uid, user), "info"), string(b)); err != nil {
		return nil, err
	}
	return tsfToProto(info), nil
}
