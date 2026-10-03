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
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"hash/fnv"
	"net"
	"net/url"
	"path"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const maxWebPageURLLength = 4096

// webPageRecord is the metadata that can be established without fetching a
// remote page. It is deliberately limited to facts derived from the URL.
type webPageRecord struct {
	URL        string `json:"url"`
	DisplayURL string `json:"display_url"`
	SiteName   string `json:"site_name"`
	ID         int64  `json:"id"`
	Hash       int32  `json:"hash"`
}

func webPageKey(canonicalURL string) string {
	digest := sha256.Sum256([]byte(canonicalURL))
	return "b16:webpage:" + hex.EncodeToString(digest[:])
}

func normalizeWebPageURL(raw string) (string, *url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxWebPageURLLength || strings.ContainsAny(raw, "\r\n\t") {
		return "", nil, mtproto.ErrUrlInvalid
	}
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.User != nil || u.Host == "" {
		return "", nil, mtproto.ErrUrlInvalid
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", nil, mtproto.ErrUrlInvalid
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" || blockedWebPageHost(host) {
		return "", nil, mtproto.ErrUrlInvalid
	}
	port := u.Port()
	if port != "" {
		portNumber, portErr := strconv.ParseUint(port, 10, 16)
		if portErr != nil || portNumber == 0 {
			return "", nil, mtproto.ErrUrlInvalid
		}
	}
	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		port = ""
	}
	hostPart := host
	if strings.Contains(host, ":") {
		hostPart = "[" + host + "]"
	}
	if port != "" {
		hostPart += ":" + port
	}
	u.Host = hostPart
	u.Fragment = ""
	if u.Path == "" {
		u.Path = "/"
	}
	// Clean dot segments in the escaped form so an encoded slash remains an
	// encoded slash in the canonical URL.
	escapedPath := path.Clean("/" + strings.TrimPrefix(u.EscapedPath(), "/"))
	if decoded, decodeErr := url.PathUnescape(escapedPath); decodeErr == nil {
		u.Path = decoded
		if decoded == escapedPath {
			u.RawPath = ""
		} else {
			u.RawPath = escapedPath
		}
	}
	canonical := u.String()
	if len(canonical) > maxWebPageURLLength {
		return "", nil, mtproto.ErrUrlInvalid
	}
	return canonical, u, nil
}

func blockedWebPageHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast())
}

func webPageID(canonicalURL string) int64 {
	digest := sha256.Sum256([]byte(canonicalURL))
	id := int64(binary.BigEndian.Uint64(digest[:8]) & 0x7fffffffffffffff)
	if id == 0 {
		return 1
	}
	return id
}

func webPageHash(canonicalURL string) int32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(canonicalURL))
	v := int32(h.Sum32() & 0x7fffffff)
	if v == 0 {
		return 1
	}
	return v
}

func newWebPageRecord(canonicalURL string, u *url.URL) webPageRecord {
	display := strings.ToLower(u.Host)
	if u.EscapedPath() != "" && u.EscapedPath() != "/" {
		display += u.EscapedPath()
	}
	if u.RawQuery != "" {
		display += "?" + u.RawQuery
	}
	return webPageRecord{
		URL:        canonicalURL,
		DisplayURL: display,
		SiteName:   strings.ToLower(u.Hostname()),
		ID:         webPageID(canonicalURL),
		Hash:       webPageHash(canonicalURL),
	}
}

func loadOrCreateWebPage(rawURL string) (webPageRecord, error) {
	canonicalURL, parsed, err := normalizeWebPageURL(rawURL)
	if err != nil {
		return webPageRecord{}, err
	}
	key := webPageKey(canonicalURL)
	stored, err := persist.Default.Get(key)
	if err != nil {
		return webPageRecord{}, err
	}
	if stored != "" {
		var record webPageRecord
		if err := json.Unmarshal([]byte(stored), &record); err != nil || record.URL != canonicalURL || record.Hash == 0 || record.ID == 0 {
			return webPageRecord{}, mtproto.ErrInternalServerError
		}
		return record, nil
	}
	record := newWebPageRecord(canonicalURL, parsed)
	encoded, err := json.Marshal(record)
	if err != nil {
		return webPageRecord{}, err
	}
	if err := persist.Default.Set(key, string(encoded)); err != nil {
		return webPageRecord{}, err
	}
	return record, nil
}

func (r webPageRecord) toWebPage() *mtproto.WebPage {
	return mtproto.MakeTLWebPage(&mtproto.WebPage{
		Id:         r.ID,
		Url_STRING: r.URL,
		DisplayUrl: r.DisplayURL,
		Hash:       r.Hash,
		SiteName:   wrapperspb.String(r.SiteName),
		Attributes: []*mtproto.WebPageAttribute{},
	}).To_WebPage()
}

func emptyWebPageMedia() *mtproto.MessageMedia {
	return mtproto.MakeTLMessageMediaEmpty(&mtproto.MessageMedia{
		AltDocuments:                             []*mtproto.Document{},
		Channels:                                 []int64{},
		CountriesIso2:                            []string{},
		Winners:                                  []int64{},
		ExtendedMedia_VECTORMESSAGEEXTENDEDMEDIA: []*mtproto.MessageExtendedMedia{},
		Completions:                              []*mtproto.TodoCompletion{},
	}).To_MessageMedia()
}

func webPageMedia(record webPageRecord) *mtproto.MessageMedia {
	return mtproto.MakeTLMessageMediaWebPage(&mtproto.MessageMedia{
		Webpage: record.toWebPage(),
	}).To_MessageMedia()
}

func entityText(message string, offset, length int32) string {
	if offset < 0 || length <= 0 {
		return ""
	}
	units := utf16.Encode([]rune(message))
	start := int(offset)
	end := start + int(length)
	if start < 0 || end < start || end > len(units) {
		return ""
	}
	return string(utf16.Decode(units[start:end]))
}

func messageURL(message string, entities []*mtproto.MessageEntity) string {
	for _, entity := range entities {
		if entity == nil {
			continue
		}
		predicate := entity.GetPredicateName()
		if predicate == mtproto.Predicate_messageEntityTextUrl || entity.GetConstructor() == mtproto.TLConstructor_CRC32_messageEntityTextUrl {
			if candidate := strings.TrimSpace(entity.GetUrl()); candidate != "" {
				if canonical, _, err := normalizeWebPageURL(candidate); err == nil {
					return canonical
				}
			}
		}
		if predicate == mtproto.Predicate_messageEntityUrl || entity.GetConstructor() == mtproto.TLConstructor_CRC32_messageEntityUrl {
			candidate := entityText(message, entity.GetOffset(), entity.GetLength())
			if canonical, _, err := normalizeWebPageURL(candidate); err == nil {
				return canonical
			}
		}
	}
	for _, token := range strings.Fields(message) {
		candidate := strings.Trim(token, "<>\"'`()[]{}.,!?;:")
		if canonical, _, err := normalizeWebPageURL(candidate); err == nil {
			return canonical
		}
	}
	return ""
}

func webPageFromURL(rawURL string, requestedHash int32) (*mtproto.WebPage, error) {
	record, err := loadOrCreateWebPage(rawURL)
	if err != nil {
		return nil, err
	}
	if requestedHash != 0 && requestedHash == record.Hash {
		return mtproto.MakeTLWebPageNotModified(&mtproto.WebPage{}).To_WebPage(), nil
	}
	return record.toWebPage(), nil
}

// RPCWebPageServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) MessagesGetWebPagePreview570D6F6F(in *mtproto.TLMessagesGetWebPagePreview570D6F6F) (*mtproto.Messages_WebPagePreview, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	message, entities := "", []*mtproto.MessageEntity(nil)
	if in != nil {
		message, entities = in.GetMessage(), in.GetEntities()
	}
	media := emptyWebPageMedia()
	if rawURL := messageURL(message, entities); rawURL != "" {
		record, err := loadOrCreateWebPage(rawURL)
		if err != nil {
			return nil, err
		}
		media = webPageMedia(record)
	}
	return mtproto.MakeTLMessagesWebPagePreview(&mtproto.Messages_WebPagePreview{
		Media: media,
		Chats: []*mtproto.Chat{},
		Users: []*mtproto.User{},
	}).To_Messages_WebPagePreview(), nil
}

func (c *ApiFullCore) MessagesGetWebPage8D9692A3(in *mtproto.TLMessagesGetWebPage8D9692A3) (*mtproto.Messages_WebPage, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrUrlInvalid
	}
	page, err := webPageFromURL(in.GetUrl(), in.GetHash())
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesWebPage(&mtproto.Messages_WebPage{
		Webpage: page,
		Chats:   []*mtproto.Chat{},
		Users:   []*mtproto.User{},
	}).To_Messages_WebPage(), nil
}

func (c *ApiFullCore) MessagesGetWebPagePreview8B68B0CC(in *mtproto.TLMessagesGetWebPagePreview8B68B0CC) (*mtproto.MessageMedia, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	message, entities := "", []*mtproto.MessageEntity(nil)
	if in != nil {
		message, entities = in.GetMessage(), in.GetEntities()
	}
	if rawURL := messageURL(message, entities); rawURL != "" {
		record, err := loadOrCreateWebPage(rawURL)
		if err != nil {
			return nil, err
		}
		return webPageMedia(record), nil
	}
	return emptyWebPageMedia(), nil
}

func (c *ApiFullCore) MessagesGetWebPage32CA8F91(in *mtproto.TLMessagesGetWebPage32CA8F91) (*mtproto.WebPage, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrUrlInvalid
	}
	return webPageFromURL(in.GetUrl(), in.GetHash())
}
