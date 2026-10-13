package core

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
)

const cdnProviderBodyLimit = 1 << 20

type cdnProviderEnvelope struct {
	RequestID string          `json:"request_id"`
	Operation string          `json:"operation"`
	Verified  bool            `json:"verified"`
	UserID    int64           `json:"user_id"`
	Error     string          `json:"error,omitempty"`
	Result    json.RawMessage `json:"result"`
}

func cdnProviderSignature(key string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func cdnProviderIdempotencyKey(userID int64, operation string, payload []byte) string {
	digest := sha256.Sum256(append([]byte(operation+":"+strconv.FormatInt(userID, 10)+":"), payload...))
	return hex.EncodeToString(digest[:])
}

func canonicalCDNPeer(peer *mtproto.InputPeer) map[string]any {
	if peer == nil {
		return nil
	}
	result := map[string]any{"predicate_name": peer.GetPredicateName()}
	putInt64 := func(name string, value int64) {
		if value != 0 {
			result[name] = value
		}
	}
	putInt32 := func(name string, value int32) {
		if value != 0 {
			result[name] = value
		}
	}
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerChat:
		putInt64("chat_id", peer.GetChatId())
	case mtproto.Predicate_inputPeerUser:
		putInt64("user_id", peer.GetUserId())
		putInt64("access_hash", peer.GetAccessHash())
	case mtproto.Predicate_inputPeerChannel:
		putInt64("channel_id", peer.GetChannelId())
		putInt64("access_hash", peer.GetAccessHash())
	case mtproto.Predicate_inputPeerUserFromMessage:
		if nested := peer.GetPeer(); nested != nil {
			result["peer"] = canonicalCDNPeer(nested)
		}
		putInt32("msg_id", peer.GetMsgId())
		putInt64("user_id", peer.GetUserId())
	case mtproto.Predicate_inputPeerChannelFromMessage:
		if nested := peer.GetPeer(); nested != nil {
			result["peer"] = canonicalCDNPeer(nested)
		}
		putInt32("msg_id", peer.GetMsgId())
		putInt64("channel_id", peer.GetChannelId())
	case mtproto.Predicate_inputPeerUsername:
		if peer.GetUsername() != "" {
			result["username"] = peer.GetUsername()
		}
	default:
		// Keep fields needed by future InputPeer constructors while excluding the
		// generated constructor value from the provider key.
		putInt64("chat_id", peer.GetChatId())
		putInt64("user_id", peer.GetUserId())
		putInt64("access_hash", peer.GetAccessHash())
		putInt64("channel_id", peer.GetChannelId())
		if nested := peer.GetPeer(); nested != nil {
			result["peer"] = canonicalCDNPeer(nested)
		}
		putInt32("msg_id", peer.GetMsgId())
		if peer.GetUsername() != "" {
			result["username"] = peer.GetUsername()
		}
	}
	return result
}

// canonicalCDNLocation removes generated protobuf metadata from the provider
// contract. The constructor is a wire decoding detail and must not change the
// provider's stable object key when the same Telegram location is received by
// another client or layer.
func canonicalCDNLocation(location *mtproto.InputFileLocation) map[string]any {
	if location == nil {
		return nil
	}
	result := map[string]any{"predicate_name": location.GetPredicateName()}
	putInt64 := func(name string, value int64) {
		if value != 0 {
			result[name] = value
		}
	}
	putInt32 := func(name string, value int32) {
		if value != 0 {
			result[name] = value
		}
	}
	putBytes := func(name string, value []byte) {
		if len(value) != 0 {
			result[name] = value
		}
	}
	putString := func(name, value string) {
		if value != "" {
			result[name] = value
		}
	}

	switch location.GetPredicateName() {
	case mtproto.Predicate_inputFileLocation:
		putInt64("volume_id", location.GetVolumeId())
		putInt32("local_id", location.GetLocalId())
		putInt64("secret", location.GetSecret())
		putBytes("file_reference", location.GetFileReference())
	case mtproto.Predicate_inputDocumentFileLocation, mtproto.Predicate_inputPhotoFileLocation:
		putInt64("id", location.GetId())
		putInt64("access_hash", location.GetAccessHash())
		putBytes("file_reference", location.GetFileReference())
		putString("thumb_size", location.GetThumbSize())
	case mtproto.Predicate_inputEncryptedFileLocation, mtproto.Predicate_inputSecureFileLocation:
		putInt64("id", location.GetId())
		putInt64("access_hash", location.GetAccessHash())
	case mtproto.Predicate_inputPeerPhotoFileLocation:
		if location.GetBig() {
			result["big"] = true
		}
		putInt64("photo_id", location.GetPhotoId())
		if peer := location.GetPeer(); peer != nil {
			result["peer"] = canonicalCDNPeer(peer)
		}
	case mtproto.Predicate_inputStickerSetThumb:
		putInt32("thumb_version", location.GetThumbVersion())
		if stickerset := location.GetStickerset(); stickerset != nil {
			result["stickerset"] = stickerset
		}
	case mtproto.Predicate_inputGroupCallStream:
		putInt64("time_ms", location.GetTimeMs())
		putInt32("scale", location.GetScale())
		if channel := location.GetVideoChannel(); channel != nil {
			result["video_channel"] = channel.GetValue()
		}
		if quality := location.GetVideoQuality(); quality != nil {
			result["video_quality"] = quality.GetValue()
		}
		if call := location.GetCall(); call != nil {
			result["call"] = call
		}
	default:
		// Preserve fields for newly added locations while still dropping the
		// generated constructor. Existing provider implementations can reject
		// unsupported predicates explicitly.
		putInt64("id", location.GetId())
		putInt64("access_hash", location.GetAccessHash())
		putBytes("file_reference", location.GetFileReference())
		putString("thumb_size", location.GetThumbSize())
	}
	return result
}

func verifyCDNProviderSignature(key, signature string, body []byte) bool {
	signature = strings.TrimPrefix(strings.TrimSpace(signature), "sha256=")
	provided, err := hex.DecodeString(signature)
	if err != nil || len(provided) != sha256.Size || strings.TrimSpace(key) == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}

func secureCDNProviderURL(parsed *url.URL, allowInsecureHTTP bool) bool {
	if parsed == nil {
		return false
	}
	if strings.EqualFold(parsed.Scheme, "https") {
		return true
	}
	// CDN responses contain bearer-like file tokens and encryption material. An
	// HTTP endpoint is therefore only valid for the explicitly enabled fixture
	// environment; production configuration must always use HTTPS.
	if !allowInsecureHTTP || !strings.EqualFold(parsed.Scheme, "http") {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "localhost" || host == "cdn-provider-fixture" || host == "cdn-provider-fixture.test" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c *FilesCore) callCDNProvider(operation string, payload any, result any) error {
	if c == nil || c.svcCtx == nil {
		return mtproto.ErrCdnMethodInvalid
	}
	endpoint := strings.TrimSpace(c.svcCtx.Config.CdnProviderEndpoint)
	signingKey := strings.TrimSpace(c.svcCtx.Config.CdnProviderSigningKey)
	if endpoint == "" || len(signingKey) < 32 {
		return mtproto.ErrCdnMethodInvalid
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !secureCDNProviderURL(parsed, c.svcCtx.Config.AllowInsecureProviderHTTP) {
		return mtproto.ErrCdnMethodInvalid
	}
	if c.svcCtx.Config.CdnProviderTimeoutSeconds < 0 || c.svcCtx.Config.CdnProviderTimeoutSeconds > 300 {
		return mtproto.ErrCdnMethodInvalid
	}
	timeout := 5 * time.Second
	if c.svcCtx.Config.CdnProviderTimeoutSeconds > 0 {
		timeout = time.Duration(c.svcCtx.Config.CdnProviderTimeoutSeconds) * time.Second
	}
	var nonce [32]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return mtproto.ErrCdnMethodInvalid
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return mtproto.ErrCdnMethodInvalid
	}
	userID := int64(0)
	if c.MD != nil {
		userID = c.MD.GetUserId()
	}
	request := struct {
		RequestID string          `json:"request_id"`
		Operation string          `json:"operation"`
		UserID    int64           `json:"user_id"`
		Payload   json.RawMessage `json:"payload"`
	}{hex.EncodeToString(nonce[:]), operation, userID, payloadJSON}
	body, err := json.Marshal(request)
	if err != nil {
		return mtproto.ErrCdnMethodInvalid
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return mtproto.ErrCdnMethodInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	if operation == "upload.reuploadCdnFile" {
		req.Header.Set("Idempotency-Key", cdnProviderIdempotencyKey(userID, operation, payloadJSON))
	}
	if key := strings.TrimSpace(c.svcCtx.Config.CdnProviderKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("X-Teamgram-CDN-Provider-Request-Signature", cdnProviderSignature(signingKey, body))
	client := &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return mtproto.ErrCdnMethodInvalid
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, cdnProviderBodyLimit+1))
	if err != nil || len(responseBody) > cdnProviderBodyLimit || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return mtproto.ErrCdnMethodInvalid
	}
	if !verifyCDNProviderSignature(signingKey, resp.Header.Get("X-Teamgram-CDN-Provider-Signature"), responseBody) {
		return mtproto.ErrCdnMethodInvalid
	}
	var envelope cdnProviderEnvelope
	if err = json.Unmarshal(responseBody, &envelope); err != nil || !envelope.Verified || envelope.RequestID != request.RequestID || envelope.Operation != request.Operation || envelope.UserID != request.UserID || envelope.Error != "" || len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return mtproto.ErrCdnMethodInvalid
	}
	if err = json.Unmarshal(envelope.Result, result); err != nil {
		return mtproto.ErrCdnMethodInvalid
	}
	return nil
}

func (c *FilesCore) cdnProviderConfigured() bool {
	if c == nil || c.svcCtx == nil {
		return false
	}
	return strings.TrimSpace(c.svcCtx.Config.CdnProviderEndpoint) != "" && len(strings.TrimSpace(c.svcCtx.Config.CdnProviderSigningKey)) >= 32
}
