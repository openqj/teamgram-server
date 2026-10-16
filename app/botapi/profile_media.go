package botapi

import (
	"strconv"

	"github.com/teamgram/proto/mtproto"
)

// UserProfileAudios is the Bot API representation returned by
// getUserProfileAudios. Teamgram's users.getSavedMusic response carries the
// same count and document list under its Users_SavedMusic constructor.
type UserProfileAudios struct {
	TotalCount int      `json:"total_count"`
	Audios     []*Audio `json:"audios"`
}

// UserProfileAudiosFromMTProto converts the Layer 229 profile music response
// without exposing the MTProto document constructors to Bot API clients.
func UserProfileAudiosFromMTProto(in *mtproto.Users_SavedMusic) *UserProfileAudios {
	if in == nil {
		return &UserProfileAudios{Audios: []*Audio{}}
	}
	result := &UserProfileAudios{
		TotalCount: int(in.GetCount()),
		Audios:     make([]*Audio, 0, len(in.GetDocuments())),
	}
	for _, document := range in.GetDocuments() {
		if audio := audioFromDocument(document); audio != nil {
			result.Audios = append(result.Audios, audio)
		}
	}
	return result
}

// CustomEmojiStickersFromMTProto maps the document vector returned by
// messages.getCustomEmojiDocuments to Bot API Sticker objects.
func CustomEmojiStickersFromMTProto(documents []*mtproto.Document) []*Sticker {
	result := make([]*Sticker, 0, len(documents))
	for _, document := range documents {
		sticker := stickerFromDocument(document)
		if sticker == nil {
			continue
		}
		sticker.FileID = fileID("document", document.GetId(), document.GetAccessHash())
		sticker.FileUniqueID = strconv.FormatInt(document.GetId(), 10)
		if sticker.Type == "custom_emoji" && sticker.CustomEmojiID == "" {
			sticker.CustomEmojiID = strconv.FormatInt(document.GetId(), 10)
		}
		result = append(result, sticker)
	}
	return result
}

func audioFromDocument(document *mtproto.Document) *Audio {
	if document == nil {
		return nil
	}
	name, duration, _, _, performer, title, _, _, _, _ := documentAttributes(document)
	return &Audio{
		FileID:       fileID("document", document.GetId(), document.GetAccessHash()),
		FileUniqueID: strconv.FormatInt(document.GetId(), 10),
		Duration:     duration,
		Performer:    performer,
		Title:        title,
		FileName:     name,
		MimeType:     document.GetMimeType(),
		FileSize:     documentSize(document),
	}
}
