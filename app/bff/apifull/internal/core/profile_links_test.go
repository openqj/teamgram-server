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
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestB18User81018(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81018}}

	sound := mtproto.MakeTLNotificationSoundRingtone(&mtproto.NotificationSound{Id: 81018}).To_NotificationSound()
	set, err := c.AccountSetReactionsNotifySettings(&mtproto.TLAccountSetReactionsNotifySettings{
		Settings: mtproto.MakeTLReactionsNotifySettings(&mtproto.ReactionsNotifySettings{
			Sound:        sound,
			ShowPreviews: mtproto.BoolTrue,
		}).To_ReactionsNotifySettings(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if set.GetSound().GetId() != 81018 {
		t.Fatalf("set sound: %d", set.GetSound().GetId())
	}
	gotReact, err := c.AccountGetReactionsNotifySettings(nil)
	if err != nil {
		t.Fatal(err)
	}
	if gotReact.GetSound().GetId() != 81018 {
		t.Fatalf("get sound: %d", gotReact.GetSound().GetId())
	}

	if _, err = c.MessagesRateTranscribedAudio(&mtproto.TLMessagesRateTranscribedAudio{TranscriptionId: 81018}); err != nil {
		t.Fatal(err)
	}
	tr, err := c.MessagesTranscribeAudio(nil)
	if err != nil {
		t.Fatal(err)
	}
	if tr.GetTranscriptionId() != 81018 {
		t.Fatalf("transcription: %d", tr.GetTranscriptionId())
	}

	edited, err := c.HelpEditUserInfo(&mtproto.TLHelpEditUserInfo{Message: "note-81018"})
	if err != nil {
		t.Fatal(err)
	}
	if edited.GetMessage() != "note-81018" {
		t.Fatalf("edit message: %q", edited.GetMessage())
	}
	info, err := c.HelpGetUserInfo(nil)
	if err != nil {
		t.Fatal(err)
	}
	if info.GetMessage() != "note-81018" {
		t.Fatalf("get message: %q", info.GetMessage())
	}

	ok, err := c.MessagesReportMessagesDelivery(&mtproto.TLMessagesReportMessagesDelivery{Id: []int32{81018}})
	if err != nil || !mtproto.FromBool(ok) {
		t.Fatalf("delivery: ok=%v err=%v", ok, err)
	}
	raw, err := persist.Default.Get(b18Key(81018, "delivery"))
	if err != nil {
		t.Fatal(err)
	}
	if raw != "81018" {
		t.Fatalf("delivery id: %q", raw)
	}
}
