package core

import (
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestReportsRejectMalformedRequests(t *testing.T) {
	const uid int64 = 81003
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	ok, err := c.AccountReportPeer(&mtproto.TLAccountReportPeer{})
	if ok != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("report: %v %#v, want nil result and INPUT_REQUEST_INVALID", err, ok)
	}
	res, err := c.MessagesReportFC78AF9B(&mtproto.TLMessagesReportFC78AF9B{Id: []int32{0}})
	if res != nil || !errors.Is(err, mtproto.ErrMessageIdInvalid) {
		t.Fatalf("result: %#v %v, want nil result and MESSAGE_ID_INVALID", res, err)
	}
}

func TestReportHandlersPersistAllSupportedMethods(t *testing.T) {
	db, err := persist.OpenPostgresDB(isolatedAuditDSN(t))
	if err != nil {
		t.Fatalf("open report cleanup database: %v", err)
	}
	uid := time.Now().UnixNano()
	t.Cleanup(func() {
		if _, err := db.Exec(`DELETE FROM apifull_report WHERE actor_user_id=$1`, uid); err != nil {
			t.Errorf("clean report fixture: %v", err)
		}
		_ = db.Close()
	})
	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	peerUser := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: uid + 1}).To_InputPeer()
	peerChat := mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: uid + 2}).To_InputPeer()
	peerChannel := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: uid + 3}).To_InputPeer()
	reason := mtproto.MakeTLInputReportReasonSpam(nil).To_ReportReason()

	if got, err := core.AccountReportPeer(&mtproto.TLAccountReportPeer{Peer: peerUser, Reason: reason}); got == nil || err != nil {
		t.Fatalf("account.reportPeer = (%v, %v)", got, err)
	}
	// Telegram retries must acknowledge the same report without creating a
	// second moderation intake row.
	if got, err := core.AccountReportPeer(&mtproto.TLAccountReportPeer{Peer: peerUser, Reason: reason}); got == nil || err != nil {
		t.Fatalf("account.reportPeer retry = (%v, %v)", got, err)
	}
	if got, err := core.AccountReportProfilePhoto(&mtproto.TLAccountReportProfilePhoto{Peer: peerUser, PhotoId: mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{Id: uid + 4}).To_InputPhoto(), Reason: reason}); got == nil || err != nil {
		t.Fatalf("account.reportProfilePhoto = (%v, %v)", got, err)
	}
	if got, err := core.MessagesReportSpam(&mtproto.TLMessagesReportSpam{Peer: peerChat}); got == nil || err != nil {
		t.Fatalf("messages.reportSpam = (%v, %v)", got, err)
	}
	if got, err := core.MessagesReportFC78AF9B(&mtproto.TLMessagesReportFC78AF9B{Peer: peerUser, Id: []int32{1, 2}, Option: []byte("spam")}); got == nil || err != nil || got.GetPredicateName() != mtproto.Predicate_reportResultReported {
		t.Fatalf("messages.report = (%v, %v)", got, err)
	}
	if got, err := core.MessagesReportEncryptedSpam(&mtproto.TLMessagesReportEncryptedSpam{Peer: mtproto.MakeTLInputEncryptedChat(&mtproto.InputEncryptedChat{ChatId: int32(uid & 0x3fffffff)}).To_InputEncryptedChat()}); got == nil || err != nil {
		t.Fatalf("messages.reportEncryptedSpam = (%v, %v)", got, err)
	}
	if got, err := core.MessagesReportReadMetrics(&mtproto.TLMessagesReportReadMetrics{Peer: peerUser, Metrics: []*mtproto.InputMessageReadMetric{{MsgId: 3, ViewId: 4, TimeInViewMs: 5}}}); got == nil || err != nil {
		t.Fatalf("messages.reportReadMetrics = (%v, %v)", got, err)
	}
	if got, err := core.MessagesReportMusicListen(&mtproto.TLMessagesReportMusicListen{Id: mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: uid + 5}).To_InputDocument(), ListenedDuration: 2}); got == nil || err != nil {
		t.Fatalf("messages.reportMusicListen = (%v, %v)", got, err)
	}
	if got, err := core.ChannelsReportSpam(&mtproto.TLChannelsReportSpam{Channel: mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: uid + 3}).To_InputChannel(), Participant: peerUser, Id: []int32{6}}); got == nil || err != nil {
		t.Fatalf("channels.reportSpam = (%v, %v)", got, err)
	}
	if got, err := core.MessagesReport8953AB4E(&mtproto.TLMessagesReport8953AB4E{Peer: peerChannel, Id: []int32{7}, Reason: reason}); got == nil || err != nil {
		t.Fatalf("messages.report8953AB4E = (%v, %v)", got, err)
	}
	if got, err := core.StoriesReport19D8EB45(&mtproto.TLStoriesReport19D8EB45{Peer: peerUser, Id: []int32{8}, Option: []byte("spam")}); got == nil || err != nil || got.GetPredicateName() != mtproto.Predicate_reportResultReported {
		t.Fatalf("stories.report = (%v, %v)", got, err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM apifull_report WHERE actor_user_id=$1`, uid).Scan(&count); err != nil {
		t.Fatalf("count persisted reports: %v", err)
	}
	if count != 10 {
		t.Fatalf("persisted report count = %d, want 10", count)
	}
	var peerCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM apifull_report WHERE actor_user_id=$1 AND kind=$2`, uid, "account.reportPeer").Scan(&peerCount); err != nil {
		t.Fatalf("count persisted account reports: %v", err)
	}
	if peerCount != 1 {
		t.Fatalf("persisted account.reportPeer count = %d, want 1 after retry", peerCount)
	}
	if !domain.Ready() {
		t.Fatal("domain should remain ready after report writes")
	}
}
