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
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RPCPredefinedServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) PredefinedCreatePredefinedUser(in *mtproto.TLPredefinedCreatePredefinedUser) (*mtproto.PredefinedUser, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	u := acctPredefined{}
	if in != nil {
		u.Phone = in.GetPhone()
		u.Code = in.GetCode()
		u.Verified = in.GetVerified()
		u.FirstName = wrapStr(in.GetFirstName())
		u.LastName = wrapStr(in.GetLastName())
		u.Username = wrapStr(in.GetUsername())
	}
	return savePredef(u)
}

func (c *ApiFullCore) PredefinedUpdatePredefinedUsername(in *mtproto.TLPredefinedUpdatePredefinedUsername) (*mtproto.PredefinedUser, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	phone, username := "", ""
	if in != nil {
		phone = in.GetPhone()
		username = in.GetUsername()
	}
	u, err := loadPredef(phone)
	if err != nil {
		return nil, err
	}
	u.Username = username
	return savePredef(u)
}

func (c *ApiFullCore) PredefinedUpdatePredefinedProfile(in *mtproto.TLPredefinedUpdatePredefinedProfile) (*mtproto.PredefinedUser, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	phone := ""
	if in != nil {
		phone = in.GetPhone()
	}
	u, err := loadPredef(phone)
	if err != nil {
		return nil, err
	}
	if in != nil {
		if in.GetFirstName() != nil {
			u.FirstName = in.GetFirstName().GetValue()
		}
		if in.GetLastName() != nil {
			u.LastName = in.GetLastName().GetValue()
		}
		if in.GetAbout() != nil {
			u.About = in.GetAbout().GetValue()
		}
	}
	return savePredef(u)
}

func (c *ApiFullCore) PredefinedUpdatePredefinedVerified(in *mtproto.TLPredefinedUpdatePredefinedVerified) (*mtproto.PredefinedUser, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	phone := ""
	verified := false
	if in != nil {
		phone = in.GetPhone()
		verified = in.GetVerified()
	}
	u, err := loadPredef(phone)
	if err != nil {
		return nil, err
	}
	u.Verified = verified
	return savePredef(u)
}

func (c *ApiFullCore) PredefinedUpdatePredefinedCode(in *mtproto.TLPredefinedUpdatePredefinedCode) (*mtproto.PredefinedUser, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	phone, code := "", ""
	if in != nil {
		phone = in.GetPhone()
		code = in.GetCode()
	}
	u, err := loadPredef(phone)
	if err != nil {
		return nil, err
	}
	u.Code = code
	return savePredef(u)
}

func (c *ApiFullCore) PredefinedGetPredefinedUser(in *mtproto.TLPredefinedGetPredefinedUser) (*mtproto.PredefinedUser, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	phone := ""
	if in != nil {
		phone = in.GetPhone()
	}
	raw, err := persist.Default.Get(predefKey(phone))
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return mtproto.MakeTLPredefinedUser(&mtproto.PredefinedUser{}).To_PredefinedUser(), nil
	}
	u, err := loadPredef(phone)
	if err != nil {
		return nil, err
	}
	return predefToUser(u), nil
}

func (c *ApiFullCore) PredefinedGetPredefinedUsers(in *mtproto.TLPredefinedGetPredefinedUsers) (*mtproto.Vector_PredefinedUser, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	_ = in
	phones, err := loadPredefPhones()
	if err != nil {
		return nil, err
	}
	out := make([]*mtproto.PredefinedUser, 0, len(phones))
	for _, phone := range phones {
		raw, err := persist.Default.Get(predefKey(phone))
		if err != nil {
			return nil, err
		}
		if raw == "" {
			continue
		}
		u, err := loadPredef(phone)
		if err != nil {
			return nil, err
		}
		out = append(out, predefToUser(u))
	}
	return &mtproto.Vector_PredefinedUser{Datas: out}, nil
}

type acctPredefined struct {
	Phone     string `json:"phone"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
	Code      string `json:"code,omitempty"`
	Verified  bool   `json:"verified,omitempty"`
	About     string `json:"about,omitempty"`
}

func predefKey(phone string) string {
	return "acct:predef:" + phone
}

func wrapStr(v *wrapperspb.StringValue) string {
	if v == nil {
		return ""
	}
	return v.GetValue()
}

func loadPredef(phone string) (acctPredefined, error) {
	u := acctPredefined{Phone: phone}
	raw, err := persist.Default.Get(predefKey(phone))
	if err != nil || raw == "" {
		return u, err
	}
	if err := json.Unmarshal([]byte(raw), &u); err != nil {
		return u, err
	}
	if u.Phone == "" {
		u.Phone = phone
	}
	return u, nil
}

func predefIndexKey() string {
	return "acct:predef:index"
}

func loadPredefPhones() ([]string, error) {
	raw, err := persist.Default.Get(predefIndexKey())
	if err != nil || raw == "" {
		return nil, err
	}
	var phones []string
	if err = json.Unmarshal([]byte(raw), &phones); err != nil {
		return nil, err
	}
	return phones, nil
}

func rememberPredefPhone(phone string) error {
	if phone == "" {
		return nil
	}
	phones, err := loadPredefPhones()
	if err != nil {
		return err
	}
	for _, p := range phones {
		if p == phone {
			return nil
		}
	}
	phones = append(phones, phone)
	b, err := json.Marshal(phones)
	if err != nil {
		return err
	}
	return persist.Default.Set(predefIndexKey(), string(b))
}

func predefToUser(u acctPredefined) *mtproto.PredefinedUser {
	out := &mtproto.PredefinedUser{Phone: u.Phone, Code: u.Code, Verified: u.Verified}
	if u.FirstName != "" {
		out.FirstName = wrapperspb.String(u.FirstName)
	}
	if u.LastName != "" {
		out.LastName = wrapperspb.String(u.LastName)
	}
	if u.Username != "" {
		out.Username = wrapperspb.String(u.Username)
	}
	return mtproto.MakeTLPredefinedUser(out).To_PredefinedUser()
}

func savePredef(u acctPredefined) (*mtproto.PredefinedUser, error) {
	if err := acctPut(predefKey(u.Phone), u); err != nil {
		return nil, err
	}
	if err := rememberPredefPhone(u.Phone); err != nil {
		return nil, err
	}
	return predefToUser(u), nil
}
