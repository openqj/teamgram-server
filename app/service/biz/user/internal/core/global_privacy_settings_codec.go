package core

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type globalPrivacyDisallowedGiftsJSON struct {
	DisallowUnlimitedStargifts    bool `json:"disallow_unlimited_stargifts"`
	DisallowLimitedStargifts      bool `json:"disallow_limited_stargifts"`
	DisallowUniqueStargifts       bool `json:"disallow_unique_stargifts"`
	DisallowPremiumGifts          bool `json:"disallow_premium_gifts"`
	DisallowStargiftsFromChannels bool `json:"disallow_stargifts_from_channels"`
}

func encodeGlobalPrivacyDisallowedGifts(settings *mtproto.DisallowedGiftsSettings) (sql.NullString, error) {
	if settings == nil {
		return sql.NullString{}, nil
	}

	payload, err := json.Marshal(globalPrivacyDisallowedGiftsJSON{
		DisallowUnlimitedStargifts:    settings.GetDisallowUnlimitedStargifts(),
		DisallowLimitedStargifts:      settings.GetDisallowLimitedStargifts(),
		DisallowUniqueStargifts:       settings.GetDisallowUniqueStargifts(),
		DisallowPremiumGifts:          settings.GetDisallowPremiumGifts(),
		DisallowStargiftsFromChannels: settings.GetDisallowStargiftsFromChannels(),
	})
	if err != nil {
		return sql.NullString{}, fmt.Errorf("encode disallowed gifts settings: %w", err)
	}

	return sql.NullString{String: string(payload), Valid: true}, nil
}

func decodeGlobalPrivacyDisallowedGifts(value sql.NullString) (*mtproto.DisallowedGiftsSettings, error) {
	if !value.Valid {
		return nil, nil
	}

	var payload *globalPrivacyDisallowedGiftsJSON
	if err := json.Unmarshal([]byte(value.String), &payload); err != nil {
		return nil, fmt.Errorf("decode disallowed gifts settings: %w", err)
	}
	if payload == nil {
		return nil, nil
	}

	return mtproto.MakeTLDisallowedGiftsSettings(&mtproto.DisallowedGiftsSettings{
		DisallowUnlimitedStargifts:    payload.DisallowUnlimitedStargifts,
		DisallowLimitedStargifts:      payload.DisallowLimitedStargifts,
		DisallowUniqueStargifts:       payload.DisallowUniqueStargifts,
		DisallowPremiumGifts:          payload.DisallowPremiumGifts,
		DisallowStargiftsFromChannels: payload.DisallowStargiftsFromChannels,
	}).To_DisallowedGiftsSettings(), nil
}

func globalPrivacyPaidStars(settings *mtproto.GlobalPrivacySettings) sql.NullInt64 {
	if settings == nil || settings.GetNoncontactPeersPaidStars() == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: settings.GetNoncontactPeersPaidStars().GetValue(), Valid: true}
}

func globalPrivacyPaidStarsValue(value sql.NullInt64) *wrapperspb.Int64Value {
	if !value.Valid {
		return nil
	}
	return wrapperspb.Int64(value.Int64)
}
