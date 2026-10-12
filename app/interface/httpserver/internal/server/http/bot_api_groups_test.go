package http

import (
	"encoding/json"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestBotAPIInviteLinkConversion(t *testing.T) {
	invite := &mtproto.ExportedChatInvite{
		Link:          "https://t.me/+example",
		RequestNeeded: true,
		Permanent:     false,
		Revoked:       true,
		Title:         wrapperspb.String("reviewers"),
		ExpireDate:    wrapperspb.Int32(1700000000),
		UsageLimit:    wrapperspb.Int32(25),
		Requested:     wrapperspb.Int32(3),
	}
	creator := &botapi.User{ID: 42, IsBot: true}
	got := botAPIInviteLinkFromMTProto(invite, creator)
	if got == nil || got.InviteLink != invite.GetLink() || got.Creator != creator {
		t.Fatalf("invite conversion lost identity: %#v", got)
	}
	if !got.CreatesJoinRequest || got.IsPrimary || !got.IsRevoked || got.Name != "reviewers" {
		t.Fatalf("invite flags/name = %#v", got)
	}
	if got.ExpireDate != 1700000000 || got.MemberLimit != 25 || got.PendingJoinRequestCount != 3 {
		t.Fatalf("invite limits = %#v", got)
	}
}

func TestBotAPIExportedChatInviteUnion(t *testing.T) {
	invite := &mtproto.ExportedChatInvite{Link: "https://t.me/+new"}
	if got := botAPIExportedChatInvite(&mtproto.Messages_ExportedChatInvite{Invite: invite}); got != invite {
		t.Fatalf("invite union = %#v, want invite", got)
	}
	if got := botAPIExportedChatInvite(&mtproto.Messages_ExportedChatInvite{NewInvite: invite}); got != invite {
		t.Fatalf("replaced invite union = %#v, want new invite", got)
	}
}

func TestBotAPIForumTopicConversion(t *testing.T) {
	got := botAPIForumTopicFromMTProto(&mtproto.ForumTopic{
		Id:          77,
		Title:       "Support",
		IconColor:   0x123456,
		IconEmojiId: wrapperspb.Int64(987654321),
	})
	if got == nil || got.MessageThreadID != 77 || got.Name != "Support" || got.IconColor != 0x123456 || got.IconCustomEmojiID != "987654321" {
		t.Fatalf("forum topic conversion = %#v", got)
	}
}

func TestBotAPIOptionalInviteLimits(t *testing.T) {
	request := &botAPIRequest{values: map[string]json.RawMessage{
		"expire_date":  json.RawMessage("1700000000"),
		"member_limit": json.RawMessage("50"),
	}}
	expire, err := botAPIOptionalInt32(request, "expire_date", 1, 2147483647)
	if err != nil || expire == nil || expire.GetValue() != 1700000000 {
		t.Fatalf("expire_date = %v, %v", expire, err)
	}
	limit, err := botAPIOptionalInt32(request, "member_limit", 1, 99999)
	if err != nil || limit == nil || limit.GetValue() != 50 {
		t.Fatalf("member_limit = %v, %v", limit, err)
	}
	request.values["member_limit"] = json.RawMessage("100000")
	if _, err = botAPIOptionalInt32(request, "member_limit", 1, 99999); err == nil {
		t.Fatal("member_limit above Telegram limit was accepted")
	}
}
