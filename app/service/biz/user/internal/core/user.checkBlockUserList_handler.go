/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import "github.com/teamgram/teamgram-server/app/service/biz/user/user"

// UserCheckBlockUserList
// user.checkBlockUserList user_id:long id:Vector<long> = Vector<long>;
func (c *UserCore) UserCheckBlockUserList(in *user.TLUserCheckBlockUserList) (*user.Vector_Long, error) {
	if err := c.requirePostgres(); err != nil {
		return nil, err
	}
	var (
		rVal = &user.Vector_Long{}
	)

	ids, err := c.svcCtx.Dao.Postgres.Store.PeerBlocks.SelectListByIdList(c.ctx, in.GetUserId(), in.GetId())
	if err != nil {
		return nil, err
	}
	blocked := make(map[int64]bool, len(ids))
	for _, id := range ids {
		blocked[id] = true
	}
	for _, id := range in.GetId() {
		if blocked[id] {
			rVal.Datas = append(rVal.Datas, id)
		}
	}

	if rVal.Datas == nil {
		rVal.Datas = []int64{}
	}

	return rVal, nil
}
