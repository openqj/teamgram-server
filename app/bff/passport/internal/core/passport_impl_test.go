package core

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
	"github.com/teamgram/teamgram-server/app/bff/passport/internal/svc"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	verification "github.com/teamgram/teamgram-server/pkg/code"
)

type passportChallengeStore struct {
	mu      sync.Mutex
	records map[string]verification.Challenge
}

func (s *passportChallengeStore) Allow(context.Context, string, int, time.Duration) (bool, time.Duration, error) {
	return true, 0, nil
}

func (s *passportChallengeStore) Put(_ context.Context, key string, challenge verification.Challenge, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.records == nil {
		s.records = make(map[string]verification.Challenge)
	}
	s.records[key] = challenge
	return nil
}

func (s *passportChallengeStore) Verify(_ context.Context, key, id, digest string, now int64, maxAttempts int, consume bool) (*verification.Challenge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[key]
	if !ok {
		return nil, verification.ErrChallengeNotFound
	}
	if record.ExpiresAt <= now {
		delete(s.records, key)
		return nil, verification.ErrChallengeExpired
	}
	if record.ID != id {
		return nil, verification.ErrChallengeInvalid
	}
	if digest != record.CodeDigest {
		record.Attempts++
		if record.Attempts >= maxAttempts {
			delete(s.records, key)
		} else {
			s.records[key] = record
		}
		return nil, verification.ErrChallengeInvalid
	}
	if consume {
		delete(s.records, key)
	}
	return &record, nil
}

func (s *passportChallengeStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, key)
	return nil
}

type passportChallengeProvider struct {
	mu         sync.Mutex
	deliveries []verification.Delivery
}

func (*passportChallengeProvider) Ready() error { return nil }

func (p *passportChallengeProvider) Deliver(_ context.Context, delivery verification.Delivery) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deliveries = append(p.deliveries, delivery)
	return nil
}

func passportUserCore(id int64, bot bool) *PassportCore {
	return &PassportCore{MD: &metadata.RpcMetadata{UserId: id, IsBot: bot}}
}

type passportTestVerifier struct {
	users map[int64]*mtproto.ImmutableUser
	err   error
}

func (v *passportTestVerifier) UserGetImmutableUser(_ context.Context, in *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	if v.err != nil {
		return nil, v.err
	}
	if in == nil {
		return nil, nil
	}
	return v.users[in.GetId()], nil
}

func passportTestUser(id, accessHash int64, bot bool) *mtproto.ImmutableUser {
	user := &mtproto.UserData{Id: id, AccessHash: accessHash}
	if bot {
		user.Bot = &mtproto.BotData{}
	}
	return &mtproto.ImmutableUser{User: user}
}

func passportVerifiedUserCore(id int64, bot bool, users ...*mtproto.ImmutableUser) *PassportCore {
	verifier := &passportTestVerifier{users: make(map[int64]*mtproto.ImmutableUser, len(users))}
	for _, user := range users {
		if user != nil && user.GetUser() != nil {
			verifier.users[user.GetUser().GetId()] = user
		}
	}
	c := passportUserCore(id, bot)
	c.userVerifier = verifier
	return c
}

func mustEncode(t *testing.T, enc interface {
	Encode(*mtproto.EncodeBuf, int32) error
}) {
	t.Helper()
	buf := mtproto.NewEncodeBuf(256)
	if err := enc.Encode(buf, 228); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if buf.GetBuf() == nil && buf.GetOffset() == 0 {
		// EncodeBuf may not expose getters; a nil error is enough.
	}
}

func TestPassportRequiresUser(t *testing.T) {
	c := &PassportCore{}
	if _, err := c.AccountGetAllSecureValues(nil); err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("getAll: %v", err)
	}
	if _, err := c.AccountSaveSecureValue(nil); err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("save: %v", err)
	}
	cfg, err := c.HelpGetPassportConfig(&mtproto.TLHelpGetPassportConfig{Hash: 5})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GetPredicateName() != mtproto.Predicate_help_passportConfig {
		t.Fatalf("config predicate %q", cfg.GetPredicateName())
	}
	if cfg.GetCountriesLangs().GetPredicateName() != mtproto.Predicate_dataJSON || cfg.GetCountriesLangs().GetData() != "{}" {
		t.Fatalf("countries %+v", cfg.GetCountriesLangs())
	}
	mustEncode(t, cfg)
}

func TestSecureValueRoundTrip(t *testing.T) {
	c := passportUserCore(8801001, false)
	saved, err := c.AccountSaveSecureValue(&mtproto.TLAccountSaveSecureValue{
		SecureSecretId: 42,
		Value: &mtproto.InputSecureValue{
			Type: &mtproto.SecureValueType{Constructor: mtproto.CRC32_secureValueTypePersonalDetails},
			Data: &mtproto.SecureData{Data: []byte("alice"), DataHash: []byte("h"), Secret: []byte("s")},
			FrontSide: &mtproto.InputSecureFile{
				PredicateName: mtproto.Predicate_inputSecureFile,
				Id:            7,
				AccessHash:    9,
				FileHash:      []byte{1},
				Secret:        []byte{2},
			},
			PlainData: &mtproto.SecurePlainData{PredicateName: mtproto.Predicate_securePlainPhone, Phone: "+1"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.GetType().GetPredicateName() != mtproto.Predicate_secureValueTypePersonalDetails {
		t.Fatalf("type %q", saved.GetType().GetPredicateName())
	}
	if saved.GetFrontSide().GetId() != 7 || len(saved.GetHash()) != 32 {
		t.Fatalf("file/hash")
	}
	mustEncode(t, saved)

	_, err = c.AccountSaveSecureValue(&mtproto.TLAccountSaveSecureValue{
		Value: &mtproto.InputSecureValue{
			Type: &mtproto.SecureValueType{PredicateName: mtproto.Predicate_secureValueTypeEmail},
			Data: &mtproto.SecureData{Data: []byte("a@b.c")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.AccountSaveSecureValue(&mtproto.TLAccountSaveSecureValue{
		Value: &mtproto.InputSecureValue{},
	}); err != mtproto.ErrTypesEmpty {
		t.Fatalf("empty type: %v", err)
	}

	one, err := c.AccountGetSecureValue(&mtproto.TLAccountGetSecureValue{
		Types: []*mtproto.SecureValueType{{
			PredicateName: mtproto.Predicate_secureValueTypePersonalDetails,
		}},
	})
	if err != nil || len(one.Datas) != 1 || !bytes.Equal(one.Datas[0].GetData().GetData(), []byte("alice")) {
		t.Fatalf("get %+v %v", one, err)
	}
	all, err := c.AccountGetAllSecureValues(nil)
	if err != nil || len(all.Datas) != 2 {
		t.Fatalf("all %d %v", len(all.GetDatas()), err)
	}
	if _, err = c.AccountDeleteSecureValue(&mtproto.TLAccountDeleteSecureValue{
		Types: []*mtproto.SecureValueType{{Constructor: mtproto.CRC32_secureValueTypeEmail}},
	}); err != nil {
		t.Fatal(err)
	}
	all, err = c.AccountGetAllSecureValues(nil)
	if err != nil || len(all.Datas) != 1 {
		t.Fatalf("after delete %d %v", len(all.GetDatas()), err)
	}
	doc, err := passportLoad(8801001)
	if err != nil || doc.SecureSecretID != 42 {
		t.Fatalf("secret id %v %v", doc, err)
	}
}

func TestAuthorizationForm(t *testing.T) {
	c := passportVerifiedUserCore(8801002, false, passportTestUser(9, 9009, true))
	if _, err := c.AccountSaveSecureValue(&mtproto.TLAccountSaveSecureValue{
		Value: &mtproto.InputSecureValue{
			Type: &mtproto.SecureValueType{PredicateName: mtproto.Predicate_secureValueTypePassport},
			Data: &mtproto.SecureData{Data: []byte("P")},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AccountSaveSecureValue(&mtproto.TLAccountSaveSecureValue{
		Value: &mtproto.InputSecureValue{
			Type: &mtproto.SecureValueType{PredicateName: mtproto.Predicate_secureValueTypeAddress},
			Data: &mtproto.SecureData{Data: []byte("A")},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AccountGetAuthorizationForm(&mtproto.TLAccountGetAuthorizationForm{
		Scope: `{"v":1,"nonce":"n","data":[{"type":"passport"}]}`,
	}); err != mtproto.ErrBotInvalid {
		t.Fatalf("bot: %v", err)
	}
	if _, err := passportUserCore(8801012, false).AccountGetAuthorizationForm(&mtproto.TLAccountGetAuthorizationForm{
		BotId: 9, PublicKey: "pk",
	}); err != mtproto.ErrBotInvalid {
		t.Fatalf("unverified bot: %v", err)
	}
	if _, err := passportVerifiedUserCore(8801013, false, passportTestUser(10, 9010, false)).AccountGetAuthorizationForm(&mtproto.TLAccountGetAuthorizationForm{
		BotId: 10, PublicKey: "pk",
	}); err != mtproto.ErrBotInvalid {
		t.Fatalf("non-bot target: %v", err)
	}
	if _, err := c.AccountGetAuthorizationForm(&mtproto.TLAccountGetAuthorizationForm{
		BotId: 9,
	}); err != mtproto.ErrPublicKeyRequired {
		t.Fatalf("nonce: %v", err)
	}
	form, err := c.AccountGetAuthorizationForm(&mtproto.TLAccountGetAuthorizationForm{
		BotId: 9,
		Scope: `{"v":1,"nonce":"n","data":[{"type":"passport","selfie":true},{"one_of":[{"type":"address","translation":true},{"type":"nope"}]}]}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if form.GetPredicateName() != mtproto.Predicate_account_authorizationForm {
		t.Fatalf("form %q", form.GetPredicateName())
	}
	if len(form.GetValues()) != 2 || form.GetValues()[0].GetType().GetPredicateName() != mtproto.Predicate_secureValueTypePassport || form.GetValues()[1].GetType().GetPredicateName() != mtproto.Predicate_secureValueTypeAddress {
		t.Fatalf("filtered values %d", len(form.GetValues()))
	}
	if len(form.GetRequiredTypes()) != 2 || !form.GetRequiredTypes()[0].GetSelfieRequired() {
		t.Fatalf("required %+v", form.GetRequiredTypes())
	}
	mustEncode(t, form)
	form, err = c.AccountGetAuthorizationForm(&mtproto.TLAccountGetAuthorizationForm{
		BotId: 9, PublicKey: "pk", Scope: "not-json",
	})
	if err != nil || len(form.GetValues()) != 2 {
		t.Fatalf("fallback %d %v", len(form.GetValues()), err)
	}
	before, err := persist.Default.Get(passportKey(8801002))
	if err != nil {
		t.Fatal(err)
	}
	ok, err := c.AccountAcceptAuthorization(&mtproto.TLAccountAcceptAuthorization{
		BotId: 9, Scope: `{"nonce":"n","data":[]}`, PublicKey: "pk",
		Credentials: &mtproto.SecureCredentialsEncrypted{Data: []byte{3}},
	})
	if err != mtproto.ErrMethodNotImpl || ok != nil {
		t.Fatalf("accept %v %v", ok, err)
	}
	after, err := persist.Default.Get(passportKey(8801002))
	if err != nil || after != before {
		t.Fatalf("accept wrote state: %q %v", after, err)
	}
	left, err := c.AccountGetAllSecureValues(nil)
	if err != nil || len(left.Datas) != 2 {
		t.Fatalf("accept wiped values")
	}
}

func TestSetSecureValueErrors(t *testing.T) {
	const (
		botID      = 8801003
		targetID   = 8801004
		targetHash = 8802004
	)
	if err := passportSave(targetID, &passportDoc{
		Values: map[string]*mtproto.SecureValue{},
		Errors: []*mtproto.SecureValueError{{PredicateName: mtproto.Predicate_secureValueError, Text: "old"}},
		Phones: map[string]passportPhone{},
	}); err != nil {
		t.Fatal(err)
	}

	c := passportUserCore(botID, false)
	_, err := c.UsersSetSecureValueErrors(&mtproto.TLUsersSetSecureValueErrors{
		Id: &mtproto.InputUser{PredicateName: mtproto.Predicate_inputUser, UserId: targetID, AccessHash: targetHash},
		Errors: []*mtproto.SecureValueError{{
			PredicateName: mtproto.Predicate_secureValueError,
			Text:          "bad",
		}},
	})
	if err != mtproto.ErrUserBotInvalid {
		t.Fatalf("non-bot: %v", err)
	}
	doc, err := passportLoad(targetID)
	if err != nil || len(doc.Errors) != 1 || doc.Errors[0].GetText() != "old" {
		t.Fatalf("non-bot wrote errors: %+v %v", doc, err)
	}

	unverifiedBot := passportUserCore(botID, true)
	if _, err = unverifiedBot.UsersSetSecureValueErrors(&mtproto.TLUsersSetSecureValueErrors{
		Id:     &mtproto.InputUser{PredicateName: mtproto.Predicate_inputUser, UserId: targetID, AccessHash: targetHash},
		Errors: []*mtproto.SecureValueError{{PredicateName: mtproto.Predicate_secureValueErrorData, Text: "bad"}},
	}); err != mtproto.ErrUserBotInvalid {
		t.Fatalf("unverified bot: %v", err)
	}
	doc, err = passportLoad(targetID)
	if err != nil || len(doc.Errors) != 1 || doc.Errors[0].GetText() != "old" {
		t.Fatalf("unverified bot wrote errors: %+v %v", doc, err)
	}

	bot := passportVerifiedUserCore(botID, true,
		passportTestUser(botID, 8802003, true),
		passportTestUser(targetID, targetHash, false),
	)
	if _, err = bot.UsersSetSecureValueErrors(nil); err != mtproto.ErrUserIdInvalid {
		t.Fatalf("nil id: %v", err)
	}
	if _, err = bot.UsersSetSecureValueErrors(&mtproto.TLUsersSetSecureValueErrors{
		Id:     &mtproto.InputUser{PredicateName: mtproto.Predicate_inputUser, UserId: targetID, AccessHash: targetHash + 1},
		Errors: []*mtproto.SecureValueError{{PredicateName: mtproto.Predicate_secureValueErrorData, Text: "bad"}},
	}); err != mtproto.ErrUserIdInvalid {
		t.Fatalf("bad target hash: %v", err)
	}
	doc, err = passportLoad(targetID)
	if err != nil || len(doc.Errors) != 1 || doc.Errors[0].GetText() != "old" {
		t.Fatalf("bad target hash wrote errors: %+v %v", doc, err)
	}
	ok, err := bot.UsersSetSecureValueErrors(&mtproto.TLUsersSetSecureValueErrors{
		Id:     &mtproto.InputUser{PredicateName: mtproto.Predicate_inputUser, UserId: targetID, AccessHash: targetHash},
		Errors: []*mtproto.SecureValueError{{PredicateName: mtproto.Predicate_secureValueErrorData, Text: "x"}},
	})
	if err != nil || ok != mtproto.BoolTrue {
		t.Fatalf("bot: %v %v", ok, err)
	}
	doc, err = passportLoad(targetID)
	if err != nil || len(doc.Errors) != 1 || doc.Errors[0].GetText() != "x" {
		t.Fatalf("target errors")
	}
}

func TestVerifyPhoneFailsClosedWithoutChallengeService(t *testing.T) {
	c := passportUserCore(8801005, false)
	if _, err := c.AccountSendVerifyPhoneCode(&mtproto.TLAccountSendVerifyPhoneCode{PhoneNumber: "  "}); err != mtproto.ErrPhoneNumberInvalid {
		t.Fatalf("empty phone: %v", err)
	}
	sent, err := c.AccountSendVerifyPhoneCode(&mtproto.TLAccountSendVerifyPhoneCode{PhoneNumber: " +1555 "})
	if sent != nil || err != mtproto.ErrSendCodeUnavailable {
		t.Fatalf("send = (%v, %v), want (nil, SEND_CODE_UNAVAILABLE)", sent, err)
	}
	doc, err := passportLoad(8801005)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := doc.Phones["+1555"]; ok {
		t.Fatal("provider-unavailable send persisted a local challenge")
	}
	if _, err = c.AccountVerifyPhone(&mtproto.TLAccountVerifyPhone{PhoneCode: "1"}); err != mtproto.ErrPhoneNumberInvalid {
		t.Fatalf("verify empty phone: %v", err)
	}
	if _, err = c.AccountVerifyPhone(&mtproto.TLAccountVerifyPhone{
		PhoneNumber: "+1555", PhoneCode: "123456",
	}); err != mtproto.ErrPhoneCodeHashEmpty {
		t.Fatalf("empty hash: %v", err)
	}
	if _, err = c.AccountVerifyPhone(&mtproto.TLAccountVerifyPhone{
		PhoneNumber: "+1555", PhoneCodeHash: "hash",
	}); err != mtproto.ErrPhoneCodeEmpty {
		t.Fatalf("empty code: %v", err)
	}
	if _, err = c.AccountVerifyPhone(&mtproto.TLAccountVerifyPhone{
		PhoneNumber: "+1555", PhoneCodeHash: "hash", PhoneCode: "123456",
	}); err != mtproto.ErrPhoneCodeExpired {
		t.Fatalf("unissued code: %v", err)
	}
}

func TestVerifyPhoneConsumesChallengeAndPersistsHash(t *testing.T) {
	const userID int64 = 8801014
	const secret = "passport-test-secret"
	store := &passportChallengeStore{}
	provider := &passportChallengeProvider{}
	service := verification.NewChallengeService(store, verification.ChallengeSettings{
		TTL: time.Minute, RateLimit: 3, RateWindow: time.Minute, MaxAttempts: 3, Secret: secret,
	}, provider, nil)
	c := passportUserCore(userID, false)
	c.ctx = context.Background()
	c.svcCtx = &svc.ServiceContext{Challenges: service}

	sent, err := c.AccountSendVerifyPhoneCode(&mtproto.TLAccountSendVerifyPhoneCode{PhoneNumber: "+14155552671"})
	if err != nil {
		t.Fatal(err)
	}
	if sent == nil || sent.GetPhoneCodeHash() == "" || len(provider.deliveries) != 1 {
		t.Fatalf("sent = %+v, deliveries = %+v", sent, provider.deliveries)
	}
	if _, err = c.AccountVerifyPhone(&mtproto.TLAccountVerifyPhone{
		PhoneNumber: "+14155552671", PhoneCodeHash: sent.GetPhoneCodeHash(), PhoneCode: "00000",
	}); !errors.Is(err, mtproto.ErrPhoneCodeInvalid) {
		t.Fatalf("wrong code = %v", err)
	}
	if ok, err := c.AccountVerifyPhone(&mtproto.TLAccountVerifyPhone{
		PhoneNumber: "+14155552671", PhoneCodeHash: sent.GetPhoneCodeHash(), PhoneCode: provider.deliveries[0].Code,
	}); err != nil || ok != mtproto.BoolTrue {
		t.Fatalf("correct code = %v, %v", ok, err)
	}
	if _, err = c.AccountVerifyPhone(&mtproto.TLAccountVerifyPhone{
		PhoneNumber: "+14155552671", PhoneCodeHash: sent.GetPhoneCodeHash(), PhoneCode: provider.deliveries[0].Code,
	}); !errors.Is(err, mtproto.ErrPhoneCodeExpired) {
		t.Fatalf("replay = %v", err)
	}
	doc, err := passportLoad(userID)
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.Phones["14155552671"].Hash; got != sent.GetPhoneCodeHash() {
		t.Fatalf("stored phone hash = %q, want %q", got, sent.GetPhoneCodeHash())
	}
}
