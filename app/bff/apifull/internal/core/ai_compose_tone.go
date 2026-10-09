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
	"hash/fnv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

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

func loadAiTones(uid int64) ([]storedAiTone, error) {
	tones, err := persist.LoadAITones(uid)
	if err != nil {
		return nil, err
	}
	return storedAiTones(tones), nil
}

func mutateAiTones(uid int64, mutate func([]storedAiTone) ([]storedAiTone, error)) ([]storedAiTone, error) {
	tones, err := persist.MutateAITones(uid, func(current []persist.AITone) ([]persist.AITone, error) {
		next, err := mutate(storedAiTones(current))
		if err != nil {
			return nil, err
		}
		return persistedAiTones(next), nil
	})
	if err != nil {
		return nil, err
	}
	return storedAiTones(tones), nil
}

func storedAiTones(tones []persist.AITone) []storedAiTone {
	out := make([]storedAiTone, 0, len(tones))
	for _, tone := range tones {
		out = append(out, storedAiTone{ID: tone.ID, Title: tone.Title, Prompt: tone.Prompt, Tone: tone.Tone, Slug: tone.Slug, EmojiID: tone.EmojiID, AccessHash: tone.AccessHash, Creator: tone.Creator, Saved: tone.Saved, DisplayAuthor: tone.DisplayAuthor})
	}
	return out
}

func persistedAiTones(tones []storedAiTone) []persist.AITone {
	out := make([]persist.AITone, 0, len(tones))
	for _, tone := range tones {
		out = append(out, persist.AITone{ID: tone.ID, Title: tone.Title, Prompt: tone.Prompt, Tone: tone.Tone, Slug: tone.Slug, EmojiID: tone.EmojiID, AccessHash: tone.AccessHash, Creator: tone.Creator, Saved: tone.Saved, DisplayAuthor: tone.DisplayAuthor})
	}
	return out
}

func aiToneHash(tones []storedAiTone) int64 {
	if len(tones) == 0 {
		return 0
	}
	h := fnv.New64a()
	for _, tone := range tones {
		_, _ = fmt.Fprintf(h, "%d:%s:%s:%s\n", tone.ID, tone.Title, tone.Prompt, tone.Tone)
	}
	v := int64(h.Sum64() & 0x7fffffffffffffff)
	if v == 0 {
		v = 1
	}
	return v
}

func aiToneToProto(tone storedAiTone) *mtproto.AiComposeTone {
	out := &mtproto.AiComposeTone{Creator: tone.Creator, Id: tone.ID, AccessHash: tone.AccessHash, Slug: tone.Slug, Title: tone.Title, Tone: tone.Tone, EmojiId_INT64: tone.EmojiID}
	if tone.Prompt != "" {
		out.Prompt = wrapperspb.String(tone.Prompt)
	}
	return mtproto.MakeTLAiComposeTone(out).To_AiComposeTone()
}

func aiTonesResult(tones []storedAiTone) *mtproto.Aicompose_Tones {
	out := make([]*mtproto.AiComposeTone, 0, len(tones))
	for _, tone := range tones {
		out = append(out, aiToneToProto(tone))
	}
	return mtproto.MakeTLAicomposeTones(&mtproto.Aicompose_Tones{Hash: aiToneHash(tones), Tones: out, Users: []*mtproto.User{}}).To_Aicompose_Tones()
}

func aiToneMatches(tone storedAiTone, in *mtproto.InputAiComposeTone) bool {
	if in == nil {
		return false
	}
	if in.GetId() != 0 {
		return tone.ID == in.GetId()
	}
	if in.GetSlug() != "" {
		return tone.Slug == in.GetSlug()
	}
	if in.GetTone() != "" {
		return tone.Tone == in.GetTone() || tone.Title == in.GetTone()
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
	tone := storedAiTone{Creator: true}
	if in != nil {
		tone.Title, tone.Prompt, tone.EmojiID, tone.DisplayAuthor = in.GetTitle(), in.GetPrompt(), in.GetEmojiId(), in.GetDisplayAuthor()
	}
	if _, err = mutateAiTones(uid, func(current []storedAiTone) ([]storedAiTone, error) {
		next := int64(1)
		for _, existing := range current {
			if existing.ID >= next {
				next = existing.ID + 1
			}
		}
		tone.ID, tone.AccessHash = next, next
		return append(current, tone), nil
	}); err != nil {
		return nil, err
	}
	return aiToneToProto(tone), nil
}

func (c *ApiFullCore) AicomposeUpdateTone(in *mtproto.TLAicomposeUpdateTone) (*mtproto.AiComposeTone, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return mtproto.MakeTLAiComposeTone(&mtproto.AiComposeTone{}).To_AiComposeTone(), nil
	}
	var updated storedAiTone
	found := false
	if _, err = mutateAiTones(uid, func(tones []storedAiTone) ([]storedAiTone, error) {
		for i := range tones {
			if !aiToneMatches(tones[i], in.GetTone()) {
				continue
			}
			if in.GetTitle() != nil {
				tones[i].Title = in.GetTitle().GetValue()
			}
			if in.GetPrompt() != nil {
				tones[i].Prompt = in.GetPrompt().GetValue()
			}
			if in.GetEmojiId() != nil {
				tones[i].EmojiID = in.GetEmojiId().GetValue()
			}
			if in.GetDisplayAuthor() != nil {
				tones[i].DisplayAuthor = mtproto.FromBool(in.GetDisplayAuthor())
			}
			if input := in.GetTone(); input != nil {
				if input.GetSlug() != "" {
					tones[i].Slug = input.GetSlug()
				}
				if input.GetTone() != "" {
					tones[i].Tone = input.GetTone()
				}
				if input.GetCustomPrompt() != "" {
					tones[i].Prompt = input.GetCustomPrompt()
				}
			}
			updated, found = tones[i], true
			break
		}
		return tones, nil
	}); err != nil {
		return nil, err
	}
	if !found {
		return mtproto.MakeTLAiComposeTone(&mtproto.AiComposeTone{}).To_AiComposeTone(), nil
	}
	return aiToneToProto(updated), nil
}

func (c *ApiFullCore) AicomposeSaveTone(in *mtproto.TLAicomposeSaveTone) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
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
	if _, err = mutateAiTones(uid, func(tones []storedAiTone) ([]storedAiTone, error) {
		for i := range tones {
			if aiToneMatches(tones[i], tone) {
				tones[i].Saved, found = saved, true
				break
			}
		}
		return tones, nil
	}); err != nil {
		return nil, err
	}
	if !found {
		return mtproto.BoolFalse, nil
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) AicomposeDeleteTone(in *mtproto.TLAicomposeDeleteTone) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var tone *mtproto.InputAiComposeTone
	if in != nil {
		tone = in.GetTone()
	}
	if _, err = mutateAiTones(uid, func(tones []storedAiTone) ([]storedAiTone, error) {
		kept := tones[:0]
		for _, existing := range tones {
			if tone != nil && aiToneMatches(existing, tone) {
				continue
			}
			kept = append(kept, existing)
		}
		return kept, nil
	}); err != nil {
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
		for _, tone := range tones {
			if aiToneMatches(tone, in.GetTone()) {
				filtered = append(filtered, tone)
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
