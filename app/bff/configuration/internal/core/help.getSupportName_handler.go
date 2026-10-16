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
	"strings"

	"github.com/teamgram/proto/mtproto"
)

// HelpGetSupportName
// help.getSupportName#d360e72c = help.SupportName;
func (c *ConfigurationCore) HelpGetSupportName(in *mtproto.TLHelpGetSupportName) (*mtproto.Help_SupportName, error) {
	_ = in

	cfg, err := c.helpConfig()
	if err != nil {
		return nil, err
	}
	name := cfg.SupportUserUsername
	if name == "" {
		name = strings.TrimSpace(strings.TrimSpace(cfg.SupportUserFirstName) + " " + strings.TrimSpace(cfg.SupportUserLastName))
	}
	// Prefer an explicitly configured support name/username, then derive the
	// display name from the support user's first and last names.
	return mtproto.MakeTLHelpSupportName(&mtproto.Help_SupportName{
		Name: name,
	}).To_Help_SupportName(), nil
}
