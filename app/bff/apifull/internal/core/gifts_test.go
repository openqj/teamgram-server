package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
)

func TestCheckGiftCodeUsesVerifiedProvider(t *testing.T) {
	const uid int64 = 902010
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Operation string `json:"operation"`
			UserID    int64  `json:"user_id"`
			Slug      string `json:"slug"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode gift-code request: %v", err)
			return
		}
		if request.Operation != "check_gift_code" || request.UserID != uid || request.Slug != "gift-slug" {
			t.Errorf("gift-code request = %+v", request)
		}
		writeSignedPaymentProviderResponse(t, w, struct {
			Verified bool                              `json:"verified"`
			UserID   int64                             `json:"user_id"`
			Slug     string                            `json:"slug"`
			Code     *mtproto.Payments_CheckedGiftCode `json:"code"`
		}{true, uid, "gift-slug", &mtproto.Payments_CheckedGiftCode{Days: 30, Months: 1}})
	}))
	defer server.Close()
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Config: config.Config{PaymentProviderEndpoint: server.URL, PaymentProviderSigningKey: paymentProviderTestSigningKey}},
		MD:     &metadata.RpcMetadata{UserId: uid},
	}
	got, err := c.PaymentsCheckGiftCode(&mtproto.TLPaymentsCheckGiftCode{Slug: "gift-slug"})
	if err != nil || got == nil || got.GetDays() != 30 || got.GetMonths() != 1 {
		t.Fatalf("checked gift code = (%#v, %v)", got, err)
	}
	if err = got.Encode(mtproto.NewEncodeBuf(2048), 229); err != nil {
		t.Fatalf("checked gift code did not encode: %v", err)
	}
}

func TestGiftProviderRejectsWrongConstructors(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		result    any
		call      func(*ApiFullCore) (any, error)
	}{
		{
			name:      "upgrade preview",
			operation: "payments.getStarGiftUpgradePreview",
			result:    map[string]any{"predicate_name": "payments_resaleStarGifts"},
			call: func(c *ApiFullCore) (any, error) {
				return c.PaymentsGetStarGiftUpgradePreview(&mtproto.TLPaymentsGetStarGiftUpgradePreview{GiftId: 1})
			},
		},
		{
			name:      "resale inventory",
			operation: "payments.getResaleStarGifts",
			result:    map[string]any{"predicate_name": "payments_starGiftUpgradePreview"},
			call: func(c *ApiFullCore) (any, error) {
				return c.PaymentsGetResaleStarGifts(&mtproto.TLPaymentsGetResaleStarGifts{})
			},
		},
		{
			name:      "withdrawal URL",
			operation: "payments.getStarGiftWithdrawalUrl",
			result:    map[string]any{"predicate_name": "payments_resaleStarGifts"},
			call: func(c *ApiFullCore) (any, error) {
				return c.PaymentsGetStarGiftWithdrawalUrl(&mtproto.TLPaymentsGetStarGiftWithdrawalUrl{})
			},
		},
		{
			name:      "upgrade updates",
			operation: "payments.upgradeStarGift",
			result:    map[string]any{"predicate_name": "payments_resaleStarGifts"},
			call: func(c *ApiFullCore) (any, error) {
				return c.PaymentsUpgradeStarGift(&mtproto.TLPaymentsUpgradeStarGift{Stargift: &mtproto.InputSavedStarGift{Slug: "gift"}})
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.call(newBusinessProviderGapCore(t, tc.operation, tc.result))
			if result != nil || !paymentRPCErrorCode(err, mtproto.ErrInternalServerError) {
				t.Fatalf("wrong gift constructor = (%v, %v), want INTERNAL_SERVER_ERROR", result, err)
			}
		})
	}
}

func TestGiftHandlersUnauthed(t *testing.T) {
	for _, c := range []*ApiFullCore{{}, {MD: &metadata.RpcMetadata{}}} {
		calls := []func() error{
			func() error { _, err := c.PaymentsGetStarGifts(nil); return err },
			func() error { _, err := c.PaymentsSaveStarGift(nil); return err },
			func() error { _, err := c.PaymentsConvertStarGift(nil); return err },
			func() error { _, err := c.PaymentsGetStarGiftUpgradePreview(nil); return err },
			func() error { _, err := c.PaymentsUpgradeStarGift(nil); return err },
			func() error { _, err := c.PaymentsTransferStarGift(nil); return err },
			func() error { _, err := c.PaymentsGetUniqueStarGift(nil); return err },
			func() error { _, err := c.PaymentsGetSavedStarGifts(nil); return err },
			func() error { _, err := c.PaymentsGetSavedStarGift(nil); return err },
			func() error { _, err := c.PaymentsGetStarGiftWithdrawalUrl(nil); return err },
			func() error { _, err := c.PaymentsToggleChatStarGiftNotifications(nil); return err },
			func() error { _, err := c.PaymentsToggleStarGiftsPinnedToTop(nil); return err },
			func() error { _, err := c.PaymentsGetResaleStarGifts(nil); return err },
			func() error { _, err := c.PaymentsUpdateStarGiftPrice(nil); return err },
			func() error { _, err := c.PaymentsGetUniqueStarGiftValueInfo(nil); return err },
			func() error { _, err := c.PaymentsCheckCanSendGift(nil); return err },
			func() error { _, err := c.PaymentsGetStarGiftAuctionState(nil); return err },
			func() error { _, err := c.PaymentsGetStarGiftAuctionAcquiredGifts(nil); return err },
			func() error { _, err := c.PaymentsGetStarGiftActiveAuctions(nil); return err },
			func() error { _, err := c.PaymentsResolveStarGiftOffer(nil); return err },
			func() error { _, err := c.PaymentsSendStarGiftOffer(nil); return err },
			func() error { _, err := c.PaymentsGetStarGiftUpgradeAttributes(nil); return err },
			func() error { _, err := c.PaymentsGetCraftStarGifts(nil); return err },
			func() error { _, err := c.PaymentsCraftStarGift(nil); return err },
			func() error { _, err := c.PaymentsGetUserStarGifts(nil); return err },
			func() error { _, err := c.PaymentsGetUserStarGift(nil); return err },
			func() error { _, err := c.PaymentsCheckGiftCode(nil); return err },
			func() error { _, err := c.PaymentsApplyGiftCode(nil); return err },
			func() error { _, err := c.PaymentsCreateStarGiftCollection(nil); return err },
			func() error { _, err := c.PaymentsUpdateStarGiftCollection(nil); return err },
			func() error { _, err := c.PaymentsReorderStarGiftCollections(nil); return err },
			func() error { _, err := c.PaymentsDeleteStarGiftCollection(nil); return err },
			func() error { _, err := c.PaymentsGetStarGiftCollections(nil); return err },
			func() error { _, err := c.PaymentsGetPremiumGiftCodeOptions(nil); return err },
			func() error { _, err := c.PaymentsGetGiveawayInfo(nil); return err },
			func() error { _, err := c.PaymentsLaunchPrepaidGiveaway(nil); return err },
			func() error { _, err := c.PaymentsGetStarsGiveawayOptions(nil); return err },
		}
		for _, call := range calls {
			if err := call(); !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
				t.Fatalf("unauthed: got %v", err)
			}
		}
	}
}

func TestGiftWritesFailClosedWithoutInventory(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	calls := []struct {
		name   string
		invoke func() (bool, error)
		want   error
	}{
		{"save", func() (bool, error) {
			result, err := c.PaymentsSaveStarGift(&mtproto.TLPaymentsSaveStarGift{Stargift: &mtproto.InputSavedStarGift{Slug: "missing-gift-for-save-test"}})
			return result != nil, err
		}, mtproto.ErrMethodNotImpl},
		{"transfer", func() (bool, error) {
			result, err := c.PaymentsTransferStarGift(nil)
			return result != nil, err
		}, mtproto.ErrInputConstructorInvalid},
		{"offer", func() (bool, error) {
			result, err := c.PaymentsSendStarGiftOffer(&mtproto.TLPaymentsSendStarGiftOffer{Slug: "gift"})
			return result != nil, err
		}, mtproto.ErrMethodNotImpl},
	}
	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.invoke()
			if result || !errors.Is(err, tc.want) {
				t.Fatalf("result present=%v err=%v, want nil result and %v", result, err, tc.want)
			}
		})
	}
}

func TestGetStarGiftsRejectsNilRequest(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	result, err := c.PaymentsGetStarGifts(nil)
	if result != nil || !errors.Is(err, mtproto.ErrInputConstructorInvalid) {
		t.Fatalf("star gifts = (%#v, %v), want (nil, INPUT_CONSTRUCTOR_INVALID)", result, err)
	}
}

func TestStarGiftMappingOmitsSyntheticSticker(t *testing.T) {
	got := makeStarGift(domain.Gift{ID: 42, Slug: "aurora", Stars: 25})
	if got == nil || got.GetId() != 42 || got.GetSlug() != "aurora" || got.GetStars() != 25 {
		t.Fatalf("gift mapping = %#v, want authoritative id, slug, and stars", got)
	}
	if got.GetSticker() != nil {
		t.Fatalf("gift mapping synthesized sticker: %#v", got.GetSticker())
	}
}

func TestUserStarGiftMappingUsesLedgerFields(t *testing.T) {
	got := makeUserStarGifts([]domain.Gift{{ID: 9, From: 7, To: 11, Slug: "aurora", Stars: 25, Saved: false}})
	if len(got) != 1 || got[0].GetGift() == nil {
		t.Fatalf("user gift mapping = %#v, want one gift", got)
	}
	if got[0].GetGift().GetId() != 9 || got[0].GetGift().GetSlug() != "aurora" || got[0].GetGift().GetStars() != 25 {
		t.Fatalf("user gift mapping = %#v, want ledger gift fields", got[0])
	}
	if got[0].GetFromId().GetValue() != 7 || !got[0].GetUnsaved() || got[0].GetConvertStars().GetValue() != 25 {
		t.Fatalf("user gift metadata = %#v, want from=7 unsaved convert=25", got[0])
	}
}

func TestLocalGiftUserIDIsSelfScoped(t *testing.T) {
	self := mtproto.MakeTLInputUserSelf(&mtproto.InputUser{}).To_InputUser()
	if got, err := localGiftUserID(71, self); err != nil || got != 71 {
		t.Fatalf("self gift user = (%d, %v), want (71, nil)", got, err)
	}
	other := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 72}).To_InputUser()
	if got, err := localGiftUserID(71, other); got != 0 || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("other gift user = (%d, %v), want (0, USER_ID_INVALID)", got, err)
	}
}

func TestSavedStarGiftsRejectMalformedOffset(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	for _, offset := range []string{"-1", "letters"} {
		result, err := c.PaymentsGetSavedStarGifts(&mtproto.TLPaymentsGetSavedStarGifts{Offset: offset})
		if result != nil || !errors.Is(err, mtproto.ErrOffsetInvalid) {
			t.Fatalf("offset %q result = (%#v, %v), want (nil, OFFSET_INVALID)", offset, result, err)
		}
	}
}

func TestGiftPageAndSlice(t *testing.T) {
	records := []domain.Gift{{ID: 1}, {ID: 2}, {ID: 3}}
	start, limit, err := giftPage("1", 1)
	if err != nil || start != 1 || limit != 1 {
		t.Fatalf("giftPage = (%d, %d, %v), want (1, 1, nil)", start, limit, err)
	}
	page, next := sliceGifts(records, start, limit)
	if len(page) != 1 || page[0].ID != 2 || next != 2 {
		t.Fatalf("sliceGifts = (%+v, %d), want gift 2 and next 2", page, next)
	}
	page, next = sliceGifts(records, 3, limit)
	if len(page) != 0 || next != -1 {
		t.Fatalf("out-of-range sliceGifts = (%+v, %d), want empty and -1", page, next)
	}
}

func TestGiftPageRejectsInvalidLimit(t *testing.T) {
	if _, _, err := giftPage("0", -1); !errors.Is(err, mtproto.ErrLimitInvalid) {
		t.Fatalf("giftPage invalid limit = %v, want LIMIT_INVALID", err)
	}
}

func TestLocalGiftSlugRejectsNonCanonicalReferences(t *testing.T) {
	valid := &mtproto.InputSavedStarGift{
		PredicateName: mtproto.Predicate_inputSavedStarGiftSlug,
		Slug:          "aurora",
	}
	if got, err := localGiftSlug(valid); err != nil || got != "aurora" {
		t.Fatalf("valid slug = (%q, %v), want aurora", got, err)
	}
	tests := []struct {
		name string
		in   *mtproto.InputSavedStarGift
		want error
	}{
		{name: "nil", want: mtproto.ErrInputConstructorInvalid},
		{name: "message reference", in: &mtproto.InputSavedStarGift{PredicateName: mtproto.Predicate_inputSavedStarGiftUser, MsgId: 7}, want: mtproto.ErrMethodNotImpl},
		{name: "saved id", in: &mtproto.InputSavedStarGift{PredicateName: mtproto.Predicate_inputSavedStarGiftSlug, Slug: "aurora", SavedId: 7}, want: mtproto.ErrMethodNotImpl},
		{name: "surrounding whitespace", in: &mtproto.InputSavedStarGift{PredicateName: mtproto.Predicate_inputSavedStarGiftSlug, Slug: " aurora"}, want: mtproto.ErrInputConstructorInvalid},
		{name: "empty slug", in: &mtproto.InputSavedStarGift{PredicateName: mtproto.Predicate_inputSavedStarGiftSlug}, want: mtproto.ErrInputConstructorInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := localGiftSlug(tc.in); got != "" || !errors.Is(err, tc.want) {
				t.Fatalf("localGiftSlug() = (%q, %v), want empty and %v", got, err, tc.want)
			}
		})
	}
}

func TestTransferStarGiftRejectsMessageReferences(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 71}}
	target := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 72}).To_InputPeer()
	for _, tc := range []struct {
		name string
		gift *mtproto.InputSavedStarGift
	}{
		{name: "message id", gift: &mtproto.InputSavedStarGift{Slug: "aurora", MsgId: 9}},
		{name: "user reference", gift: &mtproto.InputSavedStarGift{
			PredicateName: mtproto.Predicate_inputSavedStarGiftUser,
			SavedId:       9,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := c.PaymentsTransferStarGift(&mtproto.TLPaymentsTransferStarGift{
				Stargift:       tc.gift,
				ToId_INPUTPEER: target,
			})
			if result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
				t.Fatalf("transfer message reference = (%#v, %v), want nil and METHOD_NOT_IMPL", result, err)
			}
		})
	}
}

func TestUserStarGiftsSaveAndConvertUseOwnedLedger(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	cleanupDB, err := persist.OpenPostgresDB(dsn)
	if err != nil {
		t.Fatalf("open cleanup connection: %v", err)
	}
	t.Cleanup(func() { _ = cleanupDB.Close() })

	uid := time.Now().UnixNano()
	from := uid + 1
	slug := fmt.Sprintf("gift-closure-%d", uid)
	t.Cleanup(func() {
		_, _ = cleanupDB.Exec(`DELETE FROM apifull_star_tx WHERE user_id=$1`, uid)
		_, _ = cleanupDB.Exec(`DELETE FROM apifull_stars WHERE user_id=$1`, uid)
		_, _ = cleanupDB.Exec(`DELETE FROM apifull_gift WHERE to_user=$1`, uid)
	})
	if err = domain.SaveGift(from, uid, slug, 17); err != nil {
		t.Fatalf("seed gift: %v", err)
	}

	self := mtproto.MakeTLInputUserSelf(&mtproto.InputUser{}).To_InputUser()
	input := mtproto.MakeTLInputSavedStarGiftSlug(&mtproto.InputSavedStarGift{Slug: slug}).To_InputSavedStarGift()
	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	listed, err := core.PaymentsGetUserStarGifts(&mtproto.TLPaymentsGetUserStarGifts{UserId: self, Limit: 10})
	if err != nil || listed.GetCount() != 1 || len(listed.GetGifts()) != 1 {
		t.Fatalf("initial gifts = (%#v, %v), want one owned gift", listed, err)
	}
	owned := listed.GetGifts()[0]
	if owned.GetFromId().GetValue() != from || owned.GetUnsaved() || owned.GetConvertStars().GetValue() != 17 {
		t.Fatalf("initial gift = %#v, want sender, saved state, and conversion value", owned)
	}

	if result, err := core.PaymentsSaveStarGift(&mtproto.TLPaymentsSaveStarGift{Unsave: true, Stargift: input}); err != nil || result != mtproto.BoolTrue {
		t.Fatalf("unsave = (%#v, %v), want true", result, err)
	}
	listed, err = core.PaymentsGetUserStarGifts(&mtproto.TLPaymentsGetUserStarGifts{UserId: self, Limit: 10})
	if err != nil || len(listed.GetGifts()) != 1 || !listed.GetGifts()[0].GetUnsaved() {
		t.Fatalf("after unsave = (%#v, %v), want one unsaved gift", listed, err)
	}
	if result, err := core.PaymentsSaveStarGift(&mtproto.TLPaymentsSaveStarGift{Stargift: input}); err != nil || result != mtproto.BoolTrue {
		t.Fatalf("resave = (%#v, %v), want true", result, err)
	}

	before, err := domain.StarsBalance(uid)
	if err != nil {
		t.Fatalf("balance before convert: %v", err)
	}
	if result, err := core.PaymentsConvertStarGift(&mtproto.TLPaymentsConvertStarGift{Stargift: input}); err != nil || result != mtproto.BoolTrue {
		t.Fatalf("convert = (%#v, %v), want true", result, err)
	}
	after, err := domain.StarsBalance(uid)
	if err != nil || after != before+17 {
		t.Fatalf("balance after convert = (%d, %v), want %d", after, err, before+17)
	}
	if result, err := core.PaymentsConvertStarGift(&mtproto.TLPaymentsConvertStarGift{Stargift: input}); err != nil || result != mtproto.BoolTrue {
		t.Fatalf("replayed convert = (%#v, %v), want idempotent true", result, err)
	}
	replayed, err := domain.StarsBalance(uid)
	if err != nil || replayed != after {
		t.Fatalf("balance after replay = (%d, %v), want unchanged %d", replayed, err, after)
	}

	other := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid + 2}}
	if result, err := other.PaymentsSaveStarGift(&mtproto.TLPaymentsSaveStarGift{Unsave: true, Stargift: input}); result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("cross-user save = (%#v, %v), want METHOD_NOT_IMPL", result, err)
	}
}

func TestUniqueStarGiftValueInfoReadsOwnedPostgresGift(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	cleanupDB, err := persist.OpenPostgresDB(dsn)
	if err != nil {
		t.Fatalf("open cleanup connection: %v", err)
	}
	defer cleanupDB.Close()
	uid := time.Now().UnixNano()
	slug := fmt.Sprintf("value-info-%d", uid)
	defer func() { _, _ = cleanupDB.Exec(`DELETE FROM apifull_gift WHERE to_user=$1`, uid) }()
	if err = domain.SaveGift(uid+1, uid, slug, 37); err != nil {
		t.Fatalf("seed gift: %v", err)
	}
	owned, err := domain.ListGifts(uid, false)
	if err != nil || len(owned) != 1 || owned[0].Slug != slug {
		t.Fatalf("seeded gifts = (%v, %v), want %q", owned, err, slug)
	}
	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	got, err := core.PaymentsGetUniqueStarGiftValueInfo(&mtproto.TLPaymentsGetUniqueStarGiftValueInfo{Slug: slug})
	if err != nil {
		t.Fatalf("value info = (%#v, %v)", got, err)
	}
	if got == nil || got.GetPredicateName() != mtproto.Predicate_payments_uniqueStarGiftValueInfo || got.GetCurrency() != "XTR" || got.GetValue() != 37 || got.GetInitialSaleStars() != 37 || got.GetInitialSalePrice() != 37 {
		t.Fatalf("value info = %#v, want owned gift's Stars value", got)
	}
	other := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid + 1}}
	if got, err = other.PaymentsGetUniqueStarGiftValueInfo(&mtproto.TLPaymentsGetUniqueStarGiftValueInfo{Slug: slug}); got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("foreign value info = (%#v, %v), want METHOD_NOT_IMPL", got, err)
	}
}
