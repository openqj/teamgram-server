package channelview

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestHistoryAndEditDataCheckInputPeerAuthorization(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	if err := domain.OpenPostgresReadOnly(dsn); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = domain.Close() })
	cleanupDB, err := persist.OpenPostgresDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cleanupDB.Close() })

	channelID := time.Now().UnixNano()
	owner, member, outsider := channelID+1, channelID+2, channelID+3
	t.Cleanup(func() {
		for _, query := range []string{
			`DELETE FROM apifull_channel_message_hidden WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message_seq WHERE channel_id=?`,
			`DELETE FROM apifull_channel_read_state WHERE channel_id=?`,
			`DELETE FROM apifull_channel_member WHERE channel_id=?`,
			`DELETE FROM apifull_channel WHERE id=?`,
		} {
			if _, err := cleanupDB.Exec(query, channelID); err != nil {
				t.Errorf("cleanup channel fixture: %v", err)
			}
		}
	})
	if err := domain.SaveChannel(domain.Channel{
		ID: channelID, AccessHash: channelID, Creator: owner, Title: "history-auth-test", Broadcast: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := domain.JoinChannel(channelID, member); err != nil {
		t.Fatal(err)
	}
	if _, err := Post(owner, channelID, "stored", time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	if author, err := ChannelMessageAuthor(member, mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: channelID, AccessHash: channelID,
	}).To_InputChannel(), 1); err != nil || author != owner {
		t.Fatalf("APIFull channel author: author=%d err=%v, want %d", author, err, owner)
	}
	if _, err := ChannelMessageAuthor(member, mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: channelID, AccessHash: channelID + 1,
	}).To_InputChannel(), 1); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("bad APIFull channel access hash: got %v", err)
	}

	if _, _, err := domain.ChannelMessageAuthor(outsider, channelID, channelID, 1); !errors.Is(err, domain.ErrNotChannelMember) {
		t.Fatalf("channel author outsider: got %v, want ErrNotChannelMember", err)
	}
	if _, _, err := domain.ChannelMessageAuthor(member, channelID, channelID+1, 1); !errors.Is(err, domain.ErrChannelMissing) {
		t.Fatalf("channel author wrong access hash: got %v, want ErrChannelMissing", err)
	}
	if _, found, err := domain.ChannelMessageAuthor(member, channelID, channelID, 2); err != nil || found {
		t.Fatalf("missing channel message author: found=%v err=%v", found, err)
	}

	input := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId: channelID, AccessHash: channelID,
	}).To_InputPeer()
	got, err := HistoryForInputPeer(member, input, 0, 20)
	if err != nil || len(got.GetMessages()) != 1 {
		t.Fatalf("member history: messages=%d err=%v", len(got.GetMessages()), err)
	}
	layerHistory, err := HistoryForInputPeerLayer229(member, input, 0, 0, 0, 20, 0, 0)
	if err != nil || layerHistory == nil || layerHistory.GetPredicateName() != mtproto.Predicate_messages_channelMessages || layerHistory.GetCount() != 1 || layerHistory.GetPts() <= 0 || len(layerHistory.GetMessages()) != 1 {
		t.Fatalf("layer229 member history: predicate=%s count=%d pts=%d messages=%d err=%v", layerHistory.GetPredicateName(), layerHistory.GetCount(), layerHistory.GetPts(), len(layerHistory.GetMessages()), err)
	}
	search, err := SearchForInputPeer(member, input, "stored", 0, 0, 0, 0, 0, 0, 0, 20)
	if err != nil || len(search.GetMessages()) != 1 {
		t.Fatalf("member search: messages=%d err=%v", len(search.GetMessages()), err)
	}
	if _, err := Post(owner, channelID, "announcement #topic", time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	localPage, err := SearchForInputPeer(member, input, "", 0, 0, 1, 0, 0, 0, 0, 1)
	if err != nil || localPage.GetPredicateName() != mtproto.Predicate_messages_messagesSlice || localPage.GetCount() != 2 || len(localPage.GetMessages()) != 1 || localPage.GetNextRate() != nil {
		t.Fatalf("local search pagination: predicate=%s count=%d messages=%d next_rate=%v err=%v", localPage.GetPredicateName(), localPage.GetCount(), len(localPage.GetMessages()), localPage.GetNextRate(), err)
	}
	hashtagSearch, err := SearchForInputPeer(member, input, "#topic", 0, 0, 0, 0, 0, 0, 0, 20)
	if err != nil || len(hashtagSearch.GetMessages()) != 1 || hashtagSearch.GetMessages()[0].GetMessage() != "announcement #topic" {
		t.Fatalf("member hashtag search: messages=%d err=%v", len(hashtagSearch.GetMessages()), err)
	}
	dateHistory, err := HistoryForInputPeerLayer229(member, input, 0, int32(time.Now().Unix()+3600), 0, 20, 0, 0)
	if err != nil || dateHistory == nil || dateHistory.GetCount() != 2 || len(dateHistory.GetMessages()) != 2 {
		t.Fatalf("layer229 date history: count=%d messages=%d err=%v", dateHistory.GetCount(), len(dateHistory.GetMessages()), err)
	}
	addOffsetHistory, err := HistoryForInputPeerLayer229(member, input, 0, 0, 1, 1, 0, 0)
	if err != nil || addOffsetHistory == nil || len(addOffsetHistory.GetMessages()) != 1 || addOffsetHistory.GetMessages()[0].GetId() != 1 {
		t.Fatalf("layer229 add_offset history: messages=%v err=%v", addOffsetHistory.GetMessages(), err)
	}

	editData, err := GetMessageEditData(owner, input, got.GetMessages()[0].GetId())
	if err != nil || editData == nil || editData.GetCaption() {
		t.Fatalf("creator edit data: result=%v err=%v", editData, err)
	}
	if _, err = GetMessageEditData(member, input, got.GetMessages()[0].GetId()); !errors.Is(err, mtproto.ErrChatAdminRequired) {
		t.Fatalf("non-creator edit data: got %v", err)
	}
	if _, err = GetMessageEditData(owner, input, got.GetMessages()[0].GetId()+10); !errors.Is(err, mtproto.ErrMessageIdInvalid) {
		t.Fatalf("missing message edit data: got %v", err)
	}

	badHash := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId: channelID, AccessHash: channelID + 1,
	}).To_InputPeer()
	if _, err = HistoryForInputPeer(member, badHash, 0, 20); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("bad access hash: got %v", err)
	}
	if _, err = HistoryForInputPeerLayer229(member, badHash, 0, 0, 0, 20, 0, 0); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("bad layer229 access hash: got %v", err)
	}
	if _, err = SearchForInputPeer(member, badHash, "stored", 0, 0, 0, 0, 0, 0, 0, 20); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("bad search access hash: got %v", err)
	}
	if _, err = GetMessageEditData(owner, badHash, 1); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("bad edit-data access hash: got %v", err)
	}
	if _, err = HistoryForInputPeer(outsider, input, 0, 20); !errors.Is(err, mtproto.ErrChannelPrivate) {
		t.Fatalf("private non-member history: got %v", err)
	}
	if _, err = HistoryForInputPeerLayer229(outsider, input, 0, 0, 0, 20, 0, 0); !errors.Is(err, mtproto.ErrChannelPrivate) {
		t.Fatalf("private non-member layer229 history: got %v", err)
	}
	if _, err = SearchForInputPeer(outsider, input, "stored", 0, 0, 0, 0, 0, 0, 0, 20); !errors.Is(err, mtproto.ErrChannelPrivate) {
		t.Fatalf("private non-member search: got %v", err)
	}
	if _, err = GetMessageEditData(outsider, input, 1); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("non-member edit data: got %v", err)
	}
	if _, err = ValidateInputPeer(outsider, input); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("strict member validation for non-member: got %v", err)
	}

	if err = domain.SaveChannel(domain.Channel{
		ID: channelID, AccessHash: channelID, Creator: owner, Title: "history-auth-test", Broadcast: true, Username: "public-history-auth-test",
	}); err != nil {
		t.Fatal(err)
	}
	publicHistory, err := HistoryForInputPeer(outsider, input, 0, 20)
	if err != nil || len(publicHistory.GetMessages()) != 2 || !publicHistory.GetChats()[0].GetLeft() {
		t.Fatalf("public non-member history: messages=%d chats=%+v err=%v", len(publicHistory.GetMessages()), publicHistory.GetChats(), err)
	}
	publicLayerHistory, err := HistoryForInputPeerLayer229(outsider, input, 0, 0, 0, 20, 0, 0)
	if err != nil || publicLayerHistory.GetPredicateName() != mtproto.Predicate_messages_channelMessages || len(publicLayerHistory.GetMessages()) != 2 || !publicLayerHistory.GetChats()[0].GetLeft() {
		t.Fatalf("public non-member layer229 history: predicate=%s messages=%d chats=%+v err=%v", publicLayerHistory.GetPredicateName(), len(publicLayerHistory.GetMessages()), publicLayerHistory.GetChats(), err)
	}
	publicSearch, err := SearchForInputPeer(outsider, input, "stored", 0, 0, 0, 0, 0, 0, 0, 20)
	if err != nil || len(publicSearch.GetMessages()) != 1 || !publicSearch.GetChats()[0].GetLeft() {
		t.Fatalf("public non-member search: messages=%d chats=%+v err=%v", len(publicSearch.GetMessages()), publicSearch.GetChats(), err)
	}
	if err = ValidateChannelMessageWrite(outsider, input); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("public non-member write validation: got %v", err)
	}
	if _, err = ReadHistoryForInputPeer(outsider, input, 1); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("public non-member read-history: got %v", err)
	}
	if _, err = MessageViewsForInputPeer(outsider, input, []int32{1}, false); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("public non-member message views: got %v", err)
	}

	if err = domain.EditChannelBanned(channelID, owner, outsider, &domain.ChannelBannedRights{ViewMessages: true}, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err = HistoryForInputPeer(outsider, input, 0, 20); !errors.Is(err, mtproto.ErrUserBannedInChannel) {
		t.Fatalf("public channel banned non-member history: got %v", err)
	}
}
