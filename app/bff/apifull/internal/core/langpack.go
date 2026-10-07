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
	"bufio"
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RPCLangpackServer: the English fallback shipped with the Web client is the
// only language data available to APIFull. It is embedded so the server does
// not depend on a filesystem checkout or an external translations provider.
//
//go:embed fallback.strings
var fallbackStrings string

const fallbackLangCode = "en"

type fallbackLangEntry struct {
	key    string
	value  string
	plural bool
	zero   string
	one    string
	two    string
	few    string
	many   string
	other  string
}

type fallbackLangPack struct {
	entries []*fallbackLangEntry
	byKey   map[string]*fallbackLangEntry
	version int32
}

var fallbackLangPackOnce sync.Once
var fallbackLangPackValue *fallbackLangPack
var fallbackLangPackErr error

func loadFallbackLangPack() (*fallbackLangPack, error) {
	fallbackLangPackOnce.Do(func() {
		fallbackLangPackValue, fallbackLangPackErr = parseFallbackLangPack(fallbackStrings)
	})
	return fallbackLangPackValue, fallbackLangPackErr
}

func parseFallbackLangPack(raw string) (*fallbackLangPack, error) {
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	rawValues := make(map[string]string)
	var order []string
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		line = strings.TrimSpace(strings.TrimSuffix(line, ";"))
		separator := fallbackStringSeparator(line)
		if separator < 0 {
			return nil, fmt.Errorf("fallback.strings:%d: missing separator", lineNo)
		}
		key, err := strconv.Unquote(strings.TrimSpace(line[:separator]))
		if err != nil {
			return nil, fmt.Errorf("fallback.strings:%d: invalid key: %w", lineNo, err)
		}
		value, err := strconv.Unquote(strings.TrimSpace(line[separator+1:]))
		if err != nil {
			return nil, fmt.Errorf("fallback.strings:%d: invalid value: %w", lineNo, err)
		}
		if _, exists := rawValues[key]; !exists {
			order = append(order, key)
		}
		rawValues[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	entries := make([]*fallbackLangEntry, 0, len(order))
	byKey := make(map[string]*fallbackLangEntry, len(order))
	for _, key := range order {
		base, suffix := fallbackPluralKey(key)
		entry := byKey[base]
		if entry == nil {
			entry = &fallbackLangEntry{key: base}
			byKey[base] = entry
			entries = append(entries, entry)
		}
		value := rawValues[key]
		switch suffix {
		case "zero":
			entry.plural, entry.zero = true, value
		case "one":
			entry.plural, entry.one = true, value
		case "two":
			entry.plural, entry.two = true, value
		case "few":
			entry.plural, entry.few = true, value
		case "many":
			entry.plural, entry.many = true, value
		case "other":
			entry.plural, entry.other = true, value
		default:
			entry.value = value
		}
	}
	return &fallbackLangPack{entries: entries, byKey: byKey, version: 1}, nil
}

func fallbackStringSeparator(line string) int {
	escaped, quoted := false, false
	for i, r := range line {
		switch r {
		case '\\':
			escaped = !escaped
		case '"':
			if !escaped {
				quoted = !quoted
			}
			escaped = false
		case '=':
			if !quoted {
				return i
			}
		default:
			escaped = false
		}
	}
	return -1
}

func fallbackPluralKey(key string) (string, string) {
	for _, suffix := range []string{"zero", "one", "two", "few", "many", "other"} {
		marker := "_" + suffix
		if strings.HasSuffix(key, marker) {
			return strings.TrimSuffix(key, marker), suffix
		}
	}
	return key, ""
}

func langpackLanguage(pack *fallbackLangPack) *mtproto.LangPackLanguage {
	count := int32(len(pack.entries))
	return mtproto.MakeTLLangPackLanguage(&mtproto.LangPackLanguage{
		Official:        true,
		Name:            "English",
		NativeName:      "English",
		LangCode:        fallbackLangCode,
		PluralCode:      fallbackLangCode,
		StringsCount:    count,
		TranslatedCount: count,
		TranslationsUrl: "",
	}).To_LangPackLanguage()
}

func langpackEntryValue(entry *fallbackLangEntry) *mtproto.LangPackString {
	if entry.plural {
		return mtproto.MakeTLLangPackStringPluralized(&mtproto.LangPackString{
			Key:        entry.key,
			ZeroValue:  stringValue(entry.zero),
			OneValue:   stringValue(entry.one),
			TwoValue:   stringValue(entry.two),
			FewValue:   stringValue(entry.few),
			ManyValue:  stringValue(entry.many),
			OtherValue: entry.other,
		}).To_LangPackString()
	}
	return mtproto.MakeTLLangPackString(&mtproto.LangPackString{Key: entry.key, Value: entry.value}).To_LangPackString()
}

func stringValue(value string) *wrapperspb.StringValue {
	if value == "" {
		return nil
	}
	return &wrapperspb.StringValue{Value: value}
}

func validateLangpackRequest(langPack, langCode string) error {
	if langPack != "" && langPack != "android" && langPack != "ios" && langPack != "tdesktop" && langPack != "webk" && langPack != "weba" {
		return mtproto.ErrLangPackInvalid
	}
	if langCode != "" && langCode != fallbackLangCode {
		return mtproto.ErrLangCodeNotSupported
	}
	return nil
}

func fallbackLangPackDifference(pack *fallbackLangPack, from int32) *mtproto.LangPackDifference {
	strings := make([]*mtproto.LangPackString, 0, len(pack.entries))
	if from != pack.version {
		for _, entry := range pack.entries {
			strings = append(strings, langpackEntryValue(entry))
		}
	}
	return mtproto.MakeTLLangPackDifference(&mtproto.LangPackDifference{
		LangCode:    fallbackLangCode,
		FromVersion: from,
		Version:     pack.version,
		Strings:     strings,
	}).To_LangPackDifference()
}

// b13 is retained for the local translateText implementation. It is not a
// langpack catalog and is intentionally not exposed by the langpack RPCs.
func b13Key(userID int64) string {
	return fmt.Sprintf("b13:%d:", userID)
}

func loadB13(userID int64) (string, error) {
	return persist.Default.Get(b13Key(userID))
}

func (c *ApiFullCore) LangpackGetLangPack(in *mtproto.TLLangpackGetLangPack) (*mtproto.LangPackDifference, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrLangPackInvalid
	}
	if err := validateLangpackRequest(in.GetLangPack(), in.GetLangCode()); err != nil {
		return nil, err
	}
	pack, err := loadFallbackLangPack()
	if err != nil {
		return nil, err
	}
	return fallbackLangPackDifference(pack, 0), nil
}

func (c *ApiFullCore) LangpackGetStrings(in *mtproto.TLLangpackGetStrings) (*mtproto.Vector_LangPackString, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrLangPackInvalid
	}
	if err := validateLangpackRequest(in.GetLangPack(), in.GetLangCode()); err != nil {
		return nil, err
	}
	pack, err := loadFallbackLangPack()
	if err != nil {
		return nil, err
	}
	strings := make([]*mtproto.LangPackString, 0, len(in.GetKeys()))
	seen := make(map[string]struct{}, len(in.GetKeys()))
	for _, key := range in.GetKeys() {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if entry := pack.byKey[key]; entry != nil {
			strings = append(strings, langpackEntryValue(entry))
		}
	}
	return &mtproto.Vector_LangPackString{Datas: strings}, nil
}

func (c *ApiFullCore) LangpackGetDifference(in *mtproto.TLLangpackGetDifference) (*mtproto.LangPackDifference, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrLangPackInvalid
	}
	if err := validateLangpackRequest(in.GetLangPack(), in.GetLangCode()); err != nil {
		return nil, err
	}
	pack, err := loadFallbackLangPack()
	if err != nil {
		return nil, err
	}
	return fallbackLangPackDifference(pack, in.GetFromVersion()), nil
}

func (c *ApiFullCore) LangpackGetLanguages(in *mtproto.TLLangpackGetLanguages) (*mtproto.Vector_LangPackLanguage, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrLangPackInvalid
	}
	if err := validateLangpackRequest(in.GetLangPack(), ""); err != nil {
		return nil, err
	}
	pack, err := loadFallbackLangPack()
	if err != nil {
		return nil, err
	}
	return &mtproto.Vector_LangPackLanguage{Datas: []*mtproto.LangPackLanguage{langpackLanguage(pack)}}, nil
}

func (c *ApiFullCore) LangpackGetLanguage(in *mtproto.TLLangpackGetLanguage) (*mtproto.LangPackLanguage, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrLangPackInvalid
	}
	if err := validateLangpackRequest(in.GetLangPack(), in.GetLangCode()); err != nil {
		return nil, err
	}
	pack, err := loadFallbackLangPack()
	if err != nil {
		return nil, err
	}
	return langpackLanguage(pack), nil
}
