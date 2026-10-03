package core_test

import (
	"context"
	"database/sql"
	"net"
	"os"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dao/mysql_dao"
	bizdao "github.com/teamgram/teamgram-server/app/service/biz/message/internal/dao"
	message_service "github.com/teamgram/teamgram-server/app/service/biz/message/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/svc"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func startAuditMessageService(t *testing.T, serviceContext *svc.ServiceContext) messagepb.RPCMessageClient {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	messagepb.RegisterRPCMessageServer(server, message_service.New(serviceContext))
	go func() {
		_ = server.Serve(listener)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		server.Stop()
		_ = listener.Close()
		t.Fatalf("connect in-process message service: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		server.Stop()
		_ = listener.Close()
	})
	return messagepb.NewRPCMessageClient(conn)
}

func TestMessageSearchByMediaTypeAuditMySQL(t *testing.T) {
	dsn := os.Getenv("MESSAGE_AUDIT_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set MESSAGE_AUDIT_MYSQL_DSN to the isolated message audit database")
	}

	parsed, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse MESSAGE_AUDIT_MYSQL_DSN: %v", err)
	}
	if parsed.Net != "tcp" || parsed.Addr != "127.0.0.1:3306" || parsed.DBName != "teamgram_message_audit" {
		t.Fatalf("refusing non-isolated MySQL target: network=%q address=%q database=%q", parsed.Net, parsed.Addr, parsed.DBName)
	}

	adminDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer adminDB.Close()
	var database string
	if err = adminDB.QueryRow(`SELECT DATABASE()`).Scan(&database); err != nil {
		t.Fatal(err)
	}
	if database != "teamgram_message_audit" {
		t.Fatalf("connected to database %q, want isolated message audit database", database)
	}

	_, err = adminDB.Exec(`CREATE TABLE IF NOT EXISTS messages (
		id BIGINT NOT NULL AUTO_INCREMENT,
		user_id BIGINT NOT NULL,
		user_message_box_id INT NOT NULL,
		dialog_id1 BIGINT NOT NULL,
		dialog_id2 BIGINT NOT NULL,
		dialog_message_id BIGINT NOT NULL,
		sender_user_id BIGINT NOT NULL,
		peer_type INT NOT NULL,
		peer_id BIGINT NOT NULL,
		random_id BIGINT NOT NULL,
		message_filter_type INT NOT NULL,
		message_data MEDIUMTEXT NOT NULL,
		message TEXT NOT NULL,
		mentioned TINYINT NOT NULL DEFAULT 0,
		media_unread TINYINT NOT NULL DEFAULT 0,
		pinned TINYINT NOT NULL DEFAULT 0,
		has_reaction TINYINT NOT NULL DEFAULT 0,
		reaction TEXT NOT NULL,
		reaction_date BIGINT NOT NULL DEFAULT 0,
		reaction_unread TINYINT NOT NULL DEFAULT 0,
		saved_peer_type INT NOT NULL DEFAULT 0,
		saved_peer_id BIGINT NOT NULL DEFAULT 0,
		date2 BIGINT NOT NULL DEFAULT 0,
		ttl_period INT NOT NULL DEFAULT 0,
		deleted TINYINT NOT NULL DEFAULT 0,
		PRIMARY KEY (id),
		KEY idx_message_filter_fixture (user_id, dialog_id1, dialog_id2, message_filter_type, user_message_box_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err != nil {
		t.Fatal(err)
	}

	userID := time.Now().UnixNano()%1_000_000_000 + 8_000_000_000
	peerID := userID + 1
	dialogID := mtproto.MakeDialogId(userID, mtproto.PEER_USER, peerID)
	defer func() {
		_, _ = adminDB.Exec(`DELETE FROM messages WHERE user_id = ?`, userID)
	}()

	fixtures := []struct {
		id        int32
		mediaType int32
	}{
		{id: 501, mediaType: mtproto.MEDIA_VOICE_FILE},
		{id: 502, mediaType: mtproto.MEDIA_ROUND_FILE},
		{id: 503, mediaType: mtproto.MEDIA_FILE},
	}
	for _, fixture := range fixtures {
		_, err = adminDB.Exec(`INSERT INTO messages
			(user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id, sender_user_id,
			 peer_type, peer_id, random_id, message_filter_type, message_data, message, reaction)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '{}', '', '')`,
			userID, fixture.id, dialogID.A, dialogID.B, fixture.id, peerID,
			mtproto.PEER_USER, peerID, int64(fixture.id), fixture.mediaType)
		if err != nil {
			t.Fatal(err)
		}
	}

	messageDB := sqlx.NewMySQL(&sqlx.Config{DSN: dsn})
	messageDAO := mysql_dao.NewMessagesDAO(messageDB, 1)
	messageClient := startAuditMessageService(t, &svc.ServiceContext{Dao: &bizdao.Dao{
		Mysql: &bizdao.Mysql{DB: messageDB, MessagesDAO: messageDAO},
	}})

	for _, tt := range []struct {
		mediaType int32
		wantID    int32
	}{
		{mediaType: mtproto.MEDIA_VOICE_FILE, wantID: 501},
		{mediaType: mtproto.MEDIA_ROUND_FILE, wantID: 502},
	} {
		got, searchErr := messageClient.MessageSearchByMediaType(context.Background(), &messagepb.TLMessageSearchByMediaType{
			UserId: userID, PeerType: mtproto.PEER_USER, PeerId: peerID,
			MediaType: tt.mediaType, Offset: int32(^uint32(0) >> 1), Limit: 50,
		})
		if searchErr != nil {
			t.Fatalf("search media type %d: %v", tt.mediaType, searchErr)
		}
		if got == nil || len(got.GetBoxList()) != 1 || got.GetBoxList()[0].GetMessageId() != tt.wantID {
			t.Fatalf("search media type %d = %+v, want only message %d", tt.mediaType, got, tt.wantID)
		}
	}

	var missingDBCount int
	if err = adminDB.QueryRow(`SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = 'teamgram_message_missing_audit'`).Scan(&missingDBCount); err != nil {
		t.Fatal(err)
	}
	if missingDBCount != 0 {
		t.Fatal("refusing to use pre-existing message error-path database")
	}
	missingConfig := *parsed
	missingConfig.DBName = "teamgram_message_missing_audit"
	missingDSN := missingConfig.FormatDSN()
	missingDB := sqlx.NewMySQL(&sqlx.Config{DSN: missingDSN})
	missingClient := startAuditMessageService(t, &svc.ServiceContext{Dao: &bizdao.Dao{
		Mysql: &bizdao.Mysql{DB: missingDB, MessagesDAO: mysql_dao.NewMessagesDAO(missingDB, 1)},
	}})
	_, err = missingClient.MessageSearchByMediaType(context.Background(), &messagepb.TLMessageSearchByMediaType{
		UserId: userID, PeerType: mtproto.PEER_USER, PeerId: peerID,
		MediaType: mtproto.MEDIA_VOICE_FILE, Offset: int32(^uint32(0) >> 1), Limit: 50,
	})
	if err == nil {
		t.Fatal("search against absent isolated test schema returned success")
	}
}
