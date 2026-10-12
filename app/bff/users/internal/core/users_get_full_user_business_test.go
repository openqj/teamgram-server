package core

import (
	"encoding/json"
	"testing"

	"github.com/teamgram/proto/mtproto"
	apifullpersist "github.com/teamgram/teamgram-server/app/bff/apifull/persist"
)

type fullUserBusinessProfileStore struct {
	values map[string]string
}

func (s *fullUserBusinessProfileStore) Get(key string) (string, error) {
	return s.values[key], nil
}

func (s *fullUserBusinessProfileStore) Set(key, value string) error {
	s.values[key] = value
	return nil
}

func TestUsersGetFullUserReadsBusinessProfile(t *testing.T) {
	store := &fullUserBusinessProfileStore{values: make(map[string]string)}
	previous := apifullpersist.Default
	apifullpersist.Default = store
	t.Cleanup(func() { apifullpersist.Default = previous })

	workHours, err := json.Marshal(struct {
		OpenNow  bool               `json:"open_now"`
		Timezone string             `json:"timezone_id"`
		Weekly   []map[string]int32 `json:"weekly_open"`
	}{OpenNow: true, Timezone: "UTC", Weekly: []map[string]int32{{"start": 60, "end": 120}}})
	if err != nil {
		t.Fatal(err)
	}
	location, err := json.Marshal(struct {
		Lat     float64 `json:"lat"`
		Long    float64 `json:"long"`
		Address string  `json:"address"`
	}{Lat: 31.2304, Long: 121.4737, Address: "Layer229"})
	if err != nil {
		t.Fatal(err)
	}
	greeting, err := json.Marshal(&mtproto.InputBusinessGreetingMessage{
		ShortcutId:     4,
		Recipients:     &mtproto.InputBusinessRecipients{Contacts: true},
		NoActivityDays: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	away, err := json.Marshal(&mtproto.InputBusinessAwayMessage{
		OfflineOnly: true,
		Schedule:    mtproto.MakeTLBusinessAwayMessageScheduleOutsideWorkHours(nil).To_BusinessAwayMessageSchedule(),
		Recipients:  &mtproto.InputBusinessRecipients{Contacts: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	intro, err := json.Marshal(struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}{Title: "Support", Description: "Layer229"})
	if err != nil {
		t.Fatal(err)
	}
	store.values["bhours:2"] = string(workHours)
	store.values["bloc:2"] = string(location)
	store.values["greet:2"] = string(greeting)
	store.values["greet:away:2"] = string(away)
	store.values["bintro:2"] = string(intro)

	client := &fullUserProfilePrivacyClient{allowed: map[int32]bool{}}
	full, err := newFullUserProfilePrivacyCore(client).UsersGetFullUser(&mtproto.TLUsersGetFullUser{
		Id: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 2, AccessHash: 200}).To_InputUser(),
	})
	if err != nil || full == nil || full.GetFullUser() == nil {
		t.Fatalf("UsersGetFullUser() = (%v, %v)", full, err)
	}
	got := full.GetFullUser()
	if got.GetBusinessWorkHours() == nil || !got.GetBusinessWorkHours().GetOpenNow() || got.GetBusinessWorkHours().GetTimezoneId() != "UTC" {
		t.Fatalf("business work hours = %v", got.GetBusinessWorkHours())
	}
	if len(got.GetBusinessWorkHours().GetWeeklyOpen()) != 1 || got.GetBusinessWorkHours().GetWeeklyOpen()[0].GetStartMinute() != 60 {
		t.Fatalf("business weekly hours = %v", got.GetBusinessWorkHours().GetWeeklyOpen())
	}
	if got.GetBusinessLocation() == nil || got.GetBusinessLocation().GetGeoPoint() == nil || got.GetBusinessLocation().GetAddress() != "Layer229" {
		t.Fatalf("business location = %v", got.GetBusinessLocation())
	}
	if got.GetBusinessGreetingMessage() == nil || got.GetBusinessGreetingMessage().GetNoActivityDays() != 7 || !got.GetBusinessGreetingMessage().GetRecipients().GetContacts() {
		t.Fatalf("business greeting = %v", got.GetBusinessGreetingMessage())
	}
	if got.GetBusinessAwayMessage() == nil || !got.GetBusinessAwayMessage().GetOfflineOnly() || got.GetBusinessAwayMessage().GetSchedule().GetPredicateName() != mtproto.Predicate_businessAwayMessageScheduleOutsideWorkHours {
		t.Fatalf("business away = %v", got.GetBusinessAwayMessage())
	}
	if got.GetBusinessIntro() == nil || got.GetBusinessIntro().GetTitle() != "Support" || got.GetBusinessIntro().GetDescription() != "Layer229" {
		t.Fatalf("business intro = %v", got.GetBusinessIntro())
	}
	if err := full.Encode(mtproto.NewEncodeBuf(4096), 229); err != nil {
		t.Fatalf("users.UserFull Layer 229 encoding failed: %v", err)
	}
}
