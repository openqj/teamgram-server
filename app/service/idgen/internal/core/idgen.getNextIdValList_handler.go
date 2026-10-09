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
	"math"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/idgen/counter"
	"github.com/teamgram/teamgram-server/app/service/idgen/idgen"
)

// IdgenGetNextIdValList
// idgen.getNextIdValList id:Vector<InputId> = Vector<IdVal>;
func (c *IdgenCore) IdgenGetNextIdValList(in *idgen.TLIdgenGetNextIdValList) (*idgen.Vector_IdVal, error) {
	idList := make([]*idgen.IdVal, len(in.GetId()))
	counts := make(map[string]int64)
	for _, id := range in.GetId() {
		if id == nil {
			return nil, mtproto.ErrInputRequestInvalid
		}
		switch id.GetPredicateName() {
		case idgen.Predicate_inputId:
		case idgen.Predicate_inputIds:
			if id.Num < 0 || id.Num > maxNextIdsNum {
				return nil, mtproto.ErrInputRequestInvalid
			}
		case idgen.Predicate_inputSeqId:
			if counts[id.Key] == math.MaxInt64 {
				return nil, counter.ErrCounterOverflow
			}
			counts[id.Key]++
		case idgen.Predicate_inputNSeqId:
			if id.N <= 0 {
				return nil, mtproto.ErrInputRequestInvalid
			}
			if counts[id.Key] > math.MaxInt64-int64(id.N) {
				return nil, counter.ErrCounterOverflow
			}
			counts[id.Key] += int64(id.N)
		default:
			return nil, mtproto.ErrInputRequestInvalid
		}
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	// All list calls acquire rows in key order, including overlapping vectors.
	sort.Strings(keys)
	err := c.svcCtx.Dao.InTx(c.ctx, func(tx pgx.Tx) error {
		values := make(map[string]int64, len(keys))
		for _, key := range keys {
			value, err := counter.NextOn(c.ctx, tx, key, counts[key])
			if err != nil {
				return err
			}
			values[key] = value - counts[key]
		}
		for i, id := range in.GetId() {
			switch id.GetPredicateName() {
			case idgen.Predicate_inputId:
				idList[i] = idgen.MakeTLIdVal(&idgen.IdVal{Id_INT64: c.svcCtx.Node.Generate().Int64()}).To_IdVal()
			case idgen.Predicate_inputIds:
				ids := make([]int64, id.Num)
				for j := range ids {
					ids[j] = c.svcCtx.Node.Generate().Int64()
				}
				idList[i] = idgen.MakeTLIdVals(&idgen.IdVal{Id_VECTORINT64: ids}).To_IdVal()
			case idgen.Predicate_inputSeqId, idgen.Predicate_inputNSeqId:
				delta := int64(1)
				if id.GetPredicateName() == idgen.Predicate_inputNSeqId {
					delta = int64(id.N)
				}
				values[id.Key] += delta
				idList[i] = idgen.MakeTLSeqIdVal(&idgen.IdVal{Id_INT64: values[id.Key]}).To_IdVal()
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &idgen.Vector_IdVal{
		Datas: idList,
	}, nil
}
