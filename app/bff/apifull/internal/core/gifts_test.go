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
	"github.com/teamgram/teamgram-server/pkg/rpc/businessctx"
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

func TestBusinessTransferStarGiftUsesProviderScope(t *testing.T) {
	core := newBusinessProviderGapCore(t, "payments.transferStarGift", map[string]any{
		"predicate_name": "updates",
		"updates":        []any{},
		"users":          []any{},
		"chats":          []any{},
		"date":           1760000000,
		"seq":            0,
	})
	core.ctx = businessctx.WithScope(context.Background(), businessctx.Scope{
		ConnectionID:     "ptb-business-connection",
		OwnerID:          42,
		CanTransferGifts: true,
	})
	core.MD = &metadata.RpcMetadata{UserId: 900000000001}
	updates, err := core.PaymentsTransferStarGift(&mtproto.TLPaymentsTransferStarGift{
		Stargift: mtproto.MakeTLInputSavedStarGiftChat(&mtproto.InputSavedStarGift{
			Peer:    mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
			SavedId: 77,
		}).To_InputSavedStarGift(),
		ToId_INPUTPEER: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 8, AccessHash: 8008}).To_InputPeer(),
	})
	if err != nil || updates == nil || updates.GetPredicateName() != mtproto.Predicate_updates {
		t.Fatalf("business transfer provider result = (%#v, %v)", updates, err)
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

func TestStarGiftCatalogHashReturnsNotModifiedForStableCatalog(t *testing.T) {
	records := []domain.Gift{
		{ID: 41, Slug: "aurora", Stars: 25},
		{ID: 42, Slug: "cobalt", Stars: 50},
	}
	first := starGiftCatalogResult(records, 0)
	if first == nil || first.GetPredicateName() != mtproto.Predicate_payments_starGifts || first.GetHash() == 0 || len(first.GetGifts()) != len(records) {
		t.Fatalf("initial catalog result = %#v, want typed StarGifts with a nonzero hash", first)
	}
	second := starGiftCatalogResult(records, first.GetHash())
	if second == nil || second.GetPredicateName() != mtproto.Predicate_payments_starGiftsNotModified || second.GetHash() != first.GetHash() || len(second.GetGifts()) != 0 {
		t.Fatalf("matching catalog result = %#v, want StarGiftsNotModified with the same hash", second)
	}
	changed := starGiftCatalogResult(append(records, domain.Gift{ID: 43, Slug: "dawn", Stars: 75}), first.GetHash())
	if changed == nil || changed.GetPredicateName() != mtproto.Predicate_payments_starGifts || changed.GetHash() == first.GetHash() || len(changed.GetGifts()) != 3 {
		t.Fatalf("changed catalog result = %#v, want refreshed StarGifts with a new hash", changed)
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

func TestResolveGiftUserIDAllowsBotTargetButKeepsNativeSelfScope(t *testing.T) {
	target := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 72, AccessHash: 7200}).To_InputUser()
	if got, err := resolveGiftUserID(71, target, true); err != nil || got != 72 {
		t.Fatalf("bot gift user = (%d, %v), want (72, nil)", got, err)
	}
	if got, err := resolveGiftUserID(71, target, false); got != 0 || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("native gift user = (%d, %v), want (0, USER_ID_INVALID)", got, err)
	}
}

func TestSavedStarGiftsRejectMalformedOffset(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	selfPeer := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
	for _, offset := range []string{"-1", "letters"} {
		result, err := c.PaymentsGetSavedStarGifts(&mtproto.TLPaymentsGetSavedStarGifts{Peer: selfPeer, Offset: offset})
		if result != nil || !errors.Is(err, mtproto.ErrOffsetInvalid) {
			t.Fatalf("offset %q result = (%#v, %v), want (nil, OFFSET_INVALID)", offset, result, err)
		}
	}
}

func TestSavedStarGiftsRequireOwnerPeerAndAccessHash(t *testing.T) {
	const uid int64 = 904301
	zeroHashPeer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: uid}).To_InputPeer()
	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	tests := []struct {
		name string
		peer *mtproto.InputPeer
	}{
		{name: "missing peer"},
		{name: "zero access hash", peer: zeroHashPeer},
		{name: "other owner", peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: uid + 1, AccessHash: uid + 1}).To_InputPeer()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := core.PaymentsGetSavedStarGifts(&mtproto.TLPaymentsGetSavedStarGifts{Peer: tc.peer})
			if result != nil || !errors.Is(err, mtproto.ErrPeerIdInvalid) {
				t.Fatalf("saved gifts = (%#v, %v), want (nil, PEER_ID_INVALID)", result, err)
			}
		})
	}
}

func TestSavedStarGiftsEmptyProjectionAllowsWebAFilters(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	if err := domain.OpenPostgresReadOnly(dsn); err != nil {
		t.Fatalf("open postgres domain: %v", err)
	}
	owner := time.Now().UnixNano()
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: owner, AccessHash: owner + 100}).To_InputPeer()
	result, err := (&ApiFullCore{MD: &metadata.RpcMetadata{UserId: owner}}).PaymentsGetSavedStarGifts(&mtproto.TLPaymentsGetSavedStarGifts{
		ExcludeUnlimited:    true,
		ExcludeUpgradable:   true,
		ExcludeUnupgradable: true,
		Peer:                peer,
	})
	if err != nil || result == nil || result.GetPredicateName() != mtproto.Predicate_payments_savedStarGifts || result.GetCount() != 0 || len(result.GetGifts()) != 0 {
		t.Fatalf("empty filtered saved gifts = (%#v, %v), want typed empty SavedStarGifts", result, err)
	}
	if err = result.Encode(mtproto.NewEncodeBuf(1024), 229); err != nil {
		t.Fatalf("empty filtered saved gifts did not encode: %v", err)
	}
}

func TestStarGiftActiveAuctionsWithoutProviderReturnsEmptyInitialResult(t *testing.T) {
	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 42}}
	result, err := core.PaymentsGetStarGiftActiveAuctions(&mtproto.TLPaymentsGetStarGiftActiveAuctions{})
	if err != nil || result == nil || result.GetPredicateName() != mtproto.Predicate_payments_starGiftActiveAuctions || len(result.GetAuctions()) != 0 || len(result.GetUsers()) != 0 || len(result.GetChats()) != 0 {
		t.Fatalf("empty active auctions = (%#v, %v), want typed empty StarGiftActiveAuctions", result, err)
	}
	if err = result.Encode(mtproto.NewEncodeBuf(1024), 229); err != nil {
		t.Fatalf("empty active auctions did not encode: %v", err)
	}
	if result, err = core.PaymentsGetStarGiftActiveAuctions(&mtproto.TLPaymentsGetStarGiftActiveAuctions{Hash: 1}); result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("cached active auctions without provider = (%#v, %v), want METHOD_NOT_IMPL", result, err)
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
	page, next = sliceGifts(records, -1, 1)
	if len(page) != 0 || next != -1 {
		t.Fatalf("negative-start sliceGifts = (%+v, %d), want empty and -1", page, next)
	}
	page, next = sliceGifts(records, 1, int(^uint(0)>>1))
	if len(page) != 2 || page[0].ID != 2 || next != -1 {
		t.Fatalf("large-limit sliceGifts = (%+v, %d), want remaining gifts and -1", page, next)
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

func TestLocalSavedGiftIDScopesActor(t *testing.T) {
	const ownerID int64 = 71
	self := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
	ownedUser := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: ownerID, AccessHash: 7101}).To_InputPeer()
	foreignUser := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: ownerID + 1, AccessHash: 7102}).To_InputPeer()
	for _, tc := range []struct {
		name      string
		peer      *mtproto.InputPeer
		wantID    int64
		wantByID  bool
		wantError error
	}{
		{name: "self", peer: self, wantID: 9, wantByID: true},
		{name: "owner user", peer: ownedUser, wantID: 10, wantByID: true},
		{name: "foreign user", peer: foreignUser, wantID: 9, wantError: mtproto.ErrPeerIdInvalid},
		{name: "missing peer", wantID: 9, wantError: mtproto.ErrPeerIdInvalid},
		{name: "slug reference", peer: self, wantByID: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := &mtproto.InputSavedStarGift{PredicateName: mtproto.Predicate_inputSavedStarGiftChat, Peer: tc.peer, SavedId: tc.wantID}
			if tc.name == "slug reference" {
				input = &mtproto.InputSavedStarGift{PredicateName: mtproto.Predicate_inputSavedStarGiftSlug, Slug: "aurora"}
			}
			gotID, gotByID, err := localSavedGiftID(ownerID, input)
			if tc.wantError != nil {
				if gotID != 0 || gotByID || !errors.Is(err, tc.wantError) {
					t.Fatalf("localSavedGiftID() = (%d, %v, %v), want error %v", gotID, gotByID, err, tc.wantError)
				}
				return
			}
			if err != nil || gotID != tc.wantID || gotByID != tc.wantByID {
				t.Fatalf("localSavedGiftID() = (%d, %v, %v), want (%d, %v, nil)", gotID, gotByID, err, tc.wantID, tc.wantByID)
			}
		})
	}
}

func TestLocalSavedGiftReferenceAcceptsBotNumericAndSlugForms(t *testing.T) {
	const ownerID int64 = 71
	self := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
	tests := []struct {
		name     string
		input    *mtproto.InputSavedStarGift
		wantID   int64
		wantSlug string
		wantErr  error
	}{
		{
			name:   "numeric bot reference",
			input:  mtproto.MakeTLInputSavedStarGiftChat(&mtproto.InputSavedStarGift{Peer: self, SavedId: 9}).To_InputSavedStarGift(),
			wantID: 9,
		},
		{
			name:     "slug reference",
			input:    mtproto.MakeTLInputSavedStarGiftSlug(&mtproto.InputSavedStarGift{Slug: "aurora"}).To_InputSavedStarGift(),
			wantSlug: "aurora",
		},
		{
			name:    "message reference",
			input:   &mtproto.InputSavedStarGift{PredicateName: mtproto.Predicate_inputSavedStarGiftChat, Peer: self, SavedId: 9, MsgId: 3},
			wantErr: mtproto.ErrMethodNotImpl,
		},
		{
			name:    "foreign owner",
			input:   mtproto.MakeTLInputSavedStarGiftChat(&mtproto.InputSavedStarGift{Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: ownerID + 1, AccessHash: 1}).To_InputPeer(), SavedId: 9}).To_InputSavedStarGift(),
			wantErr: mtproto.ErrPeerIdInvalid,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			id, slug, err := localSavedGiftReference(ownerID, tc.input)
			if tc.wantErr != nil {
				if id != 0 || slug != "" || !errors.Is(err, tc.wantErr) {
					t.Fatalf("localSavedGiftReference() = (%d, %q, %v), want %v", id, slug, err, tc.wantErr)
				}
				return
			}
			if err != nil || id != tc.wantID || slug != tc.wantSlug {
				t.Fatalf("localSavedGiftReference() = (%d, %q, %v), want (%d, %q, nil)", id, slug, err, tc.wantID, tc.wantSlug)
			}
		})
	}
}

func TestLocalGiftRecipientKeySupportsChatAndChannel(t *testing.T) {
	const selfID int64 = 71
	tests := []struct {
		name string
		peer *mtproto.InputPeer
		want int64
	}{
		{name: "chat", peer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 77}).To_InputPeer(), want: -77},
		{name: "channel", peer: mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 88, AccessHash: 99}).To_InputPeer(), want: -1000000000088},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := localGiftRecipientKey(selfID, nil, tc.peer); err != nil || got != tc.want {
				t.Fatalf("localGiftRecipientKey() = (%d, %v), want (%d, nil)", got, err, tc.want)
			}
		})
	}
}

func TestLocalGiftRecipientKeyRejectsZeroHashPeerUser(t *testing.T) {
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 72}).To_InputPeer()
	if got, err := localGiftRecipientKey(71, nil, peer); got != 0 || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("localGiftRecipientKey() = (%d, %v), want USER_ID_INVALID for zero access hash", got, err)
	}
}

func TestLocalGiftRecipientKeyRequiresUserAccessHash(t *testing.T) {
	user := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 72}).To_InputUser()
	if got, err := localGiftRecipientKey(71, user, nil); got != 0 || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("localGiftRecipientKey() = (%d, %v), want USER_ID_INVALID for zero access hash", got, err)
	}
}

func TestGiftTransferTargetRequiresUserAccessHash(t *testing.T) {
	zeroHashPeer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 72}).To_InputPeer()
	if got, err := giftTransferTarget(71, &mtproto.TLPaymentsTransferStarGift{ToId_INPUTPEER: zeroHashPeer}); got != 0 || !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("giftTransferTarget(peer) = (%d, %v), want PEER_ID_INVALID", got, err)
	}
	zeroHashUser := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 72}).To_InputUser()
	if got, err := giftTransferTarget(71, &mtproto.TLPaymentsTransferStarGift{ToId_INPUTUSER: zeroHashUser}); got != 0 || !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("giftTransferTarget(user) = (%d, %v), want PEER_ID_INVALID", got, err)
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
	numericInput := mtproto.MakeTLInputSavedStarGiftChat(&mtproto.InputSavedStarGift{
		Peer:    mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		SavedId: owned.GetGift().GetId(),
	}).To_InputSavedStarGift()
	singular, err := core.PaymentsGetSavedStarGift(&mtproto.TLPaymentsGetSavedStarGift{Stargift: []*mtproto.InputSavedStarGift{numericInput}})
	if err != nil || singular == nil || singular.GetCount() != 1 || len(singular.GetGifts()) != 1 || singular.GetGifts()[0].GetSavedId().GetValue() != owned.GetGift().GetId() {
		t.Fatalf("numeric saved-gift lookup = (%#v, %v), want one matching gift", singular, err)
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
	if result, err := core.PaymentsConvertStarGift(&mtproto.TLPaymentsConvertStarGift{Stargift: numericInput}); err != nil || result != mtproto.BoolTrue {
		t.Fatalf("convert = (%#v, %v), want true", result, err)
	}
	after, err := domain.StarsBalance(uid)
	if err != nil || after != before+17 {
		t.Fatalf("balance after convert = (%d, %v), want %d", after, err, before+17)
	}
	if result, err := core.PaymentsConvertStarGift(&mtproto.TLPaymentsConvertStarGift{Stargift: numericInput}); err != nil || result != mtproto.BoolTrue {
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

func TestUserStarGiftsTransferUsesNumericSavedID(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	cleanupDB, err := persist.OpenPostgresDB(dsn)
	if err != nil {
		t.Fatalf("open cleanup connection: %v", err)
	}
	t.Cleanup(func() { _ = cleanupDB.Close() })

	owner := time.Now().UnixNano()
	recipient := owner + 1
	from := owner + 2
	slug := fmt.Sprintf("gift-transfer-id-%d", owner)
	t.Cleanup(func() {
		_, _ = cleanupDB.Exec(`DELETE FROM apifull_gift WHERE to_user IN ($1,$2) OR slug=$3`, owner, recipient, slug)
	})
	if err = domain.SaveGift(from, owner, slug, 19); err != nil {
		t.Fatalf("seed gift: %v", err)
	}
	records, err := domain.ListGifts(owner, false)
	if err != nil || len(records) != 1 || records[0].Slug != slug {
		t.Fatalf("seeded gifts = (%v, %v), want one gift %q", records, err, slug)
	}
	stargift := mtproto.MakeTLInputSavedStarGiftChat(&mtproto.InputSavedStarGift{
		Peer:    mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		SavedId: records[0].ID,
	}).To_InputSavedStarGift()
	target := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: recipient, AccessHash: recipient + 100}).To_InputPeer()
	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: owner}}
	updates, err := core.PaymentsTransferStarGift(&mtproto.TLPaymentsTransferStarGift{Stargift: stargift, ToId_INPUTPEER: target})
	if err != nil || updates == nil {
		t.Fatalf("numeric transfer = (%#v, %v), want updates", updates, err)
	}
	owned, err := domain.ListGifts(owner, false)
	if err != nil || len(owned) != 0 {
		t.Fatalf("owner gifts after transfer = (%v, %v), want empty", owned, err)
	}
	moved, err := domain.ListGifts(recipient, false)
	if err != nil || len(moved) != 1 || moved[0].ID != records[0].ID || moved[0].Slug != slug {
		t.Fatalf("recipient gifts after transfer = (%v, %v), want moved gift", moved, err)
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
