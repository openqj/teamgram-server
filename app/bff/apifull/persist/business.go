package persist

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"
)

// BusinessProfile contains the public business settings exposed by
// users.getFullUser. The update RPCs keep these records in the shared
// process store, so this reader is shared by the users service as well.
type BusinessProfile struct {
	WorkHours *mtproto.BusinessWorkHours
	Location  *mtproto.BusinessLocation
	Greeting  *mtproto.BusinessGreetingMessage
	Away      *mtproto.BusinessAwayMessage
	Intro     *mtproto.BusinessIntro
}

type businessWeeklyOpenStored struct {
	Start int32 `json:"start"`
	End   int32 `json:"end"`
}

type businessWorkHoursStored struct {
	OpenNow  bool                       `json:"open_now"`
	Timezone string                     `json:"timezone_id"`
	Weekly   []businessWeeklyOpenStored `json:"weekly_open"`
}

type businessLocationStored struct {
	Lat     float64 `json:"lat"`
	Long    float64 `json:"long"`
	Address string  `json:"address"`
}

type businessIntroStored struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// LoadBusinessProfile reads the five account business settings from the
// shared PostgreSQL-backed KV store and converts them to output constructors
// used by users.getFullUser. Empty keys are treated as unset.
func LoadBusinessProfile(userID int64) (*BusinessProfile, error) {
	if userID <= 0 {
		return nil, nil
	}

	profile := &BusinessProfile{}
	found := false

	if raw, ok, err := loadBusinessValue("bhours:", userID); err != nil {
		return nil, err
	} else if ok {
		var stored businessWorkHoursStored
		if err := json.Unmarshal([]byte(raw), &stored); err != nil {
			return nil, fmt.Errorf("decode business work hours: %w", err)
		}
		weekly := make([]*mtproto.BusinessWeeklyOpen, 0, len(stored.Weekly))
		for _, value := range stored.Weekly {
			weekly = append(weekly, mtproto.MakeTLBusinessWeeklyOpen(&mtproto.BusinessWeeklyOpen{
				StartMinute: value.Start,
				EndMinute:   value.End,
			}).To_BusinessWeeklyOpen())
		}
		profile.WorkHours = mtproto.MakeTLBusinessWorkHours(&mtproto.BusinessWorkHours{
			OpenNow:    stored.OpenNow,
			TimezoneId: stored.Timezone,
			WeeklyOpen: weekly,
		}).To_BusinessWorkHours()
		found = true
	}

	if raw, ok, err := loadBusinessValue("bloc:", userID); err != nil {
		return nil, err
	} else if ok {
		var stored businessLocationStored
		if err := json.Unmarshal([]byte(raw), &stored); err != nil {
			return nil, fmt.Errorf("decode business location: %w", err)
		}
		profile.Location = mtproto.MakeTLBusinessLocation(&mtproto.BusinessLocation{
			GeoPoint: mtproto.MakeTLGeoPoint(&mtproto.GeoPoint{
				Lat:  stored.Lat,
				Long: stored.Long,
			}).To_GeoPoint(),
			Address: stored.Address,
		}).To_BusinessLocation()
		found = true
	}

	if raw, ok, err := loadBusinessValue("greet:", userID); err != nil {
		return nil, err
	} else if ok {
		greeting, err := decodeBusinessGreeting([]byte(raw))
		if err != nil {
			return nil, err
		}
		profile.Greeting = greeting
		found = true
	}

	if raw, ok, err := loadBusinessValue("greet:away:", userID); err != nil {
		return nil, err
	} else if ok {
		away, err := decodeBusinessAway([]byte(raw))
		if err != nil {
			return nil, err
		}
		profile.Away = away
		found = true
	}

	if raw, ok, err := loadBusinessValue("bintro:", userID); err != nil {
		return nil, err
	} else if ok {
		var stored businessIntroStored
		if err := json.Unmarshal([]byte(raw), &stored); err != nil {
			return nil, fmt.Errorf("decode business intro: %w", err)
		}
		profile.Intro = mtproto.MakeTLBusinessIntro(&mtproto.BusinessIntro{
			Title:       stored.Title,
			Description: stored.Description,
		}).To_BusinessIntro()
		found = true
	}

	if !found {
		return nil, nil
	}
	return profile, nil
}

func loadBusinessValue(prefix string, userID int64) (string, bool, error) {
	raw, err := Default.Get(prefix + strconv.FormatInt(userID, 10))
	if err != nil {
		return "", false, err
	}
	return raw, strings.TrimSpace(raw) != "", nil
}

func decodeBusinessGreeting(raw []byte) (*mtproto.BusinessGreetingMessage, error) {
	var input mtproto.InputBusinessGreetingMessage
	if err := json.Unmarshal(raw, &input); err == nil {
		return mtproto.MakeTLBusinessGreetingMessage(&mtproto.BusinessGreetingMessage{
			ShortcutId:     input.ShortcutId,
			Recipients:     businessRecipientsFromInput(input.Recipients),
			NoActivityDays: input.NoActivityDays,
		}).To_BusinessGreetingMessage(), nil
	}

	var output mtproto.BusinessGreetingMessage
	if err := json.Unmarshal(raw, &output); err != nil {
		return nil, fmt.Errorf("decode business greeting message: %w", err)
	}
	if output.Recipients == nil {
		output.Recipients = emptyBusinessRecipients()
	}
	return mtproto.MakeTLBusinessGreetingMessage(&output).To_BusinessGreetingMessage(), nil
}

func decodeBusinessAway(raw []byte) (*mtproto.BusinessAwayMessage, error) {
	var input mtproto.InputBusinessAwayMessage
	if err := json.Unmarshal(raw, &input); err == nil {
		return mtproto.MakeTLBusinessAwayMessage(&mtproto.BusinessAwayMessage{
			OfflineOnly: input.OfflineOnly,
			ShortcutId:  input.ShortcutId,
			Schedule:    businessAwaySchedule(input.Schedule),
			Recipients:  businessRecipientsFromInput(input.Recipients),
		}).To_BusinessAwayMessage(), nil
	}

	var output mtproto.BusinessAwayMessage
	if err := json.Unmarshal(raw, &output); err != nil {
		return nil, fmt.Errorf("decode business away message: %w", err)
	}
	if output.Schedule == nil {
		output.Schedule = emptyBusinessAwaySchedule()
	}
	if output.Recipients == nil {
		output.Recipients = emptyBusinessRecipients()
	}
	return mtproto.MakeTLBusinessAwayMessage(&output).To_BusinessAwayMessage(), nil
}

func businessAwaySchedule(schedule *mtproto.BusinessAwayMessageSchedule) *mtproto.BusinessAwayMessageSchedule {
	if schedule == nil {
		return emptyBusinessAwaySchedule()
	}
	if schedule.GetPredicateName() != "" {
		return schedule
	}
	switch schedule.GetConstructor() {
	case mtproto.TLConstructor_CRC32_businessAwayMessageScheduleAlways:
		schedule.PredicateName = mtproto.Predicate_businessAwayMessageScheduleAlways
	case mtproto.TLConstructor_CRC32_businessAwayMessageScheduleOutsideWorkHours:
		schedule.PredicateName = mtproto.Predicate_businessAwayMessageScheduleOutsideWorkHours
	case mtproto.TLConstructor_CRC32_businessAwayMessageScheduleCustom:
		schedule.PredicateName = mtproto.Predicate_businessAwayMessageScheduleCustom
	default:
		return emptyBusinessAwaySchedule()
	}
	return schedule
}

func emptyBusinessAwaySchedule() *mtproto.BusinessAwayMessageSchedule {
	return mtproto.MakeTLBusinessAwayMessageScheduleAlways(nil).To_BusinessAwayMessageSchedule()
}

func businessRecipientsFromInput(input *mtproto.InputBusinessRecipients) *mtproto.BusinessRecipients {
	if input == nil {
		return emptyBusinessRecipients()
	}
	users := make([]int64, 0, len(input.Users))
	for _, user := range input.Users {
		if user != nil {
			users = append(users, user.GetUserId())
		}
	}
	return mtproto.MakeTLBusinessRecipients(&mtproto.BusinessRecipients{
		ExistingChats:   input.ExistingChats,
		NewChats:        input.NewChats,
		Contacts:        input.Contacts,
		NonContacts:     input.NonContacts,
		ExcludeSelected: input.ExcludeSelected,
		Users:           users,
	}).To_BusinessRecipients()
}

func emptyBusinessRecipients() *mtproto.BusinessRecipients {
	return mtproto.MakeTLBusinessRecipients(nil).To_BusinessRecipients()
}
