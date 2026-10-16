package http

import (
	"encoding/json"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
)

func TestBotAPIQueryIDParsing(t *testing.T) {
	request := &botAPIRequest{values: map[string]json.RawMessage{
		"query_id": json.RawMessage(`"9001"`),
	}}
	if got, err := parseBotAPIQueryID(request, "query_id"); err != nil || got != 9001 {
		t.Fatalf("query id = (%d, %v), want (9001, nil)", got, err)
	}
	for _, raw := range []string{"", "0", "-1", "not-a-number"} {
		request.values["query_id"] = json.RawMessage(raw)
		if got, err := parseBotAPIQueryID(request, "query_id"); err == nil || got != 0 {
			t.Fatalf("query id %q = (%d, %v), want invalid", raw, got, err)
		}
	}
}

func TestJoinChatBotResultConstructors(t *testing.T) {
	for _, test := range []struct {
		name      string
		predicate string
		url       string
	}{
		{name: "approve", predicate: mtproto.Predicate_joinChatBotResultApproved},
		{name: "decline", predicate: mtproto.Predicate_joinChatBotResultDeclined},
		{name: "queue", predicate: mtproto.Predicate_joinChatBotResultQueued},
		{name: "web_view", predicate: mtproto.Predicate_joinChatBotResultWebView, url: "https://example.test/app"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := joinChatBotResult(test.name, test.url)
			if err != nil || result == nil || result.GetPredicateName() != test.predicate || result.GetUrl() != test.url {
				t.Fatalf("result = (%v, %v), want predicate %q and url %q", result, err, test.predicate, test.url)
			}
		})
	}
	if _, err := joinChatBotResult("web_view", ""); err == nil {
		t.Fatal("web_view without URL was accepted")
	}
}

func TestBotAPIChatMemberTagValidation(t *testing.T) {
	for _, test := range []struct {
		value string
		want  bool
	}{
		{value: "support", want: false},
		{value: "😀", want: true},
		{value: "⭐", want: true},
		{value: "\u200d", want: true},
	} {
		if got := botAPIContainsEmoji(test.value); got != test.want {
			t.Fatalf("botAPIContainsEmoji(%q) = %v, want %v", test.value, got, test.want)
		}
	}
}

func TestBotAPIInviteLinkIncludesSubscriptionPricing(t *testing.T) {
	invite := mtproto.MakeTLChatInviteExported(&mtproto.ExportedChatInvite{
		Link:                "https://t.me/+subscription",
		SubscriptionPricing: &mtproto.StarsSubscriptionPricing{Period: int32(botAPIChatSubscriptionPeriod), Amount: 250},
	}).To_ExportedChatInvite()
	converted := botAPIInviteLinkFromMTProto(invite, &botapi.User{ID: 42, IsBot: true})
	if converted == nil || converted.SubscriptionPeriod != int(botAPIChatSubscriptionPeriod) || converted.SubscriptionPrice != 250 {
		t.Fatalf("converted subscription invite = %+v", converted)
	}
}
