package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestRequireChannelAdminUsesAPIFullCreatorProjection(t *testing.T) {
	core := newUpdateUsernameTestCore(&updateUsernameUserClient{})
	core.channelChatsByID = func(userID int64, ids []int64) []*mtproto.Chat {
		if userID != 42 || len(ids) != 1 || ids[0] != 77 {
			t.Fatalf("channel lookup got user ID %d and channel IDs %v", userID, ids)
		}
		return []*mtproto.Chat{
			mtproto.MakeTLChannel(&mtproto.Chat{Id: 77, Creator: true}).To_Chat(),
		}
	}

	if err := core.requireChannelAdmin(77); err != nil {
		t.Fatalf("creator was rejected: %v", err)
	}
}

func TestRequireChannelAdminRejectsNonCreatorAPIFullProjection(t *testing.T) {
	core := newUpdateUsernameTestCore(&updateUsernameUserClient{})
	core.channelChatsByID = func(int64, []int64) []*mtproto.Chat {
		return []*mtproto.Chat{
			mtproto.MakeTLChannel(&mtproto.Chat{Id: 77}).To_Chat(),
		}
	}

	if err := core.requireChannelAdmin(77); err != mtproto.ErrChatAdminRequired {
		t.Fatalf("got error %v, want %v", err, mtproto.ErrChatAdminRequired)
	}
}
