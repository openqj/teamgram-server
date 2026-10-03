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

package core

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// locatedFix is one live location. The store cannot list keys, so every
// publisher is also indexed at contacts:0:located.
type locatedFix struct {
	UserId  int64   `json:"user_id"`
	Lat     float64 `json:"lat"`
	Long    float64 `json:"long"`
	Expires int32   `json:"expires"`
}

func locatedUserKey(userId int64) string {
	return fmt.Sprintf("contacts:%d:located", userId)
}

const locatedIndexKey = "contacts:0:located"

func loadLocatedIndex() ([]locatedFix, error) {
	raw, err := persist.Default.Get(locatedIndexKey)
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return []locatedFix{}, nil
	}
	var list []locatedFix
	if err = json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	if list == nil {
		list = []locatedFix{}
	}
	return list, nil
}

func saveLocatedIndex(list []locatedFix) error {
	if list == nil {
		list = []locatedFix{}
	}
	raw, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return persist.Default.Set(locatedIndexKey, string(raw))
}

func haversineMeters(lat1, lon1, lat2, lon2 float64) int32 {
	const earth = 6371000.0
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	meters := earth * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	if meters < 0 {
		return 0
	}
	if meters > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(meters)
}

// ContactsGetLocated
// contacts.getLocated#d348bc44 flags:# background:flags.1?true geo_point:InputGeoPoint self_expires:flags.0?int = Updates;
func (c *ContactsCore) ContactsGetLocated(in *mtproto.TLContactsGetLocated) (*mtproto.Updates, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	geo := in.GetGeoPoint()
	if geo == nil || geo.GetPredicateName() == mtproto.Predicate_inputGeoPointEmpty {
		c.Logger.Errorf("contacts.getLocated - error: %v", mtproto.ErrGeoPointInvalid)
		return nil, mtproto.ErrGeoPointInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrInternalServerError
	}

	contacts, err := c.svcCtx.Dao.UserClient.UserGetContactList(c.ctx, &userpb.TLUserGetContactList{
		UserId: c.MD.UserId,
	})
	if err != nil {
		c.Logger.Errorf("contacts.getLocated - user.getContactList error: %v", err)
		return nil, err
	}
	if contacts == nil {
		return nil, mtproto.ErrInternalServerError
	}
	mutualContacts := make(map[int64]struct{}, len(contacts.GetDatas()))
	for _, contact := range contacts.GetDatas() {
		if contact == nil || contact.GetContactUserId() <= 0 {
			return nil, mtproto.ErrContactIdInvalid
		}
		if contact.GetMutualContact() {
			mutualContacts[contact.GetContactUserId()] = struct{}{}
		}
	}

	now := int32(time.Now().Unix())
	list, err := loadLocatedIndex()
	if err != nil {
		c.Logger.Errorf("contacts.getLocated - error: %v", err)
		return nil, err
	}

	live := make([]locatedFix, 0, len(list)+1)
	var existingSelf *locatedFix
	for _, row := range list {
		if row.UserId == 0 || row.Expires <= now {
			continue
		}
		if row.UserId == c.MD.UserId {
			row := row
			existingSelf = &row
			continue
		}
		live = append(live, row)
	}

	if selfExpires := in.GetSelfExpires(); selfExpires != nil {
		if selfExpires.GetValue() <= now {
			if err = persist.Default.Set(locatedUserKey(c.MD.UserId), ""); err != nil {
				c.Logger.Errorf("contacts.getLocated - error: %v", err)
				return nil, err
			}
		} else {
			self := locatedFix{
				UserId:  c.MD.UserId,
				Lat:     geo.GetLat(),
				Long:    geo.GetLong(),
				Expires: selfExpires.GetValue(),
			}
			raw, mErr := json.Marshal(self)
			if mErr != nil {
				c.Logger.Errorf("contacts.getLocated - error: %v", mErr)
				return nil, mErr
			}
			if err = persist.Default.Set(locatedUserKey(c.MD.UserId), string(raw)); err != nil {
				c.Logger.Errorf("contacts.getLocated - error: %v", err)
				return nil, err
			}
			live = append(live, self)
		}
	} else if existingSelf != nil {
		live = append(live, *existingSelf)
	}

	if err = saveLocatedIndex(live); err != nil {
		c.Logger.Errorf("contacts.getLocated - error: %v", err)
		return nil, err
	}

	visible := make([]locatedFix, 0, len(live))
	for _, row := range live {
		if row.UserId == c.MD.UserId {
			visible = append(visible, row)
			continue
		}
		if _, ok := mutualContacts[row.UserId]; ok {
			visible = append(visible, row)
		}
	}

	sort.SliceStable(visible, func(i, j int) bool {
		if visible[i].UserId == c.MD.UserId {
			return false
		}
		if visible[j].UserId == c.MD.UserId {
			return true
		}
		return visible[i].UserId < visible[j].UserId
	})

	peers := make([]*mtproto.PeerLocated, 0, len(visible))
	ids := make([]int64, 0, len(visible))
	for _, row := range visible {
		ids = append(ids, row.UserId)
		if row.UserId == c.MD.UserId {
			peers = append(peers, mtproto.MakeTLPeerSelfLocated(&mtproto.PeerLocated{
				Expires: row.Expires,
			}).To_PeerLocated())
			continue
		}
		peers = append(peers, mtproto.MakeTLPeerLocated(&mtproto.PeerLocated{
			Peer:     mtproto.MakePeerUser(row.UserId),
			Expires:  row.Expires,
			Distance: haversineMeters(geo.GetLat(), geo.GetLong(), row.Lat, row.Long),
		}).To_PeerLocated())
	}

	userObjs := []*mtproto.User{}
	if len(ids) > 0 {
		fetch := []int64{c.MD.UserId}
		seen := map[int64]struct{}{c.MD.UserId: {}}
		for _, id := range ids {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			fetch = append(fetch, id)
		}
		users, uErr := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
			Id: fetch,
			To: []int64{c.MD.UserId},
		})
		if uErr != nil {
			c.Logger.Errorf("contacts.getLocated - error: %v", uErr)
			return nil, uErr
		}
		userObjs = users.GetUserListByIdList(c.MD.UserId, ids...)
		if userObjs == nil {
			userObjs = []*mtproto.User{}
		}
	}

	return mtproto.MakeUpdatesByUpdatesUsers(
		userObjs,
		mtproto.MakeTLUpdatePeerLocated(&mtproto.Update{Peers: peers}).To_Update(),
	), nil
}
