package core

import (
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/updates"
	"github.com/zeromicro/go-zero/core/jsonx"
)

// UpdatesGetDifferenceV2 reads committed rows and state from one PG snapshot.
func (c *UpdatesCore) UpdatesGetDifferenceV2(in *updates.TLUpdatesGetDifferenceV2) (*updates.Difference, error) {
	if in == nil || in.UserId <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.Date <= 0 {
		return nil, mtproto.ErrDateEmpty
	}
	if in.GetPtsTotalLimit() != nil && in.GetPtsTotalLimit().GetValue() < 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	limit := in.GetPtsLimit().GetValue()
	if limit <= 0 || limit > 5000 {
		limit = 5000
	}
	var totalLimit *int32
	if in.GetPtsTotalLimit() != nil {
		value := in.GetPtsTotalLimit().GetValue()
		totalLimit = &value
	}
	snapshot, err := c.svcCtx.Dao.LoadUpdateSnapshot(c.ctx, in.UserId, in.AuthKeyId, in.Pts, in.Date, limit, totalLimit)
	if err != nil {
		return nil, err
	}
	state := snapshot.State
	state.Date = max(state.Date, int32(in.Date))
	if snapshot.TooLong {
		return updates.MakeTLDifferenceTooLong(&updates.Difference{Pts: state.Pts}).To_Difference(), nil
	}
	if len(snapshot.PtsUpdates) == 0 && len(snapshot.SeqUpdates) == 0 {
		return updates.MakeTLDifferenceEmpty(&updates.Difference{State: state}).To_Difference(), nil
	}
	newMessages := make([]*mtproto.Message, 0)
	otherUpdates := make([]*mtproto.Update, 0)
	lastPts := in.Pts
	lastSeq := int32(0)
	lastDate := in.Date
	if len(snapshot.PtsUpdates) > 0 {
		lastPts = snapshot.PtsUpdates[len(snapshot.PtsUpdates)-1].Pts
	}
	add := func(data string, updateType int32) error {
		update := new(mtproto.Update)
		if err := jsonx.UnmarshalFromString(data, update); err != nil {
			return fmt.Errorf("updates: decode committed update: %w", err)
		}
		if mtproto.GetUpdateType(update) != updateType {
			return fmt.Errorf("updates: committed update type does not match payload")
		}
		if mtproto.GetClazzID(update.GetPredicateName(), 229) == 0 {
			return fmt.Errorf("updates: committed payload has no Layer 229 constructor")
		}
		update = update.FixData()
		if update.GetPredicateName() == mtproto.Predicate_updateNewMessage {
			newMessages = append(newMessages, update.GetMessage_MESSAGE())
		} else {
			otherUpdates = append(otherUpdates, update)
		}
		return nil
	}
	for a, b := 0, 0; a < len(snapshot.PtsUpdates) || b < len(snapshot.SeqUpdates); {
		if b == len(snapshot.SeqUpdates) || (a < len(snapshot.PtsUpdates) && snapshot.PtsUpdates[a].Date2 < snapshot.SeqUpdates[b].Date2) {
			row := snapshot.PtsUpdates[a]
			if err := add(row.UpdateData, row.UpdateType); err != nil {
				return nil, err
			}
			a++
		} else {
			row := snapshot.SeqUpdates[b]
			if err := add(row.UpdateData, row.UpdateType); err != nil {
				return nil, err
			}
			lastSeq = max(lastSeq, row.Seq)
			lastDate = max(lastDate, row.Date2)
			b++
		}
	}
	if snapshot.HasMore {
		state.Pts = lastPts
		if lastSeq > 0 {
			state.Seq = lastSeq
		}
		state.Date = int32(lastDate)
		return updates.MakeTLDifferenceSlice(&updates.Difference{
			NewMessages: newMessages, OtherUpdates: otherUpdates, IntermediateState: state,
		}).To_Difference(), nil
	}
	return updates.MakeTLDifference(&updates.Difference{
		NewMessages: newMessages, OtherUpdates: otherUpdates, State: state,
	}).To_Difference(), nil
}
