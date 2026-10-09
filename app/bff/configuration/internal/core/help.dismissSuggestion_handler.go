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
	"encoding/json"
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
)

const dismissSuggestionKeyPrefix = "help:dismiss_suggestion:"

func dismissSuggestionKey(userID int64) string {
	return fmt.Sprintf("%s%d", dismissSuggestionKeyPrefix, userID)
}

func dismissSuggestionPeerKey(selfID int64, peer *mtproto.InputPeer) (string, error) {
	if peer == nil {
		return "", mtproto.ErrPeerIdInvalid
	}
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		return fmt.Sprintf("user:%d", selfID), nil
	case mtproto.Predicate_inputPeerUser:
		if peer.GetUserId() <= 0 {
			return "", mtproto.ErrPeerIdInvalid
		}
		return fmt.Sprintf("user:%d", peer.GetUserId()), nil
	case mtproto.Predicate_inputPeerChat:
		if peer.GetChatId() <= 0 {
			return "", mtproto.ErrPeerIdInvalid
		}
		return fmt.Sprintf("chat:%d", peer.GetChatId()), nil
	case mtproto.Predicate_inputPeerChannel:
		if peer.GetChannelId() <= 0 {
			return "", mtproto.ErrPeerIdInvalid
		}
		return fmt.Sprintf("channel:%d", peer.GetChannelId()), nil
	default:
		return "", mtproto.ErrPeerIdInvalid
	}
}

// HelpDismissSuggestion
// help.dismissSuggestion#f50dbaa1 peer:InputPeer suggestion:string = Bool;
func (c *ConfigurationCore) HelpDismissSuggestion(in *mtproto.TLHelpDismissSuggestion) (*mtproto.Bool, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetPeer() == nil || in.GetSuggestion() == "" {
		return nil, mtproto.ErrInputRequestInvalid
	}
	peerKey, err := dismissSuggestionPeerKey(c.MD.UserId, in.GetPeer())
	if err != nil {
		return nil, err
	}
	key := dismissSuggestionKey(c.MD.UserId)
	err = persist.Update(key, func(raw string) (string, error) {
		entries := make(map[string][]string)
		if raw != "" {
			if err := json.Unmarshal([]byte(raw), &entries); err != nil {
				return "", fmt.Errorf("decode dismissed suggestions: %w", err)
			}
		}
		for _, suggestion := range entries[peerKey] {
			if suggestion == in.GetSuggestion() {
				return raw, nil
			}
		}
		entries[peerKey] = append(entries[peerKey], in.GetSuggestion())
		encoded, err := json.Marshal(entries)
		if err != nil {
			return "", fmt.Errorf("encode dismissed suggestions: %w", err)
		}
		return string(encoded), nil
	})
	if err != nil {
		c.Logger.Errorf("help.dismissSuggestion - error: %v", err)
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
