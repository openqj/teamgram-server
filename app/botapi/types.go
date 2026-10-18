// Copyright 2026 Teamgram Authors
// All rights reserved.

package botapi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/mtprotoext"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Response is the envelope used by the Telegram Bot API.
type Response struct {
	OK          bool                `json:"ok"`
	Result      any                 `json:"result,omitempty"`
	ErrorCode   int                 `json:"error_code,omitempty"`
	Description string              `json:"description,omitempty"`
	Parameters  *ResponseParameters `json:"parameters,omitempty"`
}

// ResponseParameters carries the retry or chat migration hint returned with
// a failed Bot API request.
type ResponseParameters struct {
	MigrateToChatID int64 `json:"migrate_to_chat_id,omitempty"`
	RetryAfter      int   `json:"retry_after,omitempty"`
}

type User struct {
	ID                         int64  `json:"id"`
	IsBot                      bool   `json:"is_bot"`
	FirstName                  string `json:"first_name"`
	LastName                   string `json:"last_name,omitempty"`
	Username                   string `json:"username,omitempty"`
	LanguageCode               string `json:"language_code,omitempty"`
	IsPremium                  bool   `json:"is_premium,omitempty"`
	AddedToAttachmentMenu      bool   `json:"added_to_attachment_menu,omitempty"`
	CanJoinGroups              bool   `json:"can_join_groups,omitempty"`
	CanReadAllGroupMessages    bool   `json:"can_read_all_group_messages,omitempty"`
	SupportsGuestQueries       bool   `json:"supports_guest_queries,omitempty"`
	SupportsInlineQueries      bool   `json:"supports_inline_queries,omitempty"`
	CanConnectToBusiness       bool   `json:"can_connect_to_business,omitempty"`
	HasMainWebApp              bool   `json:"has_main_web_app,omitempty"`
	HasTopicsEnabled           bool   `json:"has_topics_enabled,omitempty"`
	AllowsUsersToCreateTopics  bool   `json:"allows_users_to_create_topics,omitempty"`
	CanManageBots              bool   `json:"can_manage_bots,omitempty"`
	SupportsJoinRequestQueries bool   `json:"supports_join_request_queries,omitempty"`
}

type Chat struct {
	ID                        int64      `json:"id"`
	Type                      string     `json:"type"`
	Title                     string     `json:"title,omitempty"`
	Username                  string     `json:"username,omitempty"`
	FirstName                 string     `json:"first_name,omitempty"`
	LastName                  string     `json:"last_name,omitempty"`
	IsForum                   bool       `json:"is_forum,omitempty"`
	IsDirectMessages          bool       `json:"is_direct_messages,omitempty"`
	Photo                     any        `json:"photo,omitempty"`
	ActiveUsernames           []string   `json:"active_usernames,omitempty"`
	HasPrivateForwards        bool       `json:"has_private_forwards,omitempty"`
	HasRestrictedVoice        bool       `json:"has_restricted_voice_and_video_messages,omitempty"`
	JoinToSendMessages        bool       `json:"join_to_send_messages,omitempty"`
	JoinByRequest             bool       `json:"join_by_request,omitempty"`
	Description               string     `json:"description,omitempty"`
	InviteLink                string     `json:"invite_link,omitempty"`
	PinnedMessage             *Message   `json:"pinned_message,omitempty"`
	Permissions               any        `json:"permissions,omitempty"`
	SlowModeDelay             int        `json:"slow_mode_delay,omitempty"`
	MessageAutoDeleteTime     int        `json:"message_auto_delete_timer_changed,omitempty"`
	HasHiddenMembers          bool       `json:"has_hidden_members,omitempty"`
	HasAggressiveAntiSpam     bool       `json:"has_aggressive_anti_spam_enabled,omitempty"`
	EmojiStatusCustomID       string     `json:"emoji_status_custom_emoji_id,omitempty"`
	AccentColorID             int        `json:"accent_color_id,omitempty"`
	BackgroundCustomEmoji     string     `json:"background_custom_emoji_id,omitempty"`
	MaxReactionCount          int        `json:"max_reaction_count,omitempty"`
	AcceptedGiftTypes         any        `json:"accepted_gift_types,omitempty"`
	CanSendPaidMedia          bool       `json:"can_send_paid_media,omitempty"`
	UnrestrictBoostCount      int        `json:"unrestrict_boost_count,omitempty"`
	HasProtectedContent       bool       `json:"has_protected_content,omitempty"`
	HasVisibleHistory         bool       `json:"has_visible_history,omitempty"`
	StickerSetName            string     `json:"sticker_set_name,omitempty"`
	CanSetStickerSet          bool       `json:"can_set_sticker_set,omitempty"`
	CustomEmojiStickerSetName string     `json:"custom_emoji_sticker_set_name,omitempty"`
	LinkedChatID              int64      `json:"linked_chat_id,omitempty"`
	PaidMessageStarCount      int64      `json:"paid_message_star_count,omitempty"`
	GuardBot                  *User      `json:"guard_bot,omitempty"`
	Community                 *Community `json:"community,omitempty"`
}

// Community represents a group of chats linked around a shared topic.
type Community struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Story is the Bot API result returned by story publishing methods.
type Story struct {
	Chat *Chat `json:"chat"`
	ID   int   `json:"id"`
}

// ChatPhoto is the Bot API representation of a chat avatar. MTProto exposes
// the small and large variants either as one Photo constructor or as a
// ChatPhoto reference, so both Bot API file ids intentionally point at the
// same canonical photo id when only the reference is available.
type ChatPhoto struct {
	SmallFileID       string `json:"small_file_id"`
	SmallFileUniqueID string `json:"small_file_unique_id"`
	BigFileID         string `json:"big_file_id"`
	BigFileUniqueID   string `json:"big_file_unique_id"`
}

type UserProfilePhotos struct {
	TotalCount int           `json:"total_count"`
	Photos     [][]PhotoSize `json:"photos"`
}

type WebAppInfo struct {
	URL string `json:"url"`
}

// MenuButton is the Bot API representation of a bot's chat menu button.
// The zero value is the default commands button.
type MenuButton struct {
	Type   string      `json:"type"`
	Text   string      `json:"text,omitempty"`
	WebApp *WebAppInfo `json:"web_app,omitempty"`
}

type BotName struct {
	Name string `json:"name"`
}

type BotDescription struct {
	Description string `json:"description"`
}

type BotShortDescription struct {
	ShortDescription string `json:"short_description"`
}

type File struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	FileSize     int64  `json:"file_size,omitempty"`
	FilePath     string `json:"file_path,omitempty"`
}

// MessageID is returned by copyMessage and identifies the newly created
// message in the destination chat.
type MessageID struct {
	MessageID int `json:"message_id"`
}

type PhotoSize struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	FileSize     int64  `json:"file_size,omitempty"`
}

type Document struct {
	FileID       string     `json:"file_id"`
	FileUniqueID string     `json:"file_unique_id"`
	Thumbnail    *PhotoSize `json:"thumbnail,omitempty"`
	FileName     string     `json:"file_name,omitempty"`
	MimeType     string     `json:"mime_type,omitempty"`
	FileSize     int64      `json:"file_size,omitempty"`
}

type Video struct {
	FileID       string     `json:"file_id"`
	FileUniqueID string     `json:"file_unique_id"`
	Width        int        `json:"width"`
	Height       int        `json:"height"`
	Duration     int        `json:"duration"`
	Thumbnail    *PhotoSize `json:"thumbnail,omitempty"`
	FileName     string     `json:"file_name,omitempty"`
	MimeType     string     `json:"mime_type,omitempty"`
	FileSize     int64      `json:"file_size,omitempty"`
}

type VideoNote struct {
	FileID       string     `json:"file_id"`
	FileUniqueID string     `json:"file_unique_id"`
	Length       int        `json:"length"`
	Duration     int        `json:"duration"`
	Thumbnail    *PhotoSize `json:"thumbnail,omitempty"`
	FileSize     int64      `json:"file_size,omitempty"`
}

type MaskPosition struct {
	Point  string  `json:"point"`
	XShift float64 `json:"x_shift"`
	YShift float64 `json:"y_shift"`
	Scale  float64 `json:"scale"`
}

type Sticker struct {
	FileID           string        `json:"file_id"`
	FileUniqueID     string        `json:"file_unique_id"`
	Type             string        `json:"type"`
	Width            int           `json:"width"`
	Height           int           `json:"height"`
	IsAnimated       bool          `json:"is_animated"`
	IsVideo          bool          `json:"is_video"`
	Thumbnail        *PhotoSize    `json:"thumbnail,omitempty"`
	Emoji            string        `json:"emoji,omitempty"`
	SetName          string        `json:"set_name,omitempty"`
	PremiumAnimation *File         `json:"premium_animation,omitempty"`
	MaskPosition     *MaskPosition `json:"mask_position,omitempty"`
	CustomEmojiID    string        `json:"custom_emoji_id,omitempty"`
	NeedsRepainting  bool          `json:"needs_repainting,omitempty"`
	FileSize         int64         `json:"file_size,omitempty"`
}

type Audio struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Duration     int    `json:"duration"`
	Performer    string `json:"performer,omitempty"`
	Title        string `json:"title,omitempty"`
	FileName     string `json:"file_name,omitempty"`
	MimeType     string `json:"mime_type,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
}

type Voice struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Duration     int    `json:"duration"`
	MimeType     string `json:"mime_type,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
}

type Animation struct {
	FileID       string     `json:"file_id"`
	FileUniqueID string     `json:"file_unique_id"`
	Width        int        `json:"width"`
	Height       int        `json:"height"`
	Duration     int        `json:"duration"`
	Thumbnail    *PhotoSize `json:"thumbnail,omitempty"`
	FileName     string     `json:"file_name,omitempty"`
	MimeType     string     `json:"mime_type,omitempty"`
	FileSize     int64      `json:"file_size,omitempty"`
}

type Location struct {
	Longitude            float64 `json:"longitude"`
	Latitude             float64 `json:"latitude"`
	HorizontalAccuracy   float64 `json:"horizontal_accuracy,omitempty"`
	LivePeriod           int     `json:"live_period,omitempty"`
	Heading              int     `json:"heading,omitempty"`
	ProximityAlertRadius int     `json:"proximity_alert_radius,omitempty"`
}

type Venue struct {
	Location        *Location `json:"location"`
	Title           string    `json:"title"`
	Address         string    `json:"address"`
	FoursquareID    string    `json:"foursquare_id,omitempty"`
	FoursquareType  string    `json:"foursquare_type,omitempty"`
	GooglePlaceID   string    `json:"google_place_id,omitempty"`
	GooglePlaceType string    `json:"google_place_type,omitempty"`
}

type Contact struct {
	PhoneNumber string `json:"phone_number"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name,omitempty"`
	Vcard       string `json:"vcard,omitempty"`
	UserID      int64  `json:"user_id,omitempty"`
}

type ShippingAddress struct {
	CountryCode string `json:"country_code"`
	State       string `json:"state"`
	City        string `json:"city"`
	StreetLine1 string `json:"street_line1"`
	StreetLine2 string `json:"street_line2"`
	PostCode    string `json:"post_code"`
}

type ShippingQuery struct {
	ID              string           `json:"id"`
	From            *User            `json:"from"`
	InvoicePayload  string           `json:"invoice_payload"`
	ShippingAddress *ShippingAddress `json:"shipping_address"`
}

type LabeledPrice struct {
	Label  string `json:"label"`
	Amount int64  `json:"amount"`
}

type ShippingOption struct {
	ID     string         `json:"id"`
	Title  string         `json:"title"`
	Prices []LabeledPrice `json:"prices"`
}

type OrderInfo struct {
	Name            string           `json:"name,omitempty"`
	PhoneNumber     string           `json:"phone_number,omitempty"`
	Email           string           `json:"email,omitempty"`
	ShippingAddress *ShippingAddress `json:"shipping_address,omitempty"`
}

type PreCheckoutQuery struct {
	ID               string     `json:"id"`
	From             *User      `json:"from"`
	Currency         string     `json:"currency"`
	TotalAmount      int64      `json:"total_amount"`
	InvoicePayload   string     `json:"invoice_payload"`
	ShippingOptionID string     `json:"shipping_option_id,omitempty"`
	OrderInfo        *OrderInfo `json:"order_info,omitempty"`
}

type PollOption struct {
	PersistentID string          `json:"persistent_id,omitempty"`
	Text         string          `json:"text"`
	TextEntities []MessageEntity `json:"text_entities,omitempty"`
	Media        *PollMedia      `json:"media,omitempty"`
	VoterCount   int             `json:"voter_count"`
	AddedByUser  *User           `json:"added_by_user,omitempty"`
	AddedByChat  *Chat           `json:"added_by_chat,omitempty"`
	AdditionDate int64           `json:"addition_date,omitempty"`
}

// PollMedia is the media union allowed in poll options and explanations.
type PollMedia struct {
	Animation *Animation  `json:"animation,omitempty"`
	Audio     *Audio      `json:"audio,omitempty"`
	Document  *Document   `json:"document,omitempty"`
	Link      *Link       `json:"link,omitempty"`
	LivePhoto *LivePhoto  `json:"live_photo,omitempty"`
	Location  *Location   `json:"location,omitempty"`
	Photo     []PhotoSize `json:"photo,omitempty"`
	Sticker   *Sticker    `json:"sticker,omitempty"`
	Venue     *Venue      `json:"venue,omitempty"`
	Video     *Video      `json:"video,omitempty"`
}

type Link struct {
	URL string `json:"url"`
}

type Poll struct {
	ID                    string          `json:"id"`
	Question              string          `json:"question"`
	QuestionEntities      []MessageEntity `json:"question_entities,omitempty"`
	Options               []PollOption    `json:"options"`
	TotalVoterCount       int             `json:"total_voter_count"`
	IsClosed              bool            `json:"is_closed"`
	IsAnonymous           bool            `json:"is_anonymous"`
	Type                  string          `json:"type"`
	AllowsMultipleAnswers bool            `json:"allows_multiple_answers"`
	AllowsRevoting        bool            `json:"allows_revoting,omitempty"`
	MembersOnly           bool            `json:"members_only,omitempty"`
	CountryCodes          []string        `json:"country_codes,omitempty"`
	CorrectOptionID       int             `json:"correct_option_id,omitempty"`
	CorrectOptionIDs      []int           `json:"correct_option_ids,omitempty"`
	Explanation           string          `json:"explanation,omitempty"`
	ExplanationEntities   []MessageEntity `json:"explanation_entities,omitempty"`
	ExplanationMedia      *PollMedia      `json:"explanation_media,omitempty"`
	Description           string          `json:"description,omitempty"`
	DescriptionEntities   []MessageEntity `json:"description_entities,omitempty"`
	Media                 *PollMedia      `json:"media,omitempty"`
	OpenPeriod            int             `json:"open_period,omitempty"`
	CloseDate             int64           `json:"close_date,omitempty"`
}

type Dice struct {
	Emoji string `json:"emoji"`
	Value int    `json:"value"`
}

type Game struct {
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	Photo        []PhotoSize     `json:"photo"`
	Text         string          `json:"text,omitempty"`
	TextEntities []MessageEntity `json:"text_entities,omitempty"`
	Animation    *Animation      `json:"animation,omitempty"`
}

type GameHighScore struct {
	Position int   `json:"position"`
	User     *User `json:"user"`
	Score    int   `json:"score"`
}

type Invoice struct {
	Title          string `json:"title"`
	Description    string `json:"description"`
	StartParameter string `json:"start_parameter"`
	Currency       string `json:"currency"`
	TotalAmount    int64  `json:"total_amount"`
}

// EphemeralMessageParameters controls delivery of a message that is visible
// only to a single recipient. It mirrors the Bot API request object; the
// corresponding incoming message is represented by Message.EphemeralMessageID.
type EphemeralMessageParameters struct {
	ReceiverUserID              int64  `json:"receiver_user_id"`
	CallbackQueryID             string `json:"callback_query_id,omitempty"`
	ReplaceCallbackQueryMessage bool   `json:"replace_callback_query_message,omitempty"`
}

// DirectMessagesTopic identifies the topic that contains a direct message.
// Layer 229 does not currently expose this object on ordinary Message values,
// but keeping the Bot API shape here lets the adapter add it without changing
// the public contract when the underlying update gains the required fields.
type DirectMessagesTopic struct {
	TopicID int64 `json:"topic_id"`
	User    *User `json:"user,omitempty"`
}

// ManagedBotUpdated describes a managed bot lifecycle update.
type ManagedBotUpdated struct {
	User *User `json:"user"`
	Bot  *User `json:"bot"`
}

// BotSubscriptionUpdated describes a payment subscription state transition.
type BotSubscriptionUpdated struct {
	User           *User  `json:"user"`
	InvoicePayload string `json:"invoice_payload"`
	State          string `json:"state"`
}

// MessageGenerationStopped is the Bot API shape for a stopped draft. The
// current Layer 229 updateBotStopped constructor has no draft id, so the
// converter intentionally does not emit this object until that field exists.
type MessageGenerationStopped struct {
	Chat            *Chat `json:"chat"`
	MessageThreadID int   `json:"message_thread_id,omitempty"`
	DraftID         int64 `json:"draft_id"`
}

// ManagedBotCreated is emitted on the service message confirming a managed
// bot was created by the current bot.
type ManagedBotCreated struct {
	Bot *User `json:"bot"`
}

type CommunityChatAdded struct {
	Community *Community `json:"community"`
}

type CommunityChatJoined struct {
	Community *Community `json:"community"`
}

type CommunityChatRemoved struct{}

type SuggestedPostPrice struct {
	Currency string `json:"currency"`
	Amount   int64  `json:"amount"`
}

type SuggestedPostInfo struct {
	State    string              `json:"state"`
	Price    *SuggestedPostPrice `json:"price,omitempty"`
	SendDate int64               `json:"send_date,omitempty"`
}

type SuggestedPostApproved struct {
	SuggestedPostMessage *Message            `json:"suggested_post_message,omitempty"`
	Price                *SuggestedPostPrice `json:"price,omitempty"`
	SendDate             int64               `json:"send_date"`
}

type SuggestedPostApprovalFailed struct {
	SuggestedPostMessage *Message            `json:"suggested_post_message,omitempty"`
	Price                *SuggestedPostPrice `json:"price,omitempty"`
}

type SuggestedPostDeclined struct {
	SuggestedPostMessage *Message `json:"suggested_post_message,omitempty"`
	Comment              string   `json:"comment,omitempty"`
}

type SuggestedPostPaid struct {
	SuggestedPostMessage *Message    `json:"suggested_post_message,omitempty"`
	Currency             string      `json:"currency"`
	Amount               int64       `json:"amount,omitempty"`
	StarAmount           *StarAmount `json:"star_amount,omitempty"`
}

type SuggestedPostRefunded struct {
	SuggestedPostMessage *Message `json:"suggested_post_message,omitempty"`
	Reason               string   `json:"reason"`
}

type ChecklistTask struct {
	ID              int             `json:"id"`
	Text            string          `json:"text"`
	TextEntities    []MessageEntity `json:"text_entities,omitempty"`
	CompletedByUser *User           `json:"completed_by_user,omitempty"`
	CompletedByChat *Chat           `json:"completed_by_chat,omitempty"`
	CompletionDate  int64           `json:"completion_date,omitempty"`
}

type Checklist struct {
	Title                  string          `json:"title"`
	TitleEntities          []MessageEntity `json:"title_entities,omitempty"`
	Tasks                  []ChecklistTask `json:"tasks"`
	OthersCanAddTasks      bool            `json:"others_can_add_tasks,omitempty"`
	OthersCanMarkTasksDone bool            `json:"others_can_mark_tasks_as_done,omitempty"`
}

type ChecklistTasksDone struct {
	ChecklistMessage       *Message `json:"checklist_message,omitempty"`
	MarkedAsDoneTaskIDs    []int    `json:"marked_as_done_task_ids,omitempty"`
	MarkedAsNotDoneTaskIDs []int    `json:"marked_as_not_done_task_ids,omitempty"`
}

type ChecklistTasksAdded struct {
	ChecklistMessage *Message        `json:"checklist_message,omitempty"`
	Tasks            []ChecklistTask `json:"tasks"`
}

type GiftBackground struct {
	CenterColor int `json:"center_color"`
	EdgeColor   int `json:"edge_color"`
	TextColor   int `json:"text_color"`
}

type Gift struct {
	ID                     string          `json:"id"`
	Sticker                *Sticker        `json:"sticker"`
	StarCount              int64           `json:"star_count"`
	UpgradeStarCount       int64           `json:"upgrade_star_count,omitempty"`
	IsPremium              bool            `json:"is_premium,omitempty"`
	HasColors              bool            `json:"has_colors,omitempty"`
	TotalCount             int             `json:"total_count,omitempty"`
	RemainingCount         int             `json:"remaining_count,omitempty"`
	PersonalTotalCount     int             `json:"personal_total_count,omitempty"`
	PersonalRemainingCount int             `json:"personal_remaining_count,omitempty"`
	Background             *GiftBackground `json:"background,omitempty"`
	UniqueGiftVariantCount int             `json:"unique_gift_variant_count,omitempty"`
	PublisherChat          *Chat           `json:"publisher_chat,omitempty"`
}

// OwnedGifts is the Bot API result used by the user and business gift
// inventory methods. The MTProto gift unions are flattened into GiftInfo so
// callers do not need to know the underlying SavedStarGift/UserStarGift
// constructors.
type OwnedGifts struct {
	TotalCount int         `json:"total_count"`
	Gifts      []*GiftInfo `json:"gifts"`
	NextOffset string      `json:"next_offset,omitempty"`
}

// UserChatBoosts is returned by getUserChatBoosts.
type UserChatBoosts struct {
	Boosts []*ChatBoost `json:"boosts"`
}

// BotAccessSettings is the public representation of bots.accessSettings.
type BotAccessSettings struct {
	IsAccessRestricted bool    `json:"is_access_restricted"`
	AddedUserIDs       []int64 `json:"added_user_ids"`
}

type UniqueGiftModel struct {
	Name           string   `json:"name"`
	Sticker        *Sticker `json:"sticker"`
	RarityPerMille int      `json:"rarity_per_mille"`
	Rarity         string   `json:"rarity,omitempty"`
}

type UniqueGiftSymbol struct {
	Name           string   `json:"name"`
	Sticker        *Sticker `json:"sticker"`
	RarityPerMille int      `json:"rarity_per_mille"`
}

type UniqueGiftBackdropColors struct {
	CenterColor int `json:"center_color"`
	EdgeColor   int `json:"edge_color"`
	SymbolColor int `json:"symbol_color"`
	TextColor   int `json:"text_color"`
}

type UniqueGiftBackdrop struct {
	Name           string                    `json:"name"`
	Colors         *UniqueGiftBackdropColors `json:"colors"`
	RarityPerMille int                       `json:"rarity_per_mille"`
}

type UniqueGiftColors struct {
	ModelCustomEmojiID    string `json:"model_custom_emoji_id"`
	SymbolCustomEmojiID   string `json:"symbol_custom_emoji_id"`
	LightThemeMainColor   int    `json:"light_theme_main_color"`
	LightThemeOtherColors []int  `json:"light_theme_other_colors"`
	DarkThemeMainColor    int    `json:"dark_theme_main_color"`
	DarkThemeOtherColors  []int  `json:"dark_theme_other_colors"`
}

type UniqueGift struct {
	GiftID           string              `json:"gift_id"`
	BaseName         string              `json:"base_name"`
	Name             string              `json:"name"`
	Number           int                 `json:"number"`
	Model            *UniqueGiftModel    `json:"model"`
	Symbol           *UniqueGiftSymbol   `json:"symbol"`
	Backdrop         *UniqueGiftBackdrop `json:"backdrop"`
	IsPremium        bool                `json:"is_premium,omitempty"`
	IsBurned         bool                `json:"is_burned,omitempty"`
	IsFromBlockchain bool                `json:"is_from_blockchain,omitempty"`
	Colors           *UniqueGiftColors   `json:"colors,omitempty"`
	PublisherChat    *Chat               `json:"publisher_chat,omitempty"`
}

type GiftInfo struct {
	Gift                    *Gift           `json:"gift"`
	OwnedGiftID             string          `json:"owned_gift_id,omitempty"`
	ConvertStarCount        int64           `json:"convert_star_count,omitempty"`
	PrepaidUpgradeStarCount int64           `json:"prepaid_upgrade_star_count,omitempty"`
	IsUpgradeSeparate       bool            `json:"is_upgrade_separate,omitempty"`
	CanBeUpgraded           bool            `json:"can_be_upgraded,omitempty"`
	Text                    string          `json:"text,omitempty"`
	Entities                []MessageEntity `json:"entities,omitempty"`
	IsPrivate               bool            `json:"is_private,omitempty"`
	UniqueGiftNumber        int             `json:"unique_gift_number,omitempty"`
}

type UniqueGiftInfo struct {
	Gift               *UniqueGift     `json:"gift"`
	Origin             string          `json:"origin"`
	Text               string          `json:"text,omitempty"`
	Entities           []MessageEntity `json:"entities,omitempty"`
	IsPrivate          bool            `json:"is_private,omitempty"`
	LastResaleCurrency string          `json:"last_resale_currency,omitempty"`
	LastResaleAmount   int64           `json:"last_resale_amount,omitempty"`
	OwnedGiftID        string          `json:"owned_gift_id,omitempty"`
	TransferStarCount  int64           `json:"transfer_star_count,omitempty"`
	NextTransferDate   int64           `json:"next_transfer_date,omitempty"`
}

type PaidMediaInfo struct {
	StarCount int   `json:"star_count"`
	PaidMedia []any `json:"paid_media"`
}

type PaidMediaPreview struct {
	Type     string `json:"type"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	Duration int    `json:"duration,omitempty"`
}

type PaidMediaPhoto struct {
	Type  string      `json:"type"`
	Photo []PhotoSize `json:"photo"`
}

type PaidMediaVideo struct {
	Type  string `json:"type"`
	Video *Video `json:"video"`
}

type LivePhoto struct {
	Photo        []PhotoSize `json:"photo,omitempty"`
	FileID       string      `json:"file_id"`
	FileUniqueID string      `json:"file_unique_id"`
	Width        int         `json:"width"`
	Height       int         `json:"height"`
	Duration     int         `json:"duration"`
	MimeType     string      `json:"mime_type,omitempty"`
	FileSize     int64       `json:"file_size,omitempty"`
}

type PaidMediaLivePhoto struct {
	Type      string     `json:"type"`
	LivePhoto *LivePhoto `json:"live_photo"`
}

// SuccessfulPayment is emitted on the service message delivered after a
// provider confirms an invoice payment.
type SuccessfulPayment struct {
	Currency                   string     `json:"currency"`
	TotalAmount                int64      `json:"total_amount"`
	InvoicePayload             string     `json:"invoice_payload"`
	ShippingOptionID           string     `json:"shipping_option_id,omitempty"`
	OrderInfo                  *OrderInfo `json:"order_info,omitempty"`
	TelegramPaymentChargeID    string     `json:"telegram_payment_charge_id"`
	ProviderPaymentChargeID    string     `json:"provider_payment_charge_id"`
	SubscriptionExpirationDate int64      `json:"subscription_expiration_date,omitempty"`
	IsRecurring                bool       `json:"is_recurring,omitempty"`
	IsFirstRecurring           bool       `json:"is_first_recurring,omitempty"`
}

// StarAmount is the Bot API representation of a Telegram Stars balance or
// transaction amount. Nanostars are kept separately because the Bot API uses
// the field only when a fractional Star value is present.
type StarAmount struct {
	Amount         int64 `json:"amount"`
	NanostarAmount int   `json:"nanostar_amount,omitempty"`
}

// TransactionPartner is the common Bot API envelope for a Stars transaction
// counterparty. The Layer 229 ledger may omit a counterparty, so both fields
// remain optional when the server does not have that entity in the result.
type TransactionPartner struct {
	Type string `json:"type"`
	User *User  `json:"user,omitempty"`
	Chat *Chat  `json:"chat,omitempty"`
}

type StarTransaction struct {
	ID             string              `json:"id"`
	Amount         int64               `json:"amount"`
	NanostarAmount int                 `json:"nanostar_amount,omitempty"`
	Date           int64               `json:"date"`
	Source         *TransactionPartner `json:"source,omitempty"`
	Receiver       *TransactionPartner `json:"receiver,omitempty"`
}

type StarTransactions struct {
	Transactions []StarTransaction `json:"transactions"`
	NextOffset   string            `json:"next_offset,omitempty"`
}

type BusinessConnection struct {
	ID         string             `json:"id"`
	User       *User              `json:"user"`
	UserChatID int64              `json:"user_chat_id"`
	Date       int64              `json:"date"`
	CanReply   bool               `json:"can_reply"`
	IsEnabled  bool               `json:"is_enabled"`
	Rights     *BusinessBotRights `json:"rights,omitempty"`
}

type BusinessBotRights struct {
	CanReply                   bool `json:"can_reply,omitempty"`
	CanReadMessages            bool `json:"can_read_messages,omitempty"`
	CanDeleteSentMessages      bool `json:"can_delete_sent_messages,omitempty"`
	CanDeleteAllMessages       bool `json:"can_delete_all_messages,omitempty"`
	CanEditName                bool `json:"can_edit_name,omitempty"`
	CanEditBio                 bool `json:"can_edit_bio,omitempty"`
	CanEditProfilePhoto        bool `json:"can_edit_profile_photo,omitempty"`
	CanEditUsername            bool `json:"can_edit_username,omitempty"`
	CanChangeGiftSettings      bool `json:"can_change_gift_settings,omitempty"`
	CanViewGifts               bool `json:"can_view_gifts,omitempty"`
	CanSellGifts               bool `json:"can_sell_gifts,omitempty"`
	CanTransferAndUpgradeGifts bool `json:"can_transfer_and_upgrade_gifts,omitempty"`
	CanTransferStars           bool `json:"can_transfer_stars,omitempty"`
	CanManageStories           bool `json:"can_manage_stories,omitempty"`
}

// ReactionType is the Bot API input shape for a message reaction. Telegram
// currently accepts emoji and custom emoji reactions from bots.
type ReactionType struct {
	Type          string `json:"type"`
	Emoji         string `json:"emoji,omitempty"`
	CustomEmojiID string `json:"custom_emoji_id,omitempty"`
}

type MessageEntity struct {
	Type          string `json:"type"`
	Offset        int    `json:"offset"`
	Length        int    `json:"length"`
	URL           string `json:"url,omitempty"`
	User          *User  `json:"user,omitempty"`
	Language      string `json:"language,omitempty"`
	CustomEmojiID string `json:"custom_emoji_id,omitempty"`
}

type InlineKeyboardButton struct {
	Text                         string      `json:"text"`
	URL                          string      `json:"url,omitempty"`
	CallbackData                 string      `json:"callback_data,omitempty"`
	WebApp                       *WebAppInfo `json:"web_app,omitempty"`
	SwitchInlineQuery            string      `json:"switch_inline_query,omitempty"`
	SwitchInlineQueryCurrentChat string      `json:"switch_inline_query_current_chat,omitempty"`
	Pay                          bool        `json:"pay,omitempty"`
	Disabled                     bool        `json:"disabled,omitempty"`
}

type ReplyMarkupInput struct {
	InlineKeyboard        [][]InlineKeyboardButton `json:"inline_keyboard,omitempty"`
	Keyboard              [][]map[string]any       `json:"keyboard,omitempty"`
	ResizeKeyboard        bool                     `json:"resize_keyboard,omitempty"`
	OneTimeKeyboard       bool                     `json:"one_time_keyboard,omitempty"`
	Selective             bool                     `json:"selective,omitempty"`
	IsPersistent          bool                     `json:"is_persistent,omitempty"`
	RemoveKeyboard        bool                     `json:"remove_keyboard,omitempty"`
	ForceReply            bool                     `json:"force_reply,omitempty"`
	InputFieldPlaceholder string                   `json:"input_field_placeholder,omitempty"`
}

type Message struct {
	MessageID                   int                          `json:"message_id"`
	MessageThreadID             int                          `json:"message_thread_id,omitempty"`
	DirectMessagesTopic         *DirectMessagesTopic         `json:"direct_messages_topic,omitempty"`
	From                        *User                        `json:"from,omitempty"`
	SenderChat                  *Chat                        `json:"sender_chat,omitempty"`
	SenderBoostCount            int                          `json:"sender_boost_count,omitempty"`
	SenderBusinessBot           *User                        `json:"sender_business_bot,omitempty"`
	SenderTag                   string                       `json:"sender_tag,omitempty"`
	ReceiverUser                *User                        `json:"receiver_user,omitempty"`
	Date                        int64                        `json:"date"`
	Chat                        *Chat                        `json:"chat"`
	ForwardFrom                 *User                        `json:"forward_from,omitempty"`
	ForwardFromChat             *Chat                        `json:"forward_from_chat,omitempty"`
	ForwardFromMessageID        int                          `json:"forward_from_message_id,omitempty"`
	ForwardOrigin               any                          `json:"forward_origin,omitempty"`
	IsTopicMessage              bool                         `json:"is_topic_message,omitempty"`
	IsFromOffline               bool                         `json:"is_from_offline,omitempty"`
	EphemeralMessageID          int                          `json:"ephemeral_message_id,omitempty"`
	GuestQueryID                string                       `json:"guest_query_id,omitempty"`
	GuestBotCallerUser          *User                        `json:"guest_bot_caller_user,omitempty"`
	GuestBotCallerChat          *Chat                        `json:"guest_bot_caller_chat,omitempty"`
	ReplyToMessage              *Message                     `json:"reply_to_message,omitempty"`
	Text                        string                       `json:"text,omitempty"`
	Entities                    []MessageEntity              `json:"entities,omitempty"`
	Caption                     string                       `json:"caption,omitempty"`
	CaptionEntities             []MessageEntity              `json:"caption_entities,omitempty"`
	ShowCaptionAboveMedia       bool                         `json:"show_caption_above_media,omitempty"`
	ViaBot                      *User                        `json:"via_bot,omitempty"`
	EditDate                    int64                        `json:"edit_date,omitempty"`
	EffectID                    string                       `json:"effect_id,omitempty"`
	HasProtectedContent         bool                         `json:"has_protected_content,omitempty"`
	IsPaidPost                  bool                         `json:"is_paid_post,omitempty"`
	MediaGroupID                string                       `json:"media_group_id,omitempty"`
	AuthorSignature             string                       `json:"author_signature,omitempty"`
	PaidStarCount               int64                        `json:"paid_star_count,omitempty"`
	IsAutomaticForward          bool                         `json:"is_automatic_forward,omitempty"`
	ConnectedWebsite            string                       `json:"connected_website,omitempty"`
	ReplyMarkup                 any                          `json:"reply_markup,omitempty"`
	Photo                       []PhotoSize                  `json:"photo,omitempty"`
	LivePhoto                   *LivePhoto                   `json:"live_photo,omitempty"`
	Audio                       *Audio                       `json:"audio,omitempty"`
	Document                    *Document                    `json:"document,omitempty"`
	Sticker                     *Sticker                     `json:"sticker,omitempty"`
	Video                       *Video                       `json:"video,omitempty"`
	VideoNote                   *VideoNote                   `json:"video_note,omitempty"`
	Animation                   *Animation                   `json:"animation,omitempty"`
	Voice                       *Voice                       `json:"voice,omitempty"`
	Location                    *Location                    `json:"location,omitempty"`
	Venue                       *Venue                       `json:"venue,omitempty"`
	Contact                     *Contact                     `json:"contact,omitempty"`
	Poll                        *Poll                        `json:"poll,omitempty"`
	Dice                        *Dice                        `json:"dice,omitempty"`
	Game                        *Game                        `json:"game,omitempty"`
	Invoice                     *Invoice                     `json:"invoice,omitempty"`
	SuccessfulPayment           *SuccessfulPayment           `json:"successful_payment,omitempty"`
	SuggestedPostInfo           *SuggestedPostInfo           `json:"suggested_post_info,omitempty"`
	SuggestedPostApproved       *SuggestedPostApproved       `json:"suggested_post_approved,omitempty"`
	SuggestedPostApprovalFailed *SuggestedPostApprovalFailed `json:"suggested_post_approval_failed,omitempty"`
	SuggestedPostDeclined       *SuggestedPostDeclined       `json:"suggested_post_declined,omitempty"`
	SuggestedPostPaid           *SuggestedPostPaid           `json:"suggested_post_paid,omitempty"`
	SuggestedPostRefunded       *SuggestedPostRefunded       `json:"suggested_post_refunded,omitempty"`
	ManagedBotCreated           *ManagedBotCreated           `json:"managed_bot_created,omitempty"`
	PaidMedia                   *PaidMediaInfo               `json:"paid_media,omitempty"`
	Checklist                   *Checklist                   `json:"checklist,omitempty"`
	Gift                        *GiftInfo                    `json:"gift,omitempty"`
	UniqueGift                  *UniqueGiftInfo              `json:"unique_gift,omitempty"`
	GiftUpgradeSent             *GiftInfo                    `json:"gift_upgrade_sent,omitempty"`
	ChecklistTasksDone          *ChecklistTasksDone          `json:"checklist_tasks_done,omitempty"`
	ChecklistTasksAdded         *ChecklistTasksAdded         `json:"checklist_tasks_added,omitempty"`
	CommunityChatAdded          *CommunityChatAdded          `json:"community_chat_added,omitempty"`
	CommunityChatJoined         *CommunityChatJoined         `json:"community_chat_joined,omitempty"`
	CommunityChatRemoved        *CommunityChatRemoved        `json:"community_chat_removed,omitempty"`
	BusinessConnectionID        string                       `json:"business_connection_id,omitempty"`
	RichMessage                 *mtproto.RichMessage         `json:"rich_message,omitempty"`
}

type MessageOrigin struct {
	Type            string `json:"type"`
	Date            int64  `json:"date"`
	SenderUser      *User  `json:"sender_user,omitempty"`
	SenderUserName  string `json:"sender_user_name,omitempty"`
	SenderChat      *Chat  `json:"sender_chat,omitempty"`
	AuthorSignature string `json:"author_signature,omitempty"`
	Chat            *Chat  `json:"chat,omitempty"`
	MessageID       int    `json:"message_id,omitempty"`
}

type InlineQuery struct {
	ID       string `json:"id"`
	From     *User  `json:"from"`
	ChatType string `json:"chat_type"`
	Query    string `json:"query"`
	Offset   string `json:"offset"`
}

// ChosenInlineResult is emitted after a user selects one of the bot's inline
// results. The MTProto update carries the selected result id and, when the
// result was sent as an inline message, its opaque message identifier.
type ChosenInlineResult struct {
	ResultID        string    `json:"result_id"`
	From            *User     `json:"from"`
	Location        *Location `json:"location,omitempty"`
	InlineMessageID string    `json:"inline_message_id,omitempty"`
	Query           string    `json:"query"`
}

// InlineQueryResultsButton is the optional button shown above inline results.
// Telegram accepts either a Mini App button or a deep-link parameter that
// switches the user to the bot's private chat.
type InlineQueryResultsButton struct {
	Text           string      `json:"text"`
	WebApp         *WebAppInfo `json:"web_app,omitempty"`
	StartParameter string      `json:"start_parameter,omitempty"`
}

type CallbackQuery struct {
	ID              string   `json:"id"`
	From            *User    `json:"from"`
	Message         *Message `json:"message,omitempty"`
	InlineMessageID string   `json:"inline_message_id,omitempty"`
	ChatInstance    string   `json:"chat_instance"`
	Data            string   `json:"data,omitempty"`
	GameShortName   string   `json:"game_short_name,omitempty"`
}

type PollAnswer struct {
	PollID              string   `json:"poll_id"`
	VoterChat           *Chat    `json:"voter_chat,omitempty"`
	User                *User    `json:"user,omitempty"`
	OptionIDs           []int    `json:"option_ids"`
	OptionPersistentIDs []string `json:"option_persistent_ids,omitempty"`
}

type ChatJoinRequest struct {
	Chat       *Chat           `json:"chat"`
	From       *User           `json:"from"`
	UserChatID int64           `json:"user_chat_id"`
	Date       int64           `json:"date"`
	Bio        string          `json:"bio,omitempty"`
	InviteLink *ChatInviteLink `json:"invite_link,omitempty"`
	QueryID    string          `json:"query_id,omitempty"`
}

type MessageReactionUpdated struct {
	Chat        *Chat          `json:"chat"`
	MessageID   int            `json:"message_id"`
	User        *User          `json:"user,omitempty"`
	ActorChat   *Chat          `json:"actor_chat,omitempty"`
	Date        int64          `json:"date"`
	OldReaction []ReactionType `json:"old_reaction"`
	NewReaction []ReactionType `json:"new_reaction"`
}

type ReactionCount struct {
	Type       ReactionType `json:"type"`
	TotalCount int          `json:"total_count"`
}

type MessageReactionCountUpdated struct {
	Chat      *Chat           `json:"chat"`
	MessageID int             `json:"message_id"`
	Date      int64           `json:"date"`
	Reactions []ReactionCount `json:"reactions"`
}

type ChatBoostSource struct {
	Source            string `json:"source"`
	User              *User  `json:"user,omitempty"`
	GiveawayMessageID int    `json:"giveaway_message_id,omitempty"`
}

type ChatBoost struct {
	BoostID        string           `json:"boost_id"`
	AddDate        int64            `json:"add_date"`
	ExpirationDate int64            `json:"expiration_date"`
	Source         *ChatBoostSource `json:"source"`
}

type ChatBoostUpdated struct {
	Chat  *Chat      `json:"chat"`
	Boost *ChatBoost `json:"boost"`
}

type ChatBoostRemoved struct {
	Chat       *Chat            `json:"chat"`
	BoostID    string           `json:"boost_id"`
	RemoveDate int64            `json:"remove_date"`
	Source     *ChatBoostSource `json:"source"`
}

type PaidMediaPurchased struct {
	From             *User  `json:"from"`
	PaidMediaPayload string `json:"paid_media_payload"`
}

// WebAppQuery is emitted when a Mini App sends data back to its bot. The
// MTProto update carries the opaque query id and JSON payload; installations
// that include identity fields in that payload may expose them separately at
// the application layer.
type WebAppQuery struct {
	ID           string `json:"id"`
	From         *User  `json:"from,omitempty"`
	ChatInstance string `json:"chat_instance"`
	Data         string `json:"data"`
}

// SentWebAppMessage is returned by answerWebAppQuery. Telegram exposes the
// opaque inline message id so callers can edit the message later.
type SentWebAppMessage struct {
	InlineMessageID string `json:"inline_message_id,omitempty"`
}

// SentGuestMessage is returned by answerGuestQuery.
type SentGuestMessage struct {
	InlineMessageID string `json:"inline_message_id"`
}

type Update struct {
	UpdateID                 int64                        `json:"update_id"`
	Message                  *Message                     `json:"message,omitempty"`
	EditedMessage            *Message                     `json:"edited_message,omitempty"`
	ChannelPost              *Message                     `json:"channel_post,omitempty"`
	EditedChannelPost        *Message                     `json:"edited_channel_post,omitempty"`
	InlineQuery              *InlineQuery                 `json:"inline_query,omitempty"`
	ChosenInlineResult       *ChosenInlineResult          `json:"chosen_inline_result,omitempty"`
	CallbackQuery            *CallbackQuery               `json:"callback_query,omitempty"`
	GuestMessage             *Message                     `json:"guest_message,omitempty"`
	Poll                     *Poll                        `json:"poll,omitempty"`
	PollAnswer               *PollAnswer                  `json:"poll_answer,omitempty"`
	MyChatMember             *ChatMemberUpdated           `json:"my_chat_member,omitempty"`
	ChatMember               *ChatMemberUpdated           `json:"chat_member,omitempty"`
	ChatJoinRequest          *ChatJoinRequest             `json:"chat_join_request,omitempty"`
	MessageReaction          *MessageReactionUpdated      `json:"message_reaction,omitempty"`
	MessageReactionCount     *MessageReactionCountUpdated `json:"message_reaction_count,omitempty"`
	ChatBoost                *ChatBoostUpdated            `json:"chat_boost,omitempty"`
	RemovedChatBoost         *ChatBoostRemoved            `json:"removed_chat_boost,omitempty"`
	ManagedBot               *ManagedBotUpdated           `json:"managed_bot,omitempty"`
	Subscription             *BotSubscriptionUpdated      `json:"subscription,omitempty"`
	StoppedMessageGeneration *MessageGenerationStopped    `json:"stopped_message_generation,omitempty"`
	PurchasedPaidMedia       *PaidMediaPurchased          `json:"purchased_paid_media,omitempty"`
	WebAppQuery              *WebAppQuery                 `json:"web_app_query,omitempty"`
	ShippingQuery            *ShippingQuery               `json:"shipping_query,omitempty"`
	PreCheckoutQuery         *PreCheckoutQuery            `json:"pre_checkout_query,omitempty"`
	BusinessConnection       *BusinessConnection          `json:"business_connection,omitempty"`
	BusinessMessage          *Message                     `json:"business_message,omitempty"`
	EditedBusinessMessage    *Message                     `json:"edited_business_message,omitempty"`
	DeletedBusinessMessages  *BusinessMessagesDeleted     `json:"deleted_business_messages,omitempty"`
}

type ChatMemberUpdated struct {
	Chat           *Chat           `json:"chat"`
	From           *User           `json:"from"`
	Date           int64           `json:"date"`
	OldChatMember  *ChatMember     `json:"old_chat_member"`
	NewChatMember  *ChatMember     `json:"new_chat_member"`
	InviteLink     *ChatInviteLink `json:"invite_link,omitempty"`
	ViaChatFolder  bool            `json:"via_chat_folder,omitempty"`
	ViaJoinRequest bool            `json:"via_join_request,omitempty"`
}

type BusinessMessagesDeleted struct {
	BusinessConnectionID string `json:"business_connection_id"`
	Chat                 *Chat  `json:"chat"`
	MessageIDs           []int  `json:"message_ids"`
}

type ChatPermissions struct {
	CanSendMessages       bool `json:"can_send_messages,omitempty"`
	CanSendAudios         bool `json:"can_send_audios,omitempty"`
	CanSendDocuments      bool `json:"can_send_documents,omitempty"`
	CanSendPhotos         bool `json:"can_send_photos,omitempty"`
	CanSendVideos         bool `json:"can_send_videos,omitempty"`
	CanSendVideoNotes     bool `json:"can_send_video_notes,omitempty"`
	CanSendVoiceNotes     bool `json:"can_send_voice_notes,omitempty"`
	CanSendPolls          bool `json:"can_send_polls,omitempty"`
	CanSendOtherMessages  bool `json:"can_send_other_messages,omitempty"`
	CanAddWebPagePreviews bool `json:"can_add_web_page_previews,omitempty"`
	CanChangeInfo         bool `json:"can_change_info,omitempty"`
	CanInviteUsers        bool `json:"can_invite_users,omitempty"`
	CanPinMessages        bool `json:"can_pin_messages,omitempty"`
	CanManageTopics       bool `json:"can_manage_topics,omitempty"`
	CanReactToMessages    bool `json:"can_react_to_messages,omitempty"`
}

type ChatAdministratorRights struct {
	IsAnonymous             bool `json:"is_anonymous"`
	CanManageChat           bool `json:"can_manage_chat"`
	CanDeleteMessages       bool `json:"can_delete_messages"`
	CanManageVideoChats     bool `json:"can_manage_video_chats"`
	CanRestrictMembers      bool `json:"can_restrict_members"`
	CanPromoteMembers       bool `json:"can_promote_members"`
	CanChangeInfo           bool `json:"can_change_info"`
	CanInviteUsers          bool `json:"can_invite_users"`
	CanPostMessages         bool `json:"can_post_messages"`
	CanEditMessages         bool `json:"can_edit_messages"`
	CanPinMessages          bool `json:"can_pin_messages"`
	CanManageTopics         bool `json:"can_manage_topics"`
	CanPostStories          bool `json:"can_post_stories"`
	CanEditStories          bool `json:"can_edit_stories"`
	CanDeleteStories        bool `json:"can_delete_stories"`
	CanManageDirectMessages bool `json:"can_manage_direct_messages"`
	CanManageTags           bool `json:"can_manage_tags"`
	CanSendWelcomeMessages  bool `json:"can_send_welcome_messages"`
}

// ChatInviteLink is the Bot API representation of an exported MTProto invite.
type ChatInviteLink struct {
	InviteLink              string `json:"invite_link"`
	Creator                 *User  `json:"creator"`
	CreatesJoinRequest      bool   `json:"creates_join_request"`
	IsPrimary               bool   `json:"is_primary"`
	IsRevoked               bool   `json:"is_revoked"`
	Name                    string `json:"name,omitempty"`
	ExpireDate              int64  `json:"expire_date,omitempty"`
	MemberLimit             int    `json:"member_limit,omitempty"`
	PendingJoinRequestCount int    `json:"pending_join_request_count,omitempty"`
	SubscriptionPeriod      int    `json:"subscription_period,omitempty"`
	SubscriptionPrice       int64  `json:"subscription_price,omitempty"`
}

// ForumTopic is the Bot API representation of a forum topic.
type ForumTopic struct {
	MessageThreadID   int    `json:"message_thread_id"`
	Name              string `json:"name"`
	IconColor         int    `json:"icon_color"`
	IconCustomEmojiID string `json:"icon_custom_emoji_id,omitempty"`
}

// ChatMember is the common JSON shape used by Telegram's ChatMember
// constructors. The status selects which optional fields are meaningful.
type ChatMember struct {
	Status                  string `json:"status"`
	User                    *User  `json:"user"`
	Tag                     string `json:"tag,omitempty"`
	IsAnonymous             bool   `json:"is_anonymous,omitempty"`
	CustomTitle             string `json:"custom_title,omitempty"`
	CanBeEdited             bool   `json:"can_be_edited,omitempty"`
	CanManageChat           bool   `json:"can_manage_chat,omitempty"`
	CanDeleteMessages       bool   `json:"can_delete_messages,omitempty"`
	CanManageVideoChats     bool   `json:"can_manage_video_chats,omitempty"`
	CanRestrictMembers      bool   `json:"can_restrict_members,omitempty"`
	CanPromoteMembers       bool   `json:"can_promote_members,omitempty"`
	CanChangeInfo           bool   `json:"can_change_info,omitempty"`
	CanInviteUsers          bool   `json:"can_invite_users,omitempty"`
	CanPostMessages         bool   `json:"can_post_messages,omitempty"`
	CanEditMessages         bool   `json:"can_edit_messages,omitempty"`
	CanPinMessages          bool   `json:"can_pin_messages,omitempty"`
	CanManageTopics         bool   `json:"can_manage_topics,omitempty"`
	CanManageDirectMessages bool   `json:"can_manage_direct_messages,omitempty"`
	CanManageTags           bool   `json:"can_manage_tags,omitempty"`
	CanSendWelcomeMessages  bool   `json:"can_send_welcome_messages,omitempty"`
	CanReactToMessages      bool   `json:"can_react_to_messages,omitempty"`
	CanEditTag              bool   `json:"can_edit_tag,omitempty"`
	CanPostStories          bool   `json:"can_post_stories,omitempty"`
	CanEditStories          bool   `json:"can_edit_stories,omitempty"`
	CanDeleteStories        bool   `json:"can_delete_stories,omitempty"`
	UntilDate               int64  `json:"until_date,omitempty"`
	IsMember                bool   `json:"is_member,omitempty"`
	CanSendMessages         bool   `json:"can_send_messages,omitempty"`
	CanSendAudios           bool   `json:"can_send_audios,omitempty"`
	CanSendDocuments        bool   `json:"can_send_documents,omitempty"`
	CanSendPhotos           bool   `json:"can_send_photos,omitempty"`
	CanSendVideos           bool   `json:"can_send_videos,omitempty"`
	CanSendVideoNotes       bool   `json:"can_send_video_notes,omitempty"`
	CanSendVoiceNotes       bool   `json:"can_send_voice_notes,omitempty"`
	CanSendPolls            bool   `json:"can_send_polls,omitempty"`
	CanSendOtherMessages    bool   `json:"can_send_other_messages,omitempty"`
	CanAddWebPagePreviews   bool   `json:"can_add_web_page_previews,omitempty"`
}

type WebhookInfo struct {
	URL                          string   `json:"url"`
	HasCustomCertificate         bool     `json:"has_custom_certificate"`
	PendingUpdateCount           int      `json:"pending_update_count"`
	IPAddress                    string   `json:"ip_address,omitempty"`
	LastErrorDate                int64    `json:"last_error_date,omitempty"`
	LastErrorMessage             string   `json:"last_error_message,omitempty"`
	LastSynchronizationErrorDate int64    `json:"last_synchronization_error_date,omitempty"`
	MaxConnections               int      `json:"max_connections,omitempty"`
	AllowedUpdates               []string `json:"allowed_updates,omitempty"`
}

type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
	IsEphemeral bool   `json:"is_ephemeral,omitempty"`
}

// MessageEntitiesToMTProto converts the Bot API's UTF-16 offsets directly to
// the equivalent Teamgram entity envelope.
func MessageEntitiesToMTProto(in []MessageEntity) []*mtproto.MessageEntity {
	result := make([]*mtproto.MessageEntity, 0, len(in))
	for _, entity := range in {
		entityType := strings.TrimSpace(entity.Type)
		if entityType == "" || entity.Offset < 0 || entity.Length <= 0 {
			continue
		}
		predicate := "messageEntity" + strings.ToUpper(entityType[:1]) + entityType[1:]
		switch entityType {
		case "text_link":
			predicate = mtproto.Predicate_messageEntityTextUrl
		case "text_mention":
			predicate = mtproto.Predicate_messageEntityMentionName
		case "custom_emoji":
			predicate = mtproto.Predicate_messageEntityCustomEmoji
		case "phone_number":
			predicate = mtproto.Predicate_messageEntityPhone
		}
		converted := &mtproto.MessageEntity{PredicateName: predicate, Offset: int32(entity.Offset), Length: int32(entity.Length), Url: entity.URL, Language: entity.Language, DocumentId: parseInt64(entity.CustomEmojiID)}
		if entity.User != nil {
			converted.UserId_INT64 = entity.User.ID
		}
		result = append(result, converted)
	}
	return result
}

func ReplyMarkupToMTProto(in ReplyMarkupInput) *mtproto.ReplyMarkup {
	if in.ForceReply {
		return mtproto.MakeTLReplyKeyboardForceReply(&mtproto.ReplyMarkup{Placeholder: wrapperspb.String(in.InputFieldPlaceholder)}).To_ReplyMarkup()
	}
	if in.RemoveKeyboard {
		return mtproto.MakeTLReplyKeyboardHide(&mtproto.ReplyMarkup{Selective: in.Selective}).To_ReplyMarkup()
	}
	rows := make([]*mtproto.KeyboardButtonRow, 0)
	if len(in.InlineKeyboard) > 0 {
		for _, row := range in.InlineKeyboard {
			buttons := make([]*mtproto.KeyboardButton, 0, len(row))
			for _, button := range row {
				base := &mtproto.KeyboardButton{Text: button.Text, Url: button.URL, Data: []byte(button.CallbackData), Query: button.SwitchInlineQuery}
				switch {
				case button.WebApp != nil:
					base.Url = button.WebApp.URL
					buttons = append(buttons, mtproto.MakeTLKeyboardButtonWebView(base).To_KeyboardButton())
				case button.CallbackData != "":
					buttons = append(buttons, mtproto.MakeTLKeyboardButtonCallback(base).To_KeyboardButton())
				case button.SwitchInlineQuery != "" || button.SwitchInlineQueryCurrentChat != "":
					if button.SwitchInlineQueryCurrentChat != "" {
						base.SamePeer = true
						base.Query = button.SwitchInlineQueryCurrentChat
					}
					buttons = append(buttons, mtproto.MakeTLKeyboardButtonSwitchInline(base).To_KeyboardButton())
				default:
					buttons = append(buttons, mtproto.MakeTLKeyboardButton(base).To_KeyboardButton())
				}
			}
			rows = append(rows, mtproto.MakeTLKeyboardButtonRow(&mtproto.KeyboardButtonRow{Buttons: buttons}).To_KeyboardButtonRow())
		}
		return mtproto.MakeTLReplyInlineMarkup(&mtproto.ReplyMarkup{Rows: rows}).To_ReplyMarkup()
	}
	for _, row := range in.Keyboard {
		buttons := make([]*mtproto.KeyboardButton, 0, len(row))
		for _, raw := range row {
			button := &mtproto.KeyboardButton{Text: stringFromMap(raw, "text")}
			switch {
			case webAppURLFromMap(raw["web_app"]) != "":
				button.Url = webAppURLFromMap(raw["web_app"])
				buttons = append(buttons, mtproto.MakeTLKeyboardButtonWebView(button).To_KeyboardButton())
			case boolFromMap(raw, "request_contact"):
				buttons = append(buttons, mtproto.MakeTLKeyboardButtonRequestPhone(button).To_KeyboardButton())
			case boolFromMap(raw, "request_location"):
				buttons = append(buttons, mtproto.MakeTLKeyboardButtonRequestGeoLocation(button).To_KeyboardButton())
			default:
				buttons = append(buttons, mtproto.MakeTLKeyboardButton(button).To_KeyboardButton())
			}
		}
		rows = append(rows, mtproto.MakeTLKeyboardButtonRow(&mtproto.KeyboardButtonRow{Buttons: buttons}).To_KeyboardButtonRow())
	}
	return mtproto.MakeTLReplyKeyboardMarkup(&mtproto.ReplyMarkup{Rows: rows, Resize: in.ResizeKeyboard, SingleUse: in.OneTimeKeyboard, Selective: in.Selective, Persistent: in.IsPersistent}).To_ReplyMarkup()
}

func stringFromMap(values map[string]any, key string) string {
	if value, ok := values[key].(string); ok {
		return value
	}
	return ""
}

func webAppURLFromMap(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		return stringFromMap(typed, "url")
	case map[string]string:
		return strings.TrimSpace(typed["url"])
	default:
		return ""
	}
}

func boolFromMap(values map[string]any, key string) bool {
	if value, ok := values[key].(bool); ok {
		return value
	}
	return false
}

func parseInt64(value string) int64 {
	result, _ := strconv.ParseInt(value, 10, 64)
	return result
}

// UserFromImmutableUser converts the user service's authoritative user shape.
func UserFromImmutableUser(in *mtproto.ImmutableUser) *User {
	if in == nil || in.GetUser() == nil {
		return nil
	}
	d := in.GetUser()
	bot := d.GetBot()
	return &User{
		ID:                      d.GetId(),
		IsBot:                   bot != nil,
		FirstName:               d.GetFirstName(),
		LastName:                d.GetLastName(),
		Username:                d.GetUsername(),
		IsPremium:               d.GetPremium(),
		AddedToAttachmentMenu:   bot != nil && bot.GetBotAttachMenu(),
		CanJoinGroups:           bot != nil && !bot.GetBotNochats(),
		CanReadAllGroupMessages: bot != nil && bot.GetBotChatHistory(),
		// ImmutableUser embeds the older BotData projection, which does not
		// carry the newer guest/forum capability bits.
		SupportsGuestQueries:      false,
		SupportsInlineQueries:     bot != nil && (bot.GetBotInlinePlaceholder() != nil || bot.GetBotInlineGeo()),
		CanConnectToBusiness:      bot != nil && bot.GetBotBusiness(),
		HasMainWebApp:             bot != nil && bot.GetBotHasMainApp(),
		HasTopicsEnabled:          false,
		AllowsUsersToCreateTopics: false,
		CanManageBots:             d.GetBotCanManageBots(),
	}
}

func UserFromMTProto(in *mtproto.User) *User {
	if in == nil {
		return nil
	}
	return &User{
		ID:                        in.GetId(),
		IsBot:                     in.GetBot(),
		FirstName:                 stringValue(in.GetFirstName()),
		LastName:                  stringValue(in.GetLastName()),
		Username:                  stringValue(in.GetUsername()),
		LanguageCode:              stringValue(in.GetLangCode()),
		IsPremium:                 in.GetPremium(),
		AddedToAttachmentMenu:     in.GetBot() && in.GetBotAttachMenu(),
		CanJoinGroups:             in.GetBot() && !in.GetBotNochats(),
		CanReadAllGroupMessages:   in.GetBot() && in.GetBotChatHistory(),
		SupportsGuestQueries:      in.GetBot() && in.GetBotGuestchat(),
		SupportsInlineQueries:     in.GetBot() && (in.GetBotInlinePlaceholder() != nil || in.GetBotInlineGeo()),
		CanConnectToBusiness:      in.GetBot() && in.GetBotBusiness(),
		HasMainWebApp:             in.GetBot() && in.GetBotHasMainApp(),
		HasTopicsEnabled:          in.GetBot() && in.GetBotForumView(),
		AllowsUsersToCreateTopics: in.GetBot() && in.GetBotForumCanManageTopics(),
		CanManageBots:             in.GetBotCanManageBots(),
	}
}

func BotCommandsFromMTProto(in []*mtproto.BotCommand) []BotCommand {
	result := make([]BotCommand, 0, len(in))
	for _, command := range in {
		if command != nil {
			result = append(result, BotCommand{Command: command.GetCommand(), Description: command.GetDescription()})
		}
	}
	return result
}

// ChatMemberFromChannelParticipant converts the participant constructors used
// by channels and supergroups into Telegram Bot API member objects.
func ChatMemberFromChannelParticipant(in *mtproto.ChannelParticipant, users []*mtproto.User) *ChatMember {
	if in == nil {
		return nil
	}
	member := &ChatMember{
		Status:      "member",
		User:        userByID(in.GetUserId(), users),
		CustomTitle: stringValue(in.GetRank()),
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_channelParticipantCreator:
		member.Status = "creator"
		member.IsAnonymous = in.GetAdminRights().GetAnonymous()
	case mtproto.Predicate_channelParticipantAdmin:
		member.Status = "administrator"
		member.CanBeEdited = in.GetCanEdit()
		member.CanManageChat = true
		applyChatAdminRights(member, in.GetAdminRights())
	case mtproto.Predicate_channelParticipantSelf:
		// A caller's own channel participant is encoded as Self even when the
		// caller is an administrator. Bot API callers still need the effective
		// administrator status and rights for getChatMember/admins.
		if in.GetAdminRights() != nil {
			member.Status = "administrator"
			member.CanManageChat = true
			applyChatAdminRights(member, in.GetAdminRights())
		}
	case mtproto.Predicate_channelParticipantBanned:
		rights := in.GetBannedRights()
		member.UntilDate = int64(rightsUntilDate(rights))
		if in.GetLeft() || (rights != nil && rights.GetViewMessages()) {
			member.Status = "kicked"
			member.IsMember = false
			return member
		}
		member.Status = "restricted"
		applyChatBannedRights(member, rights)
	case mtproto.Predicate_channelParticipantLeft:
		member.Status = "left"
		member.IsMember = false
	}
	return member
}

// ChatMemberFromChatParticipant converts the participant constructors used by
// legacy basic groups into Telegram Bot API member objects.
func ChatMemberFromChatParticipant(in *mtproto.ChatParticipant, users []*mtproto.User) *ChatMember {
	if in == nil {
		return nil
	}
	member := &ChatMember{Status: "member", User: userByID(in.GetUserId(), users), CustomTitle: stringValue(in.GetRank()), IsMember: true}
	switch in.GetPredicateName() {
	case mtproto.Predicate_chatParticipantCreator:
		member.Status = "creator"
	case mtproto.Predicate_chatParticipantAdmin:
		member.Status = "administrator"
		member.CanManageChat = true
		member.CanDeleteMessages = true
		member.CanRestrictMembers = true
		member.CanInviteUsers = true
		member.CanPinMessages = true
		member.CanChangeInfo = true
	}
	return member
}

func applyChatAdminRights(member *ChatMember, rights *mtproto.ChatAdminRights) {
	if member == nil || rights == nil {
		return
	}
	member.IsAnonymous = rights.GetAnonymous()
	member.CanDeleteMessages = rights.GetDeleteMessages()
	member.CanManageVideoChats = rights.GetManageCall()
	member.CanRestrictMembers = rights.GetBanUsers()
	member.CanPromoteMembers = rights.GetAddAdmins()
	member.CanChangeInfo = rights.GetChangeInfo()
	member.CanInviteUsers = rights.GetInviteUsers()
	member.CanPostMessages = rights.GetPostMessages()
	member.CanEditMessages = rights.GetEditMessages()
	member.CanPinMessages = rights.GetPinMessages()
	member.CanManageTopics = rights.GetManageTopics()
	member.CanManageDirectMessages = rights.GetManageDirectMessages()
	member.CanPostStories = rights.GetPostStories()
	member.CanEditStories = rights.GetEditStories()
	member.CanDeleteStories = rights.GetDeleteStories()
}

func applyChatBannedRights(member *ChatMember, rights *mtproto.ChatBannedRights) {
	if member == nil {
		return
	}
	if rights == nil {
		member.IsMember = true
		member.CanSendMessages = true
		member.CanSendAudios = true
		member.CanSendDocuments = true
		member.CanSendPhotos = true
		member.CanSendVideos = true
		member.CanSendVideoNotes = true
		member.CanSendVoiceNotes = true
		member.CanSendPolls = true
		member.CanSendOtherMessages = true
		member.CanAddWebPagePreviews = true
		member.CanChangeInfo = true
		member.CanInviteUsers = true
		member.CanPinMessages = true
		member.CanManageTopics = true
		return
	}
	member.IsMember = !rights.GetViewMessages()
	member.CanSendMessages = !rights.GetSendMessages()
	member.CanSendAudios = !rights.GetSendAudios()
	member.CanSendDocuments = !rights.GetSendDocs()
	member.CanSendPhotos = !rights.GetSendPhotos()
	member.CanSendVideos = !rights.GetSendVideos()
	member.CanSendVideoNotes = !rights.GetSendRoundvideos()
	member.CanSendVoiceNotes = !rights.GetSendVoices()
	member.CanSendPolls = !rights.GetSendPolls()
	member.CanSendOtherMessages = !rights.GetSendStickers() && !rights.GetSendGifs() && !rights.GetSendGames() && !rights.GetSendInline()
	member.CanAddWebPagePreviews = !rights.GetEmbedLinks()
	member.CanChangeInfo = !rights.GetChangeInfo()
	member.CanInviteUsers = !rights.GetInviteUsers()
	member.CanPinMessages = !rights.GetPinMessages()
	member.CanManageTopics = !rights.GetManageTopics()
	member.UntilDate = int64(rightsUntilDate(rights))
}

func rightsUntilDate(rights *mtproto.ChatBannedRights) int32 {
	if rights == nil {
		return 0
	}
	return rights.GetUntilDate()
}

func MTProtoBotCommands(in []BotCommand) []*mtproto.BotCommand {
	result := make([]*mtproto.BotCommand, 0, len(in))
	for _, command := range in {
		result = append(result, mtproto.MakeTLBotCommand(&mtproto.BotCommand{
			Command:     command.Command,
			Description: command.Description,
		}).To_BotCommand())
	}
	return result
}

func ChatIDForPeer(peer *mtproto.Peer) (int64, string) {
	if peer == nil {
		return 0, ""
	}
	switch peer.GetPredicateName() {
	case mtproto.Predicate_peerUser:
		return peer.GetUserId(), "private"
	case mtproto.Predicate_peerChat:
		return -peer.GetChatId(), "group"
	case mtproto.Predicate_peerChannel:
		return -1000000000000 - peer.GetChannelId(), "supergroup"
	default:
		if peer.GetUserId() != 0 {
			return peer.GetUserId(), "private"
		}
		if peer.GetChatId() != 0 {
			return -peer.GetChatId(), "group"
		}
		if peer.GetChannelId() != 0 {
			return -1000000000000 - peer.GetChannelId(), "supergroup"
		}
	}
	return 0, ""
}

func ChatFromPeer(peer *mtproto.Peer, users []*mtproto.User, chats []*mtproto.Chat) *Chat {
	id, chatType := ChatIDForPeer(peer)
	if id == 0 || chatType == "" {
		return nil
	}
	result := &Chat{ID: id, Type: chatType}
	if chatType == "private" {
		for _, user := range users {
			if user != nil && user.GetId() == peer.GetUserId() {
				converted := UserFromMTProto(user)
				if converted != nil {
					result.FirstName = converted.FirstName
					result.LastName = converted.LastName
					result.Username = converted.Username
				}
				break
			}
		}
		return result
	}
	chatID := peer.GetChatId()
	if chatType == "supergroup" {
		chatID = peer.GetChannelId()
	}
	for _, chat := range chats {
		if chat == nil || chat.GetId() != chatID {
			continue
		}
		result.Title = chat.GetTitle()
		result.Username = stringValue(chat.GetUsername())
		result.IsForum = chat.GetForum()
		break
	}
	return result
}

func ChatFromImmutableChat(in *mtproto.ImmutableChat, chatType string) *Chat {
	if in == nil {
		return nil
	}
	if chatType == "" {
		chatType = "group"
	}
	id := in.GetId()
	if chatType == "group" {
		id = -id
	} else if chatType == "supergroup" {
		id = -1000000000000 - id
	}
	chat := &Chat{ID: id, Type: chatType, Title: in.GetTitle(), IsForum: false, Photo: ChatPhotoFromPhoto(in.GetPhoto())}
	chat.Description = in.GetAbout()
	chat.Permissions = ChatPermissionsFromBannedRights(in.GetDefaultBannedRights())
	if invite := in.GetExportedInvite(); invite != nil {
		chat.InviteLink = invite.GetLink()
	}
	if in.GetTtlPeriod() > 0 {
		chat.MessageAutoDeleteTime = int(in.GetTtlPeriod())
	}
	return chat
}

// ChatFromMTProtoChat converts a full MTProto chat/channel constructor to the
// Bot API chat shape. Channel ids use Telegram's -100... namespace.
func ChatFromMTProtoChat(in *mtproto.Chat) *Chat {
	if in == nil || in.GetId() <= 0 {
		return nil
	}
	chatType := "group"
	id := -in.GetId()
	if in.GetPredicateName() == mtproto.Predicate_channel || in.GetBroadcast() || in.GetMegagroup() {
		chatType = "supergroup"
		if in.GetBroadcast() && !in.GetMegagroup() {
			chatType = "channel"
		}
		id = -1000000000000 - in.GetId()
	}
	return &Chat{
		ID:                 id,
		Type:               chatType,
		Title:              in.GetTitle(),
		Username:           stringValue(in.GetUsername()),
		IsForum:            in.GetForum(),
		JoinToSendMessages: in.GetJoinToSend(),
		JoinByRequest:      in.GetJoinRequest(),
		Photo:              ChatPhotoFromMTProto(in.GetPhoto()),
	}
}

func ChatPhotoFromPhoto(photo *mtproto.Photo) *ChatPhoto {
	if photo == nil || photo.GetId() <= 0 {
		return nil
	}
	fileID := fileID("photo", photo.GetId(), photo.GetAccessHash())
	uniqueID := strconv.FormatInt(photo.GetId(), 10)
	return &ChatPhoto{SmallFileID: fileID, SmallFileUniqueID: uniqueID, BigFileID: fileID, BigFileUniqueID: uniqueID}
}

func ChatPhotoFromMTProto(photo *mtproto.ChatPhoto) *ChatPhoto {
	if photo == nil || photo.GetPhotoId() <= 0 || photo.GetPredicateName() == mtproto.Predicate_chatPhotoEmpty {
		return nil
	}
	fileID := fileID("photo", photo.GetPhotoId(), 0)
	uniqueID := strconv.FormatInt(photo.GetPhotoId(), 10)
	return &ChatPhoto{SmallFileID: fileID, SmallFileUniqueID: uniqueID, BigFileID: fileID, BigFileUniqueID: uniqueID}
}

func ChatPhotoFromUserProfilePhoto(photo *mtproto.UserProfilePhoto) *ChatPhoto {
	if photo == nil || photo.GetPhotoId() <= 0 || photo.GetPredicateName() == mtproto.Predicate_userProfilePhotoEmpty {
		return nil
	}
	fileID := fileID("photo", photo.GetPhotoId(), 0)
	uniqueID := strconv.FormatInt(photo.GetPhotoId(), 10)
	return &ChatPhoto{SmallFileID: fileID, SmallFileUniqueID: uniqueID, BigFileID: fileID, BigFileUniqueID: uniqueID}
}

func UserProfilePhotosFromMTProto(photos *mtproto.Photos_Photos) *UserProfilePhotos {
	if photos == nil {
		return &UserProfilePhotos{Photos: [][]PhotoSize{}}
	}
	result := &UserProfilePhotos{TotalCount: int(photos.GetCount()), Photos: make([][]PhotoSize, 0, len(photos.GetPhotos()))}
	for _, photo := range photos.GetPhotos() {
		if sizes := photoSizesFromMTProto(photo); len(sizes) > 0 {
			result.Photos = append(result.Photos, sizes)
		}
	}
	if result.TotalCount == 0 {
		result.TotalCount = len(result.Photos)
	}
	return result
}

func ChatPermissionsFromBannedRights(rights *mtproto.ChatBannedRights) *ChatPermissions {
	if rights == nil {
		return nil
	}
	return &ChatPermissions{
		CanSendMessages:       !rights.GetSendMessages(),
		CanSendAudios:         !rights.GetSendAudios(),
		CanSendDocuments:      !rights.GetSendDocs(),
		CanSendPhotos:         !rights.GetSendPhotos(),
		CanSendVideos:         !rights.GetSendVideos(),
		CanSendVideoNotes:     !rights.GetSendRoundvideos(),
		CanSendVoiceNotes:     !rights.GetSendVoices(),
		CanSendPolls:          !rights.GetSendPolls(),
		CanSendOtherMessages:  !rights.GetSendStickers() && !rights.GetSendGifs() && !rights.GetSendGames() && !rights.GetSendInline(),
		CanAddWebPagePreviews: !rights.GetEmbedLinks(),
		CanChangeInfo:         !rights.GetChangeInfo(),
		CanInviteUsers:        !rights.GetInviteUsers(),
		CanPinMessages:        !rights.GetPinMessages(),
		CanManageTopics:       !rights.GetManageTopics(),
	}
}

func MessageFromMTProto(in *mtproto.Message, users []*mtproto.User, chats []*mtproto.Chat) *Message {
	if in == nil {
		return nil
	}
	message := &Message{
		MessageID:           int(in.GetId()),
		From:                userForPeer(in.GetFromId(), users),
		Date:                int64(in.GetDate()),
		Chat:                ChatFromPeer(in.GetPeerId(), users, chats),
		SenderBoostCount:    int(wrapperInt32(in.GetFromBoostsApplied())),
		SenderTag:           stringValue(in.GetFromRank()),
		Text:                in.GetMessage(),
		Entities:            messageEntities(in.GetEntities(), users),
		EditDate:            int64(wrapperInt32(in.GetEditDate())),
		IsFromOffline:       in.GetOffline(),
		MediaGroupID:        fmt.Sprint(wrapperInt64(in.GetGroupedId())),
		HasProtectedContent: in.GetNoforwards(),
		AuthorSignature:     stringValue(in.GetPostAuthor()),
		IsPaidPost:          in.GetPaidSuggestedPostStars() || in.GetPaidSuggestedPostTon(),
		PaidStarCount:       wrapperInt64(in.GetPaidMessageStars()),
	}
	if effect := in.GetEffect(); effect != nil && effect.GetValue() != 0 {
		message.EffectID = strconv.FormatInt(effect.GetValue(), 10)
	}
	if message.MediaGroupID == "0" {
		message.MediaGroupID = ""
	}
	if in.GetViaBotId() != nil {
		message.ViaBot = userByID(in.GetViaBotId().GetValue(), users)
	}
	if in.GetViaBusinessBotId() != nil {
		message.SenderBusinessBot = userByID(in.GetViaBusinessBotId().GetValue(), users)
	}
	if in.GetGuestchatViaFrom() != nil {
		peer := in.GetGuestchatViaFrom()
		if peer.GetUserId() != 0 {
			message.GuestBotCallerUser = userByID(peer.GetUserId(), users)
		} else {
			message.GuestBotCallerChat = ChatFromPeer(peer, users, chats)
		}
	}
	if in.GetFwdFrom() != nil {
		fwd := in.GetFwdFrom()
		message.ForwardOrigin = messageOriginFromMTProto(fwd, users, chats)
		if fwd.GetFromId() != nil {
			if fwd.GetFromId().GetUserId() != 0 {
				message.ForwardFrom = userByID(fwd.GetFromId().GetUserId(), users)
			} else {
				message.ForwardFromChat = ChatFromPeer(fwd.GetFromId(), users, chats)
			}
		}
		message.ForwardFromMessageID = int(wrapperInt32(fwd.GetChannelPost()))
		message.IsAutomaticForward = fwd.GetImported()
	}
	if in.GetFromId() != nil && in.GetFromId().GetUserId() == 0 {
		message.SenderChat = ChatFromPeer(in.GetFromId(), users, chats)
	}
	if in.GetReplyTo() != nil {
		reply := in.GetReplyTo()
		message.IsTopicMessage = reply.GetForumTopic()
		message.MessageThreadID = int(wrapperInt32(reply.GetReplyToTopId()))
		replyID := reply.GetReplyToMsgId()
		if replyID == 0 {
			replyID = reply.GetReplyToMsgId_INT32()
		}
		if replyID == 0 {
			replyID = wrapperInt32(reply.GetReplyToMsgId_FLAGINT32())
		}
		if replyID != 0 {
			message.ReplyToMessage = &Message{MessageID: int(replyID), Chat: message.Chat}
		}
	}
	message.ReplyMarkup = replyMarkupFromMTProto(in.GetReplyMarkup())
	message.RichMessage = in.GetRichMessage()
	message.ShowCaptionAboveMedia = in.GetInvertMedia()
	message.SuggestedPostInfo = suggestedPostInfoFromMTProto(in.GetSuggestedPost())
	applyMedia(message, in.GetMedia(), users, chats)
	applyMessageAction(message, in.GetAction(), users, chats)
	message.SuccessfulPayment = successfulPaymentFromMessageAction(in.GetAction())
	return message
}

func messageFromEphemeralMTProto(in *mtproto.EphemeralMessage, users []*mtproto.User, chats []*mtproto.Chat) *Message {
	if in == nil {
		return nil
	}
	message := &Message{
		MessageID:          0,
		EphemeralMessageID: int(in.GetId()),
		From:               userForPeer(in.GetFromId(), users),
		ReceiverUser:       userByID(in.GetReceiverId(), users),
		Date:               int64(in.GetDate()),
		Chat:               ChatFromPeer(in.GetPeerId(), users, chats),
		Text:               in.GetMessage(),
		Entities:           messageEntities(in.GetEntities(), users),
	}
	message.ReplyMarkup = replyMarkupFromMTProto(in.GetReplyMarkup())
	if rich, err := mtprotoext.EphemeralRichMessage(in); err == nil {
		message.RichMessage = rich
	}
	applyMedia(message, in.GetMedia(), users, chats)
	if reply := in.GetReplyTo(); reply != nil {
		message.IsTopicMessage = reply.GetForumTopic()
		message.MessageThreadID = int(wrapperInt32(reply.GetReplyToTopId()))
	}
	return message
}

func suggestedPostInfoFromMTProto(in *mtproto.SuggestedPost) *SuggestedPostInfo {
	if in == nil {
		return nil
	}
	state := "pending"
	if in.GetAccepted() {
		state = "approved"
	} else if in.GetRejected() {
		state = "declined"
	}
	return &SuggestedPostInfo{State: state, Price: suggestedPostPriceFromStars(in.GetPrice()), SendDate: int64(wrapperInt32(in.GetScheduleDate()))}
}

func suggestedPostPriceFromStars(in *mtproto.StarsAmount) *SuggestedPostPrice {
	if in == nil {
		return nil
	}
	return &SuggestedPostPrice{Currency: "XTR", Amount: in.GetAmount()}
}

func suggestedPostPriceFromAction(in *mtproto.MessageAction) *SuggestedPostPrice {
	if in == nil {
		return nil
	}
	if currency := in.GetCryptoCurrency_STRING(); currency != "" {
		return &SuggestedPostPrice{Currency: currency, Amount: in.GetCryptoAmount_INT64()}
	}
	return suggestedPostPriceFromStars(in.GetPrice())
}

func applyMessageAction(message *Message, action *mtproto.MessageAction, users []*mtproto.User, chats []*mtproto.Chat) {
	if message == nil || action == nil {
		return
	}
	switch action.GetPredicateName() {
	case mtproto.Predicate_messageActionStarGift:
		if gift := giftInfoFromMTProto(action, users, chats); gift != nil {
			if action.GetUpgradeSeparate() || action.GetUpgrade() {
				message.GiftUpgradeSent = gift
			} else {
				message.Gift = gift
			}
		}
	case mtproto.Predicate_messageActionStarGiftUnique:
		if gift := uniqueGiftInfoFromMTProto(action, users, chats); gift != nil {
			message.UniqueGift = gift
		}
	case mtproto.Predicate_messageActionTodoCompletions:
		message.ChecklistTasksDone = &ChecklistTasksDone{
			MarkedAsDoneTaskIDs:    int32SliceToInt(action.GetCompleted()),
			MarkedAsNotDoneTaskIDs: int32SliceToInt(action.GetIncompleted()),
		}
	case mtproto.Predicate_messageActionTodoAppendTasks:
		message.ChecklistTasksAdded = &ChecklistTasksAdded{Tasks: checklistTasksFromMTProto(action.GetList(), nil, users, chats)}
	case mtproto.Predicate_messageActionSuggestedPostApproval:
		message.SuggestedPostApproved = &SuggestedPostApproved{Price: suggestedPostPriceFromAction(action), SendDate: int64(wrapperInt32(action.GetScheduleDate_FLAGINT32()))}
	case mtproto.Predicate_messageActionSuggestedPostSuccess:
		message.SuggestedPostPaid = &SuggestedPostPaid{Currency: action.GetCryptoCurrency_STRING(), Amount: action.GetCryptoAmount_INT64(), StarAmount: starAmountFromMTProto(action.GetPrice())}
		if message.SuggestedPostPaid.Currency == "" {
			message.SuggestedPostPaid.Currency = "XTR"
		}
	case mtproto.Predicate_messageActionSuggestedPostRefund:
		message.SuggestedPostRefunded = &SuggestedPostRefunded{Reason: suggestedPostRefundReason(action)}
	case mtproto.Predicate_messageActionManagedBotCreated:
		if action.GetBotId() > 0 {
			message.ManagedBotCreated = &ManagedBotCreated{Bot: userByID(action.GetBotId(), users)}
		}
	}
}

func suggestedPostRefundReason(action *mtproto.MessageAction) string {
	if action == nil {
		return ""
	}
	if action.GetRejected() {
		return "post_deleted"
	}
	return "payment_refunded"
}

func starAmountFromMTProto(in *mtproto.StarsAmount) *StarAmount {
	if in == nil {
		return nil
	}
	return &StarAmount{Amount: in.GetAmount(), NanostarAmount: int(in.GetNanos())}
}

func giftInfoFromMTProto(action *mtproto.MessageAction, users []*mtproto.User, chats []*mtproto.Chat) *GiftInfo {
	if action == nil || action.GetGift() == nil {
		return nil
	}
	text, entities := actionTextFromMTProto(action, users)
	return &GiftInfo{
		Gift:                    giftFromMTProto(action.GetGift(), users, chats),
		ConvertStarCount:        wrapperInt64(action.GetConvertStars_FLAGINT64()),
		PrepaidUpgradeStarCount: wrapperInt64(action.GetUpgradeStars()),
		IsUpgradeSeparate:       action.GetUpgradeSeparate(),
		CanBeUpgraded:           action.GetCanUpgrade(),
		Text:                    text,
		Entities:                entities,
		IsPrivate:               false,
		UniqueGiftNumber:        int(wrapperInt32(action.GetGiftNum())),
	}
}

func uniqueGiftInfoFromMTProto(action *mtproto.MessageAction, users []*mtproto.User, chats []*mtproto.Chat) *UniqueGiftInfo {
	if action == nil || action.GetGift() == nil {
		return nil
	}
	text, entities := actionTextFromMTProto(action, users)
	origin := "upgrade"
	switch {
	case action.GetFromOffer():
		origin = "offer"
	case action.GetAuctionAcquired():
		origin = "resale"
	case action.GetTransferred():
		origin = "transfer"
	case action.GetAssigned():
		origin = "gifted_upgrade"
	}
	return &UniqueGiftInfo{
		Gift:              uniqueGiftFromMTProto(action.GetGift(), users, chats),
		Origin:            origin,
		Text:              text,
		Entities:          entities,
		IsPrivate:         false,
		OwnedGiftID:       stringValue(action.GetPrepaidUpgradeHash()),
		TransferStarCount: wrapperInt64(action.GetTransferStars()),
		NextTransferDate:  int64(wrapperInt32(action.GetCanTransferAt())),
	}
}

func actionTextFromMTProto(action *mtproto.MessageAction, users []*mtproto.User) (string, []MessageEntity) {
	if action == nil {
		return "", nil
	}
	if text := action.GetMessage_FLAGTEXTWITHENTITIES(); text != nil {
		return text.GetText(), messageEntities(text.GetEntities(), users)
	}
	return action.GetMessage_STRING(), nil
}

func giftFromMTProto(in *mtproto.StarGift, users []*mtproto.User, chats []*mtproto.Chat) *Gift {
	if in == nil {
		return nil
	}
	result := &Gift{
		ID:                     strconv.FormatInt(in.GetId(), 10),
		StarCount:              in.GetStars(),
		UpgradeStarCount:       wrapperInt64(in.GetUpgradeStars()),
		IsPremium:              in.GetRequirePremium(),
		HasColors:              in.GetPeerColorAvailable(),
		TotalCount:             int(wrapperInt32(in.GetAvailabilityTotal_FLAGINT32())),
		RemainingCount:         int(wrapperInt32(in.GetAvailabilityRemains())),
		PersonalTotalCount:     int(wrapperInt32(in.GetPerUserTotal())),
		PersonalRemainingCount: int(wrapperInt32(in.GetPerUserRemains())),
		UniqueGiftVariantCount: int(wrapperInt32(in.GetUpgradeVariants())),
	}
	result.Sticker = stickerFromGiftDocument(in.GetSticker())
	if in.GetBackground() != nil {
		result.Background = &GiftBackground{CenterColor: int(in.GetBackground().GetCenterColor()), EdgeColor: int(in.GetBackground().GetEdgeColor()), TextColor: int(in.GetBackground().GetTextColor())}
	}
	if in.GetReleasedBy() != nil {
		result.PublisherChat = ChatFromPeer(in.GetReleasedBy(), users, chats)
	}
	return result
}

func uniqueGiftFromMTProto(in *mtproto.StarGift, users []*mtproto.User, chats []*mtproto.Chat) *UniqueGift {
	if in == nil {
		return nil
	}
	result := &UniqueGift{
		GiftID:           strconv.FormatInt(in.GetGiftId(), 10),
		BaseName:         in.GetTitle_STRING(),
		Name:             in.GetSlug(),
		Number:           int(in.GetNum()),
		IsPremium:        in.GetRequirePremium(),
		IsBurned:         in.GetBurned(),
		IsFromBlockchain: in.GetGiftAddress() != nil,
	}
	if result.BaseName == "" {
		result.BaseName = stringValue(in.GetTitle_FLAGSTRING())
	}
	for _, attribute := range in.GetAttributes() {
		if attribute == nil {
			continue
		}
		rarity := 0
		if attribute.GetRarity() != nil {
			rarity = int(attribute.GetRarity().GetPermille())
		}
		sticker := stickerFromGiftDocument(attribute.GetDocument())
		switch {
		case strings.Contains(attribute.GetPredicateName(), "Model"):
			result.Model = &UniqueGiftModel{Name: attribute.GetName(), Sticker: sticker, RarityPerMille: rarity, Rarity: giftRarity(attribute.GetRarity())}
		case strings.Contains(attribute.GetPredicateName(), "Pattern"):
			result.Symbol = &UniqueGiftSymbol{Name: attribute.GetName(), Sticker: sticker, RarityPerMille: rarity}
		case strings.Contains(attribute.GetPredicateName(), "Backdrop"):
			result.Backdrop = &UniqueGiftBackdrop{Name: attribute.GetName(), RarityPerMille: rarity, Colors: &UniqueGiftBackdropColors{CenterColor: int(attribute.GetCenterColor()), EdgeColor: int(attribute.GetEdgeColor()), SymbolColor: int(attribute.GetPatternColor()), TextColor: int(attribute.GetTextColor())}}
		}
	}
	if in.GetReleasedBy() != nil {
		result.PublisherChat = ChatFromPeer(in.GetReleasedBy(), users, chats)
	}
	return result
}

func giftRarity(in *mtproto.StarGiftAttributeRarity) string {
	if in == nil {
		return ""
	}
	return strings.TrimPrefix(in.GetPredicateName(), "starGiftAttributeRarity")
}

func stickerFromGiftDocument(doc *mtproto.Document) *Sticker {
	if doc == nil {
		return nil
	}
	result := stickerFromDocument(doc)
	if result == nil {
		return nil
	}
	result.FileID = fileID("document", doc.GetId(), doc.GetAccessHash())
	result.FileUniqueID = strconv.FormatInt(doc.GetId(), 10)
	return result
}

func checklistFromMTProto(in *mtproto.TodoList, completions []*mtproto.TodoCompletion, users []*mtproto.User, chats []*mtproto.Chat) *Checklist {
	if in == nil {
		return nil
	}
	title := in.GetTitle()
	return &Checklist{
		Title:                  textWithEntitiesText(title),
		TitleEntities:          textWithEntitiesEntities(title, users),
		Tasks:                  checklistTasksFromMTProto(in.GetList(), completions, users, chats),
		OthersCanAddTasks:      in.GetOthersCanAppend(),
		OthersCanMarkTasksDone: in.GetOthersCanComplete(),
	}
}

func checklistTasksFromMTProto(in []*mtproto.TodoItem, completions []*mtproto.TodoCompletion, users []*mtproto.User, chats []*mtproto.Chat) []ChecklistTask {
	result := make([]ChecklistTask, 0, len(in))
	for _, item := range in {
		if item == nil {
			continue
		}
		task := ChecklistTask{ID: int(item.GetId()), Text: textWithEntitiesText(item.GetTitle()), TextEntities: textWithEntitiesEntities(item.GetTitle(), users)}
		for _, completion := range completions {
			if completion == nil || completion.GetId() != item.GetId() {
				continue
			}
			task.CompletionDate = int64(completion.GetDate())
			if peer := completion.GetCompletedBy_PEER(); peer != nil {
				if peer.GetUserId() != 0 {
					task.CompletedByUser = userByID(peer.GetUserId(), users)
				} else {
					task.CompletedByChat = ChatFromPeer(peer, users, chats)
				}
			} else if completion.GetCompletedBy_INT64() != 0 {
				task.CompletedByUser = userByID(completion.GetCompletedBy_INT64(), users)
			}
		}
		result = append(result, task)
	}
	return result
}

func textWithEntitiesText(in *mtproto.TextWithEntities) string {
	if in == nil {
		return ""
	}
	return in.GetText()
}

func textWithEntitiesEntities(in *mtproto.TextWithEntities, users []*mtproto.User) []MessageEntity {
	if in == nil {
		return nil
	}
	return messageEntities(in.GetEntities(), users)
}

func boolValue(in *mtproto.Bool) bool {
	return in != nil && in.GetPredicateName() == mtproto.Predicate_boolTrue
}

func messageOriginFromMTProto(in *mtproto.MessageFwdHeader, users []*mtproto.User, chats []*mtproto.Chat) *MessageOrigin {
	if in == nil {
		return nil
	}
	origin := &MessageOrigin{
		Date:            int64(in.GetDate()),
		AuthorSignature: stringValue(in.GetPostAuthor()),
		MessageID:       int(wrapperInt32(in.GetChannelPost())),
	}
	if peer := in.GetFromId(); peer != nil {
		if peer.GetUserId() != 0 {
			origin.Type = "user"
			origin.SenderUser = userByID(peer.GetUserId(), users)
			return origin
		}
		chat := ChatFromPeer(peer, users, chats)
		if in.GetChannelPost() != nil {
			origin.Type = "channel"
			origin.Chat = chat
		} else {
			origin.Type = "chat"
			origin.SenderChat = chat
		}
		return origin
	}
	if name := stringValue(in.GetFromName()); name != "" {
		origin.Type = "hidden_user"
		origin.SenderUserName = name
		return origin
	}
	return nil
}

func successfulPaymentFromMessageAction(action *mtproto.MessageAction) *SuccessfulPayment {
	if action == nil || (action.GetPredicateName() != mtproto.Predicate_messageActionPaymentSentMe && action.GetPredicateName() != mtproto.Predicate_messageActionPaymentSent) {
		return nil
	}
	result := &SuccessfulPayment{
		Currency:         action.GetCurrency_STRING(),
		TotalAmount:      action.GetTotalAmount(),
		InvoicePayload:   string(action.GetPayload()),
		ShippingOptionID: stringValue(action.GetShippingOptionId()),
		OrderInfo:        orderInfoFromMTProto(action.GetInfo()),
		IsRecurring:      action.GetRecurringUsed(),
		IsFirstRecurring: action.GetRecurringInit(),
	}
	if charge := action.GetCharge(); charge != nil {
		result.TelegramPaymentChargeID = charge.GetId()
		result.ProviderPaymentChargeID = charge.GetProviderChargeId()
	}
	if expiration := action.GetSubscriptionUntilDate(); expiration != nil {
		result.SubscriptionExpirationDate = int64(expiration.GetValue())
	}
	return result
}

// UpdateFromMTProto converts an update without a bot identity. Callers that
// own a bot token should use UpdateFromMTProtoForBot so participant updates
// can be classified as my_chat_member when the target is that bot.
func UpdateFromMTProto(in *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat) (*Update, bool) {
	return UpdateFromMTProtoForBot(in, users, chats, 0)
}

// UpdateFromMTProtoForBot is the Bot API update conversion with the receiving
// bot id available. Telegram distinguishes chat_member from my_chat_member by
// the affected user, so the id must be supplied by the update store rather
// than guessed from the transport update alone.
func UpdateFromMTProtoForBot(in *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, botID int64) (*Update, bool) {
	if in == nil {
		return nil, false
	}
	result := &Update{}
	switch in.GetPredicateName() {
	case mtproto.Predicate_updateNewMessage:
		result.Message = MessageFromMTProto(in.GetMessage_MESSAGE(), users, chats)
	case mtproto.Predicate_updateEditMessage:
		result.EditedMessage = MessageFromMTProto(in.GetMessage_MESSAGE(), users, chats)
	case mtproto.Predicate_updateNewChannelMessage:
		result.ChannelPost = MessageFromMTProto(in.GetMessage_MESSAGE(), users, chats)
	case mtproto.Predicate_updateEditChannelMessage:
		result.EditedChannelPost = MessageFromMTProto(in.GetMessage_MESSAGE(), users, chats)
	case mtproto.Predicate_updateBotGuestChatQuery:
		message := MessageFromMTProto(in.GetMessage_MESSAGE(), users, chats)
		if message == nil {
			return nil, false
		}
		message.GuestQueryID = strconv.FormatInt(in.GetQueryId_INT64(), 10)
		result.GuestMessage = message
	case mtproto.Predicate_updateNewEphemeralMessage:
		result.Message = messageFromEphemeralMTProto(in.GetMessage_EPHEMERALMESSAGE(), users, chats)
		if result.Message == nil {
			return nil, false
		}
	case mtproto.Predicate_updateEditEphemeralMessage:
		result.EditedMessage = messageFromEphemeralMTProto(in.GetMessage_EPHEMERALMESSAGE(), users, chats)
		if result.EditedMessage == nil {
			return nil, false
		}
	case mtproto.Predicate_updateManagedBot:
		if in.GetUserId() <= 0 || in.GetBotId() <= 0 {
			return nil, false
		}
		result.ManagedBot = &ManagedBotUpdated{
			User: userByID(in.GetUserId(), users),
			Bot:  userByID(in.GetBotId(), users),
		}
	case mtproto.Predicate_updateBotStarsSubscription:
		if in.GetUserId() <= 0 {
			return nil, false
		}
		state := ""
		switch {
		case in.GetCanceled():
			state = "canceled"
		case in.GetPaymentFailed():
			state = "failed"
		case in.GetRestored():
			state = "active"
		}
		if state == "" {
			return nil, false
		}
		result.Subscription = &BotSubscriptionUpdated{
			User:           userByID(in.GetUserId(), users),
			InvoicePayload: string(in.GetPayload_BYTES()),
			State:          state,
		}
	case mtproto.Predicate_updateBotStopped:
		// Layer 229's updateBotStopped has no draft_id, which is required by
		// Bot API MessageGenerationStopped. Do not emit a malformed update.
		return nil, false
	case mtproto.Predicate_updateBotInlineQuery:
		result.InlineQuery = &InlineQuery{
			ID:       fmt.Sprintf("%d", in.GetQueryId_INT64()),
			From:     userByID(in.GetUserId(), users),
			ChatType: inlineChatType(in.GetPeerType()),
			Query:    in.GetQuery(),
			Offset:   in.GetOffset(),
		}
	case mtproto.Predicate_updateBotInlineSend:
		result.ChosenInlineResult = &ChosenInlineResult{
			ResultID:        in.GetId_STRING(),
			From:            userByID(in.GetUserId(), users),
			Location:        locationFromGeoPoint(in.GetGeo()),
			InlineMessageID: inputBotInlineMessageIDString(in.GetMsgId_FLAGINPUTBOTINLINEMESSAGEID()),
			Query:           in.GetQuery(),
		}
	case mtproto.Predicate_updateInlineBotCallbackQuery:
		result.CallbackQuery = &CallbackQuery{
			ID:              fmt.Sprintf("%d", in.GetQueryId_INT64()),
			From:            userByID(in.GetUserId(), users),
			InlineMessageID: inputBotInlineMessageIDString(in.GetMsgId_FLAGINPUTBOTINLINEMESSAGEID()),
			ChatInstance:    fmt.Sprintf("%d", in.GetChatInstance()),
			Data:            string(in.GetData_FLAGBYTES()),
			GameShortName:   stringValue(in.GetGameShortName()),
		}
	case mtproto.Predicate_updateBotCallbackQuery:
		result.CallbackQuery = &CallbackQuery{
			ID:            fmt.Sprintf("%d", in.GetQueryId_INT64()),
			From:          userByID(in.GetUserId(), users),
			Message:       callbackMessageForBot(in, users, chats, botID),
			ChatInstance:  fmt.Sprintf("%d", in.GetChatInstance()),
			Data:          string(in.GetData_FLAGBYTES()),
			GameShortName: stringValue(in.GetGameShortName()),
		}
	case mtproto.Predicate_updateMessagePoll:
		result.Poll = pollFromMTProto(in.GetPoll(), in.GetResults(), users, chats)
		if result.Poll == nil {
			return nil, false
		}
	case mtproto.Predicate_updateMessagePollVote:
		result.PollAnswer = &PollAnswer{
			PollID:    strconv.FormatInt(in.GetPollId(), 10),
			OptionIDs: int32SliceToInt(in.GetPositions()),
		}
		if in.GetUserId() != 0 {
			result.PollAnswer.User = userByID(in.GetUserId(), users)
		} else {
			result.PollAnswer.VoterChat = ChatFromPeer(in.GetPeer_PEER(), users, chats)
		}
	case mtproto.Predicate_updateBotChatInviteRequester:
		result.ChatJoinRequest = chatJoinRequestFromMTProto(in, users, chats)
		if result.ChatJoinRequest == nil {
			return nil, false
		}
	case mtproto.Predicate_updateBotMessageReaction:
		result.MessageReaction = messageReactionUpdatedFromMTProto(in, users, chats)
		if result.MessageReaction == nil {
			return nil, false
		}
	case mtproto.Predicate_updateBotMessageReactions:
		result.MessageReactionCount = messageReactionCountUpdatedFromMTProto(in, users, chats)
		if result.MessageReactionCount == nil {
			return nil, false
		}
	case mtproto.Predicate_updateBotChatBoost:
		if boost := chatBoostFromMTProto(in.GetBoost(), users); boost != nil {
			result.ChatBoost = &ChatBoostUpdated{Chat: ChatFromPeer(in.GetPeer_PEER(), users, chats), Boost: boost}
		} else {
			return nil, false
		}
	case mtproto.Predicate_updateChatParticipant:
		return chatMemberUpdateFromMTProto(in, users, chats, botID, false)
	case mtproto.Predicate_updateChannelParticipant:
		return chatMemberUpdateFromMTProto(in, users, chats, botID, true)
	case mtproto.Predicate_updateBotPurchasedPaidMedia:
		result.PurchasedPaidMedia = &PaidMediaPurchased{
			From:             userByID(in.GetUserId(), users),
			PaidMediaPayload: in.GetPayload_STRING(),
		}
	case mtproto.Predicate_updateBotWebhookJSONQuery:
		data := ""
		if payload := in.GetData_DATAJSON(); payload != nil {
			data = payload.GetData()
		}
		result.WebAppQuery = &WebAppQuery{
			ID:           fmt.Sprintf("%d", in.GetQueryId_INT64()),
			From:         userByID(in.GetUserId(), users),
			ChatInstance: fmt.Sprintf("%d", in.GetChatId_INT64()),
			Data:         data,
		}
	case mtproto.Predicate_updateBotShippingQuery:
		result.ShippingQuery = &ShippingQuery{
			ID:              fmt.Sprintf("%d", in.GetQueryId_INT64()),
			From:            userByID(in.GetUserId(), users),
			InvoicePayload:  string(in.GetPayload_BYTES()),
			ShippingAddress: shippingAddressFromMTProto(in.GetShippingAddress()),
		}
	case mtproto.Predicate_updateBotPrecheckoutQuery:
		result.PreCheckoutQuery = &PreCheckoutQuery{
			ID:               fmt.Sprintf("%d", in.GetQueryId_INT64()),
			From:             userByID(in.GetUserId(), users),
			Currency:         in.GetCurrency(),
			TotalAmount:      in.GetTotalAmount(),
			InvoicePayload:   string(in.GetPayload_BYTES()),
			ShippingOptionID: stringValue(in.GetShippingOptionId()),
			OrderInfo:        orderInfoFromMTProto(in.GetInfo_FLAGPAYMENTREQUESTEDINFO()),
		}
	case mtproto.Predicate_updateBotBusinessConnect:
		connection := in.GetConnection()
		if connection == nil || connection.GetConnectionId() == "" {
			return nil, false
		}
		result.BusinessConnection = &BusinessConnection{
			ID:         connection.GetConnectionId(),
			User:       userByID(connection.GetUserId(), users),
			UserChatID: connection.GetUserId(),
			Date:       int64(connection.GetDate()),
			CanReply:   connection.GetCanReply(),
			IsEnabled:  !connection.GetDisabled(),
			Rights:     businessBotRightsFromMTProto(connection.GetRights()),
		}
	case mtproto.Predicate_updateBotNewBusinessMessage:
		result.BusinessMessage = businessMessageFromMTProto(in, users, chats)
	case mtproto.Predicate_updateBotEditBusinessMessage:
		result.EditedBusinessMessage = businessMessageFromMTProto(in, users, chats)
	case mtproto.Predicate_updateBotDeleteBusinessMessage:
		result.DeletedBusinessMessages = &BusinessMessagesDeleted{
			BusinessConnectionID: in.GetConnectionId(),
			Chat:                 ChatFromPeer(in.GetPeer_PEER(), users, chats),
			MessageIDs:           int32SliceToInt(in.GetIds()),
		}
	case mtproto.Predicate_updateBusinessBotCallbackQuery:
		result.CallbackQuery = &CallbackQuery{
			ID:            fmt.Sprintf("%d", in.GetQueryId_INT64()),
			From:          userByID(in.GetUserId(), users),
			Message:       businessCallbackMessage(in, users, chats),
			ChatInstance:  fmt.Sprintf("%d", in.GetChatInstance()),
			Data:          string(in.GetData_FLAGBYTES()),
			GameShortName: stringValue(in.GetGameShortName()),
		}
	default:
		return nil, false
	}
	return result, true
}

func chatMemberUpdateFromMTProto(in *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, botID int64, channel bool) (*Update, bool) {
	if in == nil {
		return nil, false
	}
	targetID := in.GetUserId()
	var oldMember, newMember *ChatMember
	var viaJoinRequest bool
	if channel {
		oldParticipant := in.GetPrevParticipant_FLAGCHANNELPARTICIPANT()
		newParticipant := in.GetNewParticipant_FLAGCHANNELPARTICIPANT()
		if targetID == 0 {
			if newParticipant != nil {
				targetID = newParticipant.GetUserId()
			} else if oldParticipant != nil {
				targetID = oldParticipant.GetUserId()
			}
		}
		oldMember = ChatMemberFromChannelParticipant(oldParticipant, users)
		newMember = ChatMemberFromChannelParticipant(newParticipant, users)
		if newParticipant != nil {
			viaJoinRequest = newParticipant.GetViaRequest()
		}
	} else {
		oldParticipant := in.GetPrevParticipant_FLAGCHATPARTICIPANT()
		newParticipant := in.GetNewParticipant_FLAGCHATPARTICIPANT()
		if targetID == 0 {
			if newParticipant != nil {
				targetID = newParticipant.GetUserId()
			} else if oldParticipant != nil {
				targetID = oldParticipant.GetUserId()
			}
		}
		oldMember = ChatMemberFromChatParticipant(oldParticipant, users)
		newMember = ChatMemberFromChatParticipant(newParticipant, users)
	}
	if targetID <= 0 || oldMember == nil && newMember == nil {
		return nil, false
	}
	if oldMember == nil {
		oldMember = &ChatMember{Status: "left", User: userByID(targetID, users), IsMember: false}
	}
	if newMember == nil {
		newMember = &ChatMember{Status: "left", User: userByID(targetID, users), IsMember: false}
	}
	var peer *mtproto.Peer
	if channel {
		if in.GetChannelId() <= 0 {
			return nil, false
		}
		peer = mtproto.MakeTLPeerChannel(&mtproto.Peer{ChannelId: in.GetChannelId()}).To_Peer()
	} else {
		if in.GetChatId_INT64() <= 0 {
			return nil, false
		}
		peer = mtproto.MakeTLPeerChat(&mtproto.Peer{ChatId: in.GetChatId_INT64()}).To_Peer()
	}
	converted := &ChatMemberUpdated{
		Chat:           ChatFromPeer(peer, users, chats),
		From:           userByID(in.GetActorId(), users),
		Date:           int64(in.GetDate_INT32()),
		OldChatMember:  oldMember,
		NewChatMember:  newMember,
		InviteLink:     chatInviteLinkFromMTProto(in.GetInvite(), users),
		ViaChatFolder:  in.GetViaChatlist(),
		ViaJoinRequest: viaJoinRequest,
	}
	if converted.From != nil && converted.From.ID == 0 {
		converted.From = nil
	}
	if botID > 0 && targetID == botID {
		return &Update{MyChatMember: converted}, true
	}
	return &Update{ChatMember: converted}, true
}

func chatJoinRequestFromMTProto(in *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat) *ChatJoinRequest {
	if in == nil || in.GetPeer_PEER() == nil || in.GetUserId() == 0 {
		return nil
	}
	return &ChatJoinRequest{
		Chat:       ChatFromPeer(in.GetPeer_PEER(), users, chats),
		From:       userByID(in.GetUserId(), users),
		UserChatID: in.GetUserId(),
		Date:       int64(in.GetDate_INT32()),
		Bio:        in.GetAbout(),
		InviteLink: chatInviteLinkFromMTProto(in.GetInvite(), users),
		QueryID:    pollQueryID(in.GetQueryId_FLAGINT64()),
	}
}

func pollQueryID(value *wrapperspb.Int64Value) string {
	if id := wrapperInt64(value); id != 0 {
		return strconv.FormatInt(id, 10)
	}
	return ""
}

func chatInviteLinkFromMTProto(in *mtproto.ExportedChatInvite, users []*mtproto.User) *ChatInviteLink {
	if in == nil {
		return nil
	}
	return &ChatInviteLink{
		InviteLink:              in.GetLink(),
		Creator:                 userByID(in.GetAdminId(), users),
		CreatesJoinRequest:      in.GetRequestNeeded(),
		IsPrimary:               in.GetPermanent(),
		IsRevoked:               in.GetRevoked(),
		Name:                    stringValue(in.GetTitle()),
		ExpireDate:              int64(wrapperInt32(in.GetExpireDate())),
		MemberLimit:             int(wrapperInt32(in.GetUsageLimit())),
		PendingJoinRequestCount: int(wrapperInt32(in.GetRequested())),
	}
}

func messageReactionUpdatedFromMTProto(in *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat) *MessageReactionUpdated {
	if in == nil || in.GetPeer_PEER() == nil {
		return nil
	}
	result := &MessageReactionUpdated{
		Chat:        ChatFromPeer(in.GetPeer_PEER(), users, chats),
		MessageID:   int(in.GetMsgId_INT32()),
		Date:        int64(in.GetDate_INT32()),
		OldReaction: reactionTypesFromMTProto(in.GetOldReactions()),
		NewReaction: reactionTypesFromMTProto(in.GetNewReactions()),
	}
	if actor := in.GetActor(); actor != nil {
		if actor.GetUserId() != 0 {
			result.User = userByID(actor.GetUserId(), users)
		} else {
			result.ActorChat = ChatFromPeer(actor, users, chats)
		}
	}
	return result
}

func messageReactionCountUpdatedFromMTProto(in *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat) *MessageReactionCountUpdated {
	if in == nil || in.GetPeer_PEER() == nil {
		return nil
	}
	result := &MessageReactionCountUpdated{
		Chat:      ChatFromPeer(in.GetPeer_PEER(), users, chats),
		MessageID: int(in.GetMsgId_INT32()),
		Date:      int64(in.GetDate_INT32()),
		Reactions: make([]ReactionCount, 0, len(in.GetReactions_VECTORREACTIONCOUNT())),
	}
	for _, reaction := range in.GetReactions_VECTORREACTIONCOUNT() {
		if reaction == nil {
			continue
		}
		result.Reactions = append(result.Reactions, ReactionCount{
			Type:       reactionTypeFromMTProto(reaction.GetReaction_REACTION(), reaction.GetReaction()),
			TotalCount: int(reaction.GetCount()),
		})
	}
	return result
}

func reactionTypesFromMTProto(values []*mtproto.Reaction) []ReactionType {
	result := make([]ReactionType, 0, len(values))
	for _, value := range values {
		if converted := reactionTypeFromMTProto(value, ""); converted.Type != "" {
			result = append(result, converted)
		}
	}
	return result
}

func reactionTypeFromMTProto(reaction *mtproto.Reaction, fallback string) ReactionType {
	if reaction != nil {
		switch reaction.GetPredicateName() {
		case mtproto.Predicate_reactionEmoji:
			return ReactionType{Type: "emoji", Emoji: reaction.GetEmoticon()}
		case mtproto.Predicate_reactionCustomEmoji:
			return ReactionType{Type: "custom_emoji", CustomEmojiID: strconv.FormatInt(reaction.GetDocumentId(), 10)}
		}
	}
	if fallback != "" {
		return ReactionType{Type: "emoji", Emoji: fallback}
	}
	return ReactionType{}
}

func chatBoostFromMTProto(in *mtproto.Boost, users []*mtproto.User) *ChatBoost {
	if in == nil || in.GetId() == "" {
		return nil
	}
	result := &ChatBoost{
		BoostID:        in.GetId(),
		AddDate:        int64(in.GetDate()),
		ExpirationDate: int64(in.GetExpires()),
	}
	if userID := wrapperInt64(in.GetUserId()); userID != 0 {
		result.Source = &ChatBoostSource{Source: "premium", User: userByID(userID, users)}
	} else if in.GetGiveaway() {
		result.Source = &ChatBoostSource{Source: "giveaway", GiveawayMessageID: int(wrapperInt32(in.GetGiveawayMsgId()))}
	} else if in.GetGift() {
		result.Source = &ChatBoostSource{Source: "gift_code"}
	}
	return result
}

func locationFromGeoPoint(in *mtproto.GeoPoint) *Location {
	if in == nil {
		return nil
	}
	location := &Location{Longitude: in.GetLong(), Latitude: in.GetLat()}
	if accuracy := in.GetAccuracyRadius(); accuracy != nil && accuracy.GetValue() > 0 {
		location.HorizontalAccuracy = float64(accuracy.GetValue())
	}
	return location
}

func inputBotInlineMessageIDString(in *mtproto.InputBotInlineMessageID) string {
	if in == nil || in.GetDcId() <= 0 || in.GetAccessHash() == 0 {
		return ""
	}
	if in.GetPredicateName() == mtproto.Predicate_inputBotInlineMessageID64 || in.GetOwnerId() != 0 {
		return fmt.Sprintf("%d:%d:%d:%d", in.GetDcId(), in.GetOwnerId(), in.GetId_INT32(), in.GetAccessHash())
	}
	return fmt.Sprintf("%d:%d:%d", in.GetDcId(), in.GetId_INT64(), in.GetAccessHash())
}

var validAllowedUpdateTypes = map[string]struct{}{
	"message": {}, "edited_message": {}, "channel_post": {}, "edited_channel_post": {},
	"business_connection": {}, "business_message": {}, "edited_business_message": {}, "deleted_business_messages": {},
	"inline_query": {}, "chosen_inline_result": {}, "callback_query": {}, "guest_message": {}, "web_app_query": {}, "shipping_query": {}, "pre_checkout_query": {},
	"purchased_paid_media": {}, "poll": {}, "poll_answer": {}, "my_chat_member": {}, "chat_member": {}, "chat_join_request": {},
	"message_reaction": {}, "message_reaction_count": {}, "chat_boost": {}, "removed_chat_boost": {},
	"managed_bot": {}, "subscription": {}, "stopped_message_generation": {},
}

// IsValidAllowedUpdateType reports whether name is a Telegram Bot API
// allowed_updates value. Some valid values are not emitted by this server yet,
// but accepting them preserves the wire contract and simply yields no such
// update until the corresponding MTProto adapter is available.
func IsValidAllowedUpdateType(name string) bool {
	_, ok := validAllowedUpdateTypes[strings.TrimSpace(name)]
	return ok
}

func businessMessageFromMTProto(in *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat) *Message {
	if in == nil || in.GetMessage_MESSAGE() == nil {
		return nil
	}
	message := MessageFromMTProto(in.GetMessage_MESSAGE(), users, chats)
	if message != nil {
		message.BusinessConnectionID = in.GetConnectionId()
		if reply := in.GetReplyToMessage(); reply != nil {
			message.ReplyToMessage = MessageFromMTProto(reply, users, chats)
		}
	}
	return message
}

func businessCallbackMessage(in *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat) *Message {
	if in == nil {
		return nil
	}
	if in.GetMessage_MESSAGE() == nil {
		return callbackMessage(in, users, chats)
	}
	message := MessageFromMTProto(in.GetMessage_MESSAGE(), users, chats)
	if message != nil {
		message.BusinessConnectionID = in.GetConnectionId()
		if reply := in.GetReplyToMessage(); reply != nil {
			message.ReplyToMessage = MessageFromMTProto(reply, users, chats)
		}
	}
	return message
}

func int32SliceToInt(values []int32) []int {
	result := make([]int, 0, len(values))
	for _, value := range values {
		result = append(result, int(value))
	}
	return result
}

func businessBotRightsFromMTProto(in *mtproto.BusinessBotRights) *BusinessBotRights {
	if in == nil {
		return nil
	}
	return &BusinessBotRights{
		CanReply:                   in.GetReply(),
		CanReadMessages:            in.GetReadMessages(),
		CanDeleteSentMessages:      in.GetDeleteSentMessages(),
		CanDeleteAllMessages:       in.GetDeleteReceivedMessages(),
		CanEditName:                in.GetEditName(),
		CanEditBio:                 in.GetEditBio(),
		CanEditProfilePhoto:        in.GetEditProfilePhoto(),
		CanEditUsername:            in.GetEditUsername(),
		CanViewGifts:               in.GetViewGifts(),
		CanSellGifts:               in.GetSellGifts(),
		CanChangeGiftSettings:      in.GetChangeGiftSettings(),
		CanTransferAndUpgradeGifts: in.GetTransferAndUpgradeGifts(),
		CanTransferStars:           in.GetTransferStars(),
		CanManageStories:           in.GetManageStories(),
	}
}

func shippingAddressFromMTProto(address *mtproto.PostAddress) *ShippingAddress {
	if address == nil {
		return nil
	}
	return &ShippingAddress{
		CountryCode: address.GetCountryIso2(),
		State:       address.GetState(),
		City:        address.GetCity(),
		StreetLine1: address.GetStreetLine1(),
		StreetLine2: address.GetStreetLine2(),
		PostCode:    address.GetPostCode(),
	}
}

func orderInfoFromMTProto(info *mtproto.PaymentRequestedInfo) *OrderInfo {
	if info == nil {
		return nil
	}
	return &OrderInfo{
		Name:            stringValue(info.GetName()),
		PhoneNumber:     stringValue(info.GetPhone()),
		Email:           stringValue(info.GetEmail()),
		ShippingAddress: shippingAddressFromMTProto(info.GetShippingAddress()),
	}
}

func EncodeUpdate(in *Update) (json.RawMessage, error) {
	if in == nil {
		return nil, nil
	}
	return json.Marshal(in)
}

func FirstMessageFromUpdates(updates *mtproto.Updates) *Message {
	messages := MessagesFromUpdates(updates)
	if len(messages) == 0 {
		return nil
	}
	return messages[0]
}

// MessagesFromUpdates preserves the order of new and edited message updates.
func MessagesFromUpdates(updates *mtproto.Updates) []*Message {
	if updates == nil {
		return nil
	}
	result := make([]*Message, 0)
	mtproto.VisitUpdates(0, updates, map[string]mtproto.UpdateVisitedFunc{
		mtproto.Predicate_updateNewMessage: func(_ int64, update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, _ int32) {
			if message := MessageFromMTProto(update.GetMessage_MESSAGE(), users, chats); message != nil {
				result = append(result, message)
			}
		},
		mtproto.Predicate_updateNewChannelMessage: func(_ int64, update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, _ int32) {
			if message := MessageFromMTProto(update.GetMessage_MESSAGE(), users, chats); message != nil {
				result = append(result, message)
			}
		},
		mtproto.Predicate_updateEditMessage: func(_ int64, update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, _ int32) {
			if message := MessageFromMTProto(update.GetMessage_MESSAGE(), users, chats); message != nil {
				result = append(result, message)
			}
		},
		mtproto.Predicate_updateEditChannelMessage: func(_ int64, update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, _ int32) {
			if message := MessageFromMTProto(update.GetMessage_MESSAGE(), users, chats); message != nil {
				result = append(result, message)
			}
		},
		mtproto.Predicate_updateNewEphemeralMessage: func(_ int64, update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, _ int32) {
			if message := messageFromEphemeralMTProto(update.GetMessage_EPHEMERALMESSAGE(), users, chats); message != nil {
				result = append(result, message)
			}
		},
		mtproto.Predicate_updateEditEphemeralMessage: func(_ int64, update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, _ int32) {
			if message := messageFromEphemeralMTProto(update.GetMessage_EPHEMERALMESSAGE(), users, chats); message != nil {
				result = append(result, message)
			}
		},
	})
	return result
}

func stringValue(value *wrapperspb.StringValue) string {
	if value == nil {
		return ""
	}
	return value.GetValue()
}

func wrapperInt32(value *wrapperspb.Int32Value) int32 {
	if value == nil {
		return 0
	}
	return value.GetValue()
}

func wrapperInt64(value *wrapperspb.Int64Value) int64 {
	if value == nil {
		return 0
	}
	return value.GetValue()
}

func userByID(id int64, users []*mtproto.User) *User {
	for _, user := range users {
		if user != nil && user.GetId() == id {
			return UserFromMTProto(user)
		}
	}
	return &User{ID: id}
}

func userForPeer(peer *mtproto.Peer, users []*mtproto.User) *User {
	if peer == nil || peer.GetUserId() == 0 {
		return nil
	}
	return userByID(peer.GetUserId(), users)
}

func messageEntities(entities []*mtproto.MessageEntity, users []*mtproto.User) []MessageEntity {
	result := make([]MessageEntity, 0, len(entities))
	for _, entity := range entities {
		if entity == nil {
			continue
		}
		converted := MessageEntity{
			Type:     entityType(entity.GetPredicateName()),
			Offset:   int(entity.GetOffset()),
			Length:   int(entity.GetLength()),
			URL:      entity.GetUrl(),
			Language: entity.GetLanguage(),
		}
		if entity.GetUserId_INT64() != 0 {
			converted.User = userByID(entity.GetUserId_INT64(), users)
		}
		if entity.GetDocumentId() != 0 {
			converted.CustomEmojiID = strconv.FormatInt(entity.GetDocumentId(), 10)
		}
		result = append(result, converted)
	}
	return result
}

func entityType(predicate string) string {
	if strings.HasPrefix(predicate, "messageEntity") {
		value := strings.TrimPrefix(predicate, "messageEntity")
		switch value {
		case "BotCommand":
			return "bot_command"
		case "TextUrl":
			return "text_link"
		case "MentionName":
			return "text_mention"
		case "Strike":
			return "strikethrough"
		case "Blockquote":
			return "blockquote"
		case "Pre":
			return "pre"
		case "CustomEmoji":
			return "custom_emoji"
		case "Phone":
			return "phone_number"
		case "":
			return ""
		}
		return strings.ToLower(value[:1]) + value[1:]
	}
	return predicate
}

func inlineChatType(peer *mtproto.InlineQueryPeerType) string {
	if peer == nil {
		return "private"
	}
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inlineQueryPeerTypeChat:
		return "group"
	case mtproto.Predicate_inlineQueryPeerTypeMegagroup, mtproto.Predicate_inlineQueryPeerTypeBroadcast:
		return "supergroup"
	default:
		return "private"
	}
}

func callbackMessage(update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat) *Message {
	return callbackMessageForBot(update, users, chats, 0)
}

func callbackMessageForBot(update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, botID int64) *Message {
	chat := ChatFromPeer(update.GetPeer_PEER(), users, chats)
	// Some callback paths carry the private peer from the clicker's dialog.
	// Bot API exposes that same dialog with the clicker's user id.
	if botID > 0 && update.GetPeer_PEER() != nil && update.GetPeer_PEER().GetUserId() == botID && update.GetUserId() > 0 && update.GetUserId() != botID {
		chat = ChatFromPeer(mtproto.MakePeerUser(update.GetUserId()), users, chats)
	}
	if chat == nil {
		return nil
	}
	return &Message{MessageID: int(update.GetMsgId_INT32()), Date: 0, Chat: chat}
}

func fileID(kind string, id, accessHash int64) string {
	return fmt.Sprintf("%s_%d_%d", kind, id, accessHash)
}

func applyMedia(message *Message, media *mtproto.MessageMedia, users []*mtproto.User, chats []*mtproto.Chat) {
	if message == nil || media == nil {
		return
	}
	switch media.GetPredicateName() {
	case mtproto.Predicate_messageMediaGeo, mtproto.Predicate_messageMediaGeoLive:
		if geo := media.GetGeo(); geo != nil {
			location := &Location{
				Longitude:            geo.GetLong(),
				Latitude:             geo.GetLat(),
				HorizontalAccuracy:   float64(wrapperInt32(geo.GetAccuracyRadius())),
				LivePeriod:           int(media.GetPeriod()),
				Heading:              int(wrapperInt32(media.GetHeading())),
				ProximityAlertRadius: int(wrapperInt32(media.GetProximityNotificationRadius())),
			}
			message.Location = location
		}
	case mtproto.Predicate_messageMediaVenue:
		if geo := media.GetGeo(); geo != nil {
			venue := &Venue{
				Location: &Location{Longitude: geo.GetLong(), Latitude: geo.GetLat(), HorizontalAccuracy: float64(wrapperInt32(geo.GetAccuracyRadius()))},
				Title:    media.GetTitle(),
				Address:  media.GetAddress(),
			}
			if strings.EqualFold(media.GetProvider(), "google") {
				venue.GooglePlaceID = media.GetVenueId()
				venue.GooglePlaceType = media.GetVenueType()
			} else {
				venue.FoursquareID = media.GetVenueId()
				venue.FoursquareType = media.GetVenueType()
			}
			message.Venue = venue
		}
	case mtproto.Predicate_messageMediaContact:
		message.Contact = &Contact{
			PhoneNumber: media.GetPhoneNumber(),
			FirstName:   media.GetFirstName(),
			LastName:    media.GetLastName(),
			Vcard:       media.GetVcard(),
			UserID:      media.GetUserId(),
		}
	case mtproto.Predicate_messageMediaPoll:
		message.Poll = pollFromMTProto(media.GetPoll(), media.GetResults(), users, chats)
	case mtproto.Predicate_messageMediaDice:
		message.Dice = &Dice{Emoji: media.GetEmoticon(), Value: int(media.GetValue())}
	case mtproto.Predicate_messageMediaGame:
		message.Game = gameFromMTProto(media.GetGame())
		if message.Game != nil && message.Text != "" {
			message.Game.Text = message.Text
			message.Game.TextEntities = message.Entities
			message.Text = ""
			message.Entities = nil
		}
	case mtproto.Predicate_messageMediaInvoice:
		message.Invoice = &Invoice{
			Title:          media.GetTitle(),
			Description:    media.GetDescription(),
			StartParameter: media.GetStartParam(),
			Currency:       media.GetCurrency(),
			TotalAmount:    media.GetTotalAmount(),
		}
	case mtproto.Predicate_messageMediaPaidMedia:
		message.PaidMedia = paidMediaInfoFromMTProto(media, users, chats)
	case mtproto.Predicate_messageMediaToDo:
		message.Checklist = checklistFromMTProto(media.GetTodo(), media.GetCompletions(), users, chats)
	case mtproto.Predicate_messageMediaPhoto:
		if photo := media.GetPhoto_FLAGPHOTO(); photo != nil {
			message.Photo = make([]PhotoSize, 0, len(photo.GetSizes()))
			for _, size := range photo.GetSizes() {
				if size == nil {
					continue
				}
				message.Photo = append(message.Photo, PhotoSize{
					FileID:       fileID("photo", photo.GetId(), photo.GetAccessHash()),
					FileUniqueID: strconv.FormatInt(photo.GetId(), 10),
					Width:        int(size.GetW()),
					Height:       int(size.GetH()),
					FileSize:     int64(size.GetSize2()),
				})
			}
			if media.GetLivePhoto() && media.GetVideo_FLAGDOCUMENT() != nil {
				doc := media.GetVideo_FLAGDOCUMENT()
				_, duration, width, height, _, _, _, _, _, _ := documentAttributes(doc)
				message.LivePhoto = &LivePhoto{
					Photo:        message.Photo,
					FileID:       fileID("document", doc.GetId(), doc.GetAccessHash()),
					FileUniqueID: strconv.FormatInt(doc.GetId(), 10),
					Width:        width,
					Height:       height,
					Duration:     duration,
					MimeType:     doc.GetMimeType(),
					FileSize:     documentSize(doc),
				}
			}
		}
	case mtproto.Predicate_messageMediaDocument:
		doc := media.GetDocument()
		if doc == nil {
			doc = media.GetVideo_FLAGDOCUMENT()
		}
		if doc == nil {
			return
		}
		name, duration, width, height, performer, title, voice, animated, video, roundVideo := documentAttributes(doc)
		id := fileID("document", doc.GetId(), doc.GetAccessHash())
		if sticker := stickerFromDocument(doc); sticker != nil {
			sticker.FileID, sticker.FileUniqueID = id, strconv.FormatInt(doc.GetId(), 10)
			message.Sticker = sticker
		} else if roundVideo {
			message.VideoNote = &VideoNote{FileID: id, FileUniqueID: strconv.FormatInt(doc.GetId(), 10), Length: width, Duration: duration, FileSize: documentSize(doc)}
		} else if video {
			message.Video = &Video{FileID: id, FileUniqueID: strconv.FormatInt(doc.GetId(), 10), Width: width, Height: height, Duration: duration, FileName: name, MimeType: doc.GetMimeType(), FileSize: documentSize(doc)}
		} else if animated {
			message.Animation = &Animation{FileID: id, FileUniqueID: strconv.FormatInt(doc.GetId(), 10), Width: width, Height: height, Duration: duration, FileName: name, MimeType: doc.GetMimeType(), FileSize: documentSize(doc)}
		} else if voice {
			message.Voice = &Voice{FileID: id, FileUniqueID: strconv.FormatInt(doc.GetId(), 10), Duration: duration, MimeType: doc.GetMimeType(), FileSize: documentSize(doc)}
		} else if performer != "" || title != "" || strings.HasPrefix(doc.GetMimeType(), "audio/") {
			message.Audio = &Audio{FileID: id, FileUniqueID: strconv.FormatInt(doc.GetId(), 10), Duration: duration, Performer: performer, Title: title, FileName: name, MimeType: doc.GetMimeType(), FileSize: documentSize(doc)}
		} else {
			message.Document = &Document{FileID: id, FileUniqueID: strconv.FormatInt(doc.GetId(), 10), FileName: name, MimeType: doc.GetMimeType(), FileSize: documentSize(doc)}
		}
		if message.Text != "" {
			message.Caption = message.Text
			message.Text = ""
			message.CaptionEntities = message.Entities
			message.Entities = nil
		}
	case mtproto.Predicate_messageMediaWebPage:
		if media.GetWebpage() != nil && media.GetWebpage().GetDescription() != nil {
			message.Caption = media.GetWebpage().GetDescription().GetValue()
		}
	}
}

func paidMediaInfoFromMTProto(media *mtproto.MessageMedia, users []*mtproto.User, chats []*mtproto.Chat) *PaidMediaInfo {
	if media == nil || media.GetPredicateName() != mtproto.Predicate_messageMediaPaidMedia {
		return nil
	}
	result := &PaidMediaInfo{StarCount: int(media.GetStarsAmount()), PaidMedia: make([]any, 0, len(media.GetExtendedMedia_VECTORMESSAGEEXTENDEDMEDIA()))}
	for _, extended := range media.GetExtendedMedia_VECTORMESSAGEEXTENDEDMEDIA() {
		if extended == nil {
			continue
		}
		if extended.GetPredicateName() == mtproto.Predicate_messageExtendedMediaPreview {
			result.PaidMedia = append(result.PaidMedia, &PaidMediaPreview{Type: "preview", Width: int(wrapperInt32(extended.GetW())), Height: int(wrapperInt32(extended.GetH())), Duration: int(wrapperInt32(extended.GetVideoDuration()))})
			continue
		}
		nested := &Message{}
		applyMedia(nested, extended.GetMedia(), users, chats)
		switch {
		case len(nested.Photo) > 0:
			result.PaidMedia = append(result.PaidMedia, &PaidMediaPhoto{Type: "photo", Photo: nested.Photo})
		case nested.Video != nil:
			result.PaidMedia = append(result.PaidMedia, &PaidMediaVideo{Type: "video", Video: nested.Video})
		case nested.Animation != nil:
			result.PaidMedia = append(result.PaidMedia, &PaidMediaVideo{Type: "video", Video: &Video{FileID: nested.Animation.FileID, FileUniqueID: nested.Animation.FileUniqueID, Width: nested.Animation.Width, Height: nested.Animation.Height, Duration: nested.Animation.Duration, FileSize: nested.Animation.FileSize, FileName: nested.Animation.FileName, MimeType: nested.Animation.MimeType}})
		}
	}
	return result
}

func gameFromMTProto(game *mtproto.Game) *Game {
	if game == nil {
		return nil
	}
	result := &Game{
		Title:       game.GetTitle(),
		Description: game.GetDescription(),
		Photo:       photoSizesFromMTProto(game.GetPhoto()),
	}
	if document := game.GetDocument(); document != nil {
		name, duration, width, height, _, _, _, animated, _, _ := documentAttributes(document)
		if animated || strings.HasPrefix(document.GetMimeType(), "video/") {
			result.Animation = &Animation{
				FileID:       fileID("document", document.GetId(), document.GetAccessHash()),
				FileUniqueID: strconv.FormatInt(document.GetId(), 10),
				Width:        width,
				Height:       height,
				Duration:     duration,
				FileName:     name,
				MimeType:     document.GetMimeType(),
				FileSize:     documentSize(document),
			}
		}
	}
	return result
}

func photoSizesFromMTProto(photo *mtproto.Photo) []PhotoSize {
	if photo == nil {
		return nil
	}
	result := make([]PhotoSize, 0, len(photo.GetSizes()))
	for _, size := range photo.GetSizes() {
		if size == nil {
			continue
		}
		result = append(result, PhotoSize{
			FileID:       fileID("photo", photo.GetId(), photo.GetAccessHash()),
			FileUniqueID: strconv.FormatInt(photo.GetId(), 10),
			Width:        int(size.GetW()),
			Height:       int(size.GetH()),
			FileSize:     int64(size.GetSize2()),
		})
	}
	return result
}

func stickerFromDocument(doc *mtproto.Document) *Sticker {
	if doc == nil {
		return nil
	}
	var stickerAttr, customEmojiAttr *mtproto.DocumentAttribute
	width, height := 0, 0
	animated, video := false, false
	for _, attr := range doc.GetAttributes() {
		if attr == nil {
			continue
		}
		switch attr.GetPredicateName() {
		case mtproto.Predicate_documentAttributeSticker:
			stickerAttr = attr
		case mtproto.Predicate_documentAttributeCustomEmoji:
			customEmojiAttr = attr
		case mtproto.Predicate_documentAttributeImageSize:
			width, height = int(attr.GetW()), int(attr.GetH())
		case mtproto.Predicate_documentAttributeAnimated:
			animated = true
		case mtproto.Predicate_documentAttributeVideo:
			width, height, video = int(attr.GetW()), int(attr.GetH()), true
		}
	}
	if stickerAttr == nil && customEmojiAttr == nil {
		return nil
	}
	attribute := stickerAttr
	if attribute == nil {
		attribute = customEmojiAttr
	}
	result := &Sticker{
		Type:       "regular",
		Width:      width,
		Height:     height,
		IsAnimated: animated,
		IsVideo:    video,
		Emoji:      attribute.GetAlt(),
		FileSize:   documentSize(doc),
	}
	if customEmojiAttr != nil {
		result.Type = "custom_emoji"
		result.NeedsRepainting = customEmojiAttr.GetTextColor()
		if set := customEmojiAttr.GetStickerset(); set != nil {
			result.SetName = set.GetShortName()
		}
	}
	if stickerAttr != nil {
		result.NeedsRepainting = stickerAttr.GetTextColor()
		if stickerAttr.GetMask() {
			result.Type = "mask"
			if coords := stickerAttr.GetMaskCoords(); coords != nil {
				result.MaskPosition = &MaskPosition{Point: maskPointName(coords.GetN()), XShift: coords.GetX(), YShift: coords.GetY(), Scale: coords.GetZoom()}
			}
		}
		if set := stickerAttr.GetStickerset(); set != nil {
			result.SetName = set.GetShortName()
		}
	}
	return result
}

func maskPointName(point int32) string {
	switch point {
	case 1:
		return "eyes"
	case 2:
		return "mouth"
	case 3:
		return "chin"
	default:
		return "forehead"
	}
}

func pollFromMTProto(poll *mtproto.Poll, results *mtproto.PollResults, users []*mtproto.User, chats []*mtproto.Chat) *Poll {
	if poll == nil {
		return nil
	}
	question := poll.GetQuestion()
	if question == "" {
		question = textWithEntitiesText(poll.GetQuestion_TEXTWITHENTITIES())
	}
	result := &Poll{
		ID:                    strconv.FormatInt(poll.GetId(), 10),
		Question:              question,
		QuestionEntities:      textWithEntitiesEntities(poll.GetQuestion_TEXTWITHENTITIES(), users),
		IsClosed:              poll.GetClosed(),
		IsAnonymous:           !poll.GetPublicVoters(),
		Type:                  "regular",
		AllowsMultipleAnswers: poll.GetMultipleChoice(),
		AllowsRevoting:        !poll.GetRevotingDisabled(),
		MembersOnly:           poll.GetSubscribersOnly(),
		CountryCodes:          append([]string(nil), poll.GetCountriesIso2()...),
		OpenPeriod:            int(wrapperInt32(poll.GetClosePeriod())),
		CloseDate:             int64(wrapperInt32(poll.GetCloseDate())),
	}
	if poll.GetQuiz() {
		result.Type = "quiz"
	}
	for index, answer := range poll.GetAnswers() {
		if answer == nil {
			continue
		}
		voterCount := 0
		if results != nil && index < len(results.GetResults()) && results.GetResults()[index] != nil {
			voters := results.GetResults()[index]
			voterCount = int(wrapperInt32(voters.GetVoters_FLAGINT32()))
			if voterCount == 0 {
				voterCount = int(voters.GetVoters_INT32())
			}
		}
		option := PollOption{
			PersistentID: pollPersistentID(answer.GetOption()),
			Text:         answer.GetText(),
			TextEntities: textWithEntitiesEntities(answer.GetText_TEXTWITHENTITIES(), users),
			Media:        pollMediaFromMTProto(answer.GetMedia_FLAGMESSAGEMEDIA(), users, chats),
			VoterCount:   voterCount,
			AdditionDate: int64(wrapperInt32(answer.GetDate())),
		}
		if addedBy := answer.GetAddedBy(); addedBy != nil {
			if addedBy.GetUserId() != 0 {
				option.AddedByUser = userByID(addedBy.GetUserId(), users)
			} else {
				option.AddedByChat = ChatFromPeer(addedBy, users, chats)
			}
		}
		result.Options = append(result.Options, option)
	}
	if results != nil {
		result.TotalVoterCount = int(wrapperInt32(results.GetTotalVoters()))
		if solution := results.GetSolution(); solution != nil {
			result.Explanation = solution.GetValue()
		}
		result.ExplanationEntities = messageEntities(results.GetSolutionEntities(), users)
		result.ExplanationMedia = pollMediaFromMTProto(results.GetSolutionMedia(), users, chats)
		for index, answer := range results.GetResults() {
			if answer == nil || !answer.GetCorrect() {
				continue
			}
			result.CorrectOptionIDs = append(result.CorrectOptionIDs, index)
		}
		if len(result.CorrectOptionIDs) == 1 {
			result.CorrectOptionID = result.CorrectOptionIDs[0]
		}
	}
	return result
}

func pollPersistentID(option []byte) string {
	if len(option) == 0 {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(option)
}

func pollMediaFromMTProto(media *mtproto.MessageMedia, users []*mtproto.User, chats []*mtproto.Chat) *PollMedia {
	if media == nil {
		return nil
	}
	if media.GetPredicateName() == mtproto.Predicate_messageMediaWebPage {
		if page := media.GetWebpage(); page != nil {
			url := stringValue(page.GetUrl_FLAGSTRING())
			if url == "" {
				url = page.GetUrl_STRING()
			}
			if url != "" {
				return &PollMedia{Link: &Link{URL: url}}
			}
		}
	}
	converted := &Message{}
	applyMedia(converted, media, users, chats)
	result := &PollMedia{
		Animation: converted.Animation,
		Audio:     converted.Audio,
		Document:  converted.Document,
		LivePhoto: converted.LivePhoto,
		Location:  converted.Location,
		Photo:     converted.Photo,
		Sticker:   converted.Sticker,
		Venue:     converted.Venue,
		Video:     converted.Video,
	}
	if result.Animation == nil && result.Audio == nil && result.Document == nil && result.LivePhoto == nil && result.Location == nil && len(result.Photo) == 0 && result.Sticker == nil && result.Venue == nil && result.Video == nil {
		return nil
	}
	return result
}

func documentSize(doc *mtproto.Document) int64 {
	if doc == nil {
		return 0
	}
	if doc.GetSize2() != 0 {
		return doc.GetSize2()
	}
	if doc.GetSize2_INT64() != 0 {
		return doc.GetSize2_INT64()
	}
	return int64(doc.GetSize2_INT32())
}

func documentAttributes(doc *mtproto.Document) (name string, duration, width, height int, performer, title string, voice, animated, video, roundVideo bool) {
	for _, attr := range doc.GetAttributes() {
		if attr == nil {
			continue
		}
		switch attr.GetPredicateName() {
		case mtproto.Predicate_documentAttributeFilename:
			name = attr.GetFileName()
		case mtproto.Predicate_documentAttributeAudio:
			duration = int(attr.GetDuration_INT32())
			performer, title, voice = stringValue(attr.GetPerformer()), stringValue(attr.GetTitle()), attr.GetVoice()
		case mtproto.Predicate_documentAttributeVideo:
			duration = int(attr.GetDuration_INT32())
			width, height, video, roundVideo = int(attr.GetW()), int(attr.GetH()), true, attr.GetRoundMessage()
		case mtproto.Predicate_documentAttributeAnimated:
			animated = true
		}
	}
	return
}

func replyMarkupFromMTProto(markup *mtproto.ReplyMarkup) any {
	if markup == nil {
		return nil
	}
	rows := make([][]map[string]any, 0, len(markup.GetRows()))
	for _, row := range markup.GetRows() {
		if row == nil {
			continue
		}
		buttons := make([]map[string]any, 0, len(row.GetButtons()))
		for _, button := range row.GetButtons() {
			if button == nil {
				continue
			}
			item := map[string]any{"text": button.GetText()}
			switch button.GetPredicateName() {
			case mtproto.Predicate_keyboardButtonUrl, mtproto.Predicate_keyboardButtonUrlAuth:
				item["url"] = button.GetUrl()
			case mtproto.Predicate_keyboardButtonCallback:
				item["callback_data"] = string(button.GetData())
			case mtproto.Predicate_keyboardButtonSwitchInline:
				item["switch_inline_query"] = button.GetQuery()
			case mtproto.Predicate_keyboardButtonWebView, mtproto.Predicate_keyboardButtonSimpleWebView:
				item["web_app"] = map[string]string{"url": button.GetUrl()}
			case mtproto.Predicate_keyboardButtonRequestPhone:
				item["request_contact"] = true
			case mtproto.Predicate_keyboardButtonRequestGeoLocation:
				item["request_location"] = true
			case mtproto.Predicate_keyboardButtonCopy:
				item["copy_text"] = map[string]string{"text": button.GetCopyText()}
			}
			buttons = append(buttons, item)
		}
		rows = append(rows, buttons)
	}
	switch markup.GetPredicateName() {
	case mtproto.Predicate_replyInlineMarkup:
		return map[string]any{"inline_keyboard": rows}
	default:
		// Telegram's Message.reply_markup field only contains inline keyboards.
		// Reply keyboards, ForceReply, and remove_keyboard affect the client but
		// are not serialized in the Message returned by sendMessage.
		return nil
	}
}
