package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/go-webauthn/webauthn/protocol"
)

func main() {
	payload := map[string]any{
		"id":    "4NFKBlhcwt69BR7_BPp8phadXM1MomvbUChehVmRAp0",
		"rawId": "4NFKBlhcwt69BR7_BPp8phadXM1MomvbUChehVmRAp0",
		"type":  "public-key",
		"response": map[string]any{
			"clientDataJSON":    "eyJ0eXBlIjoid2ViYXV0aG4uZ2V0IiwiY2hhbGxlbmdlIjoiNmw0ZWR4c25rSUltMENscHUwbkZ0ai04T3ZFa3hmaEVoTkFDdjYwdFVfUSIsIm9yaWdpbiI6Imh0dHBzOi8vcGFzc2tleS50ZWFtZ3JhbS5sb2NhbDo4NDQzIiwiY3Jvc3NPcmlnaW4iOmZhbHNlfQ",
			"authenticatorData": "HAUO-GNJE8sv6zdmqeHdFvtmHDOvvCzmhlTFmigkRcsFAAAAAg",
			"signature":         "MEUCIQCFUfayX3iSMaJBmF0FxWeCQTXxhvQQIhLaGpWNSojSTQIgVCKmt6zxMCpPsCZIW0p6KR_w_PvD2c-HxjDAd-aMEqM",
			"userHandle":        "Mjo5",
		},
	}
	data, _ := json.Marshal(payload)
	fmt.Printf("payload=%s\n", data)
	cd, cdErr := base64.RawURLEncoding.DecodeString(payload["response"].(map[string]any)["clientDataJSON"].(string))
	var cdv any
	fmt.Printf("clientdata raw=%q decode_err=%v json_err=%v\n", cd, cdErr, json.Unmarshal(cd, &cdv))
	parsed, err := protocol.ParseCredentialRequestResponseBytes(data)
	fmt.Printf("parse err=%v\n", err)
	if err != nil {
		return
	}
	fmt.Printf("challenge=%q rawID=%x userHandle=%q flags=%#x counter=%d\n", parsed.Response.CollectedClientData.Challenge, parsed.RawID, parsed.Response.UserHandle, parsed.Response.AuthenticatorData.Flags, parsed.Response.AuthenticatorData.Counter)
	pub, err := base64.StdEncoding.DecodeString("pQECAyYgASFYIOW5XJ6LvrnkSuc68UUQCXJ9uvw5HzW7Y4rmv3MkfGVgIlggFxe8MRcmYsdx5lDavJmHTr5+EVuX2OPeTIDMkY8Az7c=")
	fmt.Printf("public key decode len=%d err=%v\n", len(pub), err)
	err = parsed.Verify("6l4edxsnkIIm0Clpu0nFtj-8OvEkxfhEhNACv60tU_Q", "passkey.teamgram.local", []string{"https://passkey.teamgram.local:8443"}, nil, protocol.TopOriginDefaultVerificationMode, "", false, pub)
	fmt.Printf("verify err=%v\n", err)
}
