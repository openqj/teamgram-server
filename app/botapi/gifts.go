package botapi

import (
	"strconv"

	"github.com/teamgram/proto/mtproto"
)

// Gifts is the result object returned by getAvailableGifts.
type Gifts struct {
	Gifts []*Gift `json:"gifts"`
}

// StarGiftsFromMTProto converts the Layer 229 payments.getStarGifts result to
// the Bot API gift list shape while preserving the embedded sticker metadata.
func StarGiftsFromMTProto(in *mtproto.Payments_StarGifts) []*Gift {
	if in == nil {
		return []*Gift{}
	}
	result := make([]*Gift, 0, len(in.GetGifts()))
	for _, gift := range in.GetGifts() {
		if converted := giftFromMTProto(gift, in.GetUsers(), in.GetChats()); converted != nil {
			result = append(result, converted)
		}
	}
	return result
}

// OwnedGiftsFromSavedStarGifts converts payments.savedStarGifts into the
// Bot API OwnedGifts shape.
func OwnedGiftsFromSavedStarGifts(in *mtproto.Payments_SavedStarGifts) *OwnedGifts {
	if in == nil {
		return &OwnedGifts{Gifts: []*GiftInfo{}}
	}
	result := &OwnedGifts{
		TotalCount: int(in.GetCount()),
		Gifts:      make([]*GiftInfo, 0, len(in.GetGifts())),
		NextOffset: stringValue(in.GetNextOffset()),
	}
	for _, gift := range in.GetGifts() {
		if converted := ownedGiftInfoFromSaved(gift, in.GetUsers(), in.GetChats()); converted != nil {
			result.Gifts = append(result.Gifts, converted)
		}
	}
	return result
}

// OwnedGiftsFromUserStarGifts converts payments.userStarGifts into the Bot API
// OwnedGifts shape.
func OwnedGiftsFromUserStarGifts(in *mtproto.Payments_UserStarGifts) *OwnedGifts {
	if in == nil {
		return &OwnedGifts{Gifts: []*GiftInfo{}}
	}
	result := &OwnedGifts{
		TotalCount: int(in.GetCount()),
		Gifts:      make([]*GiftInfo, 0, len(in.GetGifts())),
		NextOffset: stringValue(in.GetNextOffset()),
	}
	for _, gift := range in.GetGifts() {
		if converted := ownedGiftInfoFromUser(gift, in.GetUsers()); converted != nil {
			result.Gifts = append(result.Gifts, converted)
		}
	}
	return result
}

// ChatBoostsFromMTProto converts the boost list returned by premium.getUserBoosts.
func ChatBoostsFromMTProto(in *mtproto.Premium_BoostsList) *UserChatBoosts {
	if in == nil {
		return &UserChatBoosts{Boosts: []*ChatBoost{}}
	}
	result := &UserChatBoosts{Boosts: make([]*ChatBoost, 0, len(in.GetBoosts()))}
	for _, boost := range in.GetBoosts() {
		if converted := chatBoostFromMTProto(boost, in.GetUsers()); converted != nil {
			result.Boosts = append(result.Boosts, converted)
		}
	}
	return result
}

func ownedGiftInfoFromSaved(in *mtproto.SavedStarGift, users []*mtproto.User, chats []*mtproto.Chat) *GiftInfo {
	if in == nil {
		return nil
	}
	result := &GiftInfo{
		Gift:                    giftFromMTProto(in.GetGift(), users, chats),
		OwnedGiftID:             strconv.FormatInt(wrapperInt64(in.GetSavedId()), 10),
		ConvertStarCount:        wrapperInt64(in.GetConvertStars()),
		PrepaidUpgradeStarCount: wrapperInt64(in.GetUpgradeStars()),
		IsUpgradeSeparate:       in.GetUpgradeSeparate(),
		CanBeUpgraded:           in.GetCanUpgrade(),
		IsPrivate:               in.GetNameHidden(),
		UniqueGiftNumber:        int(wrapperInt32(in.GetGiftNum())),
	}
	if message := in.GetMessage(); message != nil {
		result.Text = message.GetText()
		result.Entities = messageEntities(message.GetEntities(), users)
	}
	return result
}

func ownedGiftInfoFromUser(in *mtproto.UserStarGift, users []*mtproto.User) *GiftInfo {
	if in == nil {
		return nil
	}
	result := &GiftInfo{
		Gift:                    giftFromMTProto(in.GetGift(), users, nil),
		ConvertStarCount:        wrapperInt64(in.GetConvertStars()),
		PrepaidUpgradeStarCount: wrapperInt64(in.GetUpgradeStars()),
		CanBeUpgraded:           in.GetCanUpgrade(),
		IsPrivate:               in.GetNameHidden(),
	}
	if message := in.GetMessage(); message != nil {
		result.Text = message.GetText()
		result.Entities = messageEntities(message.GetEntities(), users)
	}
	return result
}
