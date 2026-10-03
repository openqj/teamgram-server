// Copyright 2025 Teamgram Authors
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

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
)

// ContactsUpdateContactNote
// contacts.updateContactNote#139f63fb id:InputUser note:TextWithEntities = Bool;
func (c *ContactsCore) ContactsUpdateContactNote(in *mtproto.TLContactsUpdateContactNote) (*mtproto.Bool, error) {
	id := mtproto.FromInputUser(c.MD.UserId, in.GetId())
	if !id.IsUser() || id.IsSelf() || id.PeerId == c.MD.UserId {
		err := mtproto.ErrPeerIdInvalid
		c.Logger.Errorf("contacts.updateContactNote - error: %v", err)
		return nil, err
	}

	note := in.GetNote()
	stored := contactNote{Text: ""}
	if note != nil {
		stored.Text = note.GetText()
		stored.Entities = note.GetEntities()
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		c.Logger.Errorf("contacts.updateContactNote - error: %v", err)
		return nil, err
	}
	key := fmt.Sprintf("contact_note:%d:%d", c.MD.UserId, id.PeerId)
	if err = persist.Default.Set(key, string(raw)); err != nil {
		c.Logger.Errorf("contacts.updateContactNote - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
