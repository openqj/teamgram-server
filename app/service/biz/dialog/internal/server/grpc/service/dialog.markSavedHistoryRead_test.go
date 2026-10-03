package service

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dao/mysql_dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/svc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestDialogMarkSavedHistoryReadRoundTrip(t *testing.T) {
	dsn := os.Getenv("SAVED_DIALOGS_AUDIT_MYSQL_DSN")
	if !strings.Contains(dsn, "127.0.0.1:13306") || (!strings.Contains(dsn, "/teamgram_audit?") && !strings.HasSuffix(dsn, "/teamgram_audit")) {
		t.Skip("requires SAVED_DIALOGS_AUDIT_MYSQL_DSN pointing at the isolated layer229-audit-mysql/teamgram_audit database")
	}

	ctx := context.Background()
	db := sqlx.NewMySQL(&sqlx.Config{DSN: dsn})
	svcCtx := &svc.ServiceContext{Dao: &dao.Dao{Mysql: &dao.Mysql{SavedDialogsDAO: mysql_dao.NewSavedDialogsDAO(db)}}}
	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	dialog.RegisterRPCDialogServer(grpcServer, New(svcCtx))
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	rpcClient := dialog.NewRPCDialogClient(conn)
	uid := time.Now().UnixNano()
	peerID := uid + 1
	const peerType int32 = mtproto.PEER_CHAT

	_, _, err = svcCtx.Dao.SavedDialogsDAO.InsertOrUpdate(ctx, &dataobject.SavedDialogsDO{
		UserId:     uid,
		PeerType:   peerType,
		PeerId:     peerID,
		TopMessage: 900,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(ctx, "delete from saved_dialogs where user_id = ? and peer_type = ? and peer_id = ?", uid, peerType, peerID)
	})

	request := func(maxID int32) *dialog.TLDialogInsertOrUpdateDialog {
		return &dialog.TLDialogInsertOrUpdateDialog{
			UserId:         uid,
			PeerType:       peerType,
			PeerId:         peerID,
			ReadInboxMaxId: wrapperspb.Int32(maxID),
		}
	}
	if result, err := rpcClient.DialogMarkSavedHistoryRead(ctx, request(450)); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("mark cursor = (%v, %v), want true", result, err)
	}
	if result, err := rpcClient.DialogMarkSavedHistoryRead(ctx, request(420)); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("repeat lower cursor = (%v, %v), want true", result, err)
	}

	var persisted struct {
		ReadMaxId int32 `db:"read_max_id"`
	}
	if err := db.QueryRowPartial(ctx, &persisted, "select read_max_id from saved_dialogs where user_id = ? and peer_type = ? and peer_id = ?", uid, peerType, peerID); err != nil {
		t.Fatal(err)
	}
	if persisted.ReadMaxId != 450 {
		t.Fatalf("persisted read_max_id = %d, want 450", persisted.ReadMaxId)
	}

	missing := request(77)
	missing.PeerId = peerID + 1
	if _, err := rpcClient.DialogMarkSavedHistoryRead(ctx, missing); status.Convert(err).Message() != "PEER_ID_INVALID" {
		t.Fatalf("missing saved-dialog error = %v, want PEER_ID_INVALID", err)
	}
	if _, err := rpcClient.DialogMarkSavedHistoryRead(ctx, &dialog.TLDialogInsertOrUpdateDialog{
		UserId:         uid,
		PeerType:       peerType,
		PeerId:         peerID,
		ReadInboxMaxId: wrapperspb.Int32(-1),
	}); status.Convert(err).Message() != "MESSAGE_ID_INVALID" {
		t.Fatalf("negative cursor error = %v, want MESSAGE_ID_INVALID", err)
	}
}
