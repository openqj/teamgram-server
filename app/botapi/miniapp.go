package botapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ValidateWebAppInitData verifies Telegram's Web App initData contract. The
// returned values retain the decoded fields so a handler can read user and
// chat data without reparsing the query string.
func ValidateWebAppInitData(initData, botToken string, now time.Time, maxAge time.Duration) (url.Values, error) {
	if strings.TrimSpace(initData) == "" || strings.TrimSpace(botToken) == "" {
		return nil, errors.New("botapi: init data and bot token are required")
	}
	values, err := url.ParseQuery(initData)
	if err != nil {
		return nil, errors.New("botapi: init data is not valid query data")
	}
	hashValue := strings.ToLower(strings.TrimSpace(values.Get("hash")))
	if len(hashValue) != sha256.Size*2 {
		return nil, errors.New("botapi: init data hash is missing")
	}
	if _, err := hex.DecodeString(hashValue); err != nil {
		return nil, errors.New("botapi: init data hash is invalid")
	}
	if len(values["hash"]) != 1 {
		return nil, errors.New("botapi: init data hash is duplicated")
	}
	if authDate := strings.TrimSpace(values.Get("auth_date")); authDate != "" && maxAge > 0 {
		seconds, err := strconv.ParseInt(authDate, 10, 64)
		if err != nil {
			return nil, errors.New("botapi: auth_date is invalid")
		}
		age := now.Unix() - seconds
		if age < -60 || time.Duration(age)*time.Second > maxAge {
			return nil, errors.New("botapi: init data has expired")
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		if key != "hash" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	checkParts := make([]string, 0, len(keys))
	for _, key := range keys {
		if len(values[key]) != 1 {
			return nil, errors.New("botapi: init data field is duplicated")
		}
		checkParts = append(checkParts, key+"="+values[key][0])
	}
	secretMAC := hmac.New(sha256.New, []byte("WebAppData"))
	_, _ = secretMAC.Write([]byte(botToken))
	secret := secretMAC.Sum(nil)
	checkMAC := hmac.New(sha256.New, secret)
	_, _ = checkMAC.Write([]byte(strings.Join(checkParts, "\n")))
	if !hmac.Equal(checkMAC.Sum(nil), mustDecodeHex(hashValue)) {
		return nil, errors.New("botapi: init data hash does not match")
	}
	return values, nil
}

func mustDecodeHex(value string) []byte {
	decoded, _ := hex.DecodeString(value)
	return decoded
}
