package schedstore

import (
	"encoding/json"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
)

var storeMu sync.Mutex

func key(userID int64) string {
	return "sched:" + strconv.FormatInt(userID, 10)
}

func nextKey(userID int64) string {
	return "sched-next:" + strconv.FormatInt(userID, 10)
}

func load(userID int64) (map[int32]*mtproto.Message, error) {
	raw, err := persist.Default.Get(key(userID))
	if err != nil {
		return nil, err
	}
	idx := map[int32]*mtproto.Message{}
	if raw == "" {
		return idx, nil
	}
	if raw[0] != '[' {
		idx[1] = mtproto.MakeTLMessage(&mtproto.Message{
			Out:           true,
			FromScheduled: true,
			Id:            1,
			FromId:        mtproto.MakePeerUser(userID),
			PeerId:        mtproto.MakePeerUser(userID),
			Date:          int32(time.Now().Unix()),
			Message:       raw,
		}).To_Message()
		return idx, nil
	}
	var msgs []*mtproto.Message
	if err = json.Unmarshal([]byte(raw), &msgs); err != nil {
		return nil, err
	}
	for _, msg := range msgs {
		if msg != nil {
			idx[msg.GetId()] = msg
		}
	}
	return idx, nil
}

func save(userID int64, idx map[int32]*mtproto.Message) error {
	msgs := make([]*mtproto.Message, 0, len(idx))
	for _, msg := range idx {
		if msg != nil {
			msgs = append(msgs, msg)
		}
	}
	sort.Slice(msgs, func(i, j int) bool { return msgs[i].GetId() < msgs[j].GetId() })
	raw, err := json.Marshal(msgs)
	if err != nil {
		return err
	}
	return persist.Default.Set(key(userID), string(raw))
}

func nextID(userID int64, idx map[int32]*mtproto.Message) (int32, error) {
	var maxID int32
	for id := range idx {
		if id > maxID {
			maxID = id
		}
	}
	next := maxID + 1
	raw, err := persist.Default.Get(nextKey(userID))
	if err != nil {
		return 0, err
	}
	if raw != "" {
		stored, err := strconv.ParseInt(raw, 10, 32)
		if err != nil {
			return 0, err
		}
		if int32(stored) > next {
			next = int32(stored)
		}
	}
	if next <= 0 {
		return 0, mtproto.ErrInternalServerError
	}
	return next, nil
}

// List returns one user's pending messages in their stable scheduled-message order.
func List(userID int64) ([]*mtproto.Message, error) {
	storeMu.Lock()
	defer storeMu.Unlock()

	idx, err := load(userID)
	if err != nil {
		return nil, err
	}
	msgs := make([]*mtproto.Message, 0, len(idx))
	for _, msg := range idx {
		if msg != nil {
			msgs = append(msgs, msg)
		}
	}
	sort.Slice(msgs, func(i, j int) bool { return msgs[i].GetId() < msgs[j].GetId() })
	return msgs, nil
}

// Delete removes the requested pending messages and reports the ids that existed.
func Delete(userID int64, ids []int32) ([]int32, error) {
	storeMu.Lock()
	defer storeMu.Unlock()

	idx, err := load(userID)
	if err != nil {
		return nil, err
	}
	deleted := make([]int32, 0, len(ids))
	seen := make(map[int32]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if idx[id] == nil {
			continue
		}
		delete(idx, id)
		deleted = append(deleted, id)
	}
	if len(deleted) == 0 {
		return deleted, nil
	}
	if err = save(userID, idx); err != nil {
		return nil, err
	}
	return deleted, nil
}

// Append stores one scheduled message and returns updateNewScheduledMessage.
// The stored JSON is the same array messages.getScheduledHistory already reads.
func Append(userID int64, peer *mtproto.Peer, text string, when int32) (*mtproto.Updates, error) {
	if userID <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if peer == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if when <= int32(time.Now().Unix()) {
		return nil, mtproto.ErrScheduleDateInvalid
	}
	storeMu.Lock()
	defer storeMu.Unlock()

	idx, err := load(userID)
	if err != nil {
		return nil, err
	}
	id, err := nextID(userID, idx)
	if err != nil {
		return nil, err
	}
	if err = persist.Default.Set(nextKey(userID), strconv.FormatInt(int64(id)+1, 10)); err != nil {
		return nil, err
	}
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Out:           true,
		FromScheduled: true,
		Id:            id,
		FromId:        mtproto.MakePeerUser(userID),
		PeerId:        peer,
		Date:          when,
		Message:       text,
	}).To_Message()
	idx[msg.GetId()] = msg
	if err = save(userID, idx); err != nil {
		return nil, err
	}
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: []*mtproto.Update{
			mtproto.MakeTLUpdateNewScheduledMessage(&mtproto.Update{Message_MESSAGE: msg}).To_Update(),
		},
		Users: []*mtproto.User{},
		Chats: []*mtproto.Chat{},
		Date:  int32(time.Now().Unix()),
	}).To_Updates(), nil
}
