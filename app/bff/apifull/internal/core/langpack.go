// Copyright 2026 Teamgram Authors
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
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// RPCLangpackServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func langpackUnavailable(c *ApiFullCore) error {
	if _, err := c.requireUserId(); err != nil {
		return err
	}
	return mtproto.ErrMethodNotImpl
}

// b13 is retained for the local translateText implementation. It is not a
// langpack catalog and is intentionally not exposed by the langpack RPCs.
func b13Key(userID int64) string {
	return fmt.Sprintf("b13:%d:", userID)
}

func loadB13(userID int64) (string, error) {
	return persist.Default.Get(b13Key(userID))
}

func (c *ApiFullCore) LangpackGetLangPack(in *mtproto.TLLangpackGetLangPack) (*mtproto.LangPackDifference, error) {
	_ = in
	return nil, langpackUnavailable(c)
}

func (c *ApiFullCore) LangpackGetStrings(in *mtproto.TLLangpackGetStrings) (*mtproto.Vector_LangPackString, error) {
	_ = in
	return nil, langpackUnavailable(c)
}

func (c *ApiFullCore) LangpackGetDifference(in *mtproto.TLLangpackGetDifference) (*mtproto.LangPackDifference, error) {
	_ = in
	return nil, langpackUnavailable(c)
}

func (c *ApiFullCore) LangpackGetLanguages(in *mtproto.TLLangpackGetLanguages) (*mtproto.Vector_LangPackLanguage, error) {
	_ = in
	return nil, langpackUnavailable(c)
}

func (c *ApiFullCore) LangpackGetLanguage(in *mtproto.TLLangpackGetLanguage) (*mtproto.LangPackLanguage, error) {
	_ = in
	return nil, langpackUnavailable(c)
}
