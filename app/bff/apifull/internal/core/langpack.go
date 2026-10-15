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
	"embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RPCLangpackServer: language packs are embedded so APIFull does not depend on
// a filesystem checkout or an external translations provider at runtime.
//
//go:embed fallback.strings
var fallbackStrings string

//go:embed langpacks/languages.json
var languageMetadataJSON string

//go:embed langpacks/weba/*.json
var languagePackFiles embed.FS

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
	langCode     string
	metadata     langpackMetadata
	entries      []*fallbackLangEntry
	byKey        map[string]*fallbackLangEntry
	version      int32
	stringsCount int32
}

type langpackMetadata struct {
	LangCode     string `json:"lang_code"`
	Name         string `json:"name"`
	NativeName   string `json:"native_name"`
	BaseLangCode string `json:"base_lang_code,omitempty"`
	PluralCode   string `json:"plural_code"`
	Official     bool   `json:"official,omitempty"`
	Rtl          bool   `json:"rtl,omitempty"`
	Beta         bool   `json:"beta,omitempty"`
}

type serializedLangPack struct {
	Version int32                     `json:"version"`
	Entries []serializedLangPackEntry `json:"entries"`
}

type serializedLangPackEntry struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

type langpackCatalog struct {
	byCode    map[string]*fallbackLangPack
	languages []*fallbackLangPack
}

var fallbackLangPackOnce sync.Once
var fallbackLangPackValue *fallbackLangPack
var fallbackLangPackErr error

var langpackCatalogOnce sync.Once
var langpackCatalogValue *langpackCatalog
var langpackCatalogErr error

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

func parseJSONLangPack(raw []byte) (*fallbackLangPack, error) {
	var serialized serializedLangPack
	if err := json.Unmarshal(raw, &serialized); err != nil {
		return nil, err
	}
	if serialized.Version <= 0 {
		serialized.Version = 1
	}

	entries := make([]*fallbackLangEntry, 0, len(serialized.Entries))
	byKey := make(map[string]*fallbackLangEntry, len(serialized.Entries))
	for _, serializedEntry := range serialized.Entries {
		if serializedEntry.Key == "" {
			return nil, fmt.Errorf("language pack contains an empty key")
		}
		if _, exists := byKey[serializedEntry.Key]; exists {
			return nil, fmt.Errorf("language pack contains duplicate key %q", serializedEntry.Key)
		}

		entry := &fallbackLangEntry{key: serializedEntry.Key}
		var value string
		if err := json.Unmarshal(serializedEntry.Value, &value); err == nil {
			entry.value = value
		} else {
			var pluralValues map[string]string
			if err := json.Unmarshal(serializedEntry.Value, &pluralValues); err != nil {
				// Telegram's Arabic pack uses the same six plural slots as the
				// map representation, but serializes them as an ordered array.
				// Accept both encodings so a valid official pack cannot make the
				// whole catalog unavailable.
				var pluralArray []string
				if arrayErr := json.Unmarshal(serializedEntry.Value, &pluralArray); arrayErr != nil {
					return nil, fmt.Errorf("language pack key %q has invalid value: %w", serializedEntry.Key, err)
				}
				pluralValues = make(map[string]string, len(pluralArray))
				for index, pluralValue := range pluralArray {
					pluralValues[strconv.Itoa(index)] = pluralValue
				}
			}
			entry.plural = true
			for pluralKey, pluralValue := range pluralValues {
				switch pluralKey {
				case "0", "zero":
					entry.zero = pluralValue
				case "1", "one":
					entry.one = pluralValue
				case "2", "two":
					entry.two = pluralValue
				case "3", "few":
					entry.few = pluralValue
				case "4", "many":
					entry.many = pluralValue
				case "5", "other":
					entry.other = pluralValue
				}
			}
		}

		entries = append(entries, entry)
		byKey[entry.key] = entry
	}

	return &fallbackLangPack{
		entries: entries,
		byKey:   byKey,
		version: serialized.Version,
	}, nil
}

func loadLangpackCatalog() (*langpackCatalog, error) {
	langpackCatalogOnce.Do(func() {
		var metadata []langpackMetadata
		if err := json.Unmarshal([]byte(languageMetadataJSON), &metadata); err != nil {
			langpackCatalogErr = err
			return
		}

		english, err := loadEnglishLangPack()
		if err != nil {
			langpackCatalogErr = err
			return
		}

		catalog := &langpackCatalog{
			byCode: make(map[string]*fallbackLangPack, len(metadata)),
		}
		for _, language := range metadata {
			if language.LangCode == "" {
				langpackCatalogErr = fmt.Errorf("language metadata contains an empty code")
				return
			}
			if _, exists := catalog.byCode[language.LangCode]; exists {
				langpackCatalogErr = fmt.Errorf("language metadata contains duplicate code %q", language.LangCode)
				return
			}

			pack := english
			if language.LangCode != fallbackLangCode {
				raw, readErr := languagePackFiles.ReadFile("langpacks/weba/" + language.LangCode + ".json")
				if readErr != nil {
					langpackCatalogErr = fmt.Errorf("read language pack %q: %w", language.LangCode, readErr)
					return
				}
				pack, err = parseJSONLangPack(raw)
				if err != nil {
					langpackCatalogErr = fmt.Errorf("parse language pack %q: %w", language.LangCode, err)
					return
				}
			}
			pack.langCode = language.LangCode
			pack.metadata = language
			catalog.byCode[language.LangCode] = pack
			catalog.languages = append(catalog.languages, pack)
		}

		stringsCount := int32(len(english.entries))
		for _, pack := range catalog.languages {
			pack.stringsCount = stringsCount
		}
		langpackCatalogValue = catalog
	})

	return langpackCatalogValue, langpackCatalogErr
}

func loadEnglishLangPack() (*fallbackLangPack, error) {
	if raw, err := languagePackFiles.ReadFile("langpacks/weba/en.json"); err == nil {
		return parseJSONLangPack(raw)
	}
	return loadFallbackLangPack()
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

func langpackLanguage(pack *fallbackLangPack, langPack string) *mtproto.LangPackLanguage {
	translationsURL := fmt.Sprintf("https://translations.telegram.org/%s/%s", pack.langCode, langPack)
	var baseLangCode *wrapperspb.StringValue
	if pack.metadata.BaseLangCode != "" {
		baseLangCode = &wrapperspb.StringValue{Value: pack.metadata.BaseLangCode}
	}
	return mtproto.MakeTLLangPackLanguage(&mtproto.LangPackLanguage{
		Official:        pack.metadata.Official,
		Rtl:             pack.metadata.Rtl,
		Beta:            pack.metadata.Beta,
		Name:            pack.metadata.Name,
		NativeName:      pack.metadata.NativeName,
		LangCode:        pack.langCode,
		BaseLangCode:    baseLangCode,
		PluralCode:      pack.metadata.PluralCode,
		StringsCount:    pack.stringsCount,
		TranslatedCount: int32(len(pack.entries)),
		TranslationsUrl: translationsURL,
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

func validateLangpackRequest(langPack string) error {
	if langPack != "" && langPack != "android" && langPack != "ios" && langPack != "tdesktop" && langPack != "macos" && langPack != "webk" && langPack != "weba" {
		return mtproto.ErrLangPackInvalid
	}
	return nil
}

func langPackForCode(catalog *langpackCatalog, langPack, langCode string) (*fallbackLangPack, error) {
	baseCode := strings.TrimSuffix(langCode, "-raw")
	if langPack != "weba" && baseCode != fallbackLangCode {
		return nil, mtproto.ErrLangCodeNotSupported
	}
	pack := catalog.byCode[langCode]
	if pack == nil && strings.HasSuffix(langCode, "-raw") {
		baseCode := strings.TrimSuffix(langCode, "-raw")
		if basePack := catalog.byCode[baseCode]; basePack != nil {
			alias := *basePack
			alias.langCode = langCode
			pack = &alias
		}
	}
	if pack == nil {
		return nil, mtproto.ErrLangCodeNotSupported
	}
	return pack, nil
}

func langPackDifference(pack *fallbackLangPack, from int32) *mtproto.LangPackDifference {
	strings := make([]*mtproto.LangPackString, 0, len(pack.entries))
	if from != pack.version {
		for _, entry := range pack.entries {
			strings = append(strings, langpackEntryValue(entry))
		}
	}
	return mtproto.MakeTLLangPackDifference(&mtproto.LangPackDifference{
		LangCode:    pack.langCode,
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
	if err := validateLangpackRequest(in.GetLangPack()); err != nil {
		return nil, err
	}
	catalog, err := loadLangpackCatalog()
	if err != nil {
		return nil, err
	}
	pack, err := langPackForCode(catalog, in.GetLangPack(), in.GetLangCode())
	if err != nil {
		return nil, err
	}
	return langPackDifference(pack, 0), nil
}

func (c *ApiFullCore) LangpackGetStrings(in *mtproto.TLLangpackGetStrings) (*mtproto.Vector_LangPackString, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrLangPackInvalid
	}
	if err := validateLangpackRequest(in.GetLangPack()); err != nil {
		return nil, err
	}
	catalog, err := loadLangpackCatalog()
	if err != nil {
		return nil, err
	}
	pack, err := langPackForCode(catalog, in.GetLangPack(), in.GetLangCode())
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
	if err := validateLangpackRequest(in.GetLangPack()); err != nil {
		return nil, err
	}
	catalog, err := loadLangpackCatalog()
	if err != nil {
		return nil, err
	}
	pack, err := langPackForCode(catalog, in.GetLangPack(), in.GetLangCode())
	if err != nil {
		return nil, err
	}
	return langPackDifference(pack, in.GetFromVersion()), nil
}

func (c *ApiFullCore) LangpackGetLanguages(in *mtproto.TLLangpackGetLanguages) (*mtproto.Vector_LangPackLanguage, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrLangPackInvalid
	}
	if err := validateLangpackRequest(in.GetLangPack()); err != nil {
		return nil, err
	}
	catalog, err := loadLangpackCatalog()
	if err != nil {
		return nil, err
	}
	availableLanguages := catalog.languages
	if in.GetLangPack() != "weba" {
		availableLanguages = catalog.languages[:1]
	}
	languages := make([]*mtproto.LangPackLanguage, 0, len(availableLanguages))
	for _, pack := range availableLanguages {
		languages = append(languages, langpackLanguage(pack, in.GetLangPack()))
	}
	return &mtproto.Vector_LangPackLanguage{Datas: languages}, nil
}

func (c *ApiFullCore) LangpackGetLanguage(in *mtproto.TLLangpackGetLanguage) (*mtproto.LangPackLanguage, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrLangPackInvalid
	}
	if err := validateLangpackRequest(in.GetLangPack()); err != nil {
		return nil, err
	}
	catalog, err := loadLangpackCatalog()
	if err != nil {
		return nil, err
	}
	pack, err := langPackForCode(catalog, in.GetLangPack(), in.GetLangCode())
	if err != nil {
		return nil, err
	}
	return langpackLanguage(pack, in.GetLangPack()), nil
}
