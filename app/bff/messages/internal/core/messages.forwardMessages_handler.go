// Copyright 2022 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"context"
	"math/rand"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// MessagesForwardMessages
// messages.forwardMessages#cc30290b flags:# silent:flags.5?true background:flags.6?true with_my_score:flags.8?true drop_author:flags.11?true drop_media_captions:flags.12?true noforwards:flags.14?true from_peer:InputPeer id:Vector<int> random_id:Vector<long> to_peer:InputPeer schedule_date:flags.10?int send_as:flags.13?InputPeer = Updates;
func (c *MessagesCore) MessagesForwardMessages(in *mtproto.TLMessagesForwardMessages) (*mtproto.Updates, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if len(in.Id) == 0 || len(in.RandomId) == 0 || len(in.Id) != len(in.RandomId) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if scheduleDate := in.GetScheduleDate(); scheduleDate != nil && scheduleDate.GetValue() == 0 {
		return nil, mtproto.ErrScheduleDateInvalid
	}
	toPeer, err := c.validateForwardDestinationPeer(in.GetToPeer())
	if err != nil {
		c.Logger.Errorf("messages.forwardMessages destination peer: %v", err)
		return nil, err
	}
	fromPeer, err := c.validateForwardSourcePeer(in.GetFromPeer())
	if err != nil {
		c.Logger.Errorf("messages.forwardMessages source peer: %v", err)
		return nil, err
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil || c.svcCtx.Dao.MsgClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	var (
		rUpdates *mtproto.Updates
		saved    = false
	)

	//if c.MD.IsBot {
	//	err := mtproto.ErrBotMethodInvalid
	//	c.Logger.Errorf("messages.forwardMessages - error: %v", err)
	//	return nil, err
	//}

	/*
	   ## android's from_peer maybe is empty
	   if (msgObj.messageOwner.to_id instanceof TLRPC.TL_peerChannel) {
	       TLRPC.Chat chat = MessagesController.getInstance(currentAccount).getChat(msgObj.messageOwner.to_id.channel_id);
	       req.from_peer = new TLRPC.TL_inputPeerChannel();
	       req.from_peer.channel_id = msgObj.messageOwner.to_id.channel_id;
	       if (chat != null) {
	           req.from_peer.access_hash = chat.access_hash;
	       }
	   } else {
	       req.from_peer = new TLRPC.TL_inputPeerEmpty();
	   }
	*/

	switch toPeer.PeerType {
	case mtproto.PEER_SELF:
		toPeer.PeerType = mtproto.PEER_USER
		saved = true
	case mtproto.PEER_USER:
		if toPeer.PeerId == c.MD.UserId {
			saved = true
		}
	case mtproto.PEER_CHAT:
	case mtproto.PEER_CHANNEL:
	default:
		err = mtproto.ErrPeerIdInvalid
		c.Logger.Errorf("messages.forwardMessages#708e0195 - error: %v", err)
		return nil, err
	}

	when := in.GetScheduleDate().GetValue()
	storedTextFallback := when != 0 || toPeer.IsChannel() || fromPeer.PeerType == mtproto.PEER_CHANNEL
	if storedTextFallback {
		if err = validateStoredTextForwardOptions(in); err != nil {
			return nil, err
		}
		if when != 0 || toPeer.IsChannel() {
			return nil, mtproto.ErrMethodNotImpl
		}
		texts, ferr := c.forwardTexts(fromPeer, in.Id)
		if ferr != nil {
			c.Logger.Errorf("messages.forwardMessages stored: %v", ferr)
			return nil, ferr
		}
		return c.forwardPlain(toPeer, in.RandomId, texts)
	}

	fwdOutboxList, err := c.makeForwardMessages(fromPeer, toPeer, saved, in)
	if err != nil {
		c.Logger.Errorf("messages.forwardMessages#708e0195 - error: %v", err)
		return nil, err
	}

	rUpdates, err = c.svcCtx.Dao.MsgClient.MsgSendMessageV2(
		c.ctx,
		&msgpb.TLMsgSendMessageV2{
			UserId:    c.MD.UserId,
			AuthKeyId: c.MD.PermAuthKeyId,
			PeerType:  toPeer.PeerType,
			PeerId:    toPeer.PeerId,
			Message:   fwdOutboxList,
		})
	if err != nil {
		c.Logger.Errorf("messages.forwardMessages - error: %v", err)
		return nil, err
	}
	if rUpdates == nil {
		return nil, mtproto.ErrInternalServerError
	}

	return rUpdates, err
}

func validateStoredTextForwardOptions(in *mtproto.TLMessagesForwardMessages) error {
	if in == nil {
		return mtproto.ErrInputRequestInvalid
	}
	if scheduleDate := in.GetScheduleDate(); scheduleDate != nil {
		when := int64(scheduleDate.GetValue())
		now := time.Now().Unix()
		if when <= now {
			return mtproto.ErrScheduleDateInvalid
		}
		if when > now+365*24*60*60 {
			return mtproto.ErrScheduleDateTooLate
		}
		return mtproto.ErrMethodNotImpl
	}
	if !in.GetDropAuthor() || in.GetSilent() || in.GetBackground() || in.GetWithMyScore() || in.GetNoforwards() ||
		in.GetAllowPaidFloodskip() || in.GetSendAs() != nil || in.GetTopMsgId() != nil || in.GetReplyTo() != nil ||
		in.GetScheduleRepeatPeriod() != nil || in.GetQuickReplyShortcut() != nil || in.GetEffect() != nil ||
		in.GetVideoTimestamp() != nil || in.GetAllowPaidStars() != nil || in.GetSuggestedPost() != nil {
		return mtproto.ErrMethodNotImpl
	}
	return nil
}

func (c *MessagesCore) validateForwardSourcePeer(input *mtproto.InputPeer) (*mtproto.PeerUtil, error) {
	if input == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, input)
	switch input.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		if peer.PeerId != c.MD.UserId {
			return nil, mtproto.ErrPeerIdInvalid
		}
	case mtproto.Predicate_inputPeerUser:
		if err := c.validateForwardUserPeer(input); err != nil {
			return nil, err
		}
	case mtproto.Predicate_inputPeerChat:
		if err := c.validateForwardChatMember(input.GetChatId()); err != nil {
			return nil, err
		}
	case mtproto.Predicate_inputPeerChannel:
		if err := c.validateForwardChannelPeer(input, false); err != nil {
			return nil, err
		}
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}
	return peer, nil
}

func (c *MessagesCore) validateForwardDestinationPeer(input *mtproto.InputPeer) (*mtproto.PeerUtil, error) {
	if input == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, input)
	switch input.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		if peer.PeerId != c.MD.UserId {
			return nil, mtproto.ErrPeerIdInvalid
		}
	case mtproto.Predicate_inputPeerUser:
		if err := c.validateForwardUserPeer(input); err != nil {
			return nil, err
		}
	case mtproto.Predicate_inputPeerChat:
		if err := c.validateForwardChatMember(input.GetChatId()); err != nil {
			return nil, err
		}
	case mtproto.Predicate_inputPeerChannel:
		if err := c.validateForwardChannelPeer(input, true); err != nil {
			return nil, err
		}
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}
	return peer, nil
}

func (c *MessagesCore) validateForwardUserPeer(input *mtproto.InputPeer) error {
	if input == nil || input.GetUserId() <= 0 || input.GetAccessHash() == 0 {
		return mtproto.ErrPeerIdInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return mtproto.ErrInternalServerError
	}
	users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
		Id: []int64{input.GetUserId()},
		To: []int64{c.MD.UserId},
	})
	if err != nil {
		return err
	}
	if users == nil {
		return mtproto.ErrInternalServerError
	}
	for _, item := range users.GetDatas() {
		if item != nil && item.GetUser() != nil && !item.GetUser().GetDeleted() &&
			item.GetUser().GetId() == input.GetUserId() && item.GetUser().GetAccessHash() == input.GetAccessHash() {
			return nil
		}
	}
	return mtproto.ErrPeerIdInvalid
}

func (c *MessagesCore) validateForwardChatMember(chatID int64) error {
	if chatID <= 0 {
		return mtproto.ErrPeerIdInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.ChatClient == nil || c.svcCtx.Dao.ChatClient.Client() == nil {
		return mtproto.ErrInternalServerError
	}
	users, err := c.svcCtx.Dao.ChatClient.Client().ChatGetUsersChatIdList(c.ctx, &chatpb.TLChatGetUsersChatIdList{
		Id: []int64{c.MD.UserId},
	})
	if err != nil {
		return err
	}
	if users == nil {
		return mtproto.ErrInternalServerError
	}
	for _, item := range users.GetDatas() {
		if item == nil || item.GetUserId() != c.MD.UserId {
			continue
		}
		for _, id := range item.GetChatIdList() {
			if id == chatID {
				return nil
			}
		}
	}
	return mtproto.ErrUserNotParticipant
}

func (c *MessagesCore) validateForwardChannelPeer(input *mtproto.InputPeer, requirePost bool) error {
	if input == nil || input.GetChannelId() <= 0 || input.GetAccessHash() == 0 {
		return mtproto.ErrChannelInvalid
	}
	history, err := channelview.HistoryForInputPeer(c.MD.UserId, input, 0, 1)
	if err != nil {
		return err
	}
	if history == nil {
		return mtproto.ErrInternalServerError
	}
	if requirePost {
		for _, chat := range history.GetChats() {
			if chat != nil && chat.GetId() == input.GetChannelId() && chat.GetCreator() {
				return nil
			}
		}
		return mtproto.ErrChatAdminRequired
	}
	return nil
}

func (c *MessagesCore) checkForwardPrivacy(ctx context.Context, selfUserId, checkId int64) (bool, error) {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return false, mtproto.ErrInternalServerError
	}
	rules, err := c.svcCtx.Dao.UserClient.UserGetPrivacy(ctx, &userpb.TLUserGetPrivacy{
		UserId:  selfUserId,
		KeyType: mtproto.FORWARDS,
	})
	if err != nil {
		return false, err
	}
	if rules == nil {
		return false, mtproto.ErrInternalServerError
	}

	if len(rules.Datas) == 0 {
		return true, nil
	}
	for _, rule := range rules.Datas {
		if rule == nil {
			return false, mtproto.ErrInternalServerError
		}
	}
	var privacyErr error
	allowed := mtproto.CheckPrivacyIsAllow(
		selfUserId,
		rules.Datas,
		checkId,
		func(id, checkId int64) bool {
			contact, err := c.svcCtx.Dao.UserClient.UserCheckContact(ctx, &userpb.TLUserCheckContact{
				UserId: id,
				Id:     checkId,
			})
			if err != nil {
				privacyErr = err
				return false
			}
			if contact == nil {
				privacyErr = mtproto.ErrInternalServerError
				return false
			}
			return mtproto.FromBool(contact)
		},
		func(checkId int64, idList []int64) bool {
			if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.ChatClient == nil || c.svcCtx.Dao.ChatClient.Client() == nil {
				privacyErr = mtproto.ErrInternalServerError
				return false
			}
			chatIdList, _ := mtproto.SplitChatAndChannelIdList(idList)
			if len(chatIdList) == 0 {
				return false
			}
			users, err := c.svcCtx.Dao.ChatClient.Client().ChatGetUsersChatIdList(ctx, &chatpb.TLChatGetUsersChatIdList{
				Id: []int64{checkId},
			})
			if err != nil {
				privacyErr = err
				return false
			}
			if users == nil {
				privacyErr = mtproto.ErrInternalServerError
				return false
			}
			for _, item := range users.GetDatas() {
				if item == nil || item.GetUserId() != checkId {
					continue
				}
				for _, chatID := range item.GetChatIdList() {
					for _, wantedID := range chatIdList {
						if chatID == wantedID {
							return true
						}
					}
				}
			}
			return false
		})
	if privacyErr != nil {
		return false, privacyErr
	}
	return allowed, nil
}

func (c *MessagesCore) makeForwardMessages(
	fromPeer, toPeer *mtproto.PeerUtil,
	saved bool,
	request *mtproto.TLMessagesForwardMessages) ([]*msgpb.OutboxMessage, error) {

	var (
		idList  = request.Id
		ridList = request.RandomId
		now     = int32(time.Now().Unix())
		// messageList []*mtproto.Message
		// err error
	)

	// TODO(@benqi): sorted map
	findRandomIdById := func(id int32) int64 {
		for i := 0; i < len(idList); i++ {
			if id == idList[i] {
				return ridList[i]
			}
		}
		return 0
	}

	var (
		messageList *message.Vector_MessageBox
		err         error
	)

	switch fromPeer.PeerType {
	case mtproto.PEER_CHANNEL:
		c.Logger.Errorf("messages.forwardMessages - error: %v", mtproto.ErrChannelInvalid)
		return nil, mtproto.ErrChannelInvalid
	default:
		if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil {
			return nil, mtproto.ErrInternalServerError
		}
		messageList, err = c.svcCtx.Dao.MessageClient.MessageGetUserMessageList(c.ctx, &message.TLMessageGetUserMessageList{
			UserId: c.MD.UserId,
			IdList: idList,
		})
		if err != nil {
			return nil, err
		}
		if messageList == nil {
			return nil, mtproto.ErrInternalServerError
		}
		if err = validateForwardMessageBoxes(c.MD.UserId, fromPeer, idList, messageList); err != nil {
			return nil, err
		}
		if messageList.Length() > 0 {
			msgBox0 := messageList.Datas[0]
			if msgBox0.PeerType == mtproto.PEER_CHAT {
				if c.svcCtx.Dao.ChatClient == nil || c.svcCtx.Dao.ChatClient.Client() == nil {
					return nil, mtproto.ErrInternalServerError
				}
				chat, err := c.svcCtx.Dao.ChatClient.Client().ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{
					ChatId: msgBox0.PeerId,
				})
				if err != nil {
					c.Logger.Errorf("messages.forwardMessages - error: %v", err)
					return nil, err
				}
				if chat == nil || chat.GetChat() == nil || chat.GetChat().GetId() != msgBox0.PeerId {
					return nil, mtproto.ErrInternalServerError
				}

				if chat.Noforwards() {
					err = mtproto.ErrChatForwardsRestricted
					c.Logger.Errorf("messages.forwardMessages - error: %v", err)
					return nil, err
				}
			}
		}
	}

	fwdOutboxList := make([]*msgpb.OutboxMessage, 0, int(messageList.Length()))
	groupedIds := make(map[int64]int64)
	for i := len(messageList.Datas) - 1; i >= 0; i-- {
		box := messageList.Datas[i]
		m := box.Message
		// TODO(@benqi): rid is 0

		if m.GetGroupedId() != nil {
			groupedId := m.GetGroupedId().GetValue()
			if _, ok := groupedIds[groupedId]; !ok {
				groupedIds[groupedId] = rand.Int63()
			}
		}
		if mtproto.IsMusicMessage(m) {
			m.FwdFrom = nil
		} else {
			if m.FwdFrom == nil {
				fwdFrom := mtproto.MakeTLMessageFwdHeader(&mtproto.MessageFwdHeader{
					Imported:       false,
					SavedOut:       false,
					FromId:         nil,
					FromName:       nil,
					Date:           m.GetDate(),
					ChannelPost:    nil,
					PostAuthor:     nil,
					SavedFromPeer:  nil,
					SavedFromMsgId: nil,
					SavedFromId:    nil,
					SavedFromName:  nil,
					SavedDate:      nil,
					PsaType:        nil,
				}).To_MessageFwdHeader()

				//fwdFrom := mtproto.MakeTLMessageFwdHeader(&mtproto.MessageFwdHeader{
				//	// FromId: m.GetFromId(),
				//	Date: m.GetDate(),
				//}).To_MessageFwdHeader()

				if m.Views != nil {
					// Broadcast
					// fwdFrom.ChannelId = &wrapperspb.Int32Value{Value: fromPeer.PeerId}
					fwdFrom.ChannelPost = &wrapperspb.Int32Value{Value: m.Id}
					fwdFrom.PostAuthor = m.PostAuthor
					fwdFrom.FromId = mtproto.MakePeerChannel(fromPeer.PeerId)
					// TODO(@benqi): saved_from_peer and saved_from_msg_id??
				} else {
					fromId := box.SenderUserId
					if fromId <= 0 {
						return nil, mtproto.ErrInternalServerError
					}
					allowed, err := c.checkForwardPrivacy(c.ctx, fromId, c.MD.UserId)
					if err != nil {
						return nil, err
					}
					if allowed {
						fwdFrom.FromId = mtproto.MakePeerUser(fromId)
					} else {
						uname, err := c.svcCtx.Dao.UserClient.UserGetAccountUsername(c.ctx, &userpb.TLUserGetAccountUsername{
							UserId: fromId,
						})
						if err != nil {
							return nil, err
						}
						if uname == nil {
							return nil, mtproto.ErrInternalServerError
						}
						fwdFrom.FromName = &wrapperspb.StringValue{Value: uname.GetUsername()}
					}
					m.Post = false
					m.PostAuthor = nil
				}

				if saved {
					if m.Views != nil {
						// fwdFrom
						fwdFrom.SavedFromPeer = box.Message.GetPeerId()
					} else {
						fwdFrom.SavedFromPeer = mtproto.MakePeerUser(box.SenderUserId)
					}
					fwdFrom.SavedFromMsgId = &wrapperspb.Int32Value{Value: m.Id}
					m.SavedPeerId = fwdFrom.SavedFromPeer
				} else {
					m.SavedPeerId = nil
				}
				m.FwdFrom = fwdFrom
			} else {
				if saved {
					if m.Views != nil {
						// fwdFrom
						m.FwdFrom.SavedFromPeer = box.Message.GetFromId()
					} else {
						m.FwdFrom.SavedFromPeer = mtproto.MakePeerUser(box.SenderUserId)
					}
					m.FwdFrom.SavedFromMsgId = &wrapperspb.Int32Value{Value: m.Id}
					m.SavedPeerId = m.FwdFrom.SavedFromPeer
				} else {
					m.FwdFrom.SavedFromPeer = nil
					m.FwdFrom.SavedFromMsgId = nil
					m.SavedPeerId = nil
				}
			}
		}

		// TODO(@benqi): make message, ref sendMessage
		m.PeerId = toPeer.ToPeer()
		m.FromId = mtproto.MakePeerUser(c.MD.UserId)
		m.Date = now
		m.Silent = request.Silent
		m.Post = false
		if m.GetGroupedId() != nil {
			groupedId := groupedIds[m.GetGroupedId().GetValue()]
			m.GroupedId = mtproto.MakeFlagsInt64(groupedId)
		} else {
			m.GroupedId = nil
		}
		m.ReplyTo = nil
		m.Reactions = nil
		if m.ReplyMarkup != nil {
			m.ReplyMarkup = m.ReplyMarkup.ToForwardMessage()
		}

		fwdOutboxList = append(fwdOutboxList, &msgpb.OutboxMessage{
			NoWebpage:    true,
			Background:   false,
			RandomId:     findRandomIdById(box.GetMessageId()),
			Message:      m,
			ScheduleDate: request.GetScheduleDate(),
		})
	}

	return fwdOutboxList, nil
}
