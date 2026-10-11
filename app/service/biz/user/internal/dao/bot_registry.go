// Copyright 2026 Teamgram Authors
// All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dao

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"strconv"

	"github.com/teamgram/proto/mtproto"
)

func (d *Dao) CreateBot(ctx context.Context, creatorUserId, managerBotId, managerAccessHash int64, name, username string) (*mtproto.ImmutableUser, error) {
	if managerBotId < 0 || (managerBotId == 0 && managerAccessHash != 0) || (managerBotId > 0 && managerAccessHash == 0) {
		return nil, mtproto.ErrUserIdInvalid
	}
	var accessHashBytes [8]byte
	if _, err := cryptorand.Read(accessHashBytes[:]); err != nil {
		return nil, err
	}
	accessHash := int64(binary.LittleEndian.Uint64(accessHashBytes[:]) & 0x7fffffffffffffff)
	if accessHash == 0 {
		accessHash = 1
	}
	var phoneBytes [12]byte
	if _, err := cryptorand.Read(phoneBytes[:]); err != nil {
		return nil, err
	}
	phone := "bot-" + hex.EncodeToString(phoneBytes[:])
	botID, err := d.pgCreateBot(ctx, creatorUserId, managerBotId, managerAccessHash, name, username, accessHash, phone)
	if err != nil {
		return nil, err
	}
	return d.GetImmutableUser(ctx, botID, true, creatorUserId)
}

func (d *Dao) ExportBotToken(ctx context.Context, botId, creatorUserId int64, revoke bool) (string, error) {
	return d.pgExportBotToken(ctx, botId, creatorUserId, revoke)
}

func newBotToken(botId int64) (string, error) {
	var secret [32]byte
	if _, err := cryptorand.Read(secret[:]); err != nil {
		return "", err
	}
	return strconv.FormatInt(botId, 10) + ":" + base64.RawURLEncoding.EncodeToString(secret[:]), nil
}
