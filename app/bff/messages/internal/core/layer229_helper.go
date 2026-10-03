package core

import (
	"sort"

	"github.com/teamgram/proto/mtproto"
)

func emptyMessages() *mtproto.Messages_Messages {
	return mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
		Messages: []*mtproto.Message{},
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}).To_Messages_Messages()
}

func emptySearchCalendar() *mtproto.Messages_SearchResultsCalendar {
	return mtproto.MakeTLMessagesSearchResultsCalendar(&mtproto.Messages_SearchResultsCalendar{
		Count:    0,
		Periods:  []*mtproto.SearchResultsCalendarPeriod{},
		Messages: []*mtproto.Message{},
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}).To_Messages_SearchResultsCalendar()
}

func messageHasGeo(msg *mtproto.Message) bool {
	if msg == nil {
		return false
	}
	switch msg.GetMedia().GetPredicateName() {
	case mtproto.Predicate_messageMediaGeo,
		mtproto.Predicate_messageMediaGeoLive,
		mtproto.Predicate_messageMediaVenue:
		return true
	default:
		return false
	}
}

func bucketByDay(found *mtproto.Messages_Messages, hitLimit bool) *mtproto.Messages_SearchResultsCalendar {
	const day int32 = 86400

	type bucket struct {
		date  int32
		minID int32
		maxID int32
		count int32
		msg   *mtproto.Message
	}

	byDay := map[int32]*bucket{}
	var (
		count    int32
		minDate  int32
		minMsgID int32
	)
	for _, msg := range found.GetMessages() {
		if msg == nil {
			continue
		}
		count++
		date := msg.GetDate()
		id := msg.GetId()
		if minDate == 0 || (date != 0 && date < minDate) {
			minDate = date
		}
		if minMsgID == 0 || (id != 0 && id < minMsgID) {
			minMsgID = id
		}
		dayStart := date - date%day
		b := byDay[dayStart]
		if b == nil {
			b = &bucket{date: dayStart, minID: id, maxID: id, msg: msg}
			byDay[dayStart] = b
		}
		b.count++
		if id < b.minID {
			b.minID = id
		}
		if id >= b.maxID {
			b.maxID = id
			b.msg = msg
		}
	}

	days := make([]int32, 0, len(byDay))
	for d := range byDay {
		days = append(days, d)
	}
	sort.Slice(days, func(i, j int) bool { return days[i] > days[j] })

	periods := make([]*mtproto.SearchResultsCalendarPeriod, 0, len(days))
	messages := make([]*mtproto.Message, 0, len(days))
	for _, d := range days {
		b := byDay[d]
		periods = append(periods, mtproto.MakeTLSearchResultsCalendarPeriod(&mtproto.SearchResultsCalendarPeriod{
			Date:     b.date,
			MinMsgId: b.minID,
			MaxMsgId: b.maxID,
			Count:    b.count,
		}).To_SearchResultsCalendarPeriod())
		if b.msg != nil {
			messages = append(messages, b.msg)
		}
	}

	users := found.GetUsers()
	chats := found.GetChats()
	if users == nil {
		users = []*mtproto.User{}
	}
	if chats == nil {
		chats = []*mtproto.Chat{}
	}

	return mtproto.MakeTLMessagesSearchResultsCalendar(&mtproto.Messages_SearchResultsCalendar{
		Inexact:  hitLimit && count > 0,
		Count:    count,
		MinDate:  minDate,
		MinMsgId: minMsgID,
		Periods:  periods,
		Messages: messages,
		Chats:    chats,
		Users:    users,
	}).To_Messages_SearchResultsCalendar()
}
