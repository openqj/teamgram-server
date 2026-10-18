// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var (
	errBotAPIRichMessageInvalid       = errors.New("rich_message is invalid")
	errBotAPIRichBlocksUnsupported    = errors.New("rich_message.blocks contains an unsupported block")
	errBotAPIRichMediaUnsupported     = errors.New("rich_message.media is not supported")
	errBotAPIRichDraftControlsMissing = errors.New("rich draft stop controls are not supported")
)

// botAPIInputRichMessage converts the public Bot API input union to the
// Layer-229 InputRichMessage union. Text blocks are converted to PageBlock so
// the BFF can persist and return the same rich message in updates.
func botAPIInputRichMessage(request *botAPIRequest) (*mtproto.InputRichMessage, error) {
	if request == nil {
		return nil, errBotAPIRichMessageInvalid
	}
	var fields map[string]json.RawMessage
	if err := request.json("rich_message", &fields); err != nil || len(fields) == 0 {
		return nil, errBotAPIRichMessageInvalid
	}
	var (
		html, markdown string
		blocks         []*mtproto.PageBlock
		media          = &botAPIRichMediaState{}
		hasHTML        = richFieldPresent(fields, "html")
		hasMarkdown    = richFieldPresent(fields, "markdown")
		hasBlocks      = richFieldPresent(fields, "blocks")
	)
	if hasHTML {
		if err := json.Unmarshal(fields["html"], &html); err != nil {
			return nil, errBotAPIRichMessageInvalid
		}
	}
	if hasMarkdown {
		if err := json.Unmarshal(fields["markdown"], &markdown); err != nil {
			return nil, errBotAPIRichMessageInvalid
		}
	}
	if hasBlocks {
		var err error
		blocks, media, err = botAPIRichBlocksWithState(fields["blocks"])
		if err != nil {
			return nil, err
		}
	}
	fieldCount := 0
	if hasHTML {
		fieldCount++
	}
	if hasMarkdown {
		fieldCount++
	}
	if hasBlocks {
		fieldCount++
	}
	if fieldCount != 1 {
		return nil, errBotAPIRichMessageInvalid
	}
	if richFieldPresent(fields, "media") {
		return nil, errBotAPIRichMediaUnsupported
	}
	rtl, err := richBoolField(fields, "is_rtl")
	if err != nil {
		return nil, errBotAPIRichMessageInvalid
	}
	noautolink, err := richBoolField(fields, "skip_entity_detection")
	if err != nil {
		return nil, errBotAPIRichMessageInvalid
	}
	value := &mtproto.InputRichMessage{
		Rtl:        rtl,
		Noautolink: noautolink,
		Blocks:     blocks,
		Photos:     media.photos,
		Documents:  media.documents,
	}
	switch {
	case hasHTML:
		value.Html = html
		return mtproto.MakeTLInputRichMessageHTML(value).To_InputRichMessage(), nil
	case hasMarkdown:
		value.Markdown = markdown
		return mtproto.MakeTLInputRichMessageMarkdown(value).To_InputRichMessage(), nil
	default:
		return mtproto.MakeTLInputRichMessage(value).To_InputRichMessage(), nil
	}
}

func richFieldPresent(fields map[string]json.RawMessage, name string) bool {
	raw, ok := fields[name]
	return ok && len(raw) > 0 && string(raw) != "null"
}

func richBoolField(fields map[string]json.RawMessage, name string) (bool, error) {
	if !richFieldPresent(fields, name) {
		return false, nil
	}
	var value bool
	if err := json.Unmarshal(fields[name], &value); err != nil {
		return false, err
	}
	return value, nil
}

type botAPIRichBlockInput struct {
	Type       string            `json:"type"`
	Text       json.RawMessage   `json:"text"`
	Credit     json.RawMessage   `json:"credit"`
	Summary    json.RawMessage   `json:"summary"`
	Blocks     []json.RawMessage `json:"blocks"`
	Expression string            `json:"expression"`
	Language   string            `json:"language"`
	Name       string            `json:"name"`
	Size       int               `json:"size"`
	IsOpen     bool              `json:"is_open"`
	Photo      json.RawMessage   `json:"photo"`
	Video      json.RawMessage   `json:"video"`
	Audio      json.RawMessage   `json:"audio"`
	Caption    json.RawMessage   `json:"caption"`
	IsSpoiler  bool              `json:"is_spoiler"`
	IsAutoplay bool              `json:"is_autoplay"`
	IsLoop     bool              `json:"is_loop"`
}

func botAPIRichBlocks(raw json.RawMessage) ([]*mtproto.PageBlock, error) {
	blocks, _, err := botAPIRichBlocksWithState(raw)
	return blocks, err
}

type botAPIRichMediaState struct {
	photos    []*mtproto.InputPhoto
	documents []*mtproto.InputDocument
}

func botAPIRichBlocksWithState(raw json.RawMessage) ([]*mtproto.PageBlock, *botAPIRichMediaState, error) {
	var inputs []json.RawMessage
	if err := json.Unmarshal(raw, &inputs); err != nil || len(inputs) == 0 {
		return nil, nil, errBotAPIRichMessageInvalid
	}
	media := &botAPIRichMediaState{}
	blocks := make([]*mtproto.PageBlock, 0, len(inputs))
	for _, rawBlock := range inputs {
		block, err := botAPIRichBlockWithState(rawBlock, media)
		if err != nil {
			return nil, nil, err
		}
		blocks = append(blocks, block)
	}
	return blocks, media, nil
}

func botAPIRichBlock(raw json.RawMessage) (*mtproto.PageBlock, error) {
	return botAPIRichBlockWithState(raw, &botAPIRichMediaState{})
}

func botAPIRichBlockWithState(raw json.RawMessage, media *botAPIRichMediaState) (*mtproto.PageBlock, error) {
	var input botAPIRichBlockInput
	if err := json.Unmarshal(raw, &input); err != nil || strings.TrimSpace(input.Type) == "" {
		return nil, errBotAPIRichMessageInvalid
	}
	if media == nil {
		media = &botAPIRichMediaState{}
	}
	text, err := botAPIRichText(input.Text)
	requiresText := false
	switch strings.ToLower(input.Type) {
	case "paragraph", "heading", "pre", "footer", "pullquote", "expandable_blockquote", "thinking":
		requiresText = true
	}
	if err != nil && requiresText {
		return nil, err
	}
	base := &mtproto.PageBlock{Text: text, Language: input.Language, Name: input.Name, Open: input.IsOpen}
	switch strings.ToLower(input.Type) {
	case "paragraph":
		return mtproto.MakeTLPageBlockParagraph(base).To_PageBlock(), nil
	case "heading":
		if input.Size < 1 || input.Size > 6 {
			return nil, errBotAPIRichMessageInvalid
		}
		switch input.Size {
		case 1:
			return mtproto.MakeTLPageBlockHeading1(base).To_PageBlock(), nil
		case 2:
			return mtproto.MakeTLPageBlockHeading2(base).To_PageBlock(), nil
		case 3:
			return mtproto.MakeTLPageBlockHeading3(base).To_PageBlock(), nil
		case 4:
			return mtproto.MakeTLPageBlockHeading4(base).To_PageBlock(), nil
		case 5:
			return mtproto.MakeTLPageBlockHeading5(base).To_PageBlock(), nil
		default:
			return mtproto.MakeTLPageBlockHeading6(base).To_PageBlock(), nil
		}
	case "pre":
		return mtproto.MakeTLPageBlockPreformatted(base).To_PageBlock(), nil
	case "footer":
		return mtproto.MakeTLPageBlockFooter(base).To_PageBlock(), nil
	case "divider":
		return mtproto.MakeTLPageBlockDivider(base).To_PageBlock(), nil
	case "math", "mathematical_expression":
		if strings.TrimSpace(input.Expression) == "" {
			return nil, errBotAPIRichMessageInvalid
		}
		base.Text = mtproto.MakeTLTextMath(&mtproto.RichText{Source: input.Expression}).To_RichText()
		return mtproto.MakeTLPageBlockMath(base).To_PageBlock(), nil
	case "anchor":
		if strings.TrimSpace(input.Name) == "" {
			return nil, errBotAPIRichMessageInvalid
		}
		return mtproto.MakeTLPageBlockAnchor(base).To_PageBlock(), nil
	case "blockquote":
		children, childErr := botAPIRichNestedBlocksWithState(input.Blocks, media)
		if childErr != nil {
			return nil, childErr
		}
		base.Blocks = children
		base.Author_RICHTEXT, _ = botAPIRichText(input.Credit)
		return mtproto.MakeTLPageBlockBlockquoteBlocks(base).To_PageBlock(), nil
	case "expandable_blockquote":
		base.Author_RICHTEXT, _ = botAPIRichText(input.Credit)
		return mtproto.MakeTLPageBlockBlockquote(base).To_PageBlock(), nil
	case "pullquote":
		base.Author_RICHTEXT, _ = botAPIRichText(input.Credit)
		return mtproto.MakeTLPageBlockPullquote(base).To_PageBlock(), nil
	case "photo":
		photoID, fileErr := botAPIRichMediaFile(input.Photo, "photo", media)
		if fileErr != nil {
			return nil, fileErr
		}
		caption, captionErr := botAPIRichPageCaption(input.Caption)
		if captionErr != nil {
			return nil, captionErr
		}
		return mtproto.MakeTLPageBlockPhoto(&mtproto.PageBlock{
			PhotoId:             photoID,
			Caption_PAGECAPTION: caption,
			Spoiler:             input.IsSpoiler,
		}).To_PageBlock(), nil
	case "video":
		videoID, fileErr := botAPIRichMediaFile(input.Video, "document", media)
		if fileErr != nil {
			return nil, fileErr
		}
		caption, captionErr := botAPIRichPageCaption(input.Caption)
		if captionErr != nil {
			return nil, captionErr
		}
		return mtproto.MakeTLPageBlockVideo(&mtproto.PageBlock{
			VideoId:             videoID,
			Caption_PAGECAPTION: caption,
			Autoplay:            input.IsAutoplay,
			Loop:                input.IsLoop,
			Spoiler:             input.IsSpoiler,
		}).To_PageBlock(), nil
	case "audio":
		audioID, fileErr := botAPIRichMediaFile(input.Audio, "document", media)
		if fileErr != nil {
			return nil, fileErr
		}
		caption, captionErr := botAPIRichPageCaption(input.Caption)
		if captionErr != nil {
			return nil, captionErr
		}
		return mtproto.MakeTLPageBlockAudio(&mtproto.PageBlock{
			AudioId:             audioID,
			Caption_PAGECAPTION: caption,
		}).To_PageBlock(), nil
	case "details":
		children, childErr := botAPIRichNestedBlocksWithState(input.Blocks, media)
		if childErr != nil {
			return nil, childErr
		}
		base.Title, _ = botAPIRichText(input.Summary)
		base.Blocks = children
		return mtproto.MakeTLPageBlockDetails(base).To_PageBlock(), nil
	case "thinking":
		return mtproto.MakeTLPageBlockThinking(base).To_PageBlock(), nil
	default:
		return nil, errBotAPIRichBlocksUnsupported
	}
}

func botAPIRichNestedBlocks(inputs []json.RawMessage) ([]*mtproto.PageBlock, error) {
	return botAPIRichNestedBlocksWithState(inputs, &botAPIRichMediaState{})
}

func botAPIRichNestedBlocksWithState(inputs []json.RawMessage, media *botAPIRichMediaState) ([]*mtproto.PageBlock, error) {
	if len(inputs) == 0 {
		return nil, errBotAPIRichMessageInvalid
	}
	if media == nil {
		media = &botAPIRichMediaState{}
	}
	blocks := make([]*mtproto.PageBlock, 0, len(inputs))
	for _, input := range inputs {
		block, err := botAPIRichBlockWithState(input, media)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func botAPIRichMediaFile(raw json.RawMessage, expectedKind string, media *botAPIRichMediaState) (int64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, errBotAPIRichMessageInvalid
	}
	var fileID string
	if err := json.Unmarshal(raw, &fileID); err != nil || strings.TrimSpace(fileID) == "" {
		return 0, errBotAPIRichMessageInvalid
	}
	ref, err := parseBotAPIFileID(fileID)
	if err != nil || ref.kind != expectedKind {
		return 0, errBotAPIRichMessageInvalid
	}
	if media == nil {
		return 0, errBotAPIRichMessageInvalid
	}
	switch ref.kind {
	case "photo":
		media.photos = append(media.photos, mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{
			Id:         ref.id,
			AccessHash: ref.accessHash,
		}).To_InputPhoto())
	case "document":
		media.documents = append(media.documents, mtproto.MakeTLInputDocument(&mtproto.InputDocument{
			Id:         ref.id,
			AccessHash: ref.accessHash,
		}).To_InputDocument())
	default:
		return 0, errBotAPIRichMessageInvalid
	}
	return ref.id, nil
}

func botAPIRichPageCaption(raw json.RawMessage) (*mtproto.PageCaption, error) {
	empty := func() *mtproto.RichText {
		return mtproto.MakeTLTextPlain(&mtproto.RichText{Text_STRING: ""}).To_RichText()
	}
	if len(raw) == 0 || string(raw) == "null" {
		return mtproto.MakeTLPageCaption(&mtproto.PageCaption{Text: empty(), Credit: empty()}).To_PageCaption(), nil
	}
	var plain string
	if json.Unmarshal(raw, &plain) == nil {
		return mtproto.MakeTLPageCaption(&mtproto.PageCaption{
			Text:   mtproto.MakeTLTextPlain(&mtproto.RichText{Text_STRING: plain}).To_RichText(),
			Credit: empty(),
		}).To_PageCaption(), nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, errBotAPIRichMessageInvalid
	}
	text, credit := empty(), empty()
	if richFieldPresent(fields, "text") {
		var err error
		text, err = botAPIRichText(fields["text"])
		if err != nil {
			return nil, err
		}
	}
	if richFieldPresent(fields, "credit") {
		var err error
		credit, err = botAPIRichText(fields["credit"])
		if err != nil {
			return nil, err
		}
	}
	return mtproto.MakeTLPageCaption(&mtproto.PageCaption{Text: text, Credit: credit}).To_PageCaption(), nil
}

func botAPIRichText(raw json.RawMessage) (*mtproto.RichText, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, errBotAPIRichMessageInvalid
	}
	var plain string
	if json.Unmarshal(raw, &plain) == nil {
		return mtproto.MakeTLTextPlain(&mtproto.RichText{Text_STRING: plain}).To_RichText(), nil
	}
	var parts []json.RawMessage
	if json.Unmarshal(raw, &parts) == nil {
		if len(parts) == 0 {
			return nil, errBotAPIRichMessageInvalid
		}
		texts := make([]*mtproto.RichText, 0, len(parts))
		for _, part := range parts {
			text, err := botAPIRichText(part)
			if err != nil {
				return nil, err
			}
			texts = append(texts, text)
		}
		return mtproto.MakeTLTextConcat(&mtproto.RichText{Texts: texts}).To_RichText(), nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, errBotAPIRichMessageInvalid
	}
	var input struct {
		Type            string          `json:"type"`
		Text            json.RawMessage `json:"text"`
		URL             string          `json:"url"`
		EmailAddress    string          `json:"email_address"`
		PhoneNumber     string          `json:"phone_number"`
		Username        string          `json:"username"`
		CustomEmojiID   string          `json:"custom_emoji_id"`
		AlternativeText string          `json:"alternative_text"`
		Expression      string          `json:"expression"`
		BankCardNumber  string          `json:"bank_card_number"`
		UnixTime        int64           `json:"unix_time"`
		DateTimeFormat  string          `json:"date_time_format"`
	}
	if err := json.Unmarshal(raw, &input); err != nil || strings.TrimSpace(input.Type) == "" {
		return nil, errBotAPIRichMessageInvalid
	}
	nested, nestedErr := botAPIRichText(input.Text)
	if nestedErr != nil && input.Type != "custom_emoji" && input.Type != "mathematical_expression" {
		return nil, nestedErr
	}
	value := &mtproto.RichText{Text_RICHTEXT: nested, Url: input.URL, Email: input.EmailAddress, Phone: input.PhoneNumber, Source: input.Expression, Alt: input.AlternativeText}
	switch strings.ToLower(input.Type) {
	case "bold":
		return mtproto.MakeTLTextBold(value).To_RichText(), nil
	case "italic":
		return mtproto.MakeTLTextItalic(value).To_RichText(), nil
	case "underline":
		return mtproto.MakeTLTextUnderline(value).To_RichText(), nil
	case "strikethrough":
		return mtproto.MakeTLTextStrike(value).To_RichText(), nil
	case "spoiler":
		return mtproto.MakeTLTextSpoiler(value).To_RichText(), nil
	case "code":
		return mtproto.MakeTLTextFixed(value).To_RichText(), nil
	case "url":
		return mtproto.MakeTLTextUrl(value).To_RichText(), nil
	case "email_address":
		return mtproto.MakeTLTextEmail(value).To_RichText(), nil
	case "phone_number":
		return mtproto.MakeTLTextPhone(value).To_RichText(), nil
	case "mention":
		value.Text_STRING = input.Username
		return mtproto.MakeTLTextMention(value).To_RichText(), nil
	case "hashtag":
		return mtproto.MakeTLTextHashtag(value).To_RichText(), nil
	case "cashtag":
		return mtproto.MakeTLTextCashtag(value).To_RichText(), nil
	case "bot_command":
		return mtproto.MakeTLTextBotCommand(value).To_RichText(), nil
	case "custom_emoji":
		id, err := strconv.ParseInt(input.CustomEmojiID, 10, 64)
		if err != nil || id <= 0 {
			return nil, errBotAPIRichMessageInvalid
		}
		value.DocumentId = id
		return mtproto.MakeTLTextCustomEmoji(value).To_RichText(), nil
	case "mathematical_expression":
		if strings.TrimSpace(input.Expression) == "" {
			return nil, errBotAPIRichMessageInvalid
		}
		value.Source = input.Expression
		return mtproto.MakeTLTextMath(value).To_RichText(), nil
	case "bank_card_number":
		value.Text_STRING = input.BankCardNumber
		return mtproto.MakeTLTextBankCard(value).To_RichText(), nil
	default:
		return nil, errBotAPIRichBlocksUnsupported
	}
}

func sendRichMessage(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	if parameter := botAPIUnsupportedMessageParameterExcept(request, "direct_messages_topic_id", "ephemeral_message_parameters", "suggested_post_parameters"); parameter != "" {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: " + parameter + " is not supported by the Layer 229 adapter"
	}
	ephemeralParameters, ephemeralErr := botAPIEphemeralParametersValue(request)
	if ephemeralErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + ephemeralErr.Error()
	}
	ephemeralRequested := ephemeralParameters != nil
	effect, effectErr := botAPIMessageEffect(request)
	if effectErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + effectErr.Error()
	}
	suggestedPost, suggestedPostErr := botAPISuggestedPost(request)
	if suggestedPostErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + suggestedPostErr.Error()
	}
	if ephemeralRequested && suggestedPost != nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: ephemeral messages with suggested_post_parameters are not supported"
	}
	richMessage, err := botAPIInputRichMessage(request)
	if err != nil {
		if errors.Is(err, errBotAPIRichBlocksUnsupported) || errors.Is(err, errBotAPIRichMediaUnsupported) {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: " + err.Error()
		}
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	botID := bot.GetUser().GetId()
	scope, business, scopeErr := resolveBotAPISendScope(ctx, r.Context(), botID, request)
	if scopeErr != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(scopeErr)
	}
	senderID := botID
	callCtx := botRPCContext(r.Context(), botID)
	if business {
		senderID = scope.OwnerID
		callCtx = businessBotCallContext(r.Context(), botID, scope)
	}
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, senderID, request.string("chat_id"))
	if err != nil || peerID == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	if suggestedPost != nil && peerType == mtproto.PEER_CHANNEL {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: suggested_post_parameters for channel sends is not supported by the Layer 229 adapter"
	}
	if ephemeralRequested && business {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: ephemeral messages on behalf of business accounts are not supported"
	}
	markup, err := botAPIReplyMarkup(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_markup is invalid"
	}
	replyTo, err := botAPIReplyTo(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_parameters is invalid"
	}
	topMsgID, err := botAPIMessageTopID(request)
	if err != nil {
		status, code, description := botAPIMessageTopIDError(err)
		return nil, status, code, description
	}
	if ephemeralRequested {
		return sendBotAPIEphemeral(ctx, r, botID, request, peerType, peerID, "", nil, nil, richMessage, markup, replyTo)
	}
	if ctx == nil || ctx.MessagesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: message service is not configured"
	}
	updates, err := ctx.MessagesClient.MessagesSendMessage(callCtx, &mtproto.TLMessagesSendMessage{
		Silent:             requestBool(request, "disable_notification"),
		Noforwards:         requestBool(request, "protect_content"),
		AllowPaidFloodskip: requestBool(request, "allow_paid_broadcast"),
		Effect:             effect,
		Peer:               botAPIInputPeer(senderID, peerType, peerID),
		ReplyTo:            replyTo,
		TopMsgId:           topMsgID,
		RandomId:           randomMessageID(),
		ReplyMarkup:        markup,
		RichMessage:        richMessage,
		SuggestedPost:      suggestedPost,
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	result := botapi.FirstMessageFromUpdates(updates)
	if result == nil {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: message was not returned"
	}
	return result, http.StatusOK, 0, ""
}

func sendRichMessageDraft(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	if parameter := botAPIUnsupportedMessageParameter(request); parameter != "" {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: " + parameter + " is not supported by the Layer 229 adapter"
	}
	canStop, err := request.bool("can_stop")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: can_stop is invalid"
	}
	keepOnStop, err := request.bool("keep_on_stop")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: keep_on_stop is invalid"
	}
	if canStop || keepOnStop {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: " + errBotAPIRichDraftControlsMissing.Error()
	}
	if ctx == nil || ctx.EphemeralClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: ephemeral message service is not configured"
	}
	richMessage, err := botAPIInputRichMessage(request)
	if err != nil {
		if errors.Is(err, errBotAPIRichBlocksUnsupported) || errors.Is(err, errBotAPIRichMediaUnsupported) {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: " + err.Error()
		}
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	botID := bot.GetUser().GetId()
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID <= 0 || peerType != mtproto.PEER_USER {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id must be a private user chat"
	}
	receiver, err := botAPIProfilePhotoInputUser(ctx, r.Context(), botID, peerID)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	draftID, err := request.int64("draft_id")
	if err != nil || draftID == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: draft_id is invalid"
	}
	markup, err := botAPIReplyMarkup(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_markup is invalid"
	}
	ok, err := ctx.EphemeralClient.EphemeralSendMessage(botRPCContext(r.Context(), botID), &mtproto.TLEphemeralSendMessage{
		Peer:        botAPIInputPeer(botID, peerType, peerID),
		ReceiverId:  receiver,
		QueryId:     wrapperspb.Int64(draftID),
		ReplyMarkup: markup,
		RichMessage: richMessage,
		RandomId:    randomMessageID(),
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if ok == nil {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: draft was not accepted"
	}
	return true, http.StatusOK, 0, ""
}
