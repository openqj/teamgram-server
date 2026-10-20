package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func paidReactionPeerForProviderTest() *mtproto.InputPeer {
	return &mtproto.InputPeer{PredicateName: mtproto.Predicate_inputPeerUser, UserId: 7}
}

func botUserForProviderTest() *mtproto.InputUser {
	return &mtproto.InputUser{PredicateName: mtproto.Predicate_inputUser, UserId: 7, AccessHash: 9}
}

func TestPaidReactionMethodsUseSignedBusinessProvider(t *testing.T) {
	core := newBusinessProviderGapCore(t, "messages.sendPaidReaction", map[string]any{
		"predicate_name": "updates", "pts": 10, "pts_count": 1, "date": 1,
	})
	updates, err := core.MessagesSendPaidReaction(&mtproto.TLMessagesSendPaidReaction{
		Peer: paidReactionPeerForProviderTest(), MsgId: 3, Count: 2, RandomId: 4,
	})
	if err != nil || updates == nil || updates.GetPredicateName() != "updates" {
		t.Fatalf("send paid reaction = (%v, %v)", updates, err)
	}

	core = newBusinessProviderGapCore(t, "messages.togglePaidReactionPrivacy", map[string]any{"value": true})
	privacy, err := core.MessagesTogglePaidReactionPrivacy(&mtproto.TLMessagesTogglePaidReactionPrivacy{
		Peer: paidReactionPeerForProviderTest(), MsgId: 3,
	})
	if err != nil || privacy != mtproto.BoolTrue {
		t.Fatalf("toggle paid reaction privacy = (%v, %v)", privacy, err)
	}

	core = newBusinessProviderGapCore(t, "messages.getPaidReactionPrivacy", map[string]any{
		"predicate_name": "updates", "pts": 11, "pts_count": 1, "date": 1,
	})
	privacyUpdates, err := core.MessagesGetPaidReactionPrivacy(&mtproto.TLMessagesGetPaidReactionPrivacy{})
	if err != nil || privacyUpdates == nil || privacyUpdates.GetPredicateName() != "updates" {
		t.Fatalf("get paid reaction privacy = (%v, %v)", privacyUpdates, err)
	}
}

func TestPaidReactionMethodsRejectUnknownProviderConstructors(t *testing.T) {
	core := newBusinessProviderGapCore(t, "messages.sendPaidReaction", map[string]any{
		"predicate_name": "updatesUnknown",
	})
	updates, err := core.MessagesSendPaidReaction(&mtproto.TLMessagesSendPaidReaction{
		Peer: paidReactionPeerForProviderTest(), MsgId: 3, Count: 2, RandomId: 4,
	})
	if updates != nil || err != mtproto.ErrInternalServerError {
		t.Fatalf("unknown send constructor = (%v, %v), want nil and INTERNAL_SERVER_ERROR", updates, err)
	}

	core = newBusinessProviderGapCore(t, "messages.getPaidReactionPrivacy", map[string]any{
		"predicate_name": "updatesUnknown",
	})
	privacy, err := core.MessagesGetPaidReactionPrivacy(&mtproto.TLMessagesGetPaidReactionPrivacy{})
	if privacy != nil || err != mtproto.ErrInternalServerError {
		t.Fatalf("unknown privacy constructor = (%v, %v), want nil and INTERNAL_SERVER_ERROR", privacy, err)
	}
}

func TestBotGapMethodsUseSignedBusinessProvider(t *testing.T) {
	core := newBusinessProviderGapCore(t, "bots.getPopularAppBots", map[string]any{
		"predicate_name": "bots_popularAppBots", "users": []*mtproto.User{},
	})
	popular, err := core.BotsGetPopularAppBots(&mtproto.TLBotsGetPopularAppBots{Limit: 20})
	if err != nil || popular == nil || popular.GetPredicateName() != "bots_popularAppBots" {
		t.Fatalf("popular app bots = (%v, %v)", popular, err)
	}

	core = newBusinessProviderGapCore(t, "bots.setJoinChatResults", map[string]any{"value": true})
	joined, err := core.BotsSetJoinChatResults(&mtproto.TLBotsSetJoinChatResults{
		QueryId: 9, Result: &mtproto.JoinChatBotResult{PredicateName: "joinChatBotResultApproved", Url: "https://example.test/join"},
	})
	if err != nil || joined != mtproto.BoolTrue {
		t.Fatalf("set join chat results = (%v, %v)", joined, err)
	}

	core = newBusinessProviderGapCore(t, "bots.updateUserEmojiStatus", map[string]any{"value": true})
	updated, err := core.BotsUpdateUserEmojiStatus(&mtproto.TLBotsUpdateUserEmojiStatus{
		UserId: botUserForProviderTest(), EmojiStatus: mtproto.MakeTLEmojiStatus(&mtproto.EmojiStatus{DocumentId: 123}).To_EmojiStatus(),
	})
	if err != nil || updated != mtproto.BoolTrue {
		t.Fatalf("update user emoji status = (%v, %v)", updated, err)
	}

	core = newBusinessProviderGapCore(t, "bots.toggleUserEmojiStatusPermission", map[string]any{"value": true})
	permission, err := core.BotsToggleUserEmojiStatusPermission(&mtproto.TLBotsToggleUserEmojiStatusPermission{
		Bot: botUserForProviderTest(), Enabled: mtproto.BoolTrue,
	})
	if err != nil || permission != mtproto.BoolTrue {
		t.Fatalf("toggle emoji permission = (%v, %v)", permission, err)
	}
}

func TestBotRecommendationsUseSignedProviderAndLayer229Result(t *testing.T) {
	for _, predicate := range []string{mtproto.Predicate_users_users, mtproto.Predicate_users_usersSlice} {
		t.Run(predicate, func(t *testing.T) {
			core := newBusinessProviderGapCore(t, "bots.getBotRecommendations", map[string]any{
				"predicate_name": predicate,
				"users":          []any{},
				"count":          0,
			})
			got, err := core.BotsGetBotRecommendations(&mtproto.TLBotsGetBotRecommendations{Bot: miniBotInput(7, 9)})
			if err != nil || got == nil || got.GetPredicateName() != predicate {
				t.Fatalf("bot recommendations = (%v, %v)", got, err)
			}
		})
	}

	core := newBusinessProviderGapCore(t, "bots.getBotRecommendations", map[string]any{
		"predicate_name": "users_unknown",
		"users":          []any{},
	})
	got, err := core.BotsGetBotRecommendations(&mtproto.TLBotsGetBotRecommendations{Bot: miniBotInput(7, 9)})
	if got != nil || err != mtproto.ErrInternalServerError {
		t.Fatalf("unknown bot recommendation constructor = (%v, %v), want nil and INTERNAL_SERVER_ERROR", got, err)
	}
}

func TestBotPaymentResultMethodsUseSignedBusinessProvider(t *testing.T) {
	if persist.PostgresEnabled() {
		t.Skip("PostgreSQL owns bot payment query callbacks; provider fallback is tested without the local store")
	}
	core := newBusinessProviderGapCore(t, "messages.setBotShippingResults", map[string]any{"predicate_name": "boolTrue"})
	shipping, err := core.MessagesSetBotShippingResults(&mtproto.TLMessagesSetBotShippingResults{QueryId: 11})
	if err != nil || shipping == nil || shipping.GetPredicateName() != mtproto.Predicate_boolTrue {
		t.Fatalf("set bot shipping results = (%v, %v)", shipping, err)
	}

	core = newBusinessProviderGapCore(t, "messages.setBotPrecheckoutResults", map[string]any{"predicate_name": "boolTrue"})
	precheckout, err := core.MessagesSetBotPrecheckoutResults(&mtproto.TLMessagesSetBotPrecheckoutResults{QueryId: 12, Success: true})
	if err != nil || precheckout == nil || precheckout.GetPredicateName() != mtproto.Predicate_boolTrue {
		t.Fatalf("set bot precheckout results = (%v, %v)", precheckout, err)
	}
}
