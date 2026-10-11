package core

import (
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestHistoryImportMethodsUseSignedProvider(t *testing.T) {
	core := newBusinessProviderGapCore(t, "messages.checkHistoryImport", map[string]any{
		"pm": true, "group": false, "title": "export",
	})
	parsed, err := core.MessagesCheckHistoryImport(&mtproto.TLMessagesCheckHistoryImport{ImportHead: "export.json"})
	if err != nil || parsed == nil || !parsed.GetPm() || parsed.GetGroup() || parsed.GetTitle() == nil || parsed.GetTitle().GetValue() != "export" {
		t.Fatalf("check history import = (%v, %v)", parsed, err)
	}

	core = newBusinessProviderGapCore(t, "messages.initHistoryImport", map[string]any{"import_id": int64(7001)})
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 43}).To_InputPeer()
	previousMeta, err := persist.Default.Get(historyImportMetaKey(42))
	if err != nil {
		t.Fatal(err)
	}
	init, err := core.MessagesInitHistoryImport(&mtproto.TLMessagesInitHistoryImport{
		Peer: peer, File: &mtproto.InputFile{Id_INT64: 12, Parts: 1, Name: "export.json"}, MediaCount: 2,
	})
	if err != nil || init == nil || init.GetId() != 7001 {
		t.Fatalf("init history import = (%v, %v)", init, err)
	}
	t.Cleanup(func() {
		_ = persist.Default.Set(historyImportMetaKey(42), previousMeta)
	})

	core = newBusinessProviderGapCore(t, "messages.startHistoryImport", map[string]any{"accepted": true})
	started, err := core.MessagesStartHistoryImport(&mtproto.TLMessagesStartHistoryImport{Peer: peer, ImportId: 7001})
	if err != nil || started != mtproto.BoolTrue {
		t.Fatalf("start history import = (%v, %v)", started, err)
	}

	core = newBusinessProviderGapCore(t, "messages.checkHistoryImportPeer", map[string]any{"confirm_text": "Import into this chat?"})
	checked, err := core.MessagesCheckHistoryImportPeer(&mtproto.TLMessagesCheckHistoryImportPeer{Peer: peer})
	if err != nil || checked == nil || checked.GetConfirmText() != "Import into this chat?" {
		t.Fatalf("check history import peer = (%v, %v)", checked, err)
	}
}

func TestStatsPublicForwardsMethodsUseSignedProvider(t *testing.T) {
	const owner int64 = 42
	channelID := time.Now().UnixNano()
	accessHash := channelID + 1
	if err := domain.SaveChannel(domain.Channel{ID: channelID, AccessHash: accessHash, Creator: owner, Title: "stats-provider", Broadcast: true}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = domain.DeleteChannel(owner, channelID) })
	message, err := domain.InsertChannelMessage(channelID, owner, 0, "forward-source")
	if err != nil {
		t.Fatal(err)
	}
	row := message.MessageID
	input := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: accessHash}).To_InputChannel()
	result := map[string]any{"count": 1, "forwards": []any{}, "users": []any{}, "chats": []any{}}
	core := newBusinessProviderGapCore(t, "stats.getMessagePublicForwards", result)
	forwards, err := core.StatsGetMessagePublicForwards5F150144(&mtproto.TLStatsGetMessagePublicForwards5F150144{Channel: input, MsgId: row, Limit: 10})
	if err != nil || forwards == nil || forwards.GetCount() != 1 || forwards.GetForwards() == nil {
		t.Fatalf("message public forwards = (%v, %v)", forwards, err)
	}

	core = newBusinessProviderGapCore(t, "stats.getMessagePublicForwardsLegacy", map[string]any{"predicate_name": "messages_messages", "messages": []any{}, "users": []any{}, "chats": []any{}})
	legacy, err := core.StatsGetMessagePublicForwards5630281B(&mtproto.TLStatsGetMessagePublicForwards5630281B{Channel: input, MsgId: row, Limit: 10})
	if err != nil || legacy == nil || legacy.GetMessages() == nil {
		t.Fatalf("legacy public forwards = (%v, %v)", legacy, err)
	}

	const storyID int32 = 77
	previousStories, err := persist.Default.Get(storyKey(owner))
	if err != nil {
		t.Fatal(err)
	}
	if err := saveUserStories(owner, &userStoryStore{
		Next: storyID, Order: []int32{storyID}, Items: map[int32]*mtproto.StoryItem{storyID: mtproto.MakeTLStoryItem(&mtproto.StoryItem{
			Id: storyID, Date: 1, Out: true, Media: mtproto.MakeTLMessageMediaEmpty(nil).To_MessageMedia(),
		}).To_StoryItem()},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = persist.Default.Set(storyKey(owner), previousStories) })

	core = newBusinessProviderGapCore(t, "stats.getStoryPublicForwards", result)
	story, err := core.StatsGetStoryPublicForwards(&mtproto.TLStatsGetStoryPublicForwards{
		Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(), Id: storyID, Limit: 10,
	})
	if err != nil || story == nil || story.GetCount() != 1 || story.GetForwards() == nil {
		t.Fatalf("story public forwards = (%v, %v)", story, err)
	}
}

func TestPlatformMethodsUseSignedProvider(t *testing.T) {
	core := newBusinessProviderGapCore(t, "fragment.getCollectibleInfo", map[string]any{
		"purchase_date": 1700000000, "currency": "USD", "amount": 100, "url": "https://fragment.example/item",
	})
	fragment, err := core.FragmentGetCollectibleInfo(&mtproto.TLFragmentGetCollectibleInfo{Collectible: &mtproto.InputCollectible{Username: "collectible"}})
	if err != nil || fragment == nil || fragment.GetAmount() != 100 || fragment.GetCurrency() != "USD" {
		t.Fatalf("fragment collectible info = (%v, %v)", fragment, err)
	}

	core = newBusinessProviderGapCore(t, "stories.startLive", map[string]any{"predicate_name": "updates", "date": 1})
	live, err := core.StoriesStartLive(&mtproto.TLStoriesStartLive{Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(), RandomId: 1})
	if err != nil || live == nil || live.GetPredicateName() != mtproto.Predicate_updates {
		t.Fatalf("stories start live = (%v, %v)", live, err)
	}

	core = newBusinessProviderGapCore(t, "phone.getGroupCallStars", map[string]any{"total_stars": 12, "top_donors": []any{}, "chats": []any{}, "users": []any{}})
	stars, err := core.PhoneGetGroupCallStars(&mtproto.TLPhoneGetGroupCallStars{Call: &mtproto.InputGroupCall{Id: 8, AccessHash: 9}})
	if err != nil || stars == nil || stars.GetTotalStars() != 12 || stars.GetTopDonors() == nil {
		t.Fatalf("group call stars = (%v, %v)", stars, err)
	}

	core = newBusinessProviderGapCore(t, "ephemeral.getCallbackAnswer", map[string]any{
		"predicate_name": "messages_botCallbackAnswer", "cache_time": 10,
	})
	answer, err := core.EphemeralGetCallbackAnswer(&mtproto.TLEphemeralGetCallbackAnswer{Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(), Id: 1, Data: []byte("ok")})
	if err != nil || answer == nil || answer.GetCacheTime() != 10 {
		t.Fatalf("ephemeral callback answer = (%v, %v)", answer, err)
	}
}

func TestPlatformProviderRejectsInvalidResponses(t *testing.T) {
	core := newBusinessProviderGapCore(t, "fragment.getCollectibleInfo", map[string]any{"purchase_date": 0, "currency": "USD", "amount": 1, "url": "https://fragment.example/item"})
	if got, err := core.FragmentGetCollectibleInfo(&mtproto.TLFragmentGetCollectibleInfo{Collectible: &mtproto.InputCollectible{Username: "collectible"}}); got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("invalid fragment response = (%v, %v)", got, err)
	}

	core = newBusinessProviderGapCore(t, "phone.getGroupCallStars", map[string]any{"total_stars": -1})
	if got, err := core.PhoneGetGroupCallStars(&mtproto.TLPhoneGetGroupCallStars{Call: &mtproto.InputGroupCall{Id: 8, AccessHash: 9}}); got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("invalid group stars response = (%v, %v)", got, err)
	}

	core = newBusinessProviderGapCore(t, "ephemeral.getCallbackAnswer", map[string]any{"predicate_name": "messages_botCallbackAnswer", "has_url": true, "cache_time": 1})
	if got, err := core.EphemeralGetCallbackAnswer(&mtproto.TLEphemeralGetCallbackAnswer{Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(), Id: 1}); got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("invalid callback response = (%v, %v)", got, err)
	}
}
