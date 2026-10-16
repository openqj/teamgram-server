/*
 * WARNING: This is a temporary stub file for development.
 * These types should be replaced by properly generated protobuf code.
 *
 * To generate the real protobuf code, run:
 * [protoc command or script to be determined]
 */

package dialog

// Temporary type stubs for recommendation feature
// TODO: Replace with generated protobuf code

// RecommendedDialog represents a single recommended dialog
type RecommendedDialog struct {
	PeerType           int32
	PeerId             int64
	RecommendationType int32
	IsPromoted         bool
	Score              float64
}

// RecommendedDialogs is the response containing a list of recommendations
type RecommendedDialogs struct {
	Dialogs []*RecommendedDialog
}

// TLDialogGetRecommendedDialogs request message
type TLDialogGetRecommendedDialogs struct {
	UserId int64
	Offset int32
	Limit  int32
}

// TLDialogRefreshRecommendations request message
type TLDialogRefreshRecommendations struct {
	UserId int64
}

// TLDialogReportRecommendationFeedback request message
type TLDialogReportRecommendationFeedback struct {
	UserId       int64
	PeerType     int32
	PeerId       int64
	FeedbackType int32
}

// TLDialogAddPromotedDialog request message
type TLDialogAddPromotedDialog struct {
	PeerType      int32
	PeerId        int64
	Priority      int32
	ExpiresAt     int64
	TargetUserIds []int64
}

// TLDialogRemovePromotedDialog request message
type TLDialogRemovePromotedDialog struct {
	PeerType      int32
	PeerId        int64
	TargetUserIds []int64
}
