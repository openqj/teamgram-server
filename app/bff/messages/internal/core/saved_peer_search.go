package core

import (
	"math"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

const savedHistoryPage int32 = 100

func (c *MessagesCore) loadSavedBoxes(saved *mtproto.InputPeer, offsetID, limit int32) (*mtproto.MessageBoxList, error) {
	if saved == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, saved)
	if peer.PeerType == mtproto.PEER_SELF {
		peer.PeerType = mtproto.PEER_USER
		peer.PeerId = c.MD.UserId
	}
	if peer.PeerId == 0 || (peer.PeerType != mtproto.PEER_USER && peer.PeerType != mtproto.PEER_CHAT && peer.PeerType != mtproto.PEER_CHANNEL) {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if limit <= 0 {
		limit = savedHistoryPage
	}
	if offsetID == 0 {
		offsetID = math.MaxInt32
	}
	boxes, err := c.svcCtx.Dao.MessageClient.MessageGetSavedHistoryMessages(c.ctx, &message.TLMessageGetSavedHistoryMessages{
		UserId:   c.MD.UserId,
		PeerType: peer.PeerType,
		PeerId:   peer.PeerId,
		OffsetId: offsetID,
		Limit:    limit,
	})
	if err != nil {
		return nil, err
	}
	if boxes == nil {
		return nil, mtproto.ErrInternalServerError
	}
	return boxes, nil
}

func savedMediaType(ft mtproto.MessagesFilterType) (int32, bool) {
	switch ft {
	case mtproto.FilterPhotos:
		return mtproto.MEDIA_PHOTOS_ONLY, true
	case mtproto.FilterVideo:
		return mtproto.MEDIA_VIDEOS_ONLY, true
	case mtproto.FilterPhotoVideo:
		return mtproto.MEDIA_PHOTOVIDEO, true
	case mtproto.FilterDocument:
		return mtproto.MEDIA_FILE, true
	case mtproto.FilterUrl:
		return mtproto.MEDIA_URL, true
	case mtproto.FilterGif:
		return mtproto.MEDIA_GIF, true
	case mtproto.FilterMusic:
		return mtproto.MEDIA_MUSIC, true
	case mtproto.FilterVoice:
		return mtproto.MEDIA_VOICE_FILE, true
	case mtproto.FilterRoundVoice:
		return mtproto.MEDIA_AUDIO, true
	case mtproto.FilterRoundVideo:
		return mtproto.MEDIA_ROUND_FILE, true
	case mtproto.FilterChatPhotos:
		return mtproto.MEDIA_CHAT_PHOTO, true
	case mtproto.FilterPhoneCalls:
		return mtproto.MEDIA_PHONE_CALL, true
	default:
		return 0, false
	}
}

func boxMatchesMedia(box *mtproto.MessageBox, mType int32) bool {
	if box == nil {
		return false
	}
	got := box.GetMessageFilterType()
	media := mtproto.GetMediaType(box.GetMessage())
	if mType == mtproto.MEDIA_PHOTOVIDEO {
		return got == mtproto.MEDIA_PHOTOVIDEO || got == mtproto.MEDIA_PHOTOS_ONLY || got == mtproto.MEDIA_VIDEOS_ONLY ||
			media == mtproto.MEDIA_PHOTOVIDEO || media == mtproto.MEDIA_PHOTOS_ONLY || media == mtproto.MEDIA_VIDEOS_ONLY
	}
	return got == mType || media == mType
}

func boxMatchesSavedFilter(box *mtproto.MessageBox, ft mtproto.MessagesFilterType) bool {
	if box == nil {
		return false
	}
	switch ft {
	case mtproto.FilterVoice:
		return mtproto.IsVoiceMessage(box.GetMessage()) || box.GetMessageFilterType() == mtproto.MEDIA_VOICE_FILE
	case mtproto.FilterRoundVideo:
		return mtproto.IsRoundVideoMessage(box.GetMessage()) || box.GetMessageFilterType() == mtproto.MEDIA_ROUND_FILE
	case mtproto.FilterPinned:
		return box.GetPinned()
	case mtproto.FilterMyMentions:
		return box.GetMentioned() || box.GetMessage().GetMentioned()
	case mtproto.FilterGeo:
		return messageHasGeo(box.GetMessage())
	default:
		mType, ok := savedMediaType(ft)
		if !ok {
			return false
		}
		return boxMatchesMedia(box, mType)
	}
}

func savedFilterKnown(ft mtproto.MessagesFilterType) bool {
	switch ft {
	case mtproto.FilterEmpty, mtproto.FilterPinned, mtproto.FilterMyMentions, mtproto.FilterGeo, mtproto.FilterContacts:
		return true
	default:
		_, ok := savedMediaType(ft)
		return ok
	}
}

func (c *MessagesCore) savedBoxMatches(box *mtproto.MessageBox, ft mtproto.MessagesFilterType, q string, fromID int64, minDate, maxDate, offsetID, minID, maxID int32) (bool, error) {
	if box == nil || box.GetMessage() == nil {
		return false, nil
	}
	id := box.GetMessageId()
	if offsetID != 0 && offsetID != math.MaxInt32 && id >= offsetID {
		return false, nil
	}
	if minID != 0 && id <= minID {
		return false, nil
	}
	if maxID != 0 && id >= maxID {
		return false, nil
	}
	date := box.GetMessage().GetDate()
	if minDate != 0 && date < minDate {
		return false, nil
	}
	if maxDate != 0 && date > maxDate {
		return false, nil
	}
	if fromID != 0 && box.GetSenderUserId() != fromID {
		return false, nil
	}
	switch ft {
	case mtproto.FilterEmpty:
	case mtproto.FilterContacts:
		sender := box.GetSenderUserId()
		if sender == 0 || sender == c.MD.UserId {
			return false, nil
		}
		ok, err := c.svcCtx.Dao.UserClient.UserCheckContact(c.ctx, &userpb.TLUserCheckContact{
			UserId: c.MD.UserId,
			Id:     sender,
		})
		if err != nil {
			return false, err
		}
		if ok == nil {
			return false, mtproto.ErrInternalServerError
		}
		if !mtproto.FromBool(ok) {
			return false, nil
		}
	default:
		if !boxMatchesSavedFilter(box, ft) {
			return false, nil
		}
	}
	if q != "" && !strings.Contains(strings.ToLower(box.GetMessage().GetMessage()), strings.ToLower(q)) {
		return false, nil
	}
	return true, nil
}

func (c *MessagesCore) searchSavedPeer(in *mtproto.TLMessagesSearch, rValues *mtproto.Messages_Messages, fromID *mtproto.PeerUtil, offsetID, limit int32) (*mtproto.Messages_Messages, error) {
	if in.GetFilter() == nil {
		c.Logger.Errorf("messages.search - error: %v", mtproto.ErrInputFilterInvalid)
		return nil, mtproto.ErrInputFilterInvalid
	}
	ft := mtproto.FromMessagesFilter(in.Filter)
	if ft == mtproto.FilterEmpty && in.Filter.GetPredicateName() != mtproto.Predicate_inputMessagesFilterEmpty {
		c.Logger.Errorf("messages.search - error: %v", mtproto.ErrInputFilterInvalid)
		return nil, mtproto.ErrInputFilterInvalid
	}
	if ft == mtproto.FilterEmpty && fromID == nil && in.Q == "" {
		c.Logger.Errorf("messages.search - error: %v", mtproto.ErrSearchQueryEmpty)
		return nil, mtproto.ErrSearchQueryEmpty
	}
	if !savedFilterKnown(ft) {
		c.Logger.Errorf("messages.search - error: %v", mtproto.ErrInputFilterInvalid)
		return nil, mtproto.ErrInputFilterInvalid
	}
	if limit <= 0 {
		limit = 50
	}
	var sender int64
	if fromID != nil {
		sender = fromID.PeerId
	}

	if offsetID == 0 {
		offsetID = math.MaxInt32
	}
	matched := make([]*mtproto.MessageBox, 0)
	var total int32
	truncated := false
	for page := 0; page < 10 && int32(len(matched)) < limit; page++ {
		saved, err := c.loadSavedBoxes(in.SavedPeerId, offsetID, savedHistoryPage)
		if err != nil {
			c.Logger.Errorf("messages.search - error: %v", err)
			return nil, err
		}
		boxes := []*mtproto.MessageBox{}
		if saved != nil {
			boxes = saved.GetBoxList()
			total = saved.GetCount()
		}
		if len(boxes) == 0 {
			break
		}
		var oldest int32
		for _, box := range boxes {
			if box == nil {
				continue
			}
			id := box.GetMessageId()
			if oldest == 0 || (id != 0 && id < oldest) {
				oldest = id
			}
			matches, err := c.savedBoxMatches(box, ft, in.Q, sender, in.MinDate, in.MaxDate, offsetID, in.MinId, in.MaxId)
			if err != nil {
				c.Logger.Errorf("messages.search - error: %v", err)
				return nil, err
			}
			if matches {
				matched = append(matched, box)
				if int32(len(matched)) >= limit {
					break
				}
			}
		}
		if int32(len(boxes)) < savedHistoryPage || oldest == 0 || oldest == offsetID {
			break
		}
		if total > 0 && int32(page+1)*savedHistoryPage >= total {
			break
		}
		offsetID = oldest
		if page == 9 {
			truncated = true
		}
	}

	boxList := &mtproto.MessageBoxList{BoxList: matched}
	if truncated && total > int32(len(matched)) {
		rValues.Count = total
		rValues.PredicateName = mtproto.Predicate_messages_messagesSlice
	}
	if err := c.populateSearchResult(boxList, rValues); err != nil {
		c.Logger.Errorf("messages.search - error: %v", err)
		return nil, err
	}

	return rValues, nil
}

func (c *MessagesCore) savedCounters(savedPeer *mtproto.InputPeer, filters []*mtproto.MessagesFilter) (*mtproto.Vector_Messages_SearchCounter, error) {
	counters := &mtproto.Vector_Messages_SearchCounter{
		Datas: make([]*mtproto.Messages_SearchCounter, 0, len(filters)),
	}
	offsetID := int32(math.MaxInt32)
	boxes := make([]*mtproto.MessageBox, 0)
	var total int32
	truncated := false
	for page := 0; page < 10; page++ {
		saved, err := c.loadSavedBoxes(savedPeer, offsetID, savedHistoryPage)
		if err != nil {
			return nil, err
		}
		pageBoxes := []*mtproto.MessageBox{}
		if saved != nil {
			pageBoxes = saved.GetBoxList()
			total = saved.GetCount()
		}
		if len(pageBoxes) == 0 {
			break
		}
		var oldest int32
		for _, box := range pageBoxes {
			if box == nil {
				continue
			}
			boxes = append(boxes, box)
			id := box.GetMessageId()
			if oldest == 0 || (id != 0 && id < oldest) {
				oldest = id
			}
		}
		if int32(len(pageBoxes)) < savedHistoryPage || oldest == 0 || oldest == offsetID {
			break
		}
		if total > 0 && int32(len(boxes)) >= total {
			break
		}
		offsetID = oldest
		if page == 9 {
			truncated = total > int32(len(boxes))
		}
	}
	inexact := truncated

	for _, filter := range filters {
		if filter == nil {
			return nil, mtproto.ErrInputFilterInvalid
		}
		ft := mtproto.FromMessagesFilter(filter)
		var count int32
		switch ft {
		case mtproto.FilterContacts:
			for _, box := range boxes {
				matches, err := c.savedBoxMatches(box, ft, "", 0, 0, 0, 0, 0, 0)
				if err != nil {
					c.Logger.Errorf("messages.getSearchCounters - error: %v", err)
					return nil, err
				}
				if matches {
					count++
				}
			}
		case mtproto.FilterPinned, mtproto.FilterMyMentions, mtproto.FilterGeo:
			for _, box := range boxes {
				if boxMatchesSavedFilter(box, ft) {
					count++
				}
			}
		default:
			if _, ok := savedMediaType(ft); !ok {
				return nil, mtproto.ErrMethodNotImpl
			}
			for _, box := range boxes {
				if boxMatchesSavedFilter(box, ft) {
					count++
				}
			}
		}
		counters.Datas = append(counters.Datas, mtproto.MakeTLMessagesSearchCounter(&mtproto.Messages_SearchCounter{
			Inexact: inexact && count > 0,
			Filter:  filter,
			Count:   count,
		}).To_Messages_SearchCounter())
	}
	return counters, nil
}

func boxInPeer(peer *mtproto.PeerUtil, box *mtproto.MessageBox, selfID int64) bool {
	if peer == nil || box == nil {
		return false
	}
	wantType := peer.PeerType
	wantID := peer.PeerId
	if wantType == mtproto.PEER_SELF {
		wantType = mtproto.PEER_USER
		wantID = selfID
	}
	gotType := box.GetPeerType()
	gotID := box.GetPeerId()
	if gotType == mtproto.PEER_SELF {
		gotType = mtproto.PEER_USER
		if gotID == 0 {
			gotID = selfID
		}
	}
	return gotType == wantType && gotID == wantID
}
