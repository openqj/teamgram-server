/*
 * Copyright (c) 2024-present,  Teamgram Authors.
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// MessagesReportRecommendationFeedback
// messages.reportRecommendationFeedback#b2c3d4e5 peer:InputPeer feedback_type:int = Bool;
func (c *MessagesCore) MessagesReportRecommendationFeedback(in *mtproto.TLMessagesReportRecommendationFeedback) (*mtproto.Bool, error) {
	var (
		peer         = in.GetPeer()
		feedbackType = in.GetFeedbackType()
	)

	// 解析 peer
	peerType, peerId := c.getPeerTypeAndId(peer)

	// 调用 Dialog 服务记录反馈
	_, err := c.svcCtx.Svc.DialogClient().DialogReportRecommendationFeedback(c.ctx, &dialog.TLDialogReportRecommendationFeedback{
		UserId:       c.MD.UserId,
		PeerType:     peerType,
		PeerId:       peerId,
		FeedbackType: feedbackType,
	})

	if err != nil {
		c.Logger.Errorf("DialogReportRecommendationFeedback error: %v", err)
		return mtproto.BoolFalse, err
	}

	return mtproto.BoolTrue, nil
}

// getPeerTypeAndId 从 InputPeer 中提取 peer_type 和 peer_id
func (c *MessagesCore) getPeerTypeAndId(peer *mtproto.InputPeer) (int32, int64) {
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerUser:
		return 1, peer.GetUserId()
	case mtproto.Predicate_inputPeerChat:
		return 2, peer.GetChatId()
	case mtproto.Predicate_inputPeerChannel:
		return 3, peer.GetChannelId()
	default:
		return 0, 0
	}
}
