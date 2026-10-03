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
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RPCProfileLinksServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func b18Key(userID int64, part string) string {
	return fmt.Sprintf("b18:%d:%s", userID, part)
}

const contactTokenTTL = 24 * time.Hour

type contactTokenRecord struct {
	UserID  int64 `json:"user_id"`
	Expires int64 `json:"expires"`
}

func contactTokenKey(token string) string {
	return "contact-token:" + token
}

func contactTokenURL(token string) string {
	return "https://t.me/+" + token
}

func contactTokenUser(data *mtproto.UserData, mutual bool) *mtproto.User {
	u := &mtproto.User{
		Id:            data.GetId(),
		Contact:       true,
		MutualContact: mutual,
		Verified:      data.GetVerified(),
		Support:       data.GetSupport(),
		Scam:          data.GetScam(),
		Fake:          data.GetFake(),
		Premium:       data.GetPremium(),
		Restricted:    data.GetRestricted(),
	}
	if data.GetAccessHash() != 0 {
		u.AccessHash = wrapperspb.Int64(data.GetAccessHash())
	}
	u.FirstName = mtproto.MakeFlagsString(data.GetFirstName())
	u.LastName = mtproto.MakeFlagsString(data.GetLastName())
	u.Username = mtproto.MakeFlagsString(data.GetUsername())
	u.Phone = mtproto.MakeFlagsString(data.GetPhone())
	return mtproto.MakeTLUser(u).To_User()
}

func (c *ApiFullCore) ContactsExportContactToken(in *mtproto.TLContactsExportContactToken) (*mtproto.ExportedContactToken, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	d := c.apifullDao()
	if d == nil || d.UserClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	owner, err := d.UserClient.UserGetImmutableUserV2(ctx, &userpb.TLUserGetImmutableUserV2{Id: uid})
	if err != nil {
		return nil, err
	}
	if owner == nil || owner.GetUser() == nil || owner.GetUser().GetId() != uid || owner.GetUser().GetDeleted() {
		return nil, mtproto.ErrUserIdInvalid
	}
	buf := make([]byte, 24)
	if _, err = rand.Read(buf); err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	expires := time.Now().Add(contactTokenTTL).Unix()
	raw, err := json.Marshal(contactTokenRecord{UserID: uid, Expires: expires})
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	if err = persist.Default.Set(contactTokenKey(token), string(raw)); err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	return mtproto.MakeTLExportedContactToken(&mtproto.ExportedContactToken{
		Url:     contactTokenURL(token),
		Expires: int32(expires),
	}).To_ExportedContactToken(), nil
}

func (c *ApiFullCore) ContactsImportContactToken(in *mtproto.TLContactsImportContactToken) (*mtproto.User, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetToken() == "" || len(in.GetToken()) > 128 {
		return nil, mtproto.ErrTokenInvalid
	}
	raw, err := persist.Default.Get(contactTokenKey(in.GetToken()))
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	if raw == "" {
		return nil, mtproto.ErrTokenInvalid
	}
	var token contactTokenRecord
	if err = json.Unmarshal([]byte(raw), &token); err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	if token.UserID <= 0 || token.Expires <= time.Now().Unix() || token.UserID == uid {
		return nil, mtproto.ErrTokenInvalid
	}
	d := c.apifullDao()
	if d == nil || d.UserClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	owner, err := d.UserClient.UserGetImmutableUserV2(ctx, &userpb.TLUserGetImmutableUserV2{
		Id:      token.UserID,
		Privacy: true,
		HasTo:   true,
		To:      []int64{uid},
	})
	if err != nil {
		if err == mtproto.ErrUserIdInvalid {
			return nil, mtproto.ErrTokenInvalid
		}
		return nil, err
	}
	if owner == nil || owner.GetUser() == nil || owner.GetUser().GetId() != token.UserID || owner.GetUser().GetDeleted() {
		return nil, mtproto.ErrTokenInvalid
	}
	profile := owner.GetUser()
	mutual, err := d.UserClient.UserAddContact(ctx, &userpb.TLUserAddContact{
		UserId:    uid,
		Id:        token.UserID,
		FirstName: profile.GetFirstName(),
		LastName:  profile.GetLastName(),
		Phone:     profile.GetPhone(),
	})
	if err != nil {
		return nil, err
	}
	return contactTokenUser(profile, mtproto.FromBool(mutual)), nil
}
