// Copyright (c) 2026 The Teamgram Authors (https://teamgram.net).
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

package core

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
)

// webDomainExc is one stored domain exception.
type webDomainExc struct {
	Domain string `json:"domain"`
	URL    string `json:"url"`
	Title  string `json:"title"`
}

// webBrowserState is the blob at webbrowser:<userId>:.
type webBrowserState struct {
	OpenExternalBrowser bool           `json:"open_external_browser"`
	DisplayCloseButton  bool           `json:"display_close_button"`
	External            []webDomainExc `json:"external,omitempty"`
	InApp               []webDomainExc `json:"inapp,omitempty"`
}

func (c *WebBrowserCore) requireWebBrowserUser() (int64, error) {
	if c == nil || c.MD == nil || c.MD.UserId == 0 {
		return 0, mtproto.ErrUserIdInvalid
	}
	return c.MD.UserId, nil
}

func webBrowserKey(userID int64) string {
	return fmt.Sprintf("webbrowser:%d:", userID)
}

func (c *WebBrowserCore) loadWebBrowser(userID int64) (webBrowserState, error) {
	raw, err := persist.Default.Get(webBrowserKey(userID))
	if err != nil || raw == "" {
		return webBrowserState{}, err
	}
	var st webBrowserState
	if err = json.Unmarshal([]byte(raw), &st); err != nil {
		return webBrowserState{}, err
	}
	return st, nil
}

func saveWebBrowser(userID int64, st webBrowserState) error {
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return persist.Default.Set(webBrowserKey(userID), string(raw))
}

func (c *WebBrowserCore) webBrowserSettingsReply(hash int64) (*mtproto.Account_WebBrowserSettings, error) {
	userID, err := c.requireWebBrowserUser()
	if err != nil {
		return nil, err
	}
	st, err := c.loadWebBrowser(userID)
	if err != nil {
		return nil, err
	}
	return webBrowserReply(st, hash), nil
}

func (c *WebBrowserCore) webBrowserUpdate(openExternal, displayClose bool) (*mtproto.Account_WebBrowserSettings, error) {
	userID, err := c.requireWebBrowserUser()
	if err != nil {
		return nil, err
	}
	st, err := c.loadWebBrowser(userID)
	if err != nil {
		return nil, err
	}
	st.OpenExternalBrowser = openExternal
	st.DisplayCloseButton = displayClose
	if err = saveWebBrowser(userID, st); err != nil {
		return nil, err
	}
	return webBrowserReply(st, 0), nil
}

// webBrowserDeleteExceptions drops every exception when urls is empty,
// otherwise only exceptions whose url or domain is listed.
func (c *WebBrowserCore) webBrowserDeleteExceptions(urls []string) (*mtproto.Account_WebBrowserSettings, error) {
	userID, err := c.requireWebBrowserUser()
	if err != nil {
		return nil, err
	}
	st, err := c.loadWebBrowser(userID)
	if err != nil {
		return nil, err
	}
	if len(urls) == 0 {
		st.External = nil
		st.InApp = nil
	} else {
		for _, raw := range urls {
			domain, urlValue, ok := splitWebURL(raw)
			if !ok {
				continue
			}
			st.External = removeException(st.External, urlValue, domain)
			st.InApp = removeException(st.InApp, urlValue, domain)
		}
	}
	if err = saveWebBrowser(userID, st); err != nil {
		return nil, err
	}
	return webBrowserReply(st, 0), nil
}

func (c *WebBrowserCore) toggleWebBrowserException(del bool, openTL *mtproto.Bool, rawURL string) (webBrowserState, webDomainExc, *bool, error) {
	userID, err := c.requireWebBrowserUser()
	if err != nil {
		return webBrowserState{}, webDomainExc{}, nil, err
	}
	open := tlBoolPtr(openTL)
	domain, urlValue, ok := splitWebURL(rawURL)
	if !ok {
		return webBrowserState{}, webDomainExc{}, nil, mtproto.ErrUrlInvalid
	}
	target := webDomainExc{Domain: domain, URL: urlValue, Title: domain}

	st, err := c.loadWebBrowser(userID)
	if err != nil {
		return webBrowserState{}, webDomainExc{}, nil, err
	}
	if del {
		switch {
		case open == nil:
			st.External = removeException(st.External, urlValue, domain)
			st.InApp = removeException(st.InApp, urlValue, domain)
		case *open:
			st.External = removeException(st.External, urlValue, domain)
		default:
			st.InApp = removeException(st.InApp, urlValue, domain)
		}
	} else if open != nil {
		if *open {
			st.External = append(removeException(st.External, urlValue, domain), target)
		} else {
			st.InApp = append(removeException(st.InApp, urlValue, domain), target)
		}
	}
	if err = saveWebBrowser(userID, st); err != nil {
		return webBrowserState{}, webDomainExc{}, nil, err
	}
	return st, target, open, nil
}

func webBrowserReply(st webBrowserState, hash int64) *mtproto.Account_WebBrowserSettings {
	sum := hashWebBrowser(st)
	if hash != 0 && hash == sum {
		out := mtproto.MakeTLAccountWebBrowserSettingsNotModified(nil).To_Account_WebBrowserSettings()
		out.Constructor = mtproto.TLConstructor_CRC32_account_webBrowserSettingsNotModified
		return out
	}
	out := mtproto.MakeTLAccountWebBrowserSettings(&mtproto.Account_WebBrowserSettings{
		OpenExternalBrowser: st.OpenExternalBrowser,
		DisplayCloseButton:  st.DisplayCloseButton,
		ExternalExceptions:  protoExceptions(st.External),
		InappExceptions:     protoExceptions(st.InApp),
		Hash:                sum,
	}).To_Account_WebBrowserSettings()
	out.Constructor = mtproto.TLConstructor_CRC32_account_webBrowserSettings
	return out
}

func protoExceptions(list []webDomainExc) []*mtproto.WebDomainException {
	if len(list) == 0 {
		return nil
	}
	out := make([]*mtproto.WebDomainException, 0, len(list))
	for _, item := range list {
		exc := mtproto.MakeTLWebDomainException(&mtproto.WebDomainException{
			Domain: item.Domain,
			Url:    item.URL,
			Title:  item.Title,
		}).To_WebDomainException()
		exc.Constructor = mtproto.TLConstructor_CRC32_webDomainException
		out = append(out, exc)
	}
	return out
}

func webBrowserExceptionUpdates(st webBrowserState, del bool, open *bool, exc webDomainExc) *mtproto.Updates {
	var openTL *mtproto.Bool
	if open != nil {
		openTL = mtproto.ToBool(*open)
	}
	settingsUpdate := mtproto.MakeTLUpdateWebBrowserSettings(&mtproto.Update{
		OpenExternalBrowser_FLAGBOOLEAN: st.OpenExternalBrowser,
		DisplayCloseButton:              st.DisplayCloseButton,
	}).To_Update()
	settingsUpdate.Constructor = mtproto.TLConstructor_CRC32_updateWebBrowserSettings

	exception := mtproto.MakeTLWebDomainException(&mtproto.WebDomainException{
		Domain: exc.Domain,
		Url:    exc.URL,
		Title:  exc.Title,
	}).To_WebDomainException()
	exception.Constructor = mtproto.TLConstructor_CRC32_webDomainException

	exceptionUpdate := mtproto.MakeTLUpdateWebBrowserException(&mtproto.Update{
		Delete:                       del,
		OpenExternalBrowser_FLAGBOOL: openTL,
		Exception_WEBDOMAINEXCEPTION: exception,
	}).To_Update()
	exceptionUpdate.Constructor = mtproto.TLConstructor_CRC32_updateWebBrowserException

	out := mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: []*mtproto.Update{settingsUpdate, exceptionUpdate},
		Users:   []*mtproto.User{},
		Chats:   []*mtproto.Chat{},
		Date:    int32(time.Now().Unix()),
		Seq:     0,
	}).To_Updates()
	out.Constructor = mtproto.TLConstructor_CRC32_updates
	return out
}

func tlBoolPtr(v *mtproto.Bool) *bool {
	if v == nil {
		return nil
	}
	value := mtproto.FromBool(v)
	return &value
}

func removeException(list []webDomainExc, urlValue, domain string) []webDomainExc {
	if len(list) == 0 {
		return list
	}
	out := make([]webDomainExc, 0, len(list))
	for _, item := range list {
		if urlValue != "" && item.URL == urlValue {
			continue
		}
		if domain != "" && item.Domain == domain {
			continue
		}
		out = append(out, item)
	}
	return out
}

func splitWebURL(raw string) (domain, urlValue string, ok bool) {
	urlValue = strings.TrimSpace(raw)
	if urlValue == "" {
		return "", "", false
	}
	candidate := urlValue
	if !strings.Contains(candidate, "://") {
		candidate = "https://" + candidate
	}
	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Hostname() == "" {
		return strings.ToLower(urlValue), urlValue, true
	}
	return strings.ToLower(parsed.Hostname()), urlValue, true
}

func hashWebBrowser(st webBrowserState) int64 {
	h := fnv.New64a()
	write := func(parts ...string) {
		for _, part := range parts {
			_, _ = h.Write([]byte(part))
			_, _ = h.Write([]byte{0})
		}
	}
	if st.OpenExternalBrowser {
		write("1")
	} else {
		write("0")
	}
	if st.DisplayCloseButton {
		write("1")
	} else {
		write("0")
	}
	writeList := func(tag string, list []webDomainExc) {
		write(tag, strconv.Itoa(len(list)))
		for _, item := range list {
			write(item.Domain, item.URL, item.Title, "0")
		}
	}
	writeList("external", st.External)
	writeList("inapp", st.InApp)
	sum := int64(h.Sum64())
	if sum == 0 {
		return 1
	}
	return sum
}
