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
	"encoding/json"
	"strconv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// RPCReactionNotificationServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

type reactionNotifySettingsRecord struct {
	MessagesNotifyFrom  string `json:"messages_notify_from,omitempty"`
	StoriesNotifyFrom   string `json:"stories_notify_from,omitempty"`
	PollVotesNotifyFrom string `json:"poll_votes_notify_from,omitempty"`
	SoundPredicate      string `json:"sound_predicate,omitempty"`
	SoundTitle          string `json:"sound_title,omitempty"`
	SoundData           string `json:"sound_data,omitempty"`
	SoundID             int64  `json:"sound_id,omitempty"`
	ShowPreviews        bool   `json:"show_previews"`
}

func reactionNotifySettingsKey(userID int64) string {
	return reactKey(userID, "notify_settings")
}

func defaultReactionNotifySettings() *mtproto.ReactionsNotifySettings {
	return mtproto.MakeTLReactionsNotifySettings(&mtproto.ReactionsNotifySettings{
		Sound:        mtproto.MakeTLNotificationSoundRingtone(&mtproto.NotificationSound{Id: 0}).To_NotificationSound(),
		ShowPreviews: mtproto.BoolFalse,
	}).To_ReactionsNotifySettings()
}

func reactionNotifySettingsRecordFromTL(settings *mtproto.ReactionsNotifySettings) reactionNotifySettingsRecord {
	if settings == nil {
		settings = defaultReactionNotifySettings()
	}
	record := reactionNotifySettingsRecord{
		ShowPreviews: mtproto.FromBool(settings.GetShowPreviews()),
	}
	if from := settings.GetMessagesNotifyFrom(); from != nil {
		record.MessagesNotifyFrom = from.GetPredicateName()
	}
	if from := settings.GetStoriesNotifyFrom(); from != nil {
		record.StoriesNotifyFrom = from.GetPredicateName()
	}
	if from := settings.GetPollVotesNotifyFrom(); from != nil {
		record.PollVotesNotifyFrom = from.GetPredicateName()
	}
	if sound := settings.GetSound(); sound != nil {
		record.SoundPredicate = sound.GetPredicateName()
		record.SoundTitle = sound.GetTitle()
		record.SoundData = sound.GetData()
		record.SoundID = sound.GetId()
	}
	return record
}

func (r reactionNotifySettingsRecord) toTL() *mtproto.ReactionsNotifySettings {
	soundPredicate := r.SoundPredicate
	if soundPredicate == "" {
		soundPredicate = mtproto.Predicate_notificationSoundRingtone
	}
	from := func(predicate string) *mtproto.ReactionNotificationsFrom {
		if predicate == "" {
			return nil
		}
		return &mtproto.ReactionNotificationsFrom{PredicateName: predicate}
	}
	sound := &mtproto.NotificationSound{
		PredicateName: soundPredicate,
		Title:         r.SoundTitle,
		Data:          r.SoundData,
		Id:            r.SoundID,
	}
	return mtproto.MakeTLReactionsNotifySettings(&mtproto.ReactionsNotifySettings{
		MessagesNotifyFrom:  from(r.MessagesNotifyFrom),
		StoriesNotifyFrom:   from(r.StoriesNotifyFrom),
		PollVotesNotifyFrom: from(r.PollVotesNotifyFrom),
		Sound:               sound,
		ShowPreviews:        mtproto.ToBool(r.ShowPreviews),
	}).To_ReactionsNotifySettings()
}

func (c *ApiFullCore) AccountGetReactionsNotifySettings(in *mtproto.TLAccountGetReactionsNotifySettings) (*mtproto.ReactionsNotifySettings, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	_ = in
	raw, err := persist.Default.Get(reactionNotifySettingsKey(uid))
	if err != nil {
		return nil, err
	}
	if raw != "" {
		var record reactionNotifySettingsRecord
		if err = json.Unmarshal([]byte(raw), &record); err != nil {
			return nil, err
		}
		return record.toTL(), nil
	}

	// Read the pre-round-trip ringtone key for users written by older builds.
	raw, err = persist.Default.Get(b18Key(uid, "react"))
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return defaultReactionNotifySettings(), nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLReactionsNotifySettings(&mtproto.ReactionsNotifySettings{
		Sound:        mtproto.MakeTLNotificationSoundRingtone(&mtproto.NotificationSound{Id: id}).To_NotificationSound(),
		ShowPreviews: mtproto.BoolFalse,
	}).To_ReactionsNotifySettings(), nil
}

func (c *ApiFullCore) AccountSetReactionsNotifySettings(in *mtproto.TLAccountSetReactionsNotifySettings) (*mtproto.ReactionsNotifySettings, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var settings *mtproto.ReactionsNotifySettings
	if in != nil {
		settings = in.GetSettings()
	}
	if settings == nil {
		settings = defaultReactionNotifySettings()
	}
	record, marshalErr := json.Marshal(reactionNotifySettingsRecordFromTL(settings))
	if marshalErr != nil {
		return nil, marshalErr
	}
	if err = persist.Default.Set(reactionNotifySettingsKey(uid), string(record)); err != nil {
		return nil, err
	}
	var id int64
	if settings.GetSound() != nil {
		id = settings.GetSound().GetId()
	}
	if err = persist.Default.Set(b18Key(uid, "react"), strconv.FormatInt(id, 10)); err != nil {
		return nil, err
	}
	return settings, nil
}
