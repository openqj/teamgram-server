// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type botAPIPaidMediaItem struct {
	Type              string `json:"type"`
	Media             string `json:"media"`
	Thumbnail         string `json:"thumbnail,omitempty"`
	Cover             string `json:"cover,omitempty"`
	StartTimestamp    int    `json:"start_timestamp,omitempty"`
	Width             int    `json:"width,omitempty"`
	Height            int    `json:"height,omitempty"`
	Duration          int    `json:"duration,omitempty"`
	SupportsStreaming bool   `json:"supports_streaming,omitempty"`
	HasSpoiler        bool   `json:"has_spoiler,omitempty"`
	Photo             string `json:"photo,omitempty"`
}

type botAPIInputChecklist struct {
	Title                  string                     `json:"title"`
	TitleEntities          []botapi.MessageEntity     `json:"title_entities,omitempty"`
	Tasks                  []botAPIInputChecklistTask `json:"tasks"`
	OthersCanAddTasks      bool                       `json:"others_can_add_tasks,omitempty"`
	OthersCanMarkTasksDone bool                       `json:"others_can_mark_tasks_as_done,omitempty"`
}

type botAPIInputChecklistTask struct {
	ID           int                    `json:"id"`
	Text         string                 `json:"text"`
	TextEntities []botapi.MessageEntity `json:"text_entities,omitempty"`
}

func sendLivePhoto(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	botID := bot.GetUser().GetId()
	photo, err := botAPILayer229MediaInput(r, ctx, botID, request.string("photo"), "photo", request, false, "photo")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: photo is invalid"
	}
	video, err := botAPILayer229VideoDocument(r, ctx, botID, request.string("live_photo"), request, "live_photo", 0, 0, 0, false)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: live_photo is invalid"
	}
	if photo == nil || video == nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: live photo is invalid"
	}
	photo.LivePhoto = true
	photo.Video = video
	return sendBotAPIStructuredMedia(ctx, r, bot, request, photo, "")
}

func sendPaidMedia(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	starCount, err := request.int("star_count")
	if err != nil || starCount < 1 || starCount > 25000 {
		return nil, http.StatusBadRequest, 400, "Bad Request: star_count is invalid"
	}
	payload := request.string("payload")
	if len([]byte(payload)) > 128 {
		return nil, http.StatusBadRequest, 400, "Bad Request: payload is too long"
	}
	var items []botAPIPaidMediaItem
	if err = request.json("media", &items); err != nil || len(items) == 0 || len(items) > 10 {
		return nil, http.StatusBadRequest, 400, "Bad Request: media is invalid"
	}
	botID := bot.GetUser().GetId()
	extended := make([]*mtproto.InputMedia, 0, len(items))
	for _, item := range items {
		input, inputErr := botAPIPaidMediaInput(r, ctx, botID, item, request)
		if inputErr != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: media is invalid"
		}
		extended = append(extended, input)
	}
	media := mtproto.MakeTLInputMediaPaidMedia(&mtproto.InputMedia{
		StarsAmount:                    int64(starCount),
		ExtendedMedia_VECTORINPUTMEDIA: extended,
	}).To_InputMedia()
	if payload != "" {
		media.Payload_FLAGSTRING = wrapperspb.String(payload)
	}
	return sendBotAPIStructuredMedia(ctx, r, bot, request, media, "")
}

func botAPIPaidMediaInput(r *http.Request, ctx *svc.ServiceContext, botID int64, item botAPIPaidMediaItem, request *botAPIRequest) (*mtproto.InputMedia, error) {
	typ := strings.ToLower(strings.TrimSpace(item.Type))
	if item.Width < 0 || item.Height < 0 || item.Duration < 0 || item.StartTimestamp < 0 {
		return nil, errors.New("invalid media metadata")
	}
	switch typ {
	case "photo":
		return botAPILayer229MediaInput(r, ctx, botID, item.Media, "photo", &botAPIRequest{values: map[string]json.RawMessage{
			"has_spoiler": json.RawMessage(fmt.Sprintf("%t", item.HasSpoiler)),
		}}, true, "")
	case "video":
		return botAPILayer229MediaInput(r, ctx, botID, item.Media, "video", &botAPIRequest{values: map[string]json.RawMessage{
			"has_spoiler":        json.RawMessage(fmt.Sprintf("%t", item.HasSpoiler)),
			"supports_streaming": json.RawMessage(fmt.Sprintf("%t", item.SupportsStreaming)),
		}}, true, "")
	case "live_photo":
		if item.Photo == "" || item.Media == "" || botAPIHTTPMediaURL(item.Photo) || botAPIHTTPMediaURL(item.Media) {
			return nil, errors.New("live photo URLs are unsupported")
		}
		photo, err := botAPILayer229MediaInput(r, ctx, botID, item.Photo, "photo", request, false, "")
		if err != nil {
			return nil, err
		}
		video, err := botAPILayer229VideoDocument(r, ctx, botID, item.Media, request, "", item.Duration, item.Width, item.Height, item.SupportsStreaming)
		if err != nil {
			return nil, err
		}
		photo.LivePhoto = true
		photo.Video = video
		return photo, nil
	default:
		return nil, errors.New("unsupported paid media type")
	}
}

func sendChecklist(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	if strings.TrimSpace(request.string("business_connection_id")) == "" {
		return nil, http.StatusBadRequest, 400, "Bad Request: business_connection_id is required"
	}
	checklist, err := botAPIInputChecklistValue(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: checklist is invalid"
	}
	media := mtproto.MakeTLInputMediaTodo(&mtproto.InputMedia{Todo: checklist}).To_InputMedia()
	return sendBotAPIStructuredMedia(ctx, r, bot, request, media, "")
}

func editMessageChecklist(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	if strings.TrimSpace(request.string("business_connection_id")) == "" {
		return nil, http.StatusBadRequest, 400, "Bad Request: business_connection_id is required"
	}
	checklist, err := botAPIInputChecklistValue(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: checklist is invalid"
	}
	markup, err := botAPIReplyMarkup(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: reply_markup is invalid"
	}
	media := mtproto.MakeTLInputMediaTodo(&mtproto.InputMedia{Todo: checklist}).To_InputMedia()
	return editBotAPIMessageWithMedia(ctx, r, bot, request, "", false, nil, markup, media)
}

func botAPIInputChecklistValue(request *botAPIRequest) (*mtproto.TodoList, error) {
	var value botAPIInputChecklist
	if err := request.json("checklist", &value); err != nil {
		return nil, err
	}
	if utf8.RuneCountInString(value.Title) < 1 || utf8.RuneCountInString(value.Title) > 255 || len(value.Tasks) < 1 || len(value.Tasks) > 30 {
		return nil, errors.New("checklist bounds are invalid")
	}
	seen := make(map[int]struct{}, len(value.Tasks))
	items := make([]*mtproto.TodoItem, 0, len(value.Tasks))
	for _, task := range value.Tasks {
		if task.ID <= 0 || utf8.RuneCountInString(task.Text) < 1 || utf8.RuneCountInString(task.Text) > 100 {
			return nil, errors.New("checklist task is invalid")
		}
		if _, exists := seen[task.ID]; exists {
			return nil, errors.New("checklist task id is duplicated")
		}
		seen[task.ID] = struct{}{}
		items = append(items, mtproto.MakeTLTodoItem(&mtproto.TodoItem{
			Id: int32(task.ID),
			Title: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
				Text: task.Text, Entities: botapi.MessageEntitiesToMTProto(task.TextEntities),
			}).To_TextWithEntities(),
		}).To_TodoItem())
	}
	return mtproto.MakeTLTodoList(&mtproto.TodoList{
		OthersCanAppend:   value.OthersCanAddTasks,
		OthersCanComplete: value.OthersCanMarkTasksDone,
		Title: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
			Text: value.Title, Entities: botapi.MessageEntitiesToMTProto(value.TitleEntities),
		}).To_TextWithEntities(),
		List: items,
	}).To_TodoList(), nil
}

func sendMessageDraft(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser, request *botAPIRequest) (any, int, int, string) {
	canStop, err := request.bool("can_stop")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: can_stop is invalid"
	}
	keepOnStop, err := request.bool("keep_on_stop")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: keep_on_stop is invalid"
	}
	if canStop || keepOnStop {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: rich draft stop controls are not supported"
	}
	if ctx == nil || ctx.EphemeralClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: ephemeral message service is not configured"
	}
	botID := bot.GetUser().GetId()
	chatID, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || chatID <= 0 || peerType != mtproto.PEER_USER {
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
	text, entities, formatErr := botAPITextEntities(request, "text", "entities", "parse_mode")
	if formatErr != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + formatErr.Error()
	}
	if utf8.RuneCountInString(text) > 4096 {
		return nil, http.StatusBadRequest, 400, "Bad Request: text is too long"
	}
	ok, err := ctx.EphemeralClient.EphemeralSendMessage(botRPCContext(r.Context(), botID), &mtproto.TLEphemeralSendMessage{
		Peer:       botAPIInputPeer(botID, peerType, peerID),
		ReceiverId: receiver,
		QueryId:    wrapperspb.Int64(draftID),
		Message:    text,
		Entities:   botapi.MessageEntitiesToMTProto(entities),
		RandomId:   randomMessageID(),
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if ok == nil {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: draft was not accepted"
	}
	return true, http.StatusOK, 0, ""
}

func uploadStickerFile(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.MediaClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: media service is not configured"
	}
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	format := strings.ToLower(strings.TrimSpace(request.string("sticker_format")))
	if format != "static" && format != "animated" && format != "video" {
		return nil, http.StatusBadRequest, 400, "Bad Request: sticker_format is invalid"
	}
	if r == nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: sticker is required"
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: sticker is required"
	}
	file, header, err := r.FormFile("sticker")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: sticker is required"
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 64<<20+1))
	if err != nil || len(data) == 0 || len(data) > 64<<20 {
		return nil, http.StatusBadRequest, 400, "Bad Request: sticker is invalid"
	}
	name := filepath.Base(header.Filename)
	if name == "." || name == "" {
		name = "sticker"
	}
	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = mime.TypeByExtension(filepath.Ext(name))
	}
	if mimeType == "" {
		switch format {
		case "static":
			mimeType = "image/webp"
		case "animated":
			mimeType = "application/x-tgsticker"
		case "video":
			mimeType = "video/webm"
		}
	}
	input, err := uploadBotAPIFile(r.Context(), ctx, botID, name, data)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	attribute := mtproto.MakeTLDocumentAttributeSticker(&mtproto.DocumentAttribute{
		Alt:        request.string("emoji"),
		Stickerset: mtproto.MakeTLInputStickerSetEmpty(nil).To_InputStickerSet(),
	}).To_DocumentAttribute()
	document, err := ctx.MediaClient.MediaUploadStickerFile(botMediaRPCContext(r.Context(), botID), &mediapb.TLMediaUploadStickerFile{
		OwnerId:                  userID,
		File:                     input,
		MimeType:                 mimeType,
		FileName:                 name,
		DocumentAttributeSticker: attribute,
	})
	if err != nil || document == nil || document.GetId() <= 0 {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	size := document.GetSize2()
	if size == 0 {
		size = document.GetSize2_INT64()
	}
	if size == 0 {
		size = int64(document.GetSize2_INT32())
	}
	return &botapi.File{
		FileID:       fmt.Sprintf("document_%d_%d", document.GetId(), document.GetAccessHash()),
		FileUniqueID: fmt.Sprintf("%d", document.GetId()),
		FileSize:     size,
	}, http.StatusOK, 0, ""
}

func botAPILayer229MediaInput(r *http.Request, ctx *svc.ServiceContext, botID int64, raw, kind string, request *botAPIRequest, allowURL bool, fallbackField string) (*mtproto.InputMedia, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" && fallbackField != "" {
		raw = "attach://" + fallbackField
	}
	if strings.HasPrefix(raw, "attach://") {
		if r == nil || ctx == nil {
			return nil, mtproto.ErrMediaInvalid
		}
		field := strings.TrimPrefix(raw, "attach://")
		if field == "" {
			return nil, mtproto.ErrMediaInvalid
		}
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			return nil, err
		}
		file, header, err := r.FormFile(field)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, 64<<20+1))
		if err != nil || len(data) == 0 || len(data) > 64<<20 {
			return nil, mtproto.ErrFilePartsInvalid
		}
		name := filepath.Base(header.Filename)
		if name == "." || name == "" {
			name = kind
		}
		mimeType := header.Header.Get("Content-Type")
		if mimeType == "" {
			mimeType = mime.TypeByExtension(filepath.Ext(name))
		}
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		input, err := uploadBotAPIFile(r.Context(), ctx, botID, name, data)
		if err != nil {
			return nil, err
		}
		return botAPIUploadedMedia(kind, input, name, mimeType, 0, 0, 0, request), nil
	}
	if botAPIHTTPMediaURL(raw) {
		if !allowURL {
			return nil, errors.New("media URL is unsupported")
		}
		data, mimeType, name, err := fetchBotAPIRemoteMedia(r.Context(), raw)
		if err != nil {
			return nil, err
		}
		input, err := uploadBotAPIFile(r.Context(), ctx, botID, name, data)
		if err != nil {
			return nil, err
		}
		return botAPIUploadedMedia(kind, input, name, mimeType, 0, 0, 0, request), nil
	}
	if raw == "" {
		return nil, mtproto.ErrFileIdInvalid
	}
	return botAPIExistingMedia(kind, raw, request)
}

func botAPILayer229VideoDocument(r *http.Request, ctx *svc.ServiceContext, botID int64, raw string, request *botAPIRequest, fallbackField string, duration, width, height int, supportsStreaming bool) (*mtproto.InputDocument, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" && fallbackField != "" {
		raw = "attach://" + fallbackField
	}
	if botAPIHTTPMediaURL(raw) {
		return nil, errors.New("live photo video URLs are unsupported")
	}
	if strings.HasPrefix(raw, "attach://") {
		if r == nil || ctx == nil {
			return nil, mtproto.ErrMediaInvalid
		}
		field := strings.TrimPrefix(raw, "attach://")
		if field == "" {
			return nil, mtproto.ErrMediaInvalid
		}
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			return nil, err
		}
		file, header, err := r.FormFile(field)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, 10<<20+1))
		if err != nil || len(data) == 0 || len(data) > 10<<20 {
			return nil, mtproto.ErrFilePartsInvalid
		}
		name := filepath.Base(header.Filename)
		if name == "." || name == "" {
			name = "live-photo-video"
		}
		mimeType := header.Header.Get("Content-Type")
		if mimeType == "" {
			mimeType = mime.TypeByExtension(filepath.Ext(name))
		}
		if mimeType == "" {
			mimeType = "video/mp4"
		}
		input, err := uploadBotAPIFile(r.Context(), ctx, botID, name, data)
		if err != nil {
			return nil, err
		}
		return botAPILayer229UploadedDocument(ctx, r, botID, input, name, mimeType, duration, width, height, supportsStreaming)
	}
	if raw == "" {
		return nil, mtproto.ErrFileIdInvalid
	}
	ref, err := parseBotAPIFileID(raw)
	if err != nil || ref.kind != "document" {
		return nil, mtproto.ErrFileIdInvalid
	}
	return mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: ref.id, AccessHash: ref.accessHash}).To_InputDocument(), nil
}

func botAPILayer229UploadedDocument(ctx *svc.ServiceContext, r *http.Request, botID int64, file *mtproto.InputFile, name, mimeType string, duration, width, height int, supportsStreaming bool) (*mtproto.InputDocument, error) {
	if ctx == nil || ctx.FilesClient == nil || file == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	attrs := []*mtproto.DocumentAttribute{
		mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{FileName: name}).To_DocumentAttribute(),
		mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{
			Duration_INT32: int32(duration), W: int32(width), H: int32(height), SupportsStreaming: supportsStreaming,
		}).To_DocumentAttribute(),
	}
	media := mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{File: file, MimeType: mimeType, Attributes: attrs}).To_InputMedia()
	result, err := ctx.FilesClient.MessagesUploadMedia(botMediaRPCContext(r.Context(), botID), &mtproto.TLMessagesUploadMedia{
		Peer:  botAPIInputPeer(botID, mtproto.PEER_USER, botID),
		Media: media,
	})
	if err != nil {
		return nil, err
	}
	if result == nil || result.GetPredicateName() != mtproto.Predicate_messageMediaDocument || result.GetDocument() == nil {
		return nil, mtproto.ErrDocumentInvalid
	}
	document := result.GetDocument()
	if document.GetId() <= 0 || document.GetAccessHash() == 0 {
		return nil, mtproto.ErrDocumentInvalid
	}
	return mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: document.GetId(), AccessHash: document.GetAccessHash()}).To_InputDocument(), nil
}
