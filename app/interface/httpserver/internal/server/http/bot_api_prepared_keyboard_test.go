// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"encoding/json"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func preparedKeyboardRequest(button string) *botAPIRequest {
	return &botAPIRequest{values: map[string]json.RawMessage{
		"button": json.RawMessage(button),
	}}
}

func TestBotAPIInputPreparedKeyboardButtonRequestUsers(t *testing.T) {
	button, err := botAPIInputPreparedKeyboardButton(preparedKeyboardRequest(`{
		"text":"Pick a user",
		"style":"danger",
		"icon_custom_emoji_id":"12345",
		"request_users":{"request_id":42,"user_is_bot":true,"user_is_premium":false,"max_quantity":3,"request_name":true,"request_username":true,"request_photo":true}
	}`))
	if err != nil {
		t.Fatalf("request_users button: %v", err)
	}
	if button.GetPredicateName() != mtproto.Predicate_inputKeyboardButtonRequestPeer || button.GetButtonId() != 42 || button.GetMaxQuantity() != 3 {
		t.Fatalf("button identity = %#v", button)
	}
	if button.GetStyle() == nil || !button.GetStyle().GetBgDanger() || button.GetStyle().GetIcon() == nil || button.GetStyle().GetIcon().GetValue() != 12345 {
		t.Fatalf("button style = %#v", button.GetStyle())
	}
	peer := button.GetPeerType()
	if peer == nil || peer.GetPredicateName() != mtproto.Predicate_requestPeerTypeUser || !mtproto.FromBool(peer.GetBot()) || mtproto.FromBool(peer.GetPremium()) {
		t.Fatalf("user peer type = %#v", peer)
	}
	if !button.GetNameRequested() || !button.GetUsernameRequested() || !button.GetPhotoRequested() {
		t.Fatalf("requested user fields were not preserved: %#v", button)
	}
}

func TestBotAPIInputPreparedKeyboardButtonRequestChat(t *testing.T) {
	button, err := botAPIInputPreparedKeyboardButton(preparedKeyboardRequest(`{
		"text":"Pick a forum",
		"style":"primary",
		"request_chat":{"request_id":-7,"chat_is_channel":false,"chat_is_forum":true,"chat_has_username":true,"chat_is_created":true,"bot_is_member":true,"request_title":true,"request_username":true,"request_photo":true,
			"user_administrator_rights":{"can_manage_chat":true,"can_delete_messages":true,"can_manage_direct_messages":true},
			"bot_administrator_rights":{"can_post_messages":true}}
	}`))
	if err != nil {
		t.Fatalf("request_chat button: %v", err)
	}
	if button.GetPredicateName() != mtproto.Predicate_inputKeyboardButtonRequestPeer || button.GetButtonId() != -7 || button.GetMaxQuantity() != 1 {
		t.Fatalf("button identity = %#v", button)
	}
	peer := button.GetPeerType()
	if peer == nil || peer.GetPredicateName() != mtproto.Predicate_requestPeerTypeChat || !peer.GetCreator() || !peer.GetBotParticipant() || !mtproto.FromBool(peer.GetHasUsername()) || !mtproto.FromBool(peer.GetForum()) {
		t.Fatalf("chat peer type = %#v", peer)
	}
	if peer.GetUserAdminRights() == nil || !peer.GetUserAdminRights().GetOther() || !peer.GetUserAdminRights().GetDeleteMessages() || !peer.GetUserAdminRights().GetManageDirectMessages() {
		t.Fatalf("user admin rights = %#v", peer.GetUserAdminRights())
	}
	if peer.GetBotAdminRights() == nil || !peer.GetBotAdminRights().GetPostMessages() {
		t.Fatalf("bot admin rights = %#v", peer.GetBotAdminRights())
	}
	if !button.GetNameRequested() || !button.GetUsernameRequested() || !button.GetPhotoRequested() || button.GetStyle() == nil || !button.GetStyle().GetBgPrimary() {
		t.Fatalf("chat button flags/style = %#v", button)
	}
}

func TestBotAPIInputPreparedKeyboardButtonRequestManagedBot(t *testing.T) {
	button, err := botAPIInputPreparedKeyboardButton(preparedKeyboardRequest(`{
		"text":"Create a bot",
		"request_managed_bot":{"request_id":9,"suggested_name":"Helper","suggested_username":"helper_bot"}
	}`))
	if err != nil {
		t.Fatalf("request_managed_bot button: %v", err)
	}
	peer := button.GetPeerType()
	if button.GetPredicateName() != mtproto.Predicate_inputKeyboardButtonRequestPeer || button.GetButtonId() != 9 || peer == nil || peer.GetPredicateName() != mtproto.Predicate_requestPeerTypeCreateBot || !peer.GetBotManaged() {
		t.Fatalf("managed bot button = %#v", button)
	}
	if peer.GetSuggestedName() == nil || peer.GetSuggestedName().GetValue() != "Helper" || peer.GetSuggestedUsername() == nil || peer.GetSuggestedUsername().GetValue() != "helper_bot" {
		t.Fatalf("managed bot suggestions = %#v", peer)
	}
}

func TestBotAPIInputPreparedKeyboardButtonRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input *botAPIRequest
	}{
		{name: "missing button", input: &botAPIRequest{values: map[string]json.RawMessage{}}},
		{name: "multiple request types", input: preparedKeyboardRequest(`{"text":"Pick","request_users":{"request_id":1},"request_chat":{"request_id":2,"chat_is_channel":false}}`)},
		{name: "missing chat_is_channel", input: preparedKeyboardRequest(`{"text":"Pick","request_chat":{"request_id":1}}`)},
		{name: "max quantity out of range", input: preparedKeyboardRequest(`{"text":"Pick","request_users":{"request_id":1,"max_quantity":11}}`)},
		{name: "request id out of range", input: preparedKeyboardRequest(`{"text":"Pick","request_users":{"request_id":2147483648}}`)},
		{name: "invalid style", input: preparedKeyboardRequest(`{"text":"Pick","style":"neutral","request_users":{"request_id":1}}`)},
		{name: "invalid icon", input: preparedKeyboardRequest(`{"text":"Pick","icon_custom_emoji_id":"nope","request_users":{"request_id":1}}`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := botAPIInputPreparedKeyboardButton(test.input); err == nil {
				t.Fatal("invalid prepared keyboard button was accepted")
			}
		})
	}
}
