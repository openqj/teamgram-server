package passkeyhelper

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

// ApplyEnvironment applies deployment-managed WebAuthn policy to a passkey
// service config. Empty values intentionally clear the policy so an
// unconfigured deployment remains fail-closed.
func ApplyEnvironment(provider *ProviderConfig) error {
	if provider == nil {
		return errors.New("passkey configuration is nil")
	}
	if value, ok := os.LookupEnv("TEAMGRAM_PASSKEY_RP_ID"); ok {
		provider.RelyingPartyId = strings.TrimSpace(value)
	}
	if value, ok := os.LookupEnv("TEAMGRAM_PASSKEY_RP_DISPLAY_NAME"); ok {
		provider.RelyingPartyDisplayName = strings.TrimSpace(value)
	}
	if value, ok := os.LookupEnv("TEAMGRAM_PASSKEY_ORIGINS"); ok {
		raw := strings.TrimSpace(value)
		if raw == "" {
			provider.Origins = nil
		} else {
			origins := strings.Split(raw, ",")
			for i := range origins {
				origins[i] = strings.TrimSpace(origins[i])
				if origins[i] == "" {
					return errors.New("TEAMGRAM_PASSKEY_ORIGINS contains an empty origin")
				}
			}
			provider.Origins = origins
		}
	}
	if value, ok := os.LookupEnv("TEAMGRAM_PASSKEY_TRUSTED_APPS"); ok {
		raw := strings.TrimSpace(value)
		if raw == "" {
			provider.TrustedApps = nil
		} else {
			apps := strings.Split(raw, ",")
			trustedApps := make([]TrustedApp, 0, len(apps))
			for _, item := range apps {
				parts := strings.Split(item, ":")
				if len(parts) != 2 {
					return errors.New("TEAMGRAM_PASSKEY_TRUSTED_APPS must use api_id:api_hash pairs")
				}
				apiID, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 32)
				apiHash := strings.TrimSpace(parts[1])
				if err != nil || apiID <= 0 || len(apiHash) != 32 {
					return errors.New("TEAMGRAM_PASSKEY_TRUSTED_APPS contains an invalid app")
				}
				for _, ch := range apiHash {
					if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') && (ch < 'A' || ch > 'F') {
						return errors.New("TEAMGRAM_PASSKEY_TRUSTED_APPS contains an invalid api hash")
					}
				}
				trustedApps = append(trustedApps, TrustedApp{ApiId: int32(apiID), ApiHash: apiHash})
			}
			provider.TrustedApps = trustedApps
		}
	}
	return nil
}
