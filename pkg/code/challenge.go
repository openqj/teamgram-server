package code

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/teamgram-server/pkg/code/conf"
)

var (
	ErrChallengeNotFound       = errors.New("verification challenge not found")
	ErrChallengeExpired        = errors.New("verification challenge expired")
	ErrChallengeInvalid        = errors.New("verification code invalid")
	ErrChallengeKeyUnavailable = errors.New("verification challenge key unavailable")
	ErrRateLimited             = errors.New("verification-code rate limit exceeded")
)

type Challenge struct {
	ID          string  `json:"id"`
	Channel     Channel `json:"channel"`
	Purpose     string  `json:"purpose"`
	Subject     string  `json:"subject"`
	Scope       string  `json:"scope"`
	Salt        string  `json:"salt"`
	CodeDigest  string  `json:"code_digest"`
	ExpiresAt   int64   `json:"expires_at"`
	Attempts    int     `json:"attempts"`
	MaxAttempts int     `json:"max_attempts"`
}

type ChallengeStore interface {
	Allow(context.Context, string, int, time.Duration) (bool, time.Duration, error)
	Put(context.Context, string, Challenge, time.Duration) error
	Verify(context.Context, string, string, string, int64, int, bool) (*Challenge, error)
	Delete(context.Context, string) error
}

type ChallengeSettings struct {
	TTL           time.Duration
	RateLimit     int
	RateWindow    time.Duration
	MaxAttempts   int
	Secret        string
	RequireSecret bool
}

func normalizeChallengeSettings(settings ChallengeSettings) ChallengeSettings {
	if settings.TTL <= 0 {
		settings.TTL = 3 * time.Minute
	}
	if settings.RateLimit <= 0 {
		settings.RateLimit = 3
	}
	if settings.RateWindow <= 0 {
		settings.RateWindow = 10 * time.Minute
	}
	if settings.MaxAttempts <= 0 {
		settings.MaxAttempts = 5
	}
	return settings
}

func ChallengeSettingsFromConfig(c *conf.SmsVerifyCodeConfig) ChallengeSettings {
	settings := ChallengeSettings{
		TTL:           3 * time.Minute,
		RateLimit:     3,
		RateWindow:    10 * time.Minute,
		MaxAttempts:   5,
		RequireSecret: true,
	}
	if c == nil {
		return settings
	}
	if c.ChallengeTTLSeconds > 0 {
		settings.TTL = time.Duration(c.ChallengeTTLSeconds) * time.Second
	}
	if c.RateLimit > 0 {
		settings.RateLimit = c.RateLimit
	}
	if c.RateWindowSeconds > 0 {
		settings.RateWindow = time.Duration(c.RateWindowSeconds) * time.Second
	}
	if c.MaxAttempts > 0 {
		settings.MaxAttempts = c.MaxAttempts
	}
	settings.Secret = c.ChallengeSecret
	return settings
}

type ChallengeService struct {
	store    ChallengeStore
	sms      DeliveryProvider
	email    DeliveryProvider
	settings ChallengeSettings
	now      func() time.Time
}

func NewChallengeService(store ChallengeStore, settings ChallengeSettings, sms, email DeliveryProvider) *ChallengeService {
	if sms == nil {
		sms = unavailableProvider{}
	}
	if email == nil {
		email = unavailableProvider{}
	}
	return &ChallengeService{
		store: store, sms: sms, email: email, settings: normalizeChallengeSettings(settings), now: time.Now,
	}
}

type IssueRequest struct {
	Channel     Channel
	Purpose     string
	Subject     string
	Scope       string
	ChallengeID string
	Code        string
	CodeLength  int
	TTL         time.Duration
}

type IssuedChallenge struct {
	ID         string
	Code       string
	ExpiresAt  time.Time
	RetryAfter time.Duration
}

type VerifyRequest struct {
	Channel     Channel
	Purpose     string
	Scope       string
	ChallengeID string
	Code        string
}

func (s *ChallengeService) Issue(ctx context.Context, req IssueRequest) (*IssuedChallenge, error) {
	if s == nil || s.store == nil {
		return nil, ErrProviderUnavailable
	}
	req.Purpose = strings.TrimSpace(req.Purpose)
	req.Subject = strings.TrimSpace(req.Subject)
	req.Scope = strings.TrimSpace(req.Scope)
	if req.Purpose == "" || req.Subject == "" || req.Scope == "" {
		return nil, ErrChallengeInvalid
	}
	provider, err := s.provider(req.Channel)
	if err != nil {
		return nil, err
	}
	if provider != nil {
		if err = provider.Ready(); err != nil {
			return nil, err
		}
	}
	if s.settings.RequireSecret && len(s.settings.Secret) < 32 {
		return nil, ErrChallengeKeyUnavailable
	}
	allowed, retryAfter, err := s.store.Allow(ctx, rateKey(req.Channel, req.Subject), s.settings.RateLimit, s.settings.RateWindow)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return &IssuedChallenge{RetryAfter: retryAfter}, ErrRateLimited
	}
	if req.ChallengeID == "" {
		req.ChallengeID, err = randomHex(16)
		if err != nil {
			return nil, err
		}
	}
	if req.CodeLength <= 0 {
		req.CodeLength = 5
	}
	if req.CodeLength > 32 {
		return nil, ErrChallengeInvalid
	}
	if req.Code == "" {
		req.Code, err = randomNumeric(req.CodeLength)
		if err != nil {
			return nil, err
		}
	}
	deliveryID := ""
	if provider != nil {
		deliveryID, err = randomHex(16)
		if err != nil {
			return nil, err
		}
	}
	ttl := req.TTL
	if ttl <= 0 {
		ttl = s.settings.TTL
	}
	now := s.now()
	record := Challenge{
		ID: req.ChallengeID, Channel: req.Channel, Purpose: req.Purpose,
		Subject: req.Subject, Scope: req.Scope, Salt: req.ChallengeID,
		CodeDigest: digestCode(s.settings.Secret, req.ChallengeID, req.Code),
		ExpiresAt:  now.Add(ttl).Unix(), MaxAttempts: s.settings.MaxAttempts,
	}
	key := challengeKey(req.Channel, req.Purpose, req.Scope, req.ChallengeID)
	if err = s.store.Put(ctx, key, record, ttl); err != nil {
		return nil, err
	}
	if provider != nil {
		err = provider.Deliver(ctx, Delivery{
			Channel: req.Channel, DeliveryID: deliveryID, Destination: req.Subject, Code: req.Code,
			ChallengeID: req.ChallengeID, Purpose: req.Purpose,
		})
		if err != nil {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			_ = s.store.Delete(cleanupCtx, key)
			cancel()
			return nil, err
		}
	}
	return &IssuedChallenge{ID: req.ChallengeID, Code: req.Code, ExpiresAt: now.Add(ttl)}, nil
}

func (s *ChallengeService) Check(ctx context.Context, req VerifyRequest) (*Challenge, error) {
	return s.verify(ctx, req, false)
}

func (s *ChallengeService) Consume(ctx context.Context, req VerifyRequest) (*Challenge, error) {
	return s.verify(ctx, req, true)
}

func (s *ChallengeService) Revoke(ctx context.Context, req VerifyRequest) error {
	if s == nil || s.store == nil || req.Purpose == "" || req.Scope == "" || req.ChallengeID == "" {
		return nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	return s.store.Delete(cleanupCtx, challengeKey(req.Channel, req.Purpose, req.Scope, req.ChallengeID))
}

func (s *ChallengeService) verify(ctx context.Context, req VerifyRequest, consume bool) (*Challenge, error) {
	if s == nil || s.store == nil {
		return nil, ErrChallengeNotFound
	}
	if req.Purpose == "" || req.Scope == "" || req.ChallengeID == "" || req.Code == "" {
		return nil, ErrChallengeInvalid
	}
	if s.settings.RequireSecret && len(s.settings.Secret) < 32 {
		return nil, ErrChallengeKeyUnavailable
	}
	key := challengeKey(req.Channel, req.Purpose, req.Scope, req.ChallengeID)
	// The store compares the HMAC atomically and deletes only a successful consume.
	digest := digestCode(s.settings.Secret, req.ChallengeID, req.Code)
	record, err := s.store.Verify(ctx, key, req.ChallengeID, digest, s.now().Unix(), s.settings.MaxAttempts, consume)
	if err != nil {
		return nil, err
	}
	expected := digestCode(s.settings.Secret, record.Salt, req.Code)
	if subtle.ConstantTimeCompare([]byte(expected), []byte(record.CodeDigest)) != 1 {
		return nil, ErrChallengeInvalid
	}
	return record, nil
}

func (s *ChallengeService) provider(channel Channel) (DeliveryProvider, error) {
	switch channel {
	case ChannelSMS:
		return s.sms, nil
	case ChannelEmail:
		return s.email, nil
	case ChannelApp:
		return nil, nil
	default:
		return nil, ErrProviderUnavailable
	}
}

func challengeKey(channel Channel, purpose, scope, id string) string {
	sum := sha256.Sum256([]byte(string(channel) + "\x00" + purpose + "\x00" + scope + "\x00" + id))
	return "verification:challenge:" + hex.EncodeToString(sum[:])
}

func rateKey(channel Channel, subject string) string {
	sum := sha256.Sum256([]byte(string(channel) + "\x00" + strings.ToLower(strings.TrimSpace(subject))))
	return "verification:rate:" + hex.EncodeToString(sum[:])
}

func digestCode(secret, salt, value string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(salt + "\x00" + value))
	return hex.EncodeToString(mac.Sum(nil))
}

func randomHex(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func randomNumeric(length int) (string, error) {
	if length <= 0 {
		return "", ErrChallengeInvalid
	}
	b := make([]byte, length)
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = byte('0' + buf[i]%10)
	}
	return string(b), nil
}

func ScopeID(value int64) string { return strconv.FormatInt(value, 10) }

func PurposeID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:16])
}

func ProviderError(err error) bool {
	return errors.Is(err, ErrProviderUnavailable) || errors.Is(err, ErrDeliveryFailed) || errors.Is(err, ErrChallengeKeyUnavailable)
}

func RateLimitSeconds(err error, issued *IssuedChallenge) int {
	if !errors.Is(err, ErrRateLimited) || issued == nil || issued.RetryAfter <= 0 {
		return 0
	}
	seconds := int(issued.RetryAfter.Round(time.Second) / time.Second)
	if seconds < 1 {
		return 1
	}
	return seconds
}

func ValidateDestination(channel Channel, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%w: empty %s destination", ErrChallengeInvalid, channel)
	}
	return nil
}
