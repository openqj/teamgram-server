package core

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestAicomposeTonePostgresRoundTrip(t *testing.T) {
	userID := time.Now().UnixNano()
	t.Cleanup(func() {
		_, _ = persist.MutateAITones(userID, func([]persist.AITone) ([]persist.AITone, error) {
			return nil, nil
		})
	})

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}
	created, err := c.AicomposeCreateTone(&mtproto.TLAicomposeCreateTone{
		Title: "warm", Prompt: "be warm", EmojiId: 42, DisplayAuthor: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created == nil || created.GetId() == 0 || created.GetTitle() != "warm" {
		t.Fatalf("created tone: %+v", created)
	}

	updated, err := c.AicomposeUpdateTone(&mtproto.TLAicomposeUpdateTone{
		Tone:  &mtproto.InputAiComposeTone{Id: created.GetId()},
		Title: wrapperspb.String("calm"), Prompt: wrapperspb.String("be calm"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated == nil || updated.GetTitle() != "calm" || updated.GetPrompt().GetValue() != "be calm" {
		t.Fatalf("updated tone: %+v", updated)
	}

	saved, err := c.AicomposeSaveTone(&mtproto.TLAicomposeSaveTone{
		Tone: &mtproto.InputAiComposeTone{Id: created.GetId()}, Unsave: mtproto.BoolFalse,
	})
	if err != nil || saved != mtproto.BoolTrue {
		t.Fatalf("save tone: result=%v err=%v", saved, err)
	}
	got, err := c.AicomposeGetTone(&mtproto.TLAicomposeGetTone{
		Tone: &mtproto.InputAiComposeTone{Id: created.GetId()},
	})
	if err != nil || got == nil || len(got.GetTones()) != 1 || got.GetTones()[0].GetTitle() != "calm" {
		t.Fatalf("get tone: result=%+v err=%v", got, err)
	}

	all, err := c.AicomposeGetTones(&mtproto.TLAicomposeGetTones{})
	if err != nil || all == nil || len(all.GetTones()) != 1 {
		t.Fatalf("get tones: result=%+v err=%v", all, err)
	}
	unchanged, err := c.AicomposeGetTones(&mtproto.TLAicomposeGetTones{Hash: all.GetHash()})
	if err != nil || unchanged == nil || unchanged.GetPredicateName() != mtproto.Predicate_aicompose_tonesNotModified {
		t.Fatalf("not modified: result=%+v err=%v", unchanged, err)
	}

	deleted, err := c.AicomposeDeleteTone(&mtproto.TLAicomposeDeleteTone{Tone: &mtproto.InputAiComposeTone{Id: created.GetId()}})
	if err != nil || deleted != mtproto.BoolTrue {
		t.Fatalf("delete tone: result=%v err=%v", deleted, err)
	}
	remaining, err := c.AicomposeGetTones(&mtproto.TLAicomposeGetTones{})
	if err != nil || remaining == nil || len(remaining.GetTones()) != 0 {
		t.Fatalf("remaining tones: result=%+v err=%v", remaining, err)
	}
}
