package core

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func stickerRequestContext(c *ApiFullCore) context.Context {
	if c != nil && c.ctx != nil {
		return c.ctx
	}
	return context.Background()
}

func stickerProviderError(c *ApiFullCore, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, persist.ErrStickerProviderUnavailable) {
		return stickersProviderUnavailable(c)
	}
	if errors.Is(err, persist.ErrStickerSetOwnerMismatch) {
		return mtproto.ErrStickersetInvalid
	}
	if errors.Is(err, persist.ErrStickerDocumentAccessMismatch) {
		return mtproto.ErrStickerIdInvalid
	}
	if errors.Is(err, persist.ErrStickerDocumentNotFound) {
		return mtproto.ErrStickerIdInvalid
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		if strings.Contains(pgErr.ConstraintName, "short_name") {
			return mtproto.ErrShortNameOccupied
		}
		return mtproto.ErrStickerIdInvalid
	}
	if errors.Is(err, sql.ErrNoRows) {
		return mtproto.ErrStickersetInvalid
	}
	return err
}

func stickerDocumentFromRecord(d persist.StickerDocument) *mtproto.Document {
	if d.MimeType == "" {
		d.MimeType = "image/webp"
	}
	attrs := []*mtproto.DocumentAttribute{
		mtproto.MakeTLDocumentAttributeSticker(&mtproto.DocumentAttribute{
			Alt: d.Alt,
			Stickerset: mtproto.MakeTLInputStickerSetID(&mtproto.InputStickerSet{
				Id:         d.SetID,
				AccessHash: d.SetAccessHash,
			}).To_InputStickerSet(),
		}).To_DocumentAttribute(),
	}
	return mtproto.MakeTLDocument(&mtproto.Document{
		Id: d.ID, AccessHash: d.AccessHash, FileReference: append([]byte(nil), d.FileRef...),
		Date: d.Date, MimeType: d.MimeType, Size2_INT64: d.SizeBytes, DcId: d.DCID,
		Attributes: attrs,
	}).To_Document()
}

func stickerSetFromRecord(s *persist.StickerSet, reqHash int32) *mtproto.Messages_StickerSet {
	if s == nil {
		return nil
	}
	set := mtproto.MakeTLStickerSet(&mtproto.StickerSet{
		Archived: s.Archived, Masks: s.Masks, Emojis: s.Emojis, TextColor: s.TextColor,
		Creator: s.Creator, Id: s.ID, AccessHash: s.AccessHash, Title: s.Title,
		ShortName: s.ShortName, Count: int32(len(s.Documents)), Hash: int32(stickerSetHash(s)),
		Animated: s.Animated, Videos: s.Videos,
	}).To_StickerSet()
	if s.ThumbDocumentID > 0 {
		set.ThumbDocumentId = wrapperspb.Int64(s.ThumbDocumentID)
	}
	if reqHash != 0 && reqHash == int32(stickerSetHash(s)) {
		return mtproto.MakeTLMessagesStickerSetNotModified(nil).To_Messages_StickerSet()
	}
	docs := make([]*mtproto.Document, 0, len(s.Documents))
	for _, d := range s.Documents {
		docs = append(docs, stickerDocumentFromRecord(d))
	}
	packs := stickerPacks(s.Documents)
	return mtproto.MakeTLMessagesStickerSet(&mtproto.Messages_StickerSet{
		Set: set, Packs: packs, Keywords: []*mtproto.StickerKeyword{}, Documents: docs,
	}).To_Messages_StickerSet()
}

func stickerSetHash(s *persist.StickerSet) int64 {
	h := s.ID*31 + int64(len(s.Documents))
	for _, d := range s.Documents {
		h = h*31 + d.ID
	}
	if h < 0 {
		return -h
	}
	return h
}

func stickerPacks(docs []persist.StickerDocument) []*mtproto.StickerPack {
	byEmoji := make(map[string][]int64)
	for _, d := range docs {
		if d.Alt != "" {
			byEmoji[d.Alt] = append(byEmoji[d.Alt], d.ID)
		}
	}
	packs := make([]*mtproto.StickerPack, 0, len(byEmoji))
	for emoji, ids := range byEmoji {
		packs = append(packs, mtproto.MakeTLStickerPack(&mtproto.StickerPack{Emoticon: emoji, Documents: ids}).To_StickerPack())
	}
	return packs
}

func stickerCovered(s persist.StickerSet) *mtproto.StickerSetCovered {
	set := mtproto.MakeTLStickerSet(&mtproto.StickerSet{
		Archived: s.Archived, Masks: s.Masks, Emojis: s.Emojis, TextColor: s.TextColor,
		Creator: s.Creator, Id: s.ID, AccessHash: s.AccessHash, Title: s.Title,
		ShortName: s.ShortName, Count: int32(len(s.Documents)), Hash: int32(stickerSetHash(&s)),
		Animated: s.Animated, Videos: s.Videos,
	}).To_StickerSet()
	var cover *mtproto.Document
	if len(s.Documents) > 0 {
		cover = stickerDocumentFromRecord(s.Documents[0])
	} else {
		cover = mtproto.MakeTLDocumentEmpty(&mtproto.Document{Id: s.ID}).To_Document()
	}
	return mtproto.MakeTLStickerSetCovered(&mtproto.StickerSetCovered{Set: set, Cover: cover}).To_StickerSetCovered()
}

func inputDocumentID(in *mtproto.InputDocument) int64 {
	if in == nil {
		return 0
	}
	return in.GetId()
}

func inputStickerDocs(items []*mtproto.InputStickerSetItem) []persist.StickerDocumentInput {
	docs := make([]persist.StickerDocumentInput, 0, len(items))
	for _, item := range items {
		if item == nil || item.GetDocument() == nil {
			continue
		}
		doc := item.GetDocument()
		keywords := ""
		if item.GetKeywords() != nil {
			keywords = item.GetKeywords().GetValue()
		}
		docs = append(docs, persist.StickerDocumentInput{ID: doc.GetId(), AccessHash: doc.GetAccessHash(), Alt: item.GetEmoji(), Keywords: keywords})
	}
	return docs
}

func stickerSetTokenID(in *mtproto.InputStickerSet) (int64, string) {
	if in == nil {
		return 0, ""
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputStickerSetID:
		return in.GetId(), ""
	case mtproto.Predicate_inputStickerSetShortName:
		return 0, strings.TrimSpace(in.GetShortName())
	default:
		return 0, ""
	}
}

func validateStickerSetAccess(in *mtproto.InputStickerSet, set *persist.StickerSet) error {
	if in == nil || set == nil {
		return mtproto.ErrStickersetInvalid
	}
	if in.GetPredicateName() == mtproto.Predicate_inputStickerSetID && in.GetAccessHash() != set.AccessHash {
		return mtproto.ErrStickersetInvalid
	}
	return nil
}
