package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
)

func (c *MessagesCore) searchChannel(in *mtproto.TLMessagesSearch, offsetID, limit int32) (*mtproto.Messages_Messages, error) {
	var sender int64
	if in.GetFromId() != nil {
		fromID := mtproto.FromInputPeer2(c.MD.UserId, in.FromId)
		if fromID == nil || !fromID.IsUser() {
			err := mtproto.ErrFromPeerInvalid
			c.Logger.Errorf("messages.search - error: %v", err)
			return nil, err
		}
		sender = fromID.PeerId
	}

	filter := in.GetFilter()
	if filter == nil {
		filter = mtproto.MakeTLInputMessagesFilterEmpty(&mtproto.MessagesFilter{}).To_MessagesFilter()
	}
	switch mtproto.FromMessagesFilter(filter) {
	case mtproto.FilterEmpty:
		if sender == 0 && in.Q == "" {
			err := mtproto.ErrSearchQueryEmpty
			c.Logger.Errorf("messages.search - error: %v", err)
			return nil, err
		}
		box, err := channelview.SearchForInputPeer(c.MD.UserId, in.GetPeer(), in.Q, sender, offsetID, in.AddOffset, in.MinDate, in.MaxDate, in.MinId, in.MaxId, limit)
		if err != nil {
			c.Logger.Errorf("messages.search - error: %v", err)
			return nil, err
		}
		return box, nil
	case mtproto.FilterPinned:
		channelID, err := channelview.ValidateInputPeer(c.MD.UserId, in.GetPeer())
		if err != nil {
			c.Logger.Errorf("messages.search - error: %v", err)
			return nil, err
		}
		box, err := channelview.Pinned(c.MD.UserId, channelID, limit)
		if err != nil {
			c.Logger.Errorf("messages.search - error: %v", err)
			return nil, err
		}
		return box, nil
	case mtproto.FilterPhotos, mtproto.FilterVideo, mtproto.FilterPhotoVideo, mtproto.FilterDocument,
		mtproto.FilterUrl, mtproto.FilterGif, mtproto.FilterVoice, mtproto.FilterMusic, mtproto.FilterChatPhotos,
		mtproto.FilterPhoneCalls, mtproto.FilterRoundVoice, mtproto.FilterRoundVideo, mtproto.FilterMyMentions,
		mtproto.FilterGeo, mtproto.FilterContacts:
		channelID, err := channelview.ValidateInputPeer(c.MD.UserId, in.GetPeer())
		if err != nil {
			c.Logger.Errorf("messages.search - error: %v", err)
			return nil, err
		}
		box, err := channelview.Empty(c.MD.UserId, channelID)
		if err != nil {
			c.Logger.Errorf("messages.search - error: %v", err)
			return nil, err
		}
		return box, nil
	default:
		err := mtproto.ErrInputFilterInvalid
		c.Logger.Errorf("messages.search - error: %v", err)
		return nil, err
	}
}
