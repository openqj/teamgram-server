// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	"github.com/teamgram/teamgram-server/pkg/rpc/businessctx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type botAPIStoryContent struct {
	Type                string  `json:"type"`
	Photo               string  `json:"photo"`
	Video               string  `json:"video"`
	Duration            float64 `json:"duration"`
	CoverFrameTimestamp float64 `json:"cover_frame_timestamp"`
	IsAnimation         bool    `json:"is_animation"`
}

type botAPIStoryArea struct {
	Position botAPIStoryAreaPosition `json:"position"`
	Type     json.RawMessage         `json:"type"`
}

type botAPIStoryAreaPosition struct {
	XPercentage      float64 `json:"x_percentage"`
	YPercentage      float64 `json:"y_percentage"`
	WidthPercentage  float64 `json:"width_percentage"`
	HeightPercentage float64 `json:"height_percentage"`
	RotationAngle    float64 `json:"rotation_angle"`
}

func decodeBotAPIJSONField(request *botAPIRequest, name string, target any) error {
	raw := request.raw(name)
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if raw[0] == '"' {
		var encoded string
		if err := json.Unmarshal(raw, &encoded); err != nil {
			return err
		}
		raw = json.RawMessage(encoded)
	}
	return json.Unmarshal(raw, target)
}

func resolveStoryScope(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (scopeOwner int64, scope businessctx.Scope, err error) {
	connectionID := strings.TrimSpace(request.string("business_connection_id"))
	if connectionID == "" || len(connectionID) > 128 {
		return 0, businessctx.Scope{}, mtproto.ErrInputRequestInvalid
	}
	scope, err = resolveBotAPIBusinessScope(ctx, r.Context(), botID, connectionID)
	if err != nil {
		return 0, businessctx.Scope{}, err
	}
	if scope.OwnerID <= 0 {
		return 0, businessctx.Scope{}, mtproto.ErrInputRequestInvalid
	}
	return scope.OwnerID, scope, nil
}

func storyBadRequest(message string) (any, int, int, string) {
	if strings.HasPrefix(message, "Bad Request:") {
		return nil, http.StatusBadRequest, 400, message
	}
	return nil, http.StatusBadRequest, 400, "Bad Request: " + message
}

func storyActivePeriod(request *botAPIRequest) (int32, error) {
	period, err := request.int64("active_period")
	if err != nil {
		return 0, err
	}
	switch period {
	case 21600, 43200, 86400, 172800:
		return int32(period), nil
	default:
		return 0, errors.New("active_period is invalid")
	}
}

func decodeBotAPIStoryContent(request *botAPIRequest) (botAPIStoryContent, error) {
	var content botAPIStoryContent
	if err := decodeBotAPIJSONField(request, "content", &content); err != nil {
		return content, err
	}
	content.Type = strings.ToLower(strings.TrimSpace(content.Type))
	switch content.Type {
	case "photo":
		if content.Photo == "" || content.Video != "" {
			return content, errors.New("content is invalid")
		}
	case "video":
		if content.Video == "" || content.Photo != "" || content.Duration < 0 || content.Duration > 60 || content.CoverFrameTimestamp < 0 || content.CoverFrameTimestamp > 60 {
			return content, errors.New("content is invalid")
		}
	default:
		return content, errors.New("content type is invalid")
	}
	return content, nil
}

func botAPIStoryMedia(r *http.Request, ctx *svc.ServiceContext, botID int64, content botAPIStoryContent) (*mtproto.InputMedia, error) {
	ref := content.Photo
	kind := "photo"
	maxBytes := int64(10 << 20)
	if content.Type == "video" {
		ref = content.Video
		kind = "video"
		maxBytes = 30 << 20
	}
	if !strings.HasPrefix(ref, "attach://") {
		return nil, errors.New("story content must use attach:// upload")
	}
	field := strings.TrimPrefix(ref, "attach://")
	if field == "" || r == nil {
		return nil, errors.New("story attachment is invalid")
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		return nil, err
	}
	file, header, err := r.FormFile(field)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || len(data) == 0 || int64(len(data)) > maxBytes {
		return nil, errors.New("story attachment is too large or empty")
	}
	name := filepath.Base(header.Filename)
	if name == "." || name == "" {
		name = field
	}
	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = mime.TypeByExtension(filepath.Ext(name))
	}
	if mimeType == "" {
		if kind == "photo" {
			mimeType = "image/jpeg"
		} else {
			mimeType = "video/mp4"
		}
	}
	inputFile, err := uploadBotAPIFile(r.Context(), ctx, botID, name, data)
	if err != nil {
		return nil, err
	}
	if kind == "photo" {
		return mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{File: inputFile}).To_InputMedia(), nil
	}
	attributes := []*mtproto.DocumentAttribute{
		mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{FileName: name}).To_DocumentAttribute(),
		mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{
			Duration_FLOAT64:  content.Duration,
			SupportsStreaming: true,
			Nosound:           content.IsAnimation,
		}).To_DocumentAttribute(),
	}
	media := &mtproto.InputMedia{File: inputFile, MimeType: mimeType, Attributes: attributes, NosoundVideo: content.IsAnimation}
	if content.CoverFrameTimestamp > 0 {
		media.VideoTimestamp = wrapperspb.Int32(int32(math.Round(content.CoverFrameTimestamp)))
	}
	return mtproto.MakeTLInputMediaUploadedDocument(media).To_InputMedia(), nil
}

func botAPIStoryCaption(request *botAPIRequest) (*wrapperspb.StringValue, []*mtproto.MessageEntity, error) {
	caption := (*wrapperspb.StringValue)(nil)
	if request.has("caption") {
		caption = wrapperspb.String(request.text("caption"))
	}
	var entities []botapi.MessageEntity
	if request.has("caption_entities") && decodeBotAPIJSONField(request, "caption_entities", &entities) != nil {
		return nil, nil, errors.New("caption_entities is invalid")
	}
	return caption, botapi.MessageEntitiesToMTProto(entities), nil
}

func storyCoordinates(position botAPIStoryAreaPosition) (*mtproto.MediaAreaCoordinates, error) {
	if position.XPercentage < 0 || position.XPercentage > 100 || position.YPercentage < 0 || position.YPercentage > 100 || position.WidthPercentage <= 0 || position.WidthPercentage > 100 || position.HeightPercentage <= 0 || position.HeightPercentage > 100 || position.RotationAngle < 0 || position.RotationAngle > 360 {
		return nil, errors.New("areas.position is invalid")
	}
	return mtproto.MakeTLMediaAreaCoordinates(&mtproto.MediaAreaCoordinates{
		X: position.XPercentage, Y: position.YPercentage, W: position.WidthPercentage, H: position.HeightPercentage, Rotation: position.RotationAngle,
	}).To_MediaAreaCoordinates(), nil
}

func storyRawString(raw map[string]json.RawMessage, name string) string {
	var value string
	if json.Unmarshal(raw[name], &value) == nil {
		return strings.TrimSpace(value)
	}
	return ""
}

func storyReaction(raw map[string]json.RawMessage) (*mtproto.Reaction, error) {
	var reactionType map[string]json.RawMessage
	if err := json.Unmarshal(raw["reaction_type"], &reactionType); err != nil {
		return nil, errors.New("areas.type.reaction_type is invalid")
	}
	switch storyRawString(reactionType, "type") {
	case "emoji":
		emoji := storyRawString(reactionType, "emoji")
		if emoji == "" {
			return nil, errors.New("areas.type.reaction_type is invalid")
		}
		return mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: emoji}).To_Reaction(), nil
	case "custom_emoji":
		id, err := strconv.ParseInt(storyRawString(reactionType, "custom_emoji_id"), 10, 64)
		if err != nil || id <= 0 {
			return nil, errors.New("areas.type.reaction_type is invalid")
		}
		return mtproto.MakeTLReactionCustomEmoji(&mtproto.Reaction{DocumentId: id}).To_Reaction(), nil
	default:
		return nil, errors.New("areas.type.reaction_type is invalid")
	}
}

func storyAreas(request *botAPIRequest) ([]*mtproto.MediaArea, error) {
	if !request.has("areas") || string(request.raw("areas")) == "null" {
		return nil, nil
	}
	var input []botAPIStoryArea
	if err := decodeBotAPIJSONField(request, "areas", &input); err != nil {
		return nil, err
	}
	result := make([]*mtproto.MediaArea, 0, len(input))
	for _, area := range input {
		coordinates, err := storyCoordinates(area.Position)
		if err != nil {
			return nil, err
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(area.Type, &fields); err != nil {
			return nil, errors.New("areas.type is invalid")
		}
		base := &mtproto.MediaArea{Coordinates: coordinates}
		switch storyRawString(fields, "type") {
		case "location":
			var latitude, longitude float64
			if json.Unmarshal(fields["latitude"], &latitude) != nil || json.Unmarshal(fields["longitude"], &longitude) != nil || latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 {
				return nil, errors.New("areas.type is invalid")
			}
			base.Geo = mtproto.MakeTLGeoPoint(&mtproto.GeoPoint{Lat: latitude, Long: longitude}).To_GeoPoint()
			var address struct {
				CountryCode string `json:"country_code"`
				State       string `json:"state"`
				City        string `json:"city"`
				Street      string `json:"street"`
			}
			if len(fields["address"]) > 0 && json.Unmarshal(fields["address"], &address) == nil {
				base.Address_FLAGGEOPOINTADDRESS = mtproto.MakeTLGeoPointAddress(&mtproto.GeoPointAddress{CountryIso2: address.CountryCode, State: wrapperspb.String(address.State), City: wrapperspb.String(address.City), Street: wrapperspb.String(address.Street)}).To_GeoPointAddress()
			}
			result = append(result, mtproto.MakeTLMediaAreaGeoPoint(base).To_MediaArea())
		case "suggested_reaction":
			base.Dark = storyBool(fields, "is_dark")
			base.Flipped = storyBool(fields, "is_flipped")
			base.Reaction, err = storyReaction(fields)
			if err != nil {
				return nil, err
			}
			result = append(result, mtproto.MakeTLMediaAreaSuggestedReaction(base).To_MediaArea())
		case "link":
			base.Url = storyRawString(fields, "url")
			parsed, parseErr := url.Parse(base.Url)
			if parseErr != nil || parsed.Host == "" && !strings.EqualFold(parsed.Scheme, "tg") || (!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") && !strings.EqualFold(parsed.Scheme, "tg")) {
				return nil, errors.New("areas.type.url is invalid")
			}
			result = append(result, mtproto.MakeTLMediaAreaUrl(base).To_MediaArea())
		case "weather":
			if json.Unmarshal(fields["temperature"], &base.TemperatureC) != nil {
				return nil, errors.New("areas.type.temperature is invalid")
			}
			base.Emoji = storyRawString(fields, "emoji")
			var color uint64
			if base.Emoji == "" || json.Unmarshal(fields["background_color"], &color) != nil || color > uint64(^uint32(0)) {
				return nil, errors.New("areas.type is invalid")
			}
			base.Color = int32(uint32(color))
			result = append(result, mtproto.MakeTLMediaAreaWeather(base).To_MediaArea())
		case "unique_gift":
			base.Slug = storyRawString(fields, "name")
			if base.Slug == "" {
				return nil, errors.New("areas.type.name is invalid")
			}
			result = append(result, mtproto.MakeTLMediaAreaStarGift(base).To_MediaArea())
		default:
			return nil, errors.New("areas.type is invalid")
		}
	}
	return result, nil
}

func storyBool(raw map[string]json.RawMessage, name string) bool {
	var value bool
	_ = json.Unmarshal(raw[name], &value)
	return value
}

func botAPIStoryFromUpdates(updates *mtproto.Updates, ownerID int64) (any, int, int, string) {
	if updates == nil {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: story was not returned"
	}
	for _, update := range append(append([]*mtproto.Update(nil), updates.GetUpdate()), updates.GetUpdates()...) {
		if update == nil || update.GetPredicateName() != mtproto.Predicate_updateStory || update.GetStory() == nil {
			continue
		}
		peer := update.GetPeer_PEER()
		if peer == nil {
			peer = mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: ownerID}).To_Peer()
		}
		chat := botapi.ChatFromPeer(peer, nil, nil)
		if chat == nil {
			chat = &botapi.Chat{ID: ownerID, Type: "private"}
		}
		return &botapi.Story{Chat: chat, ID: int(update.GetStory().GetId())}, http.StatusOK, 0, ""
	}
	return nil, http.StatusInternalServerError, 500, "Internal Server Error: story was not returned"
}

func postStory(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	ownerID, scope, err := resolveStoryScope(ctx, r, botID, request)
	if err != nil {
		return storyBadRequest(rpcDescription(err))
	}
	period, err := storyActivePeriod(request)
	if err != nil {
		return storyBadRequest(err.Error())
	}
	content, err := decodeBotAPIStoryContent(request)
	if err != nil {
		return storyBadRequest(err.Error())
	}
	media, err := botAPIStoryMedia(r, ctx, botID, content)
	if err != nil {
		return storyBadRequest(err.Error())
	}
	caption, entities, err := botAPIStoryCaption(request)
	if err != nil {
		return storyBadRequest(err.Error())
	}
	areas, err := storyAreas(request)
	if err != nil {
		return storyBadRequest(err.Error())
	}
	if ctx == nil || ctx.StoriesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: story service is not configured"
	}
	updates, err := ctx.StoriesClient.StoriesSendStory(businessBotMediaCallContext(r.Context(), botID, scope), &mtproto.TLStoriesSendStory{
		Peer:       botAPIInputPeer(ownerID, mtproto.PEER_USER, ownerID),
		Media:      media,
		Period:     wrapperspb.Int32(period),
		Caption:    caption,
		Entities:   entities,
		MediaAreas: areas,
		RandomId:   randomMessageID(),
		Noforwards: requestBool(request, "protect_content"),
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return botAPIStoryFromUpdates(updates, ownerID)
}

func repostStory(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	ownerID, scope, err := resolveStoryScope(ctx, r, botID, request)
	if err != nil {
		return storyBadRequest(rpcDescription(err))
	}
	period, err := storyActivePeriod(request)
	if err != nil {
		return storyBadRequest(err.Error())
	}
	fromChat, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, ownerID, request.string("from_chat_id"))
	if err != nil || fromChat == 0 || peerID <= 0 {
		return storyBadRequest("from_chat_id is invalid")
	}
	fromStory, err := request.int64("from_story_id")
	if err != nil || fromStory <= 0 || fromStory > int64(^uint32(0)>>1) {
		return storyBadRequest("from_story_id is invalid")
	}
	if ctx == nil || ctx.StoriesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: story service is not configured"
	}
	updates, err := ctx.StoriesClient.StoriesSendStory(businessBotMediaCallContext(r.Context(), botID, scope), &mtproto.TLStoriesSendStory{
		Peer:         botAPIInputPeer(ownerID, mtproto.PEER_USER, ownerID),
		FwdFromId:    botAPIInputPeer(ownerID, peerType, peerID),
		FwdFromStory: wrapperspb.Int32(int32(fromStory)),
		Period:       wrapperspb.Int32(period),
		RandomId:     randomMessageID(),
		Noforwards:   requestBool(request, "protect_content"),
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return botAPIStoryFromUpdates(updates, ownerID)
}

func editStory(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	ownerID, scope, err := resolveStoryScope(ctx, r, botID, request)
	if err != nil {
		return storyBadRequest(rpcDescription(err))
	}
	storyID, err := request.int64("story_id")
	if err != nil || storyID <= 0 || storyID > int64(^uint32(0)>>1) {
		return storyBadRequest("story_id is invalid")
	}
	content, err := decodeBotAPIStoryContent(request)
	if err != nil {
		return storyBadRequest(err.Error())
	}
	media, err := botAPIStoryMedia(r, ctx, botID, content)
	if err != nil {
		return storyBadRequest(err.Error())
	}
	caption, entities, err := botAPIStoryCaption(request)
	if err != nil {
		return storyBadRequest(err.Error())
	}
	areas, err := storyAreas(request)
	if err != nil {
		return storyBadRequest(err.Error())
	}
	if ctx == nil || ctx.StoriesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: story service is not configured"
	}
	updates, err := ctx.StoriesClient.StoriesEditStory(businessBotMediaCallContext(r.Context(), botID, scope), &mtproto.TLStoriesEditStory{
		Peer:       botAPIInputPeer(ownerID, mtproto.PEER_USER, ownerID),
		Id:         int32(storyID),
		Media:      media,
		Caption:    caption,
		Entities:   entities,
		MediaAreas: areas,
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return botAPIStoryFromUpdates(updates, ownerID)
}

func deleteStory(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	ownerID, scope, err := resolveStoryScope(ctx, r, botID, request)
	if err != nil {
		return storyBadRequest(rpcDescription(err))
	}
	storyID, err := request.int64("story_id")
	if err != nil || storyID <= 0 || storyID > int64(^uint32(0)>>1) {
		return storyBadRequest("story_id is invalid")
	}
	if ctx == nil || ctx.StoriesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: story service is not configured"
	}
	if _, err = ctx.StoriesClient.StoriesDeleteStories(businessBotMediaCallContext(r.Context(), botID, scope), &mtproto.TLStoriesDeleteStories{
		Peer: botAPIInputPeer(ownerID, mtproto.PEER_USER, ownerID), Id: []int32{int32(storyID)},
	}); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func suggestedPost(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest, reject bool) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID <= 0 {
		return storyBadRequest("chat_id is invalid")
	}
	messageID, err := request.int64("message_id")
	if err != nil || messageID <= 0 || messageID > int64(^uint32(0)>>1) {
		return storyBadRequest("message_id is invalid")
	}
	if ctx == nil || ctx.SuggestedPostsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: suggested post service is not configured"
	}
	call := &mtproto.TLMessagesToggleSuggestedPostApproval{Reject: reject, Peer: botAPIInputPeer(botID, peerType, peerID), MsgId: int32(messageID)}
	if reject {
		if request.has("comment") && string(request.raw("comment")) != "null" {
			comment := request.text("comment")
			if utf8.RuneCountInString(comment) > 128 {
				return storyBadRequest("comment is too long")
			}
			call.RejectComment = wrapperspb.String(comment)
		}
	} else if request.has("send_date") && string(request.raw("send_date")) != "null" {
		sendDate, parseErr := request.int64("send_date")
		if parseErr != nil || sendDate <= 0 || sendDate > int64(^uint32(0)>>1) || sendDate > time.Now().Add(30*24*time.Hour).Unix() {
			return storyBadRequest("send_date is invalid")
		}
		call.ScheduleDate = wrapperspb.Int32(int32(sendDate))
	}
	if _, err = ctx.SuggestedPostsClient.MessagesToggleSuggestedPostApproval(botRPCContext(r.Context(), botID), call); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}
