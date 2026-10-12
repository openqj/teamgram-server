// Copyright 2026 Teamgram Authors
// All rights reserved.

package botapi

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Response is the envelope used by the Telegram Bot API.
type Response struct {
	OK          bool   `json:"ok"`
	Result      any    `json:"result,omitempty"`
	ErrorCode   int    `json:"error_code,omitempty"`
	Description string `json:"description,omitempty"`
}

type User struct {
	ID                      int64  `json:"id"`
	IsBot                   bool   `json:"is_bot"`
	FirstName               string `json:"first_name"`
	LastName                string `json:"last_name,omitempty"`
	Username                string `json:"username,omitempty"`
	LanguageCode            string `json:"language_code,omitempty"`
	IsPremium               bool   `json:"is_premium,omitempty"`
	AddedToAttachmentMenu   bool   `json:"added_to_attachment_menu,omitempty"`
	CanJoinGroups           bool   `json:"can_join_groups,omitempty"`
	CanReadAllGroupMessages bool   `json:"can_read_all_group_messages,omitempty"`
	SupportsInlineQueries   bool   `json:"supports_inline_queries,omitempty"`
	CanConnectToBusiness    bool   `json:"can_connect_to_business,omitempty"`
	HasMainWebApp           bool   `json:"has_main_web_app,omitempty"`
}

type Chat struct {
	ID                    int64    `json:"id"`
	Type                  string   `json:"type"`
	Title                 string   `json:"title,omitempty"`
	Username              string   `json:"username,omitempty"`
	FirstName             string   `json:"first_name,omitempty"`
	LastName              string   `json:"last_name,omitempty"`
	IsForum               bool     `json:"is_forum,omitempty"`
	Photo                 any      `json:"photo,omitempty"`
	ActiveUsernames       []string `json:"active_usernames,omitempty"`
	HasPrivateForwards    bool     `json:"has_private_forwards,omitempty"`
	HasRestrictedVoice    bool     `json:"has_restricted_voice_and_video_messages,omitempty"`
	JoinToSendMessages    bool     `json:"join_to_send_messages,omitempty"`
	JoinByRequest         bool     `json:"join_by_request,omitempty"`
	Description           string   `json:"description,omitempty"`
	InviteLink            string   `json:"invite_link,omitempty"`
	PinnedMessage         *Message `json:"pinned_message,omitempty"`
	Permissions           any      `json:"permissions,omitempty"`
	SlowModeDelay         int      `json:"slow_mode_delay,omitempty"`
	MessageAutoDeleteTime int      `json:"message_auto_delete_timer_changed,omitempty"`
	HasHiddenMembers      bool     `json:"has_hidden_members,omitempty"`
	HasAggressiveAntiSpam bool     `json:"has_aggressive_anti_spam_enabled,omitempty"`
	EmojiStatusCustomID   string   `json:"emoji_status_custom_emoji_id,omitempty"`
	AccentColorID         int      `json:"accent_color_id,omitempty"`
	BackgroundCustomEmoji string   `json:"background_custom_emoji_id,omitempty"`
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
	Text       string `json:"text"`
	VoterCount int    `json:"voter_count"`
}

type Poll struct {
	ID                    string       `json:"id"`
	Question              string       `json:"question"`
	Options               []PollOption `json:"options"`
	TotalVoterCount       int          `json:"total_voter_count"`
	IsClosed              bool         `json:"is_closed"`
	IsAnonymous           bool         `json:"is_anonymous"`
	Type                  string       `json:"type"`
	AllowsMultipleAnswers bool         `json:"allows_multiple_answers"`
	CorrectOptionID       int          `json:"correct_option_id,omitempty"`
	OpenPeriod            int          `json:"open_period,omitempty"`
	CloseDate             int64        `json:"close_date,omitempty"`
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
	MessageID            int                  `json:"message_id"`
	MessageThreadID      int                  `json:"message_thread_id,omitempty"`
	From                 *User                `json:"from,omitempty"`
	SenderChat           *Chat                `json:"sender_chat,omitempty"`
	Date                 int64                `json:"date"`
	Chat                 *Chat                `json:"chat"`
	ForwardFrom          *User                `json:"forward_from,omitempty"`
	ForwardFromChat      *Chat                `json:"forward_from_chat,omitempty"`
	ForwardFromMessageID int                  `json:"forward_from_message_id,omitempty"`
	ReplyToMessage       *Message             `json:"reply_to_message,omitempty"`
	Text                 string               `json:"text,omitempty"`
	Entities             []MessageEntity      `json:"entities,omitempty"`
	Caption              string               `json:"caption,omitempty"`
	CaptionEntities      []MessageEntity      `json:"caption_entities,omitempty"`
	ViaBot               *User                `json:"via_bot,omitempty"`
	EditDate             int64                `json:"edit_date,omitempty"`
	HasProtectedContent  bool                 `json:"has_protected_content,omitempty"`
	MediaGroupID         string               `json:"media_group_id,omitempty"`
	AuthorSignature      string               `json:"author_signature,omitempty"`
	IsAutomaticForward   bool                 `json:"is_automatic_forward,omitempty"`
	ConnectedWebsite     string               `json:"connected_website,omitempty"`
	ReplyMarkup          any                  `json:"reply_markup,omitempty"`
	Photo                []PhotoSize          `json:"photo,omitempty"`
	Audio                *Audio               `json:"audio,omitempty"`
	Document             *Document            `json:"document,omitempty"`
	Sticker              *Sticker             `json:"sticker,omitempty"`
	Video                *Video               `json:"video,omitempty"`
	VideoNote            *VideoNote           `json:"video_note,omitempty"`
	Animation            *Animation           `json:"animation,omitempty"`
	Voice                *Voice               `json:"voice,omitempty"`
	Location             *Location            `json:"location,omitempty"`
	Venue                *Venue               `json:"venue,omitempty"`
	Contact              *Contact             `json:"contact,omitempty"`
	Poll                 *Poll                `json:"poll,omitempty"`
	Dice                 *Dice                `json:"dice,omitempty"`
	Game                 *Game                `json:"game,omitempty"`
	Invoice              *Invoice             `json:"invoice,omitempty"`
	RichMessage          *mtproto.RichMessage `json:"rich_message,omitempty"`
}

type InlineQuery struct {
	ID       string `json:"id"`
	From     *User  `json:"from"`
	ChatType string `json:"chat_type"`
	Query    string `json:"query"`
	Offset   string `json:"offset"`
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

type Update struct {
	UpdateID           int64               `json:"update_id"`
	Message            *Message            `json:"message,omitempty"`
	EditedMessage      *Message            `json:"edited_message,omitempty"`
	ChannelPost        *Message            `json:"channel_post,omitempty"`
	EditedChannelPost  *Message            `json:"edited_channel_post,omitempty"`
	InlineQuery        *InlineQuery        `json:"inline_query,omitempty"`
	CallbackQuery      *CallbackQuery      `json:"callback_query,omitempty"`
	WebAppQuery        *WebAppQuery        `json:"web_app_query,omitempty"`
	ShippingQuery      *ShippingQuery      `json:"shipping_query,omitempty"`
	PreCheckoutQuery   *PreCheckoutQuery   `json:"pre_checkout_query,omitempty"`
	BusinessConnection *BusinessConnection `json:"business_connection,omitempty"`
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
	return &User{
		ID:                      d.GetId(),
		IsBot:                   d.GetBot() != nil,
		FirstName:               d.GetFirstName(),
		LastName:                d.GetLastName(),
		Username:                d.GetUsername(),
		IsPremium:               d.GetPremium(),
		CanJoinGroups:           d.GetBot() == nil || !d.GetBot().GetBotNochats(),
		CanReadAllGroupMessages: d.GetBot() == nil || d.GetBot().GetBotChatHistory(),
		SupportsInlineQueries:   d.GetBot() != nil,
		HasMainWebApp:           d.GetBot() != nil && d.GetBot().GetBotHasMainApp(),
	}
}

func UserFromMTProto(in *mtproto.User) *User {
	if in == nil {
		return nil
	}
	return &User{
		ID:                      in.GetId(),
		IsBot:                   in.GetBot(),
		FirstName:               stringValue(in.GetFirstName()),
		LastName:                stringValue(in.GetLastName()),
		Username:                stringValue(in.GetUsername()),
		IsPremium:               in.GetPremium(),
		CanJoinGroups:           !in.GetBotNochats(),
		CanReadAllGroupMessages: in.GetBotChatHistory(),
		SupportsInlineQueries:   in.GetBotInlinePlaceholder() != nil || in.GetBotInlineGeo(),
		HasMainWebApp:           in.GetBotHasMainApp(),
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
	return &Chat{ID: id, Type: chatType, Title: in.GetTitle(), IsForum: false}
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
		Text:                in.GetMessage(),
		Entities:            messageEntities(in.GetEntities(), users),
		EditDate:            int64(wrapperInt32(in.GetEditDate())),
		MediaGroupID:        fmt.Sprint(wrapperInt64(in.GetGroupedId())),
		HasProtectedContent: in.GetNoforwards(),
		AuthorSignature:     stringValue(in.GetPostAuthor()),
	}
	if message.MediaGroupID == "0" {
		message.MediaGroupID = ""
	}
	if in.GetViaBotId() != nil {
		message.ViaBot = userByID(in.GetViaBotId().GetValue(), users)
	}
	if in.GetFwdFrom() != nil {
		fwd := in.GetFwdFrom()
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
	applyMedia(message, in.GetMedia())
	return message
}

func UpdateFromMTProto(in *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat) (*Update, bool) {
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
	case mtproto.Predicate_updateBotInlineQuery:
		result.InlineQuery = &InlineQuery{
			ID:       fmt.Sprintf("%d", in.GetQueryId_INT64()),
			From:     userByID(in.GetUserId(), users),
			ChatType: inlineChatType(in.GetPeerType()),
			Query:    in.GetQuery(),
			Offset:   in.GetOffset(),
		}
	case mtproto.Predicate_updateBotCallbackQuery:
		result.CallbackQuery = &CallbackQuery{
			ID:            fmt.Sprintf("%d", in.GetQueryId_INT64()),
			From:          userByID(in.GetUserId(), users),
			Message:       callbackMessage(in, users, chats),
			ChatInstance:  fmt.Sprintf("%d", in.GetChatInstance()),
			Data:          string(in.GetData_FLAGBYTES()),
			GameShortName: stringValue(in.GetGameShortName()),
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
	default:
		return nil, false
	}
	return result, true
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

// MessagesFromUpdates preserves the order returned by messages.send* calls.
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
		case "TextUrl":
			return "text_link"
		case "MentionName":
			return "text_mention"
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
	chat := ChatFromPeer(update.GetPeer_PEER(), users, chats)
	if chat == nil {
		return nil
	}
	return &Message{MessageID: int(update.GetMsgId_INT32()), Date: 0, Chat: chat}
}

func fileID(kind string, id, accessHash int64) string {
	return fmt.Sprintf("%s_%d_%d", kind, id, accessHash)
}

func applyMedia(message *Message, media *mtproto.MessageMedia) {
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
			message.Venue = &Venue{
				Location:       &Location{Longitude: geo.GetLong(), Latitude: geo.GetLat(), HorizontalAccuracy: float64(wrapperInt32(geo.GetAccuracyRadius()))},
				Title:          media.GetTitle(),
				Address:        media.GetAddress(),
				FoursquareID:   media.GetVenueId(),
				FoursquareType: media.GetVenueType(),
			}
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
		message.Poll = pollFromMTProto(media.GetPoll(), media.GetResults())
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
	}
	if customEmojiAttr != nil {
		result.Type = "custom_emoji"
		if set := customEmojiAttr.GetStickerset(); set != nil {
			result.SetName = set.GetShortName()
		}
	}
	if stickerAttr != nil {
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

func pollFromMTProto(poll *mtproto.Poll, results *mtproto.PollResults) *Poll {
	if poll == nil {
		return nil
	}
	result := &Poll{
		ID:                    strconv.FormatInt(poll.GetId(), 10),
		Question:              poll.GetQuestion(),
		IsClosed:              poll.GetClosed(),
		IsAnonymous:           !poll.GetPublicVoters(),
		Type:                  "regular",
		AllowsMultipleAnswers: poll.GetMultipleChoice(),
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
		result.Options = append(result.Options, PollOption{Text: answer.GetText(), VoterCount: voterCount})
	}
	if results != nil {
		result.TotalVoterCount = int(wrapperInt32(results.GetTotalVoters()))
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
	case mtproto.Predicate_replyKeyboardForceReply:
		result := map[string]any{"force_reply": true}
		if placeholder := stringValue(markup.GetPlaceholder()); placeholder != "" {
			result["input_field_placeholder"] = placeholder
		}
		return result
	case mtproto.Predicate_replyKeyboardHide:
		return map[string]any{"remove_keyboard": true, "selective": markup.GetSelective()}
	default:
		return map[string]any{
			"keyboard":          rows,
			"resize_keyboard":   markup.GetResize(),
			"one_time_keyboard": markup.GetSingleUse(),
			"selective":         markup.GetSelective(),
			"is_persistent":     markup.GetPersistent(),
		}
	}
}
