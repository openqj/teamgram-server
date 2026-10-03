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
)

// HelpGetSupport
// help.getSupport#9cdf08cd = help.Support;
func (c *ConfigurationCore) HelpGetSupport(in *mtproto.TLHelpGetSupport) (*mtproto.Help_Support, error) {
	_ = in

	// No support user in this package's config; userEmpty allows id 0.
	return mtproto.MakeTLHelpSupport(&mtproto.Help_Support{
		PhoneNumber: "",
		User:        mtproto.MakeTLUserEmpty(&mtproto.User{Id: 0}).To_User(),
	}).To_Help_Support(), nil
}
