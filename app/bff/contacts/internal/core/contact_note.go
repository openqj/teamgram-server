package core

import "github.com/teamgram/proto/mtproto"

// contactNote is the per-(self, contact) blob kept in persist.Default.
type contactNote struct {
	Text     string                   `json:"text"`
	Entities []*mtproto.MessageEntity `json:"entities,omitempty"`
}
