package dao

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestPrivacyRulesAllow(t *testing.T) {
	allowAll := &mtproto.PrivacyRule{PredicateName: mtproto.Predicate_privacyValueAllowAll}
	denyAll := &mtproto.PrivacyRule{PredicateName: mtproto.Predicate_privacyValueDisallowAll}
	allowContacts := &mtproto.PrivacyRule{PredicateName: mtproto.Predicate_privacyValueAllowContacts}
	denyContacts := &mtproto.PrivacyRule{PredicateName: mtproto.Predicate_privacyValueDisallowContacts}
	allowUser := &mtproto.PrivacyRule{PredicateName: mtproto.Predicate_privacyValueAllowUsers, Users: []int64{42}}
	denyUser := &mtproto.PrivacyRule{PredicateName: mtproto.Predicate_privacyValueDisallowUsers, Users: []int64{42}}
	allowChats := &mtproto.PrivacyRule{PredicateName: mtproto.Predicate_privacyValueAllowChatParticipants, Chats: []int64{123}}
	denyChats := &mtproto.PrivacyRule{PredicateName: mtproto.Predicate_privacyValueDisallowChatParticipants, Chats: []int64{123}}
	tests := []struct {
		name    string
		rules   []*mtproto.PrivacyRule
		peer    privacyUserContext
		member  bool
		chatErr error
		allow   bool
		wantErr error
	}{
		{name: "empty denies"},
		{name: "allow everyone", rules: []*mtproto.PrivacyRule{allowAll}, allow: true},
		{name: "deny everyone", rules: []*mtproto.PrivacyRule{denyAll}},
		{name: "allow contact", rules: []*mtproto.PrivacyRule{allowContacts}, peer: privacyUserContext{contact: true}, allow: true},
		{name: "noncontact denied", rules: []*mtproto.PrivacyRule{allowContacts}},
		{name: "deny contact", rules: []*mtproto.PrivacyRule{allowAll, denyContacts}, peer: privacyUserContext{contact: true}},
		{name: "allow explicit user", rules: []*mtproto.PrivacyRule{allowUser, denyAll}, allow: true},
		{name: "deny explicit user", rules: []*mtproto.PrivacyRule{denyUser, allowAll}},
		{name: "first exception allows", rules: []*mtproto.PrivacyRule{allowUser, denyUser, denyAll}, allow: true},
		{name: "first exception denies", rules: []*mtproto.PrivacyRule{denyUser, allowUser, allowAll}},
		{name: "explicit user overrides contact denial", rules: []*mtproto.PrivacyRule{allowUser, denyContacts, denyAll}, peer: privacyUserContext{contact: true}, allow: true},
		{name: "contact denial overrides later explicit user", rules: []*mtproto.PrivacyRule{denyContacts, allowUser, allowAll}, peer: privacyUserContext{contact: true}},
		{name: "client trailing close friend", rules: []*mtproto.PrivacyRule{denyAll, {PredicateName: mtproto.Predicate_privacyValueAllowCloseFriends}}, peer: privacyUserContext{closeFriend: true}, allow: true},
		{name: "client trailing premium", rules: []*mtproto.PrivacyRule{allowContacts, {PredicateName: mtproto.Predicate_privacyValueAllowPremium}}, peer: privacyUserContext{premium: true}, allow: true},
		{name: "premium nonmatch", rules: []*mtproto.PrivacyRule{denyAll, {PredicateName: mtproto.Predicate_privacyValueAllowPremium}}},
		{name: "client trailing bot", rules: []*mtproto.PrivacyRule{denyAll, {PredicateName: mtproto.Predicate_privacyValueAllowBots}}, peer: privacyUserContext{bot: true}, allow: true},
		{name: "deny bot", rules: []*mtproto.PrivacyRule{allowAll, {PredicateName: mtproto.Predicate_privacyValueDisallowBots}}, peer: privacyUserContext{bot: true}},
		{name: "bot nonmatch", rules: []*mtproto.PrivacyRule{denyAll, {PredicateName: mtproto.Predicate_privacyValueAllowBots}}},
		{name: "allow participant", rules: []*mtproto.PrivacyRule{allowChats, denyAll}, member: true, allow: true},
		{name: "deny participant", rules: []*mtproto.PrivacyRule{denyChats, allowAll}, member: true},
		{name: "nonparticipant", rules: []*mtproto.PrivacyRule{allowChats, denyAll}},
		{name: "empty chat list", rules: []*mtproto.PrivacyRule{{PredicateName: mtproto.Predicate_privacyValueAllowChatParticipants}, denyAll}, member: true},
		{name: "chat failure", rules: []*mtproto.PrivacyRule{allowChats, allowAll}, chatErr: context.Canceled, wantErr: context.Canceled},
		{name: "earlier match skips chat", rules: []*mtproto.PrivacyRule{allowUser, allowChats, denyAll}, chatErr: context.Canceled, allow: true},
		{name: "nil rule", rules: []*mtproto.PrivacyRule{nil}, wantErr: mtproto.ErrPrivacyValueInvalid},
		{name: "unknown rule", rules: []*mtproto.PrivacyRule{{PredicateName: "unknown"}}, wantErr: mtproto.ErrPrivacyValueInvalid},
		{name: "duplicate base", rules: []*mtproto.PrivacyRule{allowAll, denyAll}, wantErr: mtproto.ErrPrivacyValueInvalid},
		{name: "invalid user id", rules: []*mtproto.PrivacyRule{{PredicateName: mtproto.Predicate_privacyValueAllowUsers, Users: []int64{0}}}, wantErr: mtproto.ErrPrivacyValueInvalid},
		{name: "invalid chat id", rules: []*mtproto.PrivacyRule{{PredicateName: mtproto.Predicate_privacyValueAllowChatParticipants, Chats: []int64{-1}}}, wantErr: mtproto.ErrPrivacyValueInvalid},
		{name: "unexpected users", rules: []*mtproto.PrivacyRule{{PredicateName: mtproto.Predicate_privacyValueAllowAll, Users: []int64{42}}}, wantErr: mtproto.ErrPrivacyValueInvalid},
		{name: "unexpected chats", rules: []*mtproto.PrivacyRule{{PredicateName: mtproto.Predicate_privacyValueAllowUsers, Chats: []int64{123}}}, wantErr: mtproto.ErrPrivacyValueInvalid},
		{name: "matching list without base", rules: []*mtproto.PrivacyRule{allowUser}, allow: true},
		{name: "unmatched list without base", rules: []*mtproto.PrivacyRule{{PredicateName: mtproto.Predicate_privacyValueAllowUsers, Users: []int64{43}}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := privacyRulesAllow(tc.rules, 42, tc.peer, func(ids []int64) (bool, error) {
				if len(ids) != 1 || ids[0] != 123 {
					t.Fatalf("chat IDs = %v", ids)
				}
				return tc.member, tc.chatErr
			})
			if got != tc.allow || !errors.Is(err, tc.wantErr) {
				t.Fatalf("privacy = (%v, %v), want (%v, %v)", got, err, tc.allow, tc.wantErr)
			}
		})
	}
}
