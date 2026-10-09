/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/mr"
)

// UserGetLastSeens
// user.getLastSeens id:Vector<long> = Vector<LastSeenData>;
func (c *UserCore) UserGetLastSeens(in *user.TLUserGetLastSeens) (*user.Vector_LastSeenData, error) {
	type idxId struct {
		idx int
		id  int64
	}

	doList := make([]*dataobject.UserPresencesDO, len(in.Id))
	errors := make([]error, len(in.Id))
	mr.ForEach(
		func(source chan<- interface{}) {
			for i, v := range in.GetId() {
				source <- idxId{i, v}
			}
		},
		func(item interface{}) {
			id := item.(idxId)
			do, err := c.svcCtx.Dao.GetLastSeenAt(c.ctx, id.id)
			errors[id.idx] = err
			if do != nil {
				doList[id.idx] = do
			}
		})
	for _, err := range errors {
		if err != nil {
			return nil, err
		}
	}

	rValues := &user.Vector_LastSeenData{
		Datas: make([]*user.LastSeenData, 0, len(doList)),
	}

	for i := 0; i < len(doList); i++ {
		if doList[i] == nil {
			continue
		}

		rValues.Datas = append(rValues.Datas, user.MakeTLLastSeenData(&user.LastSeenData{
			UserId:     doList[i].UserId,
			LastSeenAt: doList[i].LastSeenAt,
			Expires:    doList[i].Expires,
		}).To_LastSeenData())
	}

	return rValues, nil
}
