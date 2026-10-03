package code

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/teamgram/teamgram-server/pkg/code/conf"
)

var (
	ErrProviderUnavailable = errors.New("verification-code provider unavailable")
	ErrDeliveryFailed      = errors.New("verification-code delivery failed")
)

type Channel string

const (
	ChannelSMS   Channel = "sms"
	ChannelEmail Channel = "email"
	ChannelApp   Channel = "app"
)

type Delivery struct {
	Channel     Channel `json:"channel"`
	Destination string  `json:"destination"`
	Code        string  `json:"code"`
	ChallengeID string  `json:"challenge_id"`
	Purpose     string  `json:"purpose"`
}

type DeliveryProvider interface {
	Ready() error
	Deliver(context.Context, Delivery) error
}

type unavailableProvider struct{}

func (unavailableProvider) Ready() error { return ErrProviderUnavailable }
func (unavailableProvider) Deliver(context.Context, Delivery) error {
	return ErrProviderUnavailable
}

type verifyCodeProvider struct {
	client VerifyCodeInterface
}

func (p verifyCodeProvider) Ready() error {
	if p.client == nil {
		return ErrProviderUnavailable
	}
	return nil
}

func (p verifyCodeProvider) Deliver(ctx context.Context, delivery Delivery) error {
	if err := p.Ready(); err != nil {
		return err
	}
	_, err := p.client.SendSmsVerifyCode(ctx, delivery.Destination, delivery.Code, delivery.ChallengeID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDeliveryFailed, err)
	}
	return nil
}

type httpProvider struct {
	endpoint   string
	key        string
	secret     string
	retryCount int
	client     *http.Client
}

func (p *httpProvider) Ready() error {
	if p == nil || strings.TrimSpace(p.endpoint) == "" {
		return ErrProviderUnavailable
	}
	parsed, err := url.Parse(strings.TrimSpace(p.endpoint))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%w: invalid HTTP endpoint", ErrProviderUnavailable)
	}
	return nil
}

func (p *httpProvider) Deliver(ctx context.Context, delivery Delivery) error {
	if err := p.Ready(); err != nil {
		return err
	}
	body, err := json.Marshal(delivery)
	if err != nil {
		return fmt.Errorf("%w: encode request: %v", ErrDeliveryFailed, err)
	}
	client := p.client
	if client == nil {
		return fmt.Errorf("%w: HTTP client unavailable", ErrProviderUnavailable)
	}
	for attempt := 0; ; attempt++ {
		req, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
		if requestErr != nil {
			return fmt.Errorf("%w: create request: %v", ErrDeliveryFailed, requestErr)
		}
		req.Header.Set("Content-Type", "application/json")
		if p.key != "" {
			req.Header.Set("Authorization", "Bearer "+p.key)
		}
		if p.secret != "" {
			req.Header.Set("X-Provider-Secret", p.secret)
		}
		resp, requestErr := client.Do(req)
		if requestErr != nil {
			if ctx.Err() != nil || attempt >= p.retryCount {
				return fmt.Errorf("%w: %v", ErrDeliveryFailed, requestErr)
			}
			continue
		}

		statusCode := resp.StatusCode
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		if statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices {
			return nil
		}
		if statusCode < http.StatusInternalServerError || attempt >= p.retryCount {
			return fmt.Errorf("%w: provider returned HTTP %d", ErrDeliveryFailed, statusCode)
		}
	}
}

func newHTTPProvider(endpoint string, c *conf.SmsVerifyCodeConfig) DeliveryProvider {
	timeout := 5 * time.Second
	if c != nil && c.ProviderTimeoutSeconds > 0 {
		timeout = time.Duration(c.ProviderTimeoutSeconds) * time.Second
	}
	p := &httpProvider{
		endpoint:   strings.TrimSpace(endpoint),
		retryCount: 0,
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
	if c != nil {
		p.key = c.Key
		p.secret = c.Secret
		if c.ProviderRetryCount > 0 {
			p.retryCount = c.ProviderRetryCount
		}
	}
	return p
}

func NewSMSProvider(c *conf.SmsVerifyCodeConfig, injected VerifyCodeInterface) DeliveryProvider {
	if injected != nil {
		return verifyCodeProvider{client: injected}
	}
	if c == nil {
		return unavailableProvider{}
	}
	name := strings.ToLower(strings.TrimSpace(c.SMSProvider))
	if name == "" {
		name = strings.ToLower(strings.TrimSpace(c.Name))
	}
	if name == "http" || name == "me" {
		return newHTTPProvider(c.SendCodeUrl, c)
	}
	return unavailableProvider{}
}

func NewEmailProvider(c *conf.SmsVerifyCodeConfig) DeliveryProvider {
	if c == nil {
		return unavailableProvider{}
	}
	name := strings.ToLower(strings.TrimSpace(c.EmailProvider))
	if name == "" {
		name = strings.ToLower(strings.TrimSpace(c.Name))
	}
	if name == "http" {
		return newHTTPProvider(c.EmailSendCodeUrl, c)
	}
	return unavailableProvider{}
}
