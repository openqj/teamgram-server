package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
)

type autoDownloadMemoryStore map[string]string

func (s autoDownloadMemoryStore) Get(key string) (string, error) { return s[key], nil }

func (s autoDownloadMemoryStore) Set(key, value string) error {
	s[key] = value
	return nil
}

func TestAutoDownloadSettingsRoundTripAndUserIsolation(t *testing.T) {
	previous := persist.Default
	store := autoDownloadMemoryStore{}
	persist.Default = store
	t.Cleanup(func() { persist.Default = previous })

	owner := &AutoDownloadCore{MD: &metadata.RpcMetadata{UserId: 229001}}
	other := &AutoDownloadCore{MD: &metadata.RpcMetadata{UserId: 229002}}
	settings := mtproto.MakeTLAutoDownloadSettings(&mtproto.AutoDownloadSettings{
		Disabled:              true,
		StoriesPreload:        true,
		PhotoSizeMax:          4096,
		VideoSizeMax_INT64:    4_000_000_000,
		FileSizeMax_INT64:     5_000_000_000,
		VideoUploadMaxbitrate: 3200,
	}).To_AutoDownloadSettings()

	if result, err := owner.AccountSaveAutoDownloadSettings(&mtproto.TLAccountSaveAutoDownloadSettings{
		Low: true, Settings: settings,
	}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("save settings: result=%v err=%v", result, err)
	}

	got, err := owner.AccountGetAutoDownloadSettings(&mtproto.TLAccountGetAutoDownloadSettings{})
	if err != nil {
		t.Fatal(err)
	}
	low := got.GetLow()
	if !low.GetDisabled() || !low.GetStoriesPreload() || low.GetPhotoSizeMax() != 4096 ||
		low.GetVideoSizeMax_INT64() != 4_000_000_000 || low.GetFileSizeMax_INT64() != 5_000_000_000 ||
		low.GetVideoUploadMaxbitrate() != 3200 {
		t.Fatalf("saved low settings = %+v", low)
	}
	if got.GetMedium() == nil || got.GetHigh() == nil {
		t.Fatalf("default settings missing: %+v", got)
	}

	otherSettings, err := other.AccountGetAutoDownloadSettings(&mtproto.TLAccountGetAutoDownloadSettings{})
	if err != nil {
		t.Fatal(err)
	}
	if otherSettings.GetLow().GetDisabled() || otherSettings.GetLow().GetPhotoSizeMax() == 4096 {
		t.Fatalf("other user saw saved settings: %+v", otherSettings.GetLow())
	}
}
