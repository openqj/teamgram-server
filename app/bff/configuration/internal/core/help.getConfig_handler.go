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
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/teamgram/proto/mtproto"

	"github.com/zeromicro/go-zero/core/jsonx"
	"google.golang.org/protobuf/proto"
)

const (
	configFile = "./config.json"
	// date = 1509066502,    2017/10/27 09:08:22
	// expires = 1509070295, 2017/10/27 10:11:35
	expiresTimeout = 3600 // 超时时间设置为3600秒

	// support user: @benqi
	// SUPPORT_USER_ID = 2
)

var config mtproto.TLConfig

func init() {
	configData, err := readConfigFile()
	if err != nil {
		panic(err)
	}

	err = jsonx.Unmarshal(configData, &config)
	if err != nil {
		panic(err)
		return
	}
}

func readConfigFile() ([]byte, error) {
	paths := make([]string, 0, 4)
	if configured := os.Getenv("TEAMGRAM_CONFIG_JSON"); configured != "" {
		paths = append(paths, configured)
	}
	paths = append(paths, configFile)
	if executable, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(executable), "config.json"))
	}
	if _, source, _, ok := runtime.Caller(0); ok {
		paths = append(paths, filepath.Join(filepath.Dir(source), "../../../../..", "teamgramd", "bin", "config.json"))
	}
	// The repository deployment bundle keeps the default config beside the
	// other runtime assets. This fallback also makes package-level checks
	// independent of the caller's working directory.
	paths = append(paths, filepath.Join("teamgramd", "bin", "config.json"))
	var lastErr error
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err == nil {
			return data, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("read configuration JSON: %w", lastErr)
}

// HelpGetConfig
// help.getConfig#c4f9186b = Config;
func (c *ConfigurationCore) HelpGetConfig(in *mtproto.TLHelpGetConfig) (*mtproto.Config, error) {
	_ = in

	rValue, _ := proto.Clone(&config).(*mtproto.TLConfig)
	now := int32(time.Now().Unix())
	rValue.SetDate(now)
	rValue.SetExpires(now + expiresTimeout)

	return rValue.To_Config(), nil
}
