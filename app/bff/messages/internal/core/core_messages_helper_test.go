package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	mediaclient "github.com/teamgram/teamgram-server/app/service/media/client"
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
)

type documentReferenceMediaClientStub struct {
	mediaclient.MediaClient
	document *mtproto.Document
	calls    int
}

func (s *documentReferenceMediaClientStub) MediaGetDocument(_ context.Context, _ *mediapb.TLMediaGetDocument) (*mtproto.Document, error) {
	s.calls++
	return s.document, nil
}

func newDocumentReferenceTestCore(client mediaclient.MediaClient) *MessagesCore {
	return &MessagesCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			MediaClient: client,
		}},
	}
}

func inputMediaDocumentReference(id, accessHash int64) *mtproto.InputMedia {
	return mtproto.MakeTLInputMediaDocument(&mtproto.InputMedia{
		Id_INPUTDOCUMENT: mtproto.MakeTLInputDocument(&mtproto.InputDocument{
			Id:         id,
			AccessHash: accessHash,
		}).To_InputDocument(),
	}).To_InputMedia()
}

func TestMakeMediaByInputMediaValidatesDocumentReference(t *testing.T) {
	validDocument := mtproto.MakeTLDocument(&mtproto.Document{
		Id:         10,
		AccessHash: 20,
	}).To_Document()

	tests := []struct {
		name      string
		media     *mtproto.InputMedia
		document  *mtproto.Document
		wantCalls int
		wantErr   error
	}{
		{
			name:    "missing input document",
			media:   mtproto.MakeTLInputMediaDocument(&mtproto.InputMedia{}).To_InputMedia(),
			wantErr: mtproto.ErrDocumentInvalid,
		},
		{
			name: "empty input document",
			media: mtproto.MakeTLInputMediaDocument(&mtproto.InputMedia{
				Id_INPUTDOCUMENT: mtproto.MakeTLInputDocumentEmpty(nil).To_InputDocument(),
			}).To_InputMedia(),
			wantErr: mtproto.ErrDocumentInvalid,
		},
		{
			name:    "zero access hash",
			media:   inputMediaDocumentReference(10, 0),
			wantErr: mtproto.ErrDocumentInvalid,
		},
		{
			name:      "missing document",
			media:     inputMediaDocumentReference(10, 20),
			document:  mtproto.MakeTLDocumentEmpty(&mtproto.Document{Id: 10}).To_Document(),
			wantCalls: 1,
			wantErr:   mtproto.ErrDocumentInvalid,
		},
		{
			name:  "mismatched access hash",
			media: inputMediaDocumentReference(10, 20),
			document: mtproto.MakeTLDocument(&mtproto.Document{
				Id:         10,
				AccessHash: 21,
			}).To_Document(),
			wantCalls: 1,
			wantErr:   mtproto.ErrDocumentInvalid,
		},
		{
			name:      "valid document",
			media:     inputMediaDocumentReference(10, 20),
			document:  validDocument,
			wantCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &documentReferenceMediaClientStub{document: tt.document}
			got, err := newDocumentReferenceTestCore(client).makeMediaByInputMedia(tt.media)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("makeMediaByInputMedia() error = %v, want %v", err, tt.wantErr)
			}
			if client.calls != tt.wantCalls {
				t.Fatalf("MediaGetDocument calls = %d, want %d", client.calls, tt.wantCalls)
			}
			if tt.wantErr == nil && (got == nil || got.GetDocument() != validDocument) {
				t.Fatalf("makeMediaByInputMedia() document = %v, want %v", got.GetDocument(), validDocument)
			}
		})
	}
}

func inputMediaPoll(question string, options ...[]byte) *mtproto.InputMedia {
	answers := make([]*mtproto.PollAnswer, 0, len(options))
	for _, option := range options {
		answers = append(answers, mtproto.MakeTLPollAnswer(&mtproto.PollAnswer{
			Text_TEXTWITHENTITIES: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{Text: string(option)}).To_TextWithEntities(),
			Option:                option,
		}).To_PollAnswer())
	}
	return mtproto.MakeTLInputMediaPoll(&mtproto.InputMedia{
		Poll: mtproto.MakeTLPoll(&mtproto.Poll{
			Question_TEXTWITHENTITIES: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{Text: question}).To_TextWithEntities(),
			Answers:                   answers,
		}).To_Poll(),
	}).To_InputMedia()
}

func TestMakeMediaByInputMediaPoll(t *testing.T) {
	input := inputMediaPoll("Pick one", []byte("one"), []byte("two"))
	got, err := newDocumentReferenceTestCore(nil).makeMediaByInputMedia(input)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.GetPredicateName() != mtproto.Predicate_messageMediaPoll {
		t.Fatalf("media = %+v, want messageMediaPoll", got)
	}
	poll := got.GetPoll()
	if poll == nil || poll.GetId() == 0 || poll.GetHash() == 0 {
		t.Fatalf("stored poll = %+v, want non-zero ID and hash", poll)
	}
	if input.GetPoll().GetId() != 0 || input.GetPoll().GetHash() != 0 {
		t.Fatalf("input poll was mutated: %+v", input.GetPoll())
	}
	if pollQuestionText(poll) != "Pick one" || len(poll.GetAnswers()) != 2 {
		t.Fatalf("stored poll = %+v", poll)
	}
	results := got.GetResults()
	if results == nil || results.GetTotalVoters().GetValue() != 0 || len(results.GetResults()) != 2 {
		t.Fatalf("initial results = %+v", results)
	}
	for i, result := range results.GetResults() {
		if result.GetVoters_FLAGINT32() != nil || string(result.GetOption()) != string(input.GetPoll().GetAnswers()[i].GetOption()) {
			t.Fatalf("result[%d] = %+v", i, result)
		}
	}
	buf := mtproto.NewEncodeBuf(1024)
	if err := got.Encode(buf, 229); err != nil {
		t.Fatal(err)
	}
	decoded := mtproto.NewDecodeBuf(buf.GetBuf())
	if decoded.Object() == nil || decoded.GetError() != nil || decoded.GetOffset() != decoded.GetSize() {
		t.Fatalf("messageMediaPoll layer 229 round trip failed: err=%v offset=%d size=%d", decoded.GetError(), decoded.GetOffset(), decoded.GetSize())
	}
}

func TestMakeMediaByInputMediaPollRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		media *mtproto.InputMedia
		want  error
	}{
		{
			name:  "empty question",
			media: inputMediaPoll("", []byte("one"), []byte("two")),
			want:  mtproto.ErrPollQuestionInvalid,
		},
		{
			name:  "one answer",
			media: inputMediaPoll("Pick", []byte("one")),
			want:  mtproto.ErrPollAnswersInvalid,
		},
		{
			name:  "empty option",
			media: inputMediaPoll("Pick", []byte(""), []byte("two")),
			want:  mtproto.ErrPollAnswerInvalid,
		},
		{
			name:  "duplicate option",
			media: inputMediaPoll("Pick", []byte("one"), []byte("one")),
			want:  mtproto.ErrPollAnswersInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newDocumentReferenceTestCore(nil).makeMediaByInputMedia(tt.media)
			if !errors.Is(err, tt.want) {
				t.Fatalf("makeMediaByInputMedia() error = %v, want %v", err, tt.want)
			}
		})
	}
}
