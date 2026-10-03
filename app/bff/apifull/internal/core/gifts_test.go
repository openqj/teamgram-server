package core

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

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
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if dsn == "" {
		t.Skip("APIFULL_MYSQL_DSN is not configured")
	}
	cleanupDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open cleanup connection: %v", err)
	}
	defer cleanupDB.Close()

	uid := time.Now().UnixNano()
	from := uid + 1
	slug := fmt.Sprintf("gift-closure-%d", uid)
	if err = domain.SaveGift(from, uid, slug, 17); err != nil {
		t.Fatalf("seed gift: %v", err)
	}
	t.Cleanup(func() {
		_, _ = cleanupDB.Exec(`DELETE FROM apifull_star_tx WHERE user_id=?`, uid)
		_, _ = cleanupDB.Exec(`DELETE FROM apifull_stars WHERE user_id=?`, uid)
		_, _ = cleanupDB.Exec(`DELETE FROM apifull_gift WHERE to_user=?`, uid)
	})

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
