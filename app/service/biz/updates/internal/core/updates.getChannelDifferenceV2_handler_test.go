package core

import (
	"encoding/json"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/updates"
	"google.golang.org/protobuf/proto"
)

func TestUpdatesGetChannelDifferenceV2RejectsInvalidRequest(t *testing.T) {
	core := &UpdatesCore{}
	cases := []struct {
		name string
		in   *updates.TLUpdatesGetChannelDifferenceV2
		want error
	}{
		{name: "nil request", want: mtproto.ErrInputRequestInvalid},
		{name: "invalid channel", in: &updates.TLUpdatesGetChannelDifferenceV2{UserId: 1, Limit: 1}, want: mtproto.ErrChannelInvalid},
		{name: "invalid user", in: &updates.TLUpdatesGetChannelDifferenceV2{ChannelId: 1, Limit: 1}, want: mtproto.ErrActiveUserRequired},
		{name: "negative pts", in: &updates.TLUpdatesGetChannelDifferenceV2{ChannelId: 1, UserId: 1, Pts: -1, Limit: 1}, want: mtproto.ErrInputRequestInvalid},
		{name: "zero limit", in: &updates.TLUpdatesGetChannelDifferenceV2{ChannelId: 1, UserId: 1, Limit: 0}, want: mtproto.ErrLimitInvalid},
		{name: "limit too large", in: &updates.TLUpdatesGetChannelDifferenceV2{ChannelId: 1, UserId: 1, Limit: channelDifferenceMaxLimit + 1}, want: mtproto.ErrLimitInvalid},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := core.UpdatesGetChannelDifferenceV2(tc.in)
			if err != tc.want {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestPersistedChannelMessageToMTProto(t *testing.T) {
	message := persistedChannelMessage{
		ChannelID: 42,
		MessageID: 7,
		Sender:    9,
		Date:      123,
		Text:      "hello",
		Edited:    true,
		EditedAt:  456,
		Pinned:    true,
	}

	got, err := persistedChannelMessageToMTProto(message)
	if err != nil {
		t.Fatalf("persistedChannelMessageToMTProto() error = %v", err)
	}
	if got.GetId() != message.MessageID || got.GetDate() != int32(message.Date) || got.GetMessage() != message.Text {
		t.Fatalf("message fields = id %d date %d text %q", got.GetId(), got.GetDate(), got.GetMessage())
	}
	if !got.GetOut() || !got.GetPinned() {
		t.Fatalf("message flags = out %t pinned %t", got.GetOut(), got.GetPinned())
	}
	if got.GetPeerId().GetChannelId() != message.ChannelID || got.GetFromId().GetUserId() != message.Sender {
		t.Fatalf("message peers = peer %v sender %v", got.GetPeerId(), got.GetFromId())
	}
	if got.GetEditDate() == nil || got.GetEditDate().GetValue() != int32(message.EditedAt) {
		t.Fatalf("edit date = %v", got.GetEditDate())
	}
}

func TestPersistedChannelMessageToMTProtoPreservesMediaThroughGRPC(t *testing.T) {
	content, err := json.Marshal(map[string]any{
		"media": map[string]any{
			"predicate_name": "messageMediaContact",
			"phone_number":   "+14155550101",
			"first_name":     "probe",
		},
		"grouped_id": int64(2107779513252319232),
	})
	if err != nil {
		t.Fatal("marshal content:", err)
	}
	message, err := persistedChannelMessageToMTProto(persistedChannelMessage{
		ChannelID:   42,
		MessageID:   7,
		Sender:      9,
		Date:        123,
		Text:        "media",
		ContentJSON: string(content),
	})
	if err != nil {
		t.Fatal("hydrate message:", err)
	}
	if message.GetMedia() == nil || message.GetMedia().GetPredicateName() != mtproto.Predicate_messageMediaContact {
		t.Fatalf("hydrated media = %v", message.GetMedia())
	}
	data, err := proto.Marshal(message)
	if err != nil {
		t.Fatal("marshal message:", err)
	}
	decoded := &mtproto.Message{}
	if err = proto.Unmarshal(data, decoded); err != nil {
		t.Fatal("unmarshal message:", err)
	}
	if decoded.GetMedia() == nil || decoded.GetMedia().GetPredicateName() != mtproto.Predicate_messageMediaContact {
		t.Fatalf("round-trip media = %v", decoded.GetMedia())
	}
	difference := &updates.ChannelDifference{NewMessages: []*mtproto.Message{message}}
	differenceData, err := proto.Marshal(difference)
	if err != nil {
		t.Fatal("marshal difference:", err)
	}
	decodedDifference := &updates.ChannelDifference{}
	if err = proto.Unmarshal(differenceData, decodedDifference); err != nil {
		t.Fatal("unmarshal difference:", err)
	}
	if len(decodedDifference.GetNewMessages()) != 1 || decodedDifference.GetNewMessages()[0].GetMedia() == nil || decodedDifference.GetNewMessages()[0].GetMedia().GetPredicateName() != mtproto.Predicate_messageMediaContact {
		t.Fatalf("difference round-trip media = %v", decodedDifference.GetNewMessages())
	}
}
