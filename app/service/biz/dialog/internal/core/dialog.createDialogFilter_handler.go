// Copyright 2024 Teamgram Authors
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
	"time"

	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/jsonx"
)

// DialogCreateDialogFilter
// dialog.createDialogFilter user_id:long dialog_filter:DialogFilterExt = DialogFilterExt;
func (c *DialogCore) DialogCreateDialogFilter(in *dialog.TLDialogCreateDialogFilter) (*dialog.DialogFilterExt, error) {
	dialogFilterExtList, err := c.loadDialogFilterExtList(c.ctx, in.UserId)
	if err != nil {
		return nil, err
	}
	dialogExt := in.GetDialogFilter()
	created := false

	for _, v := range dialogFilterExtList {
		if v.Slug == dialogExt.Slug {
			created = true
			dialogExt.Id = v.Id
		}
		if created {
			break
		}
		if v.Id > dialogExt.Id {
			dialogExt.Id = v.Id
		}
	}

	if created {
		return dialogExt, nil
	}

	dialogExt.Id++
	dialogExt.DialogFilter.Id = dialogExt.Id
	dData, _ := jsonx.MarshalToString(dialogExt.DialogFilter)
	if _, _, err := c.svcCtx.Dao.Postgres.Store.DialogFilters.InsertOrUpdate(c.ctx, &dataobject.DialogFiltersDO{
		UserId:         in.UserId,
		DialogFilterId: dialogExt.Id,
		IsChatlist:     true,
		JoinedBySlug:   true,
		Slug:           dialogExt.Slug,
		HasMyInvites:   0,
		DialogFilter:   dData,
		OrderValue:     time.Now().Unix() << 32,
		FromSuggested:  -1,
		Deleted:        false,
	}); err != nil {
		return nil, err
	}

	return dialogExt, nil
}
