// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
)

var errBotAPIPollOptionMediaUnsupported = errors.New("poll option media is not supported by the Layer 229 adapter")

type botAPIPollOption struct {
	Text          string                  `json:"text"`
	TextParseMode string                  `json:"text_parse_mode,omitempty"`
	TextEntities  *[]botapi.MessageEntity `json:"text_entities,omitempty"`
	Media         json.RawMessage         `json:"media,omitempty"`
}

func botAPIPollUnsupportedParameter(request *botAPIRequest) string {
	if request == nil {
		return ""
	}
	for _, name := range []string{"description", "description_parse_mode", "description_entities", "media", "explanation_media"} {
		if !request.has(name) {
			continue
		}
		raw := strings.TrimSpace(string(request.raw(name)))
		if raw != "" && raw != "null" && raw != `""` && raw != "[]" {
			return name
		}
	}
	return ""
}

func botAPIPollOptions(request *botAPIRequest) ([]*mtproto.PollAnswer, error) {
	var rawOptions []json.RawMessage
	if request == nil || request.json("options", &rawOptions) != nil {
		return nil, errors.New("options must contain between 1 and 12 answer options")
	}
	if len(rawOptions) < 1 || len(rawOptions) > 12 {
		return nil, errors.New("options must contain between 1 and 12 answer options")
	}
	answers := make([]*mtproto.PollAnswer, 0, len(rawOptions))
	seen := make(map[string]struct{}, len(rawOptions))
	for index, raw := range rawOptions {
		rawText := strings.TrimSpace(string(raw))
		option := botAPIPollOption{}
		if strings.HasPrefix(rawText, `"`) {
			if err := json.Unmarshal(raw, &option.Text); err != nil {
				return nil, errors.New("poll option is invalid")
			}
		} else if err := json.Unmarshal(raw, &option); err != nil {
			return nil, errors.New("poll option is invalid")
		}
		if strings.TrimSpace(option.Text) == "" || utf8.RuneCountInString(option.Text) > 100 {
			return nil, errors.New("poll option is invalid")
		}
		if _, exists := seen[option.Text]; exists {
			return nil, errors.New("poll option is duplicated")
		}
		seen[option.Text] = struct{}{}
		if rawMedia := strings.TrimSpace(string(option.Media)); rawMedia != "" && rawMedia != "null" && rawMedia != `""` {
			return nil, errBotAPIPollOptionMediaUnsupported
		}
		var entities []botapi.MessageEntity
		if option.TextEntities != nil {
			entities = *option.TextEntities
		} else if strings.TrimSpace(option.TextParseMode) != "" {
			var err error
			option.Text, entities, err = botAPIParseFormattedText(option.Text, option.TextParseMode)
			if err != nil {
				return nil, errors.New("poll option text_parse_mode is invalid")
			}
			if strings.TrimSpace(option.Text) == "" {
				return nil, errors.New("poll option is invalid")
			}
		}
		answers = append(answers, mtproto.MakePollAnswerWithText(option.Text, []byte{byte(index)}))
		answers[len(answers)-1].Text_TEXTWITHENTITIES = mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
			Text:     option.Text,
			Entities: botapi.MessageEntitiesToMTProto(entities),
		}).To_TextWithEntities()
	}
	return answers, nil
}

func botAPIPollCountryCodes(request *botAPIRequest) ([]string, error) {
	if request == nil || !request.has("country_codes") || strings.TrimSpace(string(request.raw("country_codes"))) == "null" {
		return nil, nil
	}
	var codes []string
	if err := request.json("country_codes", &codes); err != nil || len(codes) > 12 {
		return nil, errors.New("country_codes is invalid")
	}
	result := make([]string, len(codes))
	for i, code := range codes {
		code = strings.ToUpper(strings.TrimSpace(code))
		if len(code) != 2 || code[0] < 'A' || code[0] > 'Z' || code[1] < 'A' || code[1] > 'Z' {
			return nil, errors.New("country_codes is invalid")
		}
		result[i] = code
	}
	return result, nil
}

// stopPoll uses messages.editMessage with inputMediaPoll(stopped=true). The
// underlying message service preserves the existing poll and voter results,
// so the Bot API response remains the canonical closed Poll object.
func stopPoll(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.MessagesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: message service is not configured"
	}
	botID := bot.GetUser().GetId()
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	messageID, err := request.int("message_id")
	if err != nil || messageID <= 0 || messageID > int(^uint32(0)>>1) {
		return nil, http.StatusBadRequest, 400, "Bad Request: message_id is invalid"
	}
	markup, err := botAPIReplyMarkup(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_markup is invalid"
	}
	updates, err := ctx.MessagesClient.MessagesEditMessage(botRPCContext(r.Context(), botID), &mtproto.TLMessagesEditMessage{
		Peer:        botAPIInputPeer(botID, peerType, peerID),
		Id:          int32(messageID),
		Media:       mtproto.MakeTLInputMediaPoll(&mtproto.InputMedia{Stopped: true}).To_InputMedia(),
		ReplyMarkup: markup,
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	message := botapi.FirstMessageFromUpdates(updates)
	if message == nil || message.Poll == nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: poll was not returned"
	}
	return message.Poll, http.StatusOK, 0, ""
}
