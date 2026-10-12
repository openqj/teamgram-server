// Copyright 2026 Teamgram Authors
// All rights reserved.
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

package core

import (
	"bufio"
	_ "embed"
	"hash"
	"hash/crc32"
	"strconv"
	"strings"
	"sync"

	"github.com/teamgram/proto/mtproto"
)

// PhoneCountries.txt is the Telegram phone-entry catalog used by the clients.
// Each row is country-code;ISO2;format;English name.
//
//go:embed data/PhoneCountries.txt
var phoneCountriesData string

type phoneCountryRecord struct {
	iso2        string
	defaultName string
	hidden      bool
	codes       []phoneCountryCode
}

type phoneCountryCode struct {
	countryCode string
	pattern     string
}

var phoneCountryCatalogState struct {
	sync.Once
	records []phoneCountryRecord
	hash    int32
}

func phoneCountryCatalogData() ([]phoneCountryRecord, int32) {
	phoneCountryCatalogState.Do(func() {
		phoneCountryCatalogState.records = parsePhoneCountries(phoneCountriesData)
		phoneCountryCatalogState.hash = hashPhoneCountries(phoneCountryCatalogState.records)
	})

	return phoneCountryCatalogState.records, phoneCountryCatalogState.hash
}

func parsePhoneCountries(data string) []phoneCountryRecord {
	records := make([]phoneCountryRecord, 0, 256)
	recordIndexes := make(map[string]int, 256)

	scanner := bufio.NewScanner(strings.NewReader(data))
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for scanner.Scan() {
		fields := strings.SplitN(strings.TrimSpace(scanner.Text()), ";", 4)
		if len(fields) != 4 {
			continue
		}

		countryCode := strings.TrimSpace(fields[0])
		iso2 := strings.ToUpper(strings.TrimSpace(fields[1]))
		pattern := strings.TrimSpace(fields[2])
		defaultName := strings.TrimSpace(fields[3])
		if len(iso2) != 2 || defaultName == "" {
			continue
		}
		if _, err := strconv.Atoi(countryCode); err != nil {
			continue
		}

		index, ok := recordIndexes[iso2]
		if !ok {
			index = len(records)
			recordIndexes[iso2] = index
			records = append(records, phoneCountryRecord{
				iso2:        iso2,
				defaultName: defaultName,
				hidden:      iso2 == "FT",
			})
		}

		records[index].codes = append(records[index].codes, phoneCountryCode{
			countryCode: countryCode,
			pattern:     pattern,
		})
	}

	return records
}

func hashPhoneCountries(records []phoneCountryRecord) int32 {
	digest := crc32.NewIEEE()
	for _, record := range records {
		writeHashString(digest, record.iso2)
		writeHashString(digest, record.defaultName)
		writeHashString(digest, strconv.FormatBool(record.hidden))
		for _, code := range record.codes {
			writeHashString(digest, code.countryCode)
			writeHashString(digest, code.pattern)
		}
	}

	hashValue := int32(digest.Sum32())
	if hashValue == 0 {
		return 1
	}
	return hashValue
}

func writeHashString(digest hash.Hash32, value string) {
	_, _ = digest.Write([]byte(value))
	_, _ = digest.Write([]byte{0})
}

func buildPhoneCountryList(records []phoneCountryRecord) []*mtproto.Help_Country {
	countries := make([]*mtproto.Help_Country, 0, len(records))
	for _, record := range records {
		countryCodes := make([]*mtproto.Help_CountryCode, 0, len(record.codes))
		for _, code := range record.codes {
			var patterns []string
			if code.pattern != "" {
				patterns = []string{code.pattern}
			}

			countryCodes = append(countryCodes, mtproto.MakeTLHelpCountryCode(&mtproto.Help_CountryCode{
				CountryCode: code.countryCode,
				Patterns:    patterns,
			}).To_Help_CountryCode())
		}

		countries = append(countries, mtproto.MakeTLHelpCountry(&mtproto.Help_Country{
			Hidden:       record.hidden,
			Iso2:         record.iso2,
			DefaultName:  record.defaultName,
			CountryCodes: countryCodes,
		}).To_Help_Country())
	}

	return countries
}
