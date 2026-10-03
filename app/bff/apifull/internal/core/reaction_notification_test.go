// Copyright 2026 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

type reactionNotifySettingsStore struct {
	values map[string]string
}

func (s *reactionNotifySettingsStore) Get(key string) (string, error) {
	return s.values[key], nil
}

func (s *reactionNotifySettingsStore) Set(key, value string) error {
	s.values[key] = value
	return nil
}

func TestReactionNotifySettingsRoundTrip(t *testing.T) {
	const userID int64 = 81019001
	store := &reactionNotifySettingsStore{values: make(map[string]string)}
	previous := persist.Default
	persist.Use(store)
	t.Cleanup(func() { persist.Use(previous) })

	settings := mtproto.MakeTLReactionsNotifySettings(&mtproto.ReactionsNotifySettings{
		MessagesNotifyFrom:  mtproto.MakeTLReactionNotificationsFromContacts(nil).To_ReactionNotificationsFrom(),
		StoriesNotifyFrom:   mtproto.MakeTLReactionNotificationsFromAll(nil).To_ReactionNotificationsFrom(),
		PollVotesNotifyFrom: mtproto.MakeTLReactionNotificationsFromContacts(nil).To_ReactionNotificationsFrom(),
		Sound: mtproto.MakeTLNotificationSoundLocal(&mtproto.NotificationSound{
			Title: "reaction tone",
			Data:  "reaction-data",
		}).To_NotificationSound(),
		ShowPreviews: mtproto.BoolTrue,
	}).To_ReactionsNotifySettings()

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}
	if _, err := c.AccountSetReactionsNotifySettings(&mtproto.TLAccountSetReactionsNotifySettings{Settings: settings}); err != nil {
		t.Fatalf("set reaction notification settings: %v", err)
	}

	got, err := c.AccountGetReactionsNotifySettings(nil)
	if err != nil {
		t.Fatalf("get reaction notification settings: %v", err)
	}
	if got.GetMessagesNotifyFrom().GetPredicateName() != mtproto.Predicate_reactionNotificationsFromContacts {
		t.Fatalf("messages notify source = %q", got.GetMessagesNotifyFrom().GetPredicateName())
	}
	if got.GetStoriesNotifyFrom().GetPredicateName() != mtproto.Predicate_reactionNotificationsFromAll {
		t.Fatalf("stories notify source = %q", got.GetStoriesNotifyFrom().GetPredicateName())
	}
	if got.GetPollVotesNotifyFrom().GetPredicateName() != mtproto.Predicate_reactionNotificationsFromContacts {
		t.Fatalf("poll votes notify source = %q", got.GetPollVotesNotifyFrom().GetPredicateName())
	}
	if got.GetSound().GetPredicateName() != mtproto.Predicate_notificationSoundLocal {
		t.Fatalf("sound predicate = %q", got.GetSound().GetPredicateName())
	}
	if got.GetSound().GetTitle() != "reaction tone" || got.GetSound().GetData() != "reaction-data" {
		t.Fatalf("sound payload = title %q data %q", got.GetSound().GetTitle(), got.GetSound().GetData())
	}
	if !mtproto.FromBool(got.GetShowPreviews()) {
		t.Fatalf("show previews = %v, want true", got.GetShowPreviews())
	}

	for _, tc := range []struct {
		name      string
		sound     *mtproto.NotificationSound
		predicate string
		id        int64
	}{
		{
			name:      "default",
			sound:     mtproto.MakeTLNotificationSoundDefault(nil).To_NotificationSound(),
			predicate: mtproto.Predicate_notificationSoundDefault,
		},
		{
			name:      "none",
			sound:     mtproto.MakeTLNotificationSoundNone(nil).To_NotificationSound(),
			predicate: mtproto.Predicate_notificationSoundNone,
		},
		{
			name:      "ringtone",
			sound:     mtproto.MakeTLNotificationSoundRingtone(&mtproto.NotificationSound{Id: 81019002}).To_NotificationSound(),
			predicate: mtproto.Predicate_notificationSoundRingtone,
			id:        81019002,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := c.AccountSetReactionsNotifySettings(&mtproto.TLAccountSetReactionsNotifySettings{
				Settings: mtproto.MakeTLReactionsNotifySettings(&mtproto.ReactionsNotifySettings{
					Sound: tc.sound,
				}).To_ReactionsNotifySettings(),
			}); err != nil {
				t.Fatalf("set reaction notification settings: %v", err)
			}
			got, err := c.AccountGetReactionsNotifySettings(nil)
			if err != nil {
				t.Fatalf("get reaction notification settings: %v", err)
			}
			if got.GetSound().GetPredicateName() != tc.predicate || got.GetSound().GetId() != tc.id {
				t.Fatalf("sound = predicate %q id %d, want predicate %q id %d", got.GetSound().GetPredicateName(), got.GetSound().GetId(), tc.predicate, tc.id)
			}
		})
	}

	defaults, err := c.AccountSetReactionsNotifySettings(&mtproto.TLAccountSetReactionsNotifySettings{})
	if err != nil {
		t.Fatalf("set default reaction notification settings: %v", err)
	}
	if defaults.GetSound().GetPredicateName() != mtproto.Predicate_notificationSoundRingtone || defaults.GetSound().GetId() != 0 || mtproto.FromBool(defaults.GetShowPreviews()) {
		t.Fatalf("default reaction notification settings = %#v", defaults)
	}
	got, err = c.AccountGetReactionsNotifySettings(nil)
	if err != nil {
		t.Fatalf("get default reaction notification settings: %v", err)
	}
	if got.GetSound().GetPredicateName() != mtproto.Predicate_notificationSoundRingtone || got.GetSound().GetId() != 0 || mtproto.FromBool(got.GetShowPreviews()) {
		t.Fatalf("stored default reaction notification settings = %#v", got)
	}
}
