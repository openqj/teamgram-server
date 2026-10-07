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

// HelpGetCdnConfig
// help.getCdnConfig#52029342 = CdnConfig;
func (c *FilesCore) HelpGetCdnConfig(in *mtproto.TLHelpGetCdnConfig) (*mtproto.CdnConfig, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	// No CDN key/token provider is configured. Returning an empty CdnConfig
	// would advertise a usable CDN while upload.getCdnFile remains unavailable.
	if c != nil && c.Logger != nil {
		c.Logger.Errorf("help.getCdnConfig - error: %v", mtproto.ErrCdnMethodInvalid)
	}
	return nil, mtproto.ErrCdnMethodInvalid
}
