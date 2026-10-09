/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import "github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"

// DialogGetDialogFilters
// dialog.getDialogFilters user_id:long = Vector<DialogFilterExt>;
func (c *DialogCore) DialogGetDialogFilters(in *dialog.TLDialogGetDialogFilters) (*dialog.Vector_DialogFilterExt, error) {
	dialogFilterExtList, err := c.loadDialogFilterExtList(c.ctx, in.UserId)
	if err != nil {
		return nil, err
	}

	return &dialog.Vector_DialogFilterExt{
		Datas: dialogFilterExtList,
	}, nil
}
