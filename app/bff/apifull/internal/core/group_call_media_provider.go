package core

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

const groupCallMediaBodyLimit = 1 << 20

type groupCallMediaProvider struct {
	client     *http.Client
	endpoint   string
	apiKey     string
	signingKey string
	rtmpHost   string
}

type groupCallMediaRequest struct {
	Operation string `json:"operation"`
	UserID    int64  `json:"user_id"`
	CallID    int64  `json:"call_id"`
	ChannelID int64  `json:"channel_id"`
	Revoke    bool   `json:"revoke"`
}

type groupCallStreamChannelData struct {
	Channel       int32 `json:"channel"`
	Scale         int32 `json:"scale"`
	LastTimestamp int64 `json:"last_timestamp_ms"`
}

type groupCallMediaResponse struct {
	Verified  bool                         `json:"verified"`
	UserID    int64                        `json:"user_id"`
	CallID    int64                        `json:"call_id"`
	ChannelID int64                        `json:"channel_id"`
	Channels  []groupCallStreamChannelData `json:"channels"`
	URL       string                       `json:"url"`
	Key       string                       `json:"key"`
}

func (c *ApiFullCore) newGroupCallMediaProvider() (*groupCallMediaProvider, error) {
	if c == nil || c.svcCtx == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	cfg := c.svcCtx.Config
	if strings.TrimSpace(cfg.GroupCallMediaEndpoint) == "" || strings.TrimSpace(cfg.GroupCallMediaAPIKey) == "" || len(cfg.GroupCallMediaSigningKey) < 32 {
		return nil, mtproto.ErrMethodNotImpl
	}
	parsed, err := url.Parse(strings.TrimSpace(cfg.GroupCallMediaEndpoint))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !securePaymentProviderURL(parsed) {
		return nil, mtproto.ErrInternalServerError
	}
	timeout := 5 * time.Second
	if cfg.GroupCallMediaTimeoutSeconds < 0 || cfg.GroupCallMediaTimeoutSeconds > 300 {
		return nil, mtproto.ErrInternalServerError
	}
	if cfg.GroupCallMediaTimeoutSeconds > 0 {
		timeout = time.Duration(cfg.GroupCallMediaTimeoutSeconds) * time.Second
	}
	return &groupCallMediaProvider{
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		endpoint:   strings.TrimSpace(cfg.GroupCallMediaEndpoint),
		apiKey:     cfg.GroupCallMediaAPIKey,
		signingKey: cfg.GroupCallMediaSigningKey,
		rtmpHost:   strings.TrimSpace(cfg.GroupCallMediaRTMPHost),
	}, nil
}

func (p *groupCallMediaProvider) post(ctx context.Context, payload groupCallMediaRequest, idempotencyKey string) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("X-Teamgram-Group-Call-Request-Signature", groupCallMediaSignature(p.signingKey, body))
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, groupCallMediaBodyLimit+1))
	if err != nil || len(responseBody) > groupCallMediaBodyLimit || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, mtproto.ErrInternalServerError
	}
	if !verifyGroupCallMediaSignature(p.signingKey, resp.Header.Get("X-Teamgram-Group-Call-Signature"), responseBody) {
		return nil, mtproto.ErrInternalServerError
	}
	return responseBody, nil
}

func groupCallMediaSignature(key string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func verifyGroupCallMediaSignature(key, signature string, body []byte) bool {
	signature = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(signature), "sha256="))
	provided, err := hex.DecodeString(signature)
	if err != nil || len(provided) != sha256.Size || key == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}

func (c *ApiFullCore) getGroupCallStreamChannels(in *mtproto.TLPhoneGetGroupCallStreamChannels) (*mtproto.Phone_GroupCallStreamChannels, error) {
	if in == nil || in.GetCall() == nil || in.GetCall().GetId() == 0 {
		return nil, mtproto.ErrGroupCallInvalid
	}
	uid, call, _, err := c.loadAuthorizedGroupCall(in.GetCall())
	if err != nil {
		return nil, err
	}
	if !call.RtmpStream {
		return nil, mtproto.ErrGroupCallInvalid
	}
	provider, err := c.newGroupCallMediaProvider()
	if err != nil {
		return nil, err
	}
	body, err := provider.post(c.secretContext(), groupCallMediaRequest{
		Operation: "get_stream_channels",
		UserID:    uid,
		CallID:    call.ID,
		ChannelID: call.ChannelID,
	}, "")
	if err != nil {
		return nil, err
	}
	var result groupCallMediaResponse
	if json.Unmarshal(body, &result) != nil || !result.Verified || result.UserID != uid || result.CallID != call.ID || result.ChannelID != call.ChannelID || result.Channels == nil || len(result.Channels) > 32 {
		return nil, mtproto.ErrInternalServerError
	}
	seen := make(map[[2]int32]struct{}, len(result.Channels))
	channels := make([]*mtproto.GroupCallStreamChannel, 0, len(result.Channels))
	for _, channel := range result.Channels {
		key := [2]int32{channel.Channel, channel.Scale}
		if channel.Channel < 0 || channel.Scale < 0 || channel.LastTimestamp < 0 {
			return nil, mtproto.ErrInternalServerError
		}
		if _, ok := seen[key]; ok {
			return nil, mtproto.ErrInternalServerError
		}
		seen[key] = struct{}{}
		channels = append(channels, mtproto.MakeTLGroupCallStreamChannel(&mtproto.GroupCallStreamChannel{
			Channel:         channel.Channel,
			Scale:           channel.Scale,
			LastTimestampMs: channel.LastTimestamp,
		}).To_GroupCallStreamChannel())
	}
	return mtproto.MakeTLPhoneGroupCallStreamChannels(&mtproto.Phone_GroupCallStreamChannels{Channels: channels}).To_Phone_GroupCallStreamChannels(), nil
}

func (c *ApiFullCore) getGroupCallStreamRtmpUrl(in *mtproto.TLPhoneGetGroupCallStreamRtmpUrl) (*mtproto.Phone_GroupCallStreamRtmpUrl, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	channelID, err := c.groupCallChannelID(uid, in.GetPeer())
	if err != nil {
		return nil, err
	}
	call, found, err := domain.LoadRTMPGroupCallByChannel(channelID)
	if err != nil {
		return nil, err
	}
	if !found || !call.RtmpStream {
		return nil, mtproto.ErrGroupCallInvalid
	}
	channel, found, err := domain.LoadChannel(channelID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, mtproto.ErrChannelInvalid
	}
	if uid != call.Creator && uid != channel.Creator {
		member, isMember, memberErr := domain.LoadChannelMember(channelID, uid)
		if memberErr != nil {
			return nil, memberErr
		}
		if !isMember || member.AdminRights == nil || !member.AdminRights.ManageCall {
			return nil, mtproto.ErrChatAdminRequired
		}
	}
	provider, err := c.newGroupCallMediaProvider()
	if err != nil {
		return nil, err
	}
	rtmpHost, ok := canonicalGroupCallRTMPHost(provider.rtmpHost)
	if !ok {
		return nil, mtproto.ErrMethodNotImpl
	}
	revoke := mtproto.FromBool(in.GetRevoke())
	idempotencyKey := ""
	if !revoke {
		idempotencyKey = fmt.Sprintf("group-call-rtmp:%d:%d", call.ID, uid)
	}
	body, err := provider.post(c.secretContext(), groupCallMediaRequest{
		Operation: "get_rtmp_url",
		UserID:    uid,
		CallID:    call.ID,
		ChannelID: channelID,
		Revoke:    revoke,
	}, idempotencyKey)
	if err != nil {
		return nil, err
	}
	var result groupCallMediaResponse
	if json.Unmarshal(body, &result) != nil || !result.Verified || result.UserID != uid || result.CallID != call.ID || result.ChannelID != channelID || !validGroupCallRTMPResult(result.URL, result.Key, rtmpHost) {
		return nil, mtproto.ErrInternalServerError
	}
	return mtproto.MakeTLPhoneGroupCallStreamRtmpUrl(&mtproto.Phone_GroupCallStreamRtmpUrl{
		Url: result.URL,
		Key: result.Key,
	}).To_Phone_GroupCallStreamRtmpUrl(), nil
}

func canonicalGroupCallRTMPHost(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	parsed, err := url.Parse("rtmp://" + raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Host != raw {
		return "", false
	}
	return strings.ToLower(parsed.Host), true
}

func validGroupCallRTMPResult(rawURL, key, allowedHost string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (parsed.Scheme != "rtmp" && parsed.Scheme != "rtmps") || parsed.User != nil || parsed.Host == "" || !strings.EqualFold(parsed.Host, allowedHost) || parsed.Path == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	key = strings.TrimSpace(key)
	return key != "" && len(key) <= 1024 && strings.IndexFunc(key, unicode.IsControl) < 0
}
