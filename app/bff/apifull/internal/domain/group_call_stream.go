package domain

import (
	"database/sql"
	"errors"
)

func LoadRTMPGroupCallByChannel(channelID int64) (GroupCall, bool, error) {
	if db == nil {
		return GroupCall{}, false, errors.New("domain mysql is not open")
	}
	if channelID <= 0 {
		return GroupCall{}, false, errors.New("invalid group call channel")
	}

	var call GroupCall
	var rtmpStream, conference int
	var scheduleDate sql.NullInt64
	err := db.QueryRow(`SELECT id, access_hash, creator_user_id, channel_id, title, rtmp_stream, conference, schedule_date, participants
		FROM apifull_group_call
		WHERE channel_id=? AND rtmp_stream=1 AND conference=0
		ORDER BY created_at DESC, id DESC LIMIT 1`, channelID).
		Scan(&call.ID, &call.AccessHash, &call.Creator, &call.ChannelID, &call.Title, &rtmpStream, &conference, &scheduleDate, &call.Participants)
	if errors.Is(err, sql.ErrNoRows) {
		return GroupCall{}, false, nil
	}
	if err != nil {
		return GroupCall{}, false, err
	}
	call.RtmpStream = rtmpStream != 0
	call.Conference = conference != 0
	if scheduleDate.Valid {
		value := int32(scheduleDate.Int64)
		call.ScheduleDate = &value
	}
	return call, true, nil
}
