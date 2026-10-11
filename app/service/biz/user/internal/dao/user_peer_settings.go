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

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func (d *Dao) GetUserPeerSettings(ctx context.Context, id int64, peerType int32, peerId int64) (*mtproto.PeerSettings, error) {
	settings := mtproto.MakeTLPeerSettings(&mtproto.PeerSettings{
		ReportSpam:             false,
		AddContact:             false,
		BlockContact:           false,
		ShareContact:           false,
		NeedContactsException:  false,
		ReportGeo:              false,
		Autoarchived:           false,
		InviteMembers:          false,
		RequestChatBroadcast:   false,
		BusinessBotPaused:      false,
		BusinessBotCanReply:    false,
		GeoDistance:            nil,
		RequestChatTitle:       nil,
		RequestChatDate:        nil,
		BusinessBotId:          nil,
		BusinessBotManageUrl:   nil,
		ChargePaidMessageStars: nil,
		RegistrationMonth:      nil,
		PhoneCountry:           nil,
		NameChangeDate:         nil,
		PhotoChangeDate:        nil,
	}).To_PeerSettings()
	settingsDO, err := d.Postgres.Store.PeerSettings.Select(ctx, id, peerType, peerId)
	if err != nil {
		return nil, err
	}
	if settingsDO != nil {
		settings.ReportSpam = settingsDO.ReportSpam
		settings.AddContact = settingsDO.AddContact
		settings.BlockContact = settingsDO.BlockContact
		settings.ShareContact = settingsDO.ShareContact
		settings.NeedContactsException = settingsDO.NeedContactsException
		settings.ReportGeo = settingsDO.ReportGeo
		settings.Autoarchived = settingsDO.Autoarchived
		if settingsDO.GeoDistance != 0 {
			settings.GeoDistance = &wrapperspb.Int32Value{Value: settingsDO.GeoDistance}
		}
	}
	return settings, nil
}

func (d *Dao) SetUserPeerSettings(ctx context.Context, id int64, peerType int32, peerId int64, settings *mtproto.PeerSettings) error {
	return d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		_, _, err := d.Postgres.Store.PeerSettings.InsertOrUpdateTx(ctx, tx, &dataobject.UserPeerSettingsDO{UserId: id, PeerType: peerType, PeerId: peerId, ReportSpam: settings.ReportSpam, AddContact: settings.AddContact, BlockContact: settings.BlockContact, ShareContact: settings.ShareContact, NeedContactsException: settings.NeedContactsException, ReportGeo: settings.ReportGeo, Autoarchived: settings.Autoarchived, GeoDistance: settings.GetGeoDistance().GetValue()})
		return err
	})
}

func (d *Dao) DeleteUserPeerSettings(ctx context.Context, id int64, peerType int32, peerId int64) error {
	return d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		_, err := d.Postgres.Store.PeerSettings.DeleteTx(ctx, tx, id, peerType, peerId)
		return err
	})
}
