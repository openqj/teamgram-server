// Copyright (c) 2026 The Teamgram Authors (https://teamgram.net).
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
	"github.com/teamgram/proto/mtproto"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// messagesOfBoxes packs loaded boxes the same way messages.getMessages does.
func (c *MessagesCore) messagesOfBoxes(boxes []*mtproto.MessageBox) (*mtproto.Messages_Messages, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil {
		return nil, mtproto.ErrInternalServerError
	}
	rValues := mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
		Messages: []*mtproto.Message{},
		Users:    []*mtproto.User{},
		Chats:    []*mtproto.Chat{},
	}).To_Messages_Messages()

	var readErr error
	(&message.Vector_MessageBox{Datas: boxes}).Visit(c.MD.UserId,
		func(messageList []*mtproto.Message) {
			for _, msg := range messageList {
				if msg == nil || msg.GetId() <= 0 {
					readErr = mtproto.ErrInternalServerError
					return
				}
			}
			rValues.Messages = append(rValues.Messages, messageList...)
		},
		func(userIdList []int64) {
			if readErr != nil || c.svcCtx.Dao.UserClient == nil {
				readErr = mtproto.ErrInternalServerError
				return
			}
			mUsers, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
				Id: userIdList,
			})
			if err != nil {
				readErr = err
				return
			}
			if mUsers == nil {
				readErr = mtproto.ErrInternalServerError
				return
			}
			rValues.Users = append(rValues.Users, mUsers.GetUserListByIdList(c.MD.UserId, userIdList...)...)
		},
		func(chatIdList []int64) {
			if readErr != nil || c.svcCtx.Dao.ChatClient == nil || c.svcCtx.Dao.ChatClient.Client() == nil {
				readErr = mtproto.ErrInternalServerError
				return
			}
			mChats, err := c.svcCtx.Dao.ChatClient.Client().ChatGetChatListByIdList(c.ctx, &chatpb.TLChatGetChatListByIdList{
				IdList: chatIdList,
			})
			if err != nil {
				readErr = err
				return
			}
			if mChats == nil {
				readErr = mtproto.ErrInternalServerError
				return
			}
			rValues.Chats = append(rValues.Chats, mChats.GetChatListByIdList(c.MD.UserId, chatIdList...)...)
		},
		func(channelIdList []int64) {
			if len(channelIdList) > 0 {
				readErr = mtproto.ErrMethodNotImpl
			}
		})

	if readErr != nil {
		return nil, readErr
	}
	return rValues, nil
}

func richPeerMatches(peer *mtproto.PeerUtil, box *mtproto.MessageBox) bool {
	if peer == nil || box == nil {
		return false
	}
	if peer.PeerType == mtproto.PEER_SELF {
		return box.GetPeerId() == peer.SelfId &&
			(box.GetPeerType() == mtproto.PEER_SELF || box.GetPeerType() == mtproto.PEER_USER)
	}
	return box.GetPeerType() == peer.PeerType && box.GetPeerId() == peer.PeerId
}

func plainToRichMessage(text string) *mtproto.RichMessage {
	var blocks []*mtproto.PageBlock
	if text != "" {
		blocks = []*mtproto.PageBlock{
			mtproto.MakeTLPageBlockParagraph(&mtproto.PageBlock{
				Text: mtproto.MakeTLTextPlain(&mtproto.RichText{Text_STRING: text}).To_RichText(),
			}).To_PageBlock(),
		}
	}
	return mtproto.MakeTLRichMessage(&mtproto.RichMessage{Blocks: blocks}).To_RichMessage()
}

func inputRichEmpty(in *mtproto.InputRichMessage) bool {
	if in == nil {
		return true
	}
	return len(in.GetBlocks()) == 0 &&
		in.GetHtml() == "" &&
		in.GetMarkdown() == "" &&
		len(in.GetPhotos()) == 0 &&
		len(in.GetDocuments()) == 0 &&
		len(in.GetFiles()) == 0 &&
		len(in.GetUsers()) == 0
}

func inputPhotoToRichPhoto(in *mtproto.InputPhoto) (*mtproto.Photo, error) {
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	value := &mtproto.Photo{
		Id:            in.GetId(),
		AccessHash:    in.GetAccessHash(),
		FileReference: in.GetFileReference(),
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputPhotoEmpty:
		return mtproto.MakeTLPhotoEmpty(value).To_Photo(), nil
	case mtproto.Predicate_inputPhoto:
		if value.Id <= 0 || value.AccessHash == 0 {
			return nil, mtproto.ErrInputConstructorInvalid
		}
		return mtproto.MakeTLPhoto(value).To_Photo(), nil
	default:
		return nil, mtproto.ErrInputConstructorInvalid
	}
}

func inputDocumentToRichDocument(in *mtproto.InputDocument) (*mtproto.Document, error) {
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	value := &mtproto.Document{
		Id:            in.GetId(),
		AccessHash:    in.GetAccessHash(),
		FileReference: in.GetFileReference(),
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputDocumentEmpty:
		return mtproto.MakeTLDocumentEmpty(value).To_Document(), nil
	case mtproto.Predicate_inputDocument:
		if value.Id <= 0 || value.AccessHash == 0 {
			return nil, mtproto.ErrInputConstructorInvalid
		}
		return mtproto.MakeTLDocument(value).To_Document(), nil
	default:
		return nil, mtproto.ErrInputConstructorInvalid
	}
}

// inputRichToRich keeps the source text. No translator or model is configured.
func inputRichToRich(in *mtproto.InputRichMessage) (*mtproto.RichMessage, error) {
	if in == nil {
		return mtproto.MakeTLRichMessage(&mtproto.RichMessage{}).To_RichMessage(), nil
	}
	if len(in.GetFiles()) > 0 || len(in.GetUsers()) > 0 {
		return nil, mtproto.ErrMethodNotImpl
	}
	blocks := in.GetBlocks()
	if len(blocks) == 0 {
		text := in.GetMarkdown()
		if text == "" {
			text = in.GetHtml()
		}
		if text != "" {
			blocks = plainToRichMessage(text).GetBlocks()
		}
	}
	photos := make([]*mtproto.Photo, 0, len(in.GetPhotos()))
	for _, photo := range in.GetPhotos() {
		value, err := inputPhotoToRichPhoto(photo)
		if err != nil {
			return nil, err
		}
		photos = append(photos, value)
	}
	documents := make([]*mtproto.Document, 0, len(in.GetDocuments()))
	for _, document := range in.GetDocuments() {
		value, err := inputDocumentToRichDocument(document)
		if err != nil {
			return nil, err
		}
		documents = append(documents, value)
	}
	return mtproto.MakeTLRichMessage(&mtproto.RichMessage{
		Rtl:       in.GetRtl(),
		Blocks:    blocks,
		Photos:    photos,
		Documents: documents,
	}).To_RichMessage(), nil
}

func messageToRich(msg *mtproto.Message) *mtproto.RichMessage {
	if msg == nil {
		return mtproto.MakeTLRichMessage(&mtproto.RichMessage{}).To_RichMessage()
	}
	if rm := msg.GetRichMessage(); rm != nil {
		return rm
	}
	return plainToRichMessage(msg.GetMessage())
}
