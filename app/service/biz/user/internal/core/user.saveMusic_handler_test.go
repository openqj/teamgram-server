package core

import (
	"testing"

	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
)

func TestSavedMusicEntryMatchesRequestedDocument(t *testing.T) {
	entry := &dataobject.UserSavedMusicDO{UserId: 42, SavedMusicId: 99}
	for _, tc := range []struct {
		name  string
		entry *dataobject.UserSavedMusicDO
		id    int64
		want  bool
	}{
		{name: "requested document", entry: entry, id: 99, want: true},
		{name: "owner id is not document id", entry: entry, id: 42, want: false},
		{name: "zero document", entry: entry, id: 0, want: false},
		{name: "nil row", entry: nil, id: 99, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := savedMusicEntryMatches(tc.entry, tc.id); got != tc.want {
				t.Fatalf("savedMusicEntryMatches(%v, %d) = %v, want %v", tc.entry, tc.id, got, tc.want)
			}
		})
	}
}
