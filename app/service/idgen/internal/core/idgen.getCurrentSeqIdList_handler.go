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
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/idgen/idgen"
)

// IdgenGetCurrentSeqIdList
// idgen.getCurrentSeqIdList id:Vector<InputId> = Vector<IdVal>;
func (c *IdgenCore) IdgenGetCurrentSeqIdList(in *idgen.TLIdgenGetCurrentSeqIdList) (*idgen.Vector_IdVal, error) {
	keys := make([]string, len(in.GetId()))
	for i, id := range in.GetId() {
		if id == nil || id.GetPredicateName() != idgen.Predicate_inputSeqId {
			return nil, mtproto.ErrInputRequestInvalid
		}
		keys[i] = id.Key
	}
	values, err := c.svcCtx.Dao.CurrentList(c.ctx, keys)
	if err != nil {
		return nil, err
	}
	idList := make([]*idgen.IdVal, len(values))
	for i, value := range values {
		idList[i] = idgen.MakeTLSeqIdVal(&idgen.IdVal{Id_INT64: value}).To_IdVal()
	}

	return &idgen.Vector_IdVal{
		Datas: idList,
	}, nil
}
