//go:build recommendation_experimental

package core

import "github.com/teamgram/proto/mtproto"

// These request types are deliberately local to the experimental build. They
// provide a compile-time adapter for an out-of-tree schema and are not part of
// the generated Layer 229 MTProto registry.
type TLMessagesGetRecommendedDialogs struct {
	Offset int32
	Limit  int32
}

func (x *TLMessagesGetRecommendedDialogs) GetOffset() int32 {
	if x == nil {
		return 0
	}
	return x.Offset
}

func (x *TLMessagesGetRecommendedDialogs) GetLimit() int32 {
	if x == nil {
		return 0
	}
	return x.Limit
}

type TLMessagesReportRecommendationFeedback struct {
	Peer         *mtproto.InputPeer
	FeedbackType int32
}

func (x *TLMessagesReportRecommendationFeedback) GetPeer() *mtproto.InputPeer {
	if x == nil {
		return nil
	}
	return x.Peer
}

func (x *TLMessagesReportRecommendationFeedback) GetFeedbackType() int32 {
	if x == nil {
		return 0
	}
	return x.FeedbackType
}

type recommendationPeer struct {
	peerType int32
	peerID   int64
}
