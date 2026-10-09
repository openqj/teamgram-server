package state

import (
	"context"
	"database/sql"
	"errors"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func OpenPostgresReadOnly(dsn string) error {
	return domain.OpenPostgresReadOnly(dsn)
}

func ClosePostgres() error {
	return domain.Close()
}

// SetChannelEmojiStickerSet stores the channel emoji binding after the caller
// has checked the actor's current channel rights in the Biz Chat service.
func SetChannelEmojiStickerSet(ctx context.Context, actorID, channelID int64, input *mtproto.InputStickerSet) error {
	if actorID <= 0 || channelID <= 0 || input == nil {
		return mtproto.ErrInputRequestInvalid
	}
	setID := int64(0)
	switch input.GetPredicateName() {
	case mtproto.Predicate_inputStickerSetEmpty:
	case mtproto.Predicate_inputStickerSetID:
		if input.GetId() <= 0 {
			return mtproto.ErrStickersetInvalid
		}
		setID = input.GetId()
	case mtproto.Predicate_inputStickerSetShortName:
		if input.GetShortName() == "" {
			return mtproto.ErrStickersetInvalid
		}
	default:
		return mtproto.ErrStickersetInvalid
	}
	if setID != 0 || input.GetPredicateName() == mtproto.Predicate_inputStickerSetShortName {
		if ctx == nil {
			ctx = context.Background()
		}
		set, err := persist.GetStickerSet(ctx, actorID, setID, input.GetShortName())
		if err != nil {
			if errors.Is(err, persist.ErrStickerProviderUnavailable) {
				return mtproto.ErrMethodNotImpl
			}
			return err
		}
		if set == nil || !set.Emojis || (input.GetPredicateName() == mtproto.Predicate_inputStickerSetID && input.GetAccessHash() != set.AccessHash) {
			return mtproto.ErrStickersetInvalid
		}
		setID = set.ID
	}
	if err := domain.SetChannelEmojiStickerSetAfterAuthorization(actorID, channelID, setID); err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidChannelMember):
			return mtproto.ErrUserIdInvalid
		case errors.Is(err, sql.ErrNoRows):
			return mtproto.ErrStickersetInvalid
		default:
			return err
		}
	}
	return nil
}

func LoadChannelEmojiStickerSet(channelID int64) (int64, bool, error) {
	return domain.LoadChannelEmojiStickerSet(channelID)
}
