// Copyright 2022 Teamgram Authors
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

package dao

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	userNotifySettingsKeyPrefix = "user_notify_settings"
)

func genUserNotifySettingsCacheKey(id int64, peerType int32, peerId int64) string {
	return fmt.Sprintf("%s_%d_%d_%d", userNotifySettingsKeyPrefix, id, peerType, peerId)
}

func (d *Dao) GetUserNotifySettings(ctx context.Context, id int64, peerType int32, peerId int64) (*mtproto.PeerNotifySettings, error) {
	settings := mtproto.MakeTLPeerNotifySettings(nil).To_PeerNotifySettings()
	do, err := d.Postgres.Store.NotifySettings.Select(ctx, id, peerType, peerId)
	if err != nil {
		return nil, err
	}
	if do == nil {
		if peerType == mtproto.PEER_USERS || peerType == mtproto.PEER_CHATS || peerType == mtproto.PEER_BROADCASTS {
			settings.ShowPreviews = mtproto.BoolTrue
			settings.Silent = mtproto.BoolFalse
			settings.MuteUntil = &wrapperspb.Int32Value{Value: 0}
			settings.Sound = &wrapperspb.StringValue{Value: "default"}
		}
	} else {
		setPeerNotifySettingsByDO(settings, do)
	}
	return settings, nil
}

func setPeerNotifySettingsByDO(settings *mtproto.PeerNotifySettings, do *dataobject.UserNotifySettingsDO) {
	if do.ShowPreviews != -1 {
		settings.ShowPreviews = mtproto.ToBool(do.ShowPreviews == 1)
	}
	if do.Silent != -1 {
		settings.Silent = mtproto.ToBool(do.Silent == 1)
	}
	if do.MuteUntil != -1 {
		settings.MuteUntil = &wrapperspb.Int32Value{Value: do.MuteUntil}
	}
	if do.Sound != "-1" {
		settings.Sound = &wrapperspb.StringValue{Value: do.Sound}
	}
}

func makeDOByPeerNotifySettings(settings *mtproto.PeerNotifySettings) (doMap map[string]interface{}) {
	doMap = map[string]interface{}{}

	if settings.ShowPreviews != nil {
		if mtproto.FromBool(settings.ShowPreviews) {
			doMap["show_previews"] = 1
		} else {
			doMap["show_previews"] = 0
		}
	} else {
		doMap["show_previews"] = -1
	}

	if settings.Silent != nil {
		if mtproto.FromBool(settings.Silent) {
			doMap["silent"] = 1
		} else {
			doMap["silent"] = 0
		}
	} else {
		doMap["silent"] = -1
	}

	if settings.MuteUntil != nil {
		doMap["mute_until"] = settings.MuteUntil.Value
	} else {
		doMap["mute_until"] = -1
	}

	if settings.Sound != nil {
		doMap["sound"] = settings.Sound.Value
	} else {
		doMap["sound"] = "-1"
	}

	return
}

func (d *Dao) SetUserPeerNotifySettings(ctx context.Context, id int64, peerType int32, peerId int64, settings *mtproto.PeerNotifySettings) error {
	m := makeDOByPeerNotifySettings(settings)
	return d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		_, _, err := d.Postgres.Store.NotifySettings.InsertOrUpdateExtTx(ctx, tx, id, peerType, peerId, m)
		return err
	})
}

// ResetUserNotifySettings removes all per-peer overrides and invalidates every
// cached peer value that was active for the user.
func (d *Dao) ResetUserNotifySettings(ctx context.Context, id int64) error {
	return d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		_, err := d.Postgres.Store.NotifySettings.DeleteAllTx(ctx, tx, id)
		return err
	})
}
