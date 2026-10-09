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
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/idgen/idgen"
)

// IdgenSetCurrentSeqId
// idgen.setCurrentSeqId key:string id:long = Bool;
func (c *IdgenCore) IdgenSetCurrentSeqId(in *idgen.TLIdgenSetCurrentSeqId) (*mtproto.Bool, error) {
	err := c.svcCtx.Dao.Set(c.ctx, in.GetKey(), in.GetId())
	if err != nil {
		c.Logger.Errorf("idgen.setCurrentSeqId(%s, %d) error: %v", in.GetKey(), in.GetId(), err)
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
