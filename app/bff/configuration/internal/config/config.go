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

package config

import (
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	// PostgresDSN points at the deployment-owned PostgreSQL 18 APIFull store.
	// The configuration service persists user-scoped suggestion dismissals and
	// deployment-owned help responses in the shared APIFull KV table.
	PostgresDSN string     `json:",optional"`
	Help        HelpConfig `json:",optional"`
}

// HelpConfig contains deployment-owned responses for the help.* methods. The
// same fields can be supplied in PostgreSQL as the JSON value at
// configuration:help; that value overrides fields configured here.
type HelpConfig struct {
	InviteText            string `json:",optional"`
	SupportPhoneNumber    string `json:",optional"`
	SupportUserID         int64  `json:",optional"`
	SupportUserAccessHash int64  `json:",optional"`
	SupportUserFirstName  string `json:",optional"`
	SupportUserLastName   string `json:",optional"`
	SupportUserUsername   string `json:",optional"`
	AppUpdateCanNotSkip   bool   `json:",optional"`
	AppUpdateID           int32  `json:",optional"`
	AppUpdateVersion      string `json:",optional"`
	AppUpdateText         string `json:",optional"`
	AppUpdateURL          string `json:",optional"`
}
