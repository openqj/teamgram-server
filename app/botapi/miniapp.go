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

// SignWebAppInitData creates Telegram's URL-encoded Web App initData value.
// The caller must provide the fields that Telegram would expose to the Web
// App (for example auth_date, query_id and user); the bot token is used only
// for this operation and is never returned or persisted by this package.
func SignWebAppInitData(fields url.Values, botToken string) (string, error) {
	if len(fields) == 0 || strings.TrimSpace(botToken) == "" {
		return "", errors.New("botapi: init data fields and bot token are required")
	}

	values := make(url.Values, len(fields)+1)
	keys := make([]string, 0, len(fields))
	for key, entries := range fields {
		if key == "hash" || strings.TrimSpace(key) == "" || len(entries) != 1 {
			return "", errors.New("botapi: init data field is invalid")
		}
		values[key] = []string{entries[0]}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	checkParts := make([]string, 0, len(keys))
	for _, key := range keys {
		checkParts = append(checkParts, key+"="+values[key][0])
	}

	secretMAC := hmac.New(sha256.New, []byte("WebAppData"))
	_, _ = secretMAC.Write([]byte(botToken))
	secret := secretMAC.Sum(nil)
	checkMAC := hmac.New(sha256.New, secret)
	_, _ = checkMAC.Write([]byte(strings.Join(checkParts, "\n")))
	values.Set("hash", hex.EncodeToString(checkMAC.Sum(nil)))
	return values.Encode(), nil
}

// AddWebAppInitData adds signed Telegram Web App init data to a Web App URL.
// Telegram's official WebApp SDK reads init data from the URL hash, so the
// value is emitted as a hash query parameter and never as a normal query
// parameter. Existing non-Telegram query and hash parameters are preserved.
func AddWebAppInitData(rawURL, initData string) (string, error) {
	if strings.TrimSpace(rawURL) == "" || strings.TrimSpace(initData) == "" {
		return "", errors.New("botapi: Web App URL and init data are required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", errors.New("botapi: Web App URL must be HTTPS")
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return "", errors.New("botapi: Web App URL query is invalid")
	}
	query.Del("tgWebAppData")
	query.Del("tgWebAppStartParam")
	parsed.RawQuery = query.Encode()
	if err := setWebAppHashParam(parsed, "tgWebAppData", initData, "tgWebAppStartParam"); err != nil {
		return "", err
	}
	return parsed.String(), nil
}

// AddWebAppStartParam adds the launch parameter consumed by Telegram's
// WebApp SDK. It shares the same hash query as tgWebAppData and removes stale
// values from both the normal query and hash before writing the new value.
func AddWebAppStartParam(rawURL, startParam string) (string, error) {
	if strings.TrimSpace(rawURL) == "" {
		return "", errors.New("botapi: Web App URL is required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", errors.New("botapi: Web App URL must be HTTPS")
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return "", errors.New("botapi: Web App URL query is invalid")
	}
	query.Del("tgWebAppStartParam")
	parsed.RawQuery = query.Encode()
	if err := setWebAppHashParam(parsed, "tgWebAppStartParam", startParam); err != nil {
		return "", err
	}
	return parsed.String(), nil
}

// setWebAppHashParam updates one Telegram hash query field while preserving
// the optional hash path accepted by telegram-web-app.js (for example
// #path?tgWebAppData=...). URL.Fragment is kept decoded while RawFragment
// carries the single encoded representation used on the wire.
func setWebAppHashParam(parsed *url.URL, key, value string, clearKeys ...string) error {
	escapedFragment := parsed.EscapedFragment()
	pathPart, queryPart, _ := splitWebAppHash(escapedFragment)
	values, err := url.ParseQuery(queryPart)
	if err != nil {
		return errors.New("botapi: Web App hash query is invalid")
	}
	for _, clearKey := range clearKeys {
		values.Del(clearKey)
	}
	if value == "" {
		values.Del(key)
	} else {
		values.Del(key)
		values.Set(key, value)
	}
	escapedQuery := values.Encode()
	escaped := pathPart
	if escapedQuery != "" {
		if pathPart != "" {
			escaped = pathPart + "?" + escapedQuery
		} else {
			// A query-only fragment is Telegram's normal Web App form:
			// #tgWebAppData=... . Do not introduce a leading '?' when
			// adding another Telegram hash field.
			escaped = escapedQuery
		}
	}
	decoded, err := url.PathUnescape(escaped)
	if err != nil {
		return errors.New("botapi: Web App hash is invalid")
	}
	parsed.Fragment = decoded
	parsed.RawFragment = escaped
	return nil
}

func splitWebAppHash(fragment string) (pathPart, queryPart string, hasQuery bool) {
	if question := strings.IndexByte(fragment, '?'); question >= 0 {
		return fragment[:question], fragment[question+1:], true
	}
	if strings.Contains(fragment, "=") {
		return "", fragment, true
	}
	return fragment, "", false
}

func mustDecodeHex(value string) []byte {
	decoded, _ := hex.DecodeString(value)
	return decoded
}
