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
	"hash/fnv"
	"net/url"
	"strconv"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RPCSeamlessServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func acctKey(userID int64, key string) string {
	return "acct:" + strconv.FormatInt(userID, 10) + ":" + key
}

func (c *ApiFullCore) AccountGetWebAuthorizations(in *mtproto.TLAccountGetWebAuthorizations) (*mtproto.Account_WebAuthorizations, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	_ = in
	st, err := loadAcctWebState(userID)
	if err != nil {
		return nil, err
	}
	items := st.Items
	if len(items) == 0 && len(st.Hashes) > 0 {
		items = make([]webAuthItem, 0, len(st.Hashes))
		for _, h := range st.Hashes {
			items = append(items, webAuthItem{Hash: h})
		}
	}
	auths := make([]*mtproto.WebAuthorization, 0, len(items))
	users := make([]*mtproto.User, 0, len(items))
	seen := map[int64]struct{}{}
	for _, item := range items {
		auths = append(auths, mtproto.MakeTLWebAuthorization(&mtproto.WebAuthorization{
			Hash:        item.Hash,
			BotId:       item.BotID,
			Domain:      item.Domain,
			Browser:     item.Browser,
			Platform:    item.Platform,
			DateCreated: item.DateCreated,
			DateActive:  item.DateActive,
			Ip:          item.IP,
			Region:      item.Region,
		}).To_WebAuthorization())
		if item.BotID != 0 {
			if _, ok := seen[item.BotID]; !ok {
				seen[item.BotID] = struct{}{}
				users = append(users, mtproto.MakeTLUser(&mtproto.User{Id: item.BotID}).To_User())
			}
		}
	}
	return mtproto.MakeTLAccountWebAuthorizations(&mtproto.Account_WebAuthorizations{
		Authorizations: auths,
		Users:          users,
	}).To_Account_WebAuthorizations(), nil
}

func (c *ApiFullCore) AccountResetWebAuthorization(in *mtproto.TLAccountResetWebAuthorization) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var hash int64
	if in != nil {
		hash = in.GetHash()
	}
	st, err := loadAcctWebState(userID)
	if err != nil {
		return nil, err
	}
	keptH := st.Hashes[:0]
	for _, h := range st.Hashes {
		if h != hash {
			keptH = append(keptH, h)
		}
	}
	keptI := st.Items[:0]
	for _, item := range st.Items {
		if item.Hash != hash {
			keptI = append(keptI, item)
		}
	}
	st.Hashes = keptH
	st.Items = keptI
	if err := saveAcctWebState(userID, st); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) AccountResetWebAuthorizations(in *mtproto.TLAccountResetWebAuthorizations) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	_ = in
	if err := saveAcctWebState(userID, acctWeb{}); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) MessagesRequestUrlAuth(in *mtproto.TLMessagesRequestUrlAuth) (*mtproto.UrlAuthResult, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	urlValue := ""
	if in != nil && in.GetUrl() != nil {
		urlValue = in.GetUrl().GetValue()
	}
	if err := acctPut(acctKey(userID, "urlauth"), acctURL{URL: urlValue, Status: "request"}); err != nil {
		return nil, err
	}
	return mtproto.MakeTLUrlAuthResultDefault(&mtproto.UrlAuthResult{}).To_UrlAuthResult(), nil
}

func (c *ApiFullCore) MessagesAcceptUrlAuth(in *mtproto.TLMessagesAcceptUrlAuth) (*mtproto.UrlAuthResult, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	urlValue := ""
	matchCode := ""
	var botID int64
	if in != nil {
		if in.GetUrl() != nil {
			urlValue = in.GetUrl().GetValue()
		}
		if in.GetMatchCode() != nil {
			matchCode = in.GetMatchCode().GetValue()
		}
		botID = seamlessBotID(in.GetPeer())
	}
	if err := acctPut(acctKey(userID, "urlauth"), acctURL{URL: urlValue, Status: "accepted", MatchCode: matchCode}); err != nil {
		return nil, err
	}
	if urlValue != "" || matchCode != "" || botID != 0 {
		st, err := loadAcctWebState(userID)
		if err != nil {
			return nil, err
		}
		item := webAuthItem{
			Hash:        seamlessURLHash(urlValue),
			BotID:       botID,
			Domain:      seamlessDomain(urlValue),
			DateCreated: int32(time.Now().Unix()),
			DateActive:  int32(time.Now().Unix()),
			URL:         urlValue,
			MatchCode:   matchCode,
		}
		replaced := false
		for i := range st.Items {
			if st.Items[i].Hash == item.Hash {
				if st.Items[i].DateCreated != 0 {
					item.DateCreated = st.Items[i].DateCreated
				}
				st.Items[i] = item
				replaced = true
				break
			}
		}
		if !replaced {
			st.Items = append(st.Items, item)
		}
		found := false
		for _, h := range st.Hashes {
			if h == item.Hash {
				found = true
				break
			}
		}
		if !found {
			st.Hashes = append(st.Hashes, item.Hash)
		}
		if err = saveAcctWebState(userID, st); err != nil {
			return nil, err
		}
	}
	return mtproto.MakeTLUrlAuthResultAccepted(&mtproto.UrlAuthResult{
		Url_FLAGSTRING: wrapperspb.String(urlValue),
		Url_STRING:     urlValue,
	}).To_UrlAuthResult(), nil
}

func (c *ApiFullCore) MessagesDeclineUrlAuth(in *mtproto.TLMessagesDeclineUrlAuth) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	urlValue := ""
	if in != nil {
		urlValue = in.GetUrl()
	}
	if err := acctPut(acctKey(userID, "urlauth"), acctURL{URL: urlValue, Status: "declined"}); err != nil {
		return nil, err
	}
	if urlValue != "" {
		st, err := loadAcctWebState(userID)
		if err != nil {
			return nil, err
		}
		hash := seamlessURLHash(urlValue)
		keptH := st.Hashes[:0]
		for _, h := range st.Hashes {
			if h != hash {
				keptH = append(keptH, h)
			}
		}
		keptI := st.Items[:0]
		for _, item := range st.Items {
			if item.Hash != hash && item.URL != urlValue {
				keptI = append(keptI, item)
			}
		}
		st.Hashes = keptH
		st.Items = keptI
		if err = saveAcctWebState(userID, st); err != nil {
			return nil, err
		}
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) MessagesCheckUrlAuthMatchCode(in *mtproto.TLMessagesCheckUrlAuthMatchCode) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	code := ""
	rawURL := ""
	if in != nil {
		code = in.GetMatchCode()
		rawURL = in.GetUrl()
	}
	if code == "" {
		return mtproto.BoolFalse, nil
	}
	stored, err := loadAcctURL(userID)
	if err != nil {
		return nil, err
	}
	if stored.MatchCode != "" && stored.MatchCode == code && (rawURL == "" || stored.URL == "" || stored.URL == rawURL) {
		return mtproto.BoolTrue, nil
	}
	st, err := loadAcctWebState(userID)
	if err != nil {
		return nil, err
	}
	for _, item := range st.Items {
		if item.MatchCode == "" || item.MatchCode != code {
			continue
		}
		if rawURL == "" || item.URL == rawURL {
			return mtproto.BoolTrue, nil
		}
	}
	return mtproto.BoolFalse, nil
}

type acctURL struct {
	URL       string `json:"url,omitempty"`
	Status    string `json:"status"`
	MatchCode string `json:"match_code,omitempty"`
}

type webAuthItem struct {
	Hash        int64  `json:"hash"`
	BotID       int64  `json:"bot_id,omitempty"`
	Domain      string `json:"domain,omitempty"`
	Browser     string `json:"browser,omitempty"`
	Platform    string `json:"platform,omitempty"`
	DateCreated int32  `json:"date_created,omitempty"`
	DateActive  int32  `json:"date_active,omitempty"`
	IP          string `json:"ip,omitempty"`
	Region      string `json:"region,omitempty"`
	URL         string `json:"url,omitempty"`
	MatchCode   string `json:"match_code,omitempty"`
}

type acctWeb struct {
	Hashes []int64       `json:"hashes"`
	Items  []webAuthItem `json:"items,omitempty"`
}

func loadAcctURL(userID int64) (acctURL, error) {
	var st acctURL
	raw, err := persist.Default.Get(acctKey(userID, "urlauth"))
	if err != nil || raw == "" {
		return st, err
	}
	err = json.Unmarshal([]byte(raw), &st)
	return st, err
}

func loadAcctWebState(userID int64) (acctWeb, error) {
	var st acctWeb
	raw, err := persist.Default.Get(acctKey(userID, "web"))
	if err != nil || raw == "" {
		return st, err
	}
	err = json.Unmarshal([]byte(raw), &st)
	return st, err
}

func saveAcctWebState(userID int64, st acctWeb) error {
	if st.Hashes == nil {
		st.Hashes = []int64{}
	}
	return acctPut(acctKey(userID, "web"), st)
}

func seamlessURLHash(raw string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(raw))
	v := int64(h.Sum64() & 0x7fffffffffffffff)
	if v == 0 {
		v = 1
	}
	return v
}

func seamlessDomain(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return u.Host
}

func seamlessBotID(p *mtproto.InputPeer) int64 {
	if p == nil {
		return 0
	}
	if p.GetUserId() != 0 {
		return p.GetUserId()
	}
	if p.GetChannelId() != 0 {
		return p.GetChannelId()
	}
	return p.GetChatId()
}
