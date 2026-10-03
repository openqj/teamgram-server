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
	"encoding/json"
	"fmt"
	"hash/fnv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RPCAiComposeToneServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

type storedAiTone struct {
	ID            int64  `json:"id"`
	Title         string `json:"title,omitempty"`
	Prompt        string `json:"prompt,omitempty"`
	Tone          string `json:"tone,omitempty"`
	Slug          string `json:"slug,omitempty"`
	EmojiID       int64  `json:"emoji_id,omitempty"`
	AccessHash    int64  `json:"access_hash,omitempty"`
	Creator       bool   `json:"creator,omitempty"`
	Saved         bool   `json:"saved,omitempty"`
	DisplayAuthor bool   `json:"display_author,omitempty"`
}

func aiToneKey(uid int64) string {
	return fmt.Sprintf("b5:%d:", uid)
}

func loadAiTones(uid int64) ([]storedAiTone, error) {
	raw, err := persist.Default.Get(aiToneKey(uid))
	if err != nil || raw == "" {
		return nil, err
	}
	if raw[0] != '[' && raw[0] != '{' {
		return []storedAiTone{{ID: 1, Title: raw, Creator: true, AccessHash: 1}}, nil
	}
	var tones []storedAiTone
	if err = json.Unmarshal([]byte(raw), &tones); err != nil {
		return []storedAiTone{{ID: 1, Title: raw, Creator: true, AccessHash: 1}}, nil
	}
	return tones, nil
}

func saveAiTones(uid int64, tones []storedAiTone) error {
	if tones == nil {
		tones = []storedAiTone{}
	}
	b, err := json.Marshal(tones)
	if err != nil {
		return err
	}
	return persist.Default.Set(aiToneKey(uid), string(b))
}

func aiToneHash(tones []storedAiTone) int64 {
	if len(tones) == 0 {
		return 0
	}
	h := fnv.New64a()
	for _, t := range tones {
		_, _ = fmt.Fprintf(h, "%d:%s:%s:%s\n", t.ID, t.Title, t.Prompt, t.Tone)
	}
	v := int64(h.Sum64() & 0x7fffffffffffffff)
	if v == 0 {
		v = 1
	}
	return v
}

func aiToneToProto(t storedAiTone) *mtproto.AiComposeTone {
	out := &mtproto.AiComposeTone{
		Creator:       t.Creator,
		Id:            t.ID,
		AccessHash:    t.AccessHash,
		Slug:          t.Slug,
		Title:         t.Title,
		Tone:          t.Tone,
		EmojiId_INT64: t.EmojiID,
	}
	if t.Prompt != "" {
		out.Prompt = wrapperspb.String(t.Prompt)
	}
	return mtproto.MakeTLAiComposeTone(out).To_AiComposeTone()
}

func aiTonesResult(tones []storedAiTone) *mtproto.Aicompose_Tones {
	out := make([]*mtproto.AiComposeTone, 0, len(tones))
	for _, t := range tones {
		out = append(out, aiToneToProto(t))
	}
	return mtproto.MakeTLAicomposeTones(&mtproto.Aicompose_Tones{
		Hash:  aiToneHash(tones),
		Tones: out,
		Users: []*mtproto.User{},
	}).To_Aicompose_Tones()
}

func aiToneMatches(t storedAiTone, in *mtproto.InputAiComposeTone) bool {
	if in == nil {
		return false
	}
	if in.GetId() != 0 {
		return t.ID == in.GetId()
	}
	if in.GetSlug() != "" {
		return t.Slug == in.GetSlug()
	}
	if in.GetTone() != "" {
		return t.Tone == in.GetTone() || t.Title == in.GetTone()
	}
	return false
}

func (c *ApiFullCore) MessagesComposeMessageWithAI(_ *mtproto.TLMessagesComposeMessageWithAI) (*mtproto.Messages_ComposedMessageWithAI, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) AicomposeCreateTone(in *mtproto.TLAicomposeCreateTone) (*mtproto.AiComposeTone, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err := persistSetJSON(uid, "aicompose.createTone", in); err != nil {
		return nil, err
	}
	tones, err := loadAiTones(uid)
	if err != nil {
		return nil, err
	}
	var next int64 = 1
	for _, t := range tones {
		if t.ID >= next {
			next = t.ID + 1
		}
	}
	tone := storedAiTone{ID: next, AccessHash: next, Creator: true}
	if in != nil {
		tone.Title = in.GetTitle()
		tone.Prompt = in.GetPrompt()
		tone.EmojiID = in.GetEmojiId()
		tone.DisplayAuthor = in.GetDisplayAuthor()
		tone.Creator = true
	}
	tones = append(tones, tone)
	if err = saveAiTones(uid, tones); err != nil {
		return nil, err
	}
	return aiToneToProto(tone), nil
}

func (c *ApiFullCore) AicomposeUpdateTone(in *mtproto.TLAicomposeUpdateTone) (*mtproto.AiComposeTone, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err := persistSetJSON(uid, "aicompose.updateTone", in); err != nil {
		return nil, err
	}
	tones, err := loadAiTones(uid)
	if err != nil {
		return nil, err
	}
	if in == nil {
		return mtproto.MakeTLAiComposeTone(&mtproto.AiComposeTone{}).To_AiComposeTone(), nil
	}
	idx := -1
	for i := range tones {
		if aiToneMatches(tones[i], in.GetTone()) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return mtproto.MakeTLAiComposeTone(&mtproto.AiComposeTone{}).To_AiComposeTone(), nil
	}
	if in.GetTitle() != nil {
		tones[idx].Title = in.GetTitle().GetValue()
	}
	if in.GetPrompt() != nil {
		tones[idx].Prompt = in.GetPrompt().GetValue()
	}
	if in.GetEmojiId() != nil {
		tones[idx].EmojiID = in.GetEmojiId().GetValue()
	}
	if in.GetDisplayAuthor() != nil {
		tones[idx].DisplayAuthor = mtproto.FromBool(in.GetDisplayAuthor())
	}
	if tone := in.GetTone(); tone != nil {
		if tone.GetSlug() != "" {
			tones[idx].Slug = tone.GetSlug()
		}
		if tone.GetTone() != "" {
			tones[idx].Tone = tone.GetTone()
		}
		if tone.GetCustomPrompt() != "" {
			tones[idx].Prompt = tone.GetCustomPrompt()
		}
	}
	if err = saveAiTones(uid, tones); err != nil {
		return nil, err
	}
	return aiToneToProto(tones[idx]), nil
}

func (c *ApiFullCore) AicomposeSaveTone(in *mtproto.TLAicomposeSaveTone) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err := persistSetJSON(uid, "aicompose.saveTone", in); err != nil {
		return nil, err
	}
	tones, err := loadAiTones(uid)
	if err != nil {
		return nil, err
	}
	saved := true
	if in != nil && in.GetUnsave() != nil {
		saved = !mtproto.FromBool(in.GetUnsave())
	}
	var tone *mtproto.InputAiComposeTone
	if in != nil {
		tone = in.GetTone()
	}
	found := false
	for i := range tones {
		if aiToneMatches(tones[i], tone) {
			tones[i].Saved = saved
			found = true
			break
		}
	}
	if !found {
		return mtproto.BoolFalse, nil
	}
	if err = saveAiTones(uid, tones); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) AicomposeDeleteTone(in *mtproto.TLAicomposeDeleteTone) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err := persistSetJSON(uid, "aicompose.deleteTone", in); err != nil {
		return nil, err
	}
	tones, err := loadAiTones(uid)
	if err != nil {
		return nil, err
	}
	var tone *mtproto.InputAiComposeTone
	if in != nil {
		tone = in.GetTone()
	}
	kept := tones[:0]
	for _, t := range tones {
		if tone != nil && aiToneMatches(t, tone) {
			continue
		}
		kept = append(kept, t)
	}
	if err = saveAiTones(uid, kept); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) AicomposeGetTone(in *mtproto.TLAicomposeGetTone) (*mtproto.Aicompose_Tones, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	tones, err := loadAiTones(uid)
	if err != nil {
		return nil, err
	}
	if in != nil && in.GetTone() != nil {
		filtered := make([]storedAiTone, 0, 1)
		for _, t := range tones {
			if aiToneMatches(t, in.GetTone()) {
				filtered = append(filtered, t)
			}
		}
		tones = filtered
	}
	return aiTonesResult(tones), nil
}

func (c *ApiFullCore) AicomposeGetTones(in *mtproto.TLAicomposeGetTones) (*mtproto.Aicompose_Tones, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	tones, err := loadAiTones(uid)
	if err != nil {
		return nil, err
	}
	var hash int64
	if in != nil {
		hash = in.GetHash()
	}
	if len(tones) > 0 && hash != 0 && hash == aiToneHash(tones) {
		return mtproto.MakeTLAicomposeTonesNotModified(&mtproto.Aicompose_Tones{}).To_Aicompose_Tones(), nil
	}
	return aiTonesResult(tones), nil
}

func (c *ApiFullCore) AicomposeGetToneExample(_ *mtproto.TLAicomposeGetToneExample) (*mtproto.AiComposeToneExample, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}
