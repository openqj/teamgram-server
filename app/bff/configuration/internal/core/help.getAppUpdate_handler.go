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
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// HelpGetAppUpdate
// help.getAppUpdate#522d5a7d source:string = help.AppUpdate;
func (c *ConfigurationCore) HelpGetAppUpdate(in *mtproto.TLHelpGetAppUpdate) (*mtproto.Help_AppUpdate, error) {
	cfg, err := c.helpConfig()
	if err != nil {
		return nil, err
	}
	if cfg.AppUpdateVersion == "" && cfg.AppUpdateText == "" && cfg.AppUpdateURL == "" {
		return mtproto.MakeTLHelpNoAppUpdate(nil).To_Help_AppUpdate(), nil
	}
	update := &mtproto.Help_AppUpdate{
		CanNotSkip: cfg.AppUpdateCanNotSkip,
		Id:         cfg.AppUpdateID,
		Version:    cfg.AppUpdateVersion,
		Text:       cfg.AppUpdateText,
	}
	if cfg.AppUpdateURL != "" {
		update.Url = wrapperspb.String(cfg.AppUpdateURL)
	}
	return mtproto.MakeTLHelpAppUpdate(update).To_Help_AppUpdate(), nil
}
