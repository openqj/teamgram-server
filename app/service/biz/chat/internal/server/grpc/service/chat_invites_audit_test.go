package service

import (
	"context"
	"database/sql"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	marmota_cache "github.com/teamgram/marmota/pkg/stores/cache"
	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	rpcmetadata "github.com/teamgram/proto/mtproto/rpc/metadata"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dao/mysql_dao"
	chatdao "github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/svc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const chatInviteAuditDSNEnv = "TEAMGRAM_CHAT_INVITE_AUDIT_DSN"

type chatInviteAuditCache struct {
	marmota_cache.BatchCache
}

func (c *chatInviteAuditCache) TakeCtx(_ context.Context, value any, _ string, query func(any) error) error {
	return query(value)
}

func (c *chatInviteAuditCache) DelCtx(context.Context, ...string) error {
	return nil
}

func TestChatInviteExportReadbackOverIsolatedRPC(t *testing.T) {
	dsn := os.Getenv(chatInviteAuditDSNEnv)
	if dsn == "" {
		t.Skipf("set %s to the isolated layer229-audit-mysql/teamgram_audit database", chatInviteAuditDSNEnv)
	}

	dsnConfig, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal("parse isolated audit MySQL DSN:", err)
	}
	if dsnConfig.DBName != "teamgram_audit" || dsnConfig.Net != "tcp" || dsnConfig.Addr != "127.0.0.1:13306" {
		t.Fatalf("refusing non-isolated MySQL target: database=%q address=%q", dsnConfig.DBName, dsnConfig.Addr)
	}

	adminDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal("open isolated audit MySQL:", err)
	}
	defer adminDB.Close()
	if err := adminDB.Ping(); err != nil {
		t.Fatal("connect isolated audit MySQL:", err)
	}
	var databaseName string
	if err := adminDB.QueryRow("SELECT DATABASE()").Scan(&databaseName); err != nil {
		t.Fatal("verify selected audit database:", err)
	}
	if databaseName != "teamgram_audit" {
		t.Fatalf("connected database = %q, want teamgram_audit", databaseName)
	}
	t.Logf("verified isolated database target 127.0.0.1:13306/%s; gRPC server uses bufconn", databaseName)

	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS chats (
			id BIGINT NOT NULL PRIMARY KEY,
			creator_user_id BIGINT NOT NULL,
			access_hash BIGINT NOT NULL DEFAULT 0,
			random_id BIGINT NOT NULL DEFAULT 0,
			participant_count INT NOT NULL DEFAULT 0,
			title VARCHAR(255) NOT NULL DEFAULT '',
			about VARCHAR(255) NOT NULL DEFAULT '',
			photo_id BIGINT NOT NULL DEFAULT 0,
			default_banned_rights BIGINT NOT NULL DEFAULT 0,
			migrated_to_id BIGINT NOT NULL DEFAULT 0,
			migrated_to_access_hash BIGINT NOT NULL DEFAULT 0,
		noforwards BOOLEAN NOT NULL DEFAULT FALSE,
			available_reactions_type INT NOT NULL DEFAULT 0,
			available_reactions VARCHAR(4096) NOT NULL DEFAULT '',
			deactivated BOOLEAN NOT NULL DEFAULT FALSE,
			ttl_period INT NOT NULL DEFAULT 0,
			version INT NOT NULL DEFAULT 1,
			date BIGINT NOT NULL DEFAULT 0
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS chat_participants (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			chat_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			participant_type INT NOT NULL DEFAULT 0,
			link VARCHAR(64) NOT NULL DEFAULT '',
			usage2 INT NOT NULL DEFAULT 0,
			admin_rights INT NOT NULL DEFAULT 0,
			inviter_user_id BIGINT NOT NULL DEFAULT 0,
			invited_at BIGINT NOT NULL DEFAULT 0,
			kicked_at BIGINT NOT NULL DEFAULT 0,
			left_at BIGINT NOT NULL DEFAULT 0,
			groupcall_default_join_as_peer_type INT NOT NULL DEFAULT 0,
			groupcall_default_join_as_peer_id BIGINT NOT NULL DEFAULT 0,
			is_bot BOOLEAN NOT NULL DEFAULT FALSE,
			state INT NOT NULL DEFAULT 0,
			date2 BIGINT NOT NULL DEFAULT 0,
			rank2 VARCHAR(32) NOT NULL DEFAULT ''
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS chat_invites (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			chat_id BIGINT NOT NULL,
			admin_id BIGINT NOT NULL,
			link VARCHAR(64) NOT NULL,
			permanent BOOLEAN NOT NULL DEFAULT FALSE,
			revoked BOOLEAN NOT NULL DEFAULT FALSE,
			request_needed BOOLEAN NOT NULL DEFAULT FALSE,
			start_date BIGINT NOT NULL DEFAULT 0,
			expire_date BIGINT NOT NULL DEFAULT 0,
			usage_limit INT NOT NULL DEFAULT 0,
			usage2 INT NOT NULL DEFAULT 0,
			requested INT NOT NULL DEFAULT 0,
			title VARCHAR(64) NOT NULL DEFAULT '',
			date2 BIGINT NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS chat_invite_participants (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			chat_id BIGINT NOT NULL,
			link VARCHAR(64) NOT NULL,
			user_id BIGINT NOT NULL,
			requested INT NOT NULL DEFAULT 0,
			approved_by BIGINT NOT NULL DEFAULT 0,
			date2 BIGINT NOT NULL DEFAULT 0
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS users (
			id BIGINT NOT NULL PRIMARY KEY,
			username VARCHAR(64) NOT NULL DEFAULT '',
			first_name VARCHAR(64) NOT NULL DEFAULT '',
			last_name VARCHAR(64) NOT NULL DEFAULT '',
			deleted BOOLEAN NOT NULL DEFAULT FALSE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	} {
		if _, err := adminDB.Exec(statement); err != nil {
			t.Fatal("prepare isolated invite audit schema:", err)
		}
	}

	chatID := int64(1_000_000_000_000 + time.Now().UnixNano()%1_000_000_000)
	inactiveChatID := chatID + 10
	creatorID, memberID, outsiderID, adminID := chatID+1, chatID+2, chatID+3, chatID+4
	usernameMatchID, nameMatchID, nonMatchID := chatID+20, chatID+21, chatID+22
	defer func() {
		for _, id := range []int64{usernameMatchID, nameMatchID, nonMatchID} {
			_, _ = adminDB.Exec("DELETE FROM users WHERE id = ?", id)
		}
		for _, id := range []int64{chatID, inactiveChatID} {
			for _, query := range []string{
				"DELETE FROM chat_invites WHERE chat_id = ?",
				"DELETE FROM chat_invite_participants WHERE chat_id = ?",
				"DELETE FROM chat_participants WHERE chat_id = ?",
				"DELETE FROM chats WHERE id = ?",
			} {
				_, _ = adminDB.Exec(query, id)
			}
		}
	}()
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{"INSERT INTO chats (id, creator_user_id, title) VALUES (?, ?, ?)", []any{chatID, creatorID, "Layer229 audit invite"}},
		{"INSERT INTO chat_participants (chat_id, user_id, participant_type, state) VALUES (?, ?, ?, 0)", []any{chatID, creatorID, mtproto.ChatMemberCreator}},
		{"INSERT INTO chat_participants (chat_id, user_id, participant_type, state) VALUES (?, ?, ?, 0)", []any{chatID, memberID, mtproto.ChatMemberNormal}},
		{"INSERT INTO chat_participants (chat_id, user_id, participant_type, state) VALUES (?, ?, ?, 0)", []any{chatID, adminID, mtproto.ChatMemberAdmin}},
		{"INSERT INTO users (id, username, first_name, last_name) VALUES (?, ?, ?, ?)", []any{usernameMatchID, "alice", "Nora", ""}},
		{"INSERT INTO users (id, username, first_name, last_name) VALUES (?, ?, ?, ?)", []any{nameMatchID, "zzali", "Malik", ""}},
		{"INSERT INTO users (id, username, first_name, last_name) VALUES (?, ?, ?, ?)", []any{nonMatchID, "zzali", "Nora", ""}},
		{"INSERT INTO chat_invite_participants (chat_id, link, user_id, requested, approved_by, date2) VALUES (?, '', ?, 1, 0, 100)", []any{chatID, usernameMatchID}},
		{"INSERT INTO chat_invite_participants (chat_id, link, user_id, requested, approved_by, date2) VALUES (?, '', ?, 1, 0, 101)", []any{chatID, nameMatchID}},
		{"INSERT INTO chat_invite_participants (chat_id, link, user_id, requested, approved_by, date2) VALUES (?, '', ?, 1, 0, 102)", []any{chatID, nonMatchID}},
		{"INSERT INTO chats (id, creator_user_id, title) VALUES (?, ?, ?)", []any{inactiveChatID, creatorID, "Layer229 audit invite left"}},
		{"INSERT INTO chat_participants (chat_id, user_id, participant_type, state) VALUES (?, ?, ?, 1)", []any{inactiveChatID, memberID, mtproto.ChatMemberNormal}},
	} {
		if _, err := adminDB.Exec(statement.query, statement.args...); err != nil {
			t.Fatal("seed isolated invite workflow:", err)
		}
	}
	wrappedDB, err := sqlx.Open(&sqlx.Config{DSN: dsn})
	if err != nil {
		t.Fatal("open chat service audit DAO:", err)
	}
	chatMysql := &chatdao.Mysql{
		DB:                        wrappedDB,
		ChatInviteParticipantsDAO: mysql_dao.NewChatInviteParticipantsDAO(wrappedDB),
		ChatInvitesDAO:            mysql_dao.NewChatInvitesDAO(wrappedDB),
		ChatParticipantsDAO:       mysql_dao.NewChatParticipantsDAO(wrappedDB),
		ChatsDAO:                  mysql_dao.NewChatsDAO(wrappedDB),
		CommonDAO:                 sqlx.NewCommonDAO(wrappedDB),
	}
	serviceContext := &svc.ServiceContext{Dao: &chatdao.Dao{
		Mysql:      chatMysql,
		CachedConn: sqlc.NewConnWithCache(wrappedDB, &chatInviteAuditCache{}),
	}}

	bufListener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	chatpb.RegisterRPCChatServer(grpcServer, New(serviceContext))
	go func() { _ = grpcServer.Serve(bufListener) }()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = bufListener.Close()
	})

	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, err := grpc.DialContext(dialCtx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return bufListener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatal("dial isolated chat gRPC server:", err)
	}
	defer connection.Close()
	client := chatpb.NewRPCChatClient(connection)

	memberChats, err := client.ChatSearch(auditInviteContext(t, memberID), &chatpb.TLChatSearch{
		SelfId: memberID,
		Q:      "Layer229 audit invite",
		Limit:  10,
	})
	if err != nil {
		t.Fatal("active member search:", err)
	}
	if len(memberChats.GetDatas()) != 1 || memberChats.GetDatas()[0].GetChat().GetId() != chatID {
		t.Fatalf("active member search returned %+v, want only chat %d", memberChats.GetDatas(), chatID)
	}
	outsiderChats, err := client.ChatSearch(auditInviteContext(t, outsiderID), &chatpb.TLChatSearch{
		SelfId: outsiderID,
		Q:      "Layer229 audit invite",
		Limit:  10,
	})
	if err != nil {
		t.Fatal("outsider search:", err)
	}
	if len(outsiderChats.GetDatas()) != 0 {
		t.Fatalf("outsider search returned %+v, want no chats", outsiderChats.GetDatas())
	}
	importers, err := client.ChatGetChatInviteImporters(auditInviteContext(t, creatorID), &chatpb.TLChatGetChatInviteImporters{
		SelfId:    creatorID,
		ChatId:    chatID,
		Requested: true,
		Q:         wrapperspb.String("ali"),
		Limit:     10,
	})
	if err != nil {
		t.Fatal("search chat invite importers:", err)
	}
	gotImporterIDs := make(map[int64]bool, len(importers.GetDatas()))
	for _, importer := range importers.GetDatas() {
		gotImporterIDs[importer.GetUserId()] = true
	}
	if len(gotImporterIDs) != 2 || !gotImporterIDs[usernameMatchID] || !gotImporterIDs[nameMatchID] {
		t.Fatalf("importer search returned user IDs %v, want username-prefix and name-substring matches only", gotImporterIDs)
	}
	shortQueryImporters, err := client.ChatGetChatInviteImporters(auditInviteContext(t, creatorID), &chatpb.TLChatGetChatInviteImporters{
		SelfId:    creatorID,
		ChatId:    chatID,
		Requested: true,
		Q:         wrapperspb.String("al"),
		Limit:     10,
	})
	if err != nil {
		t.Fatal("search chat invite importers with short query:", err)
	}
	if len(shortQueryImporters.GetDatas()) != 0 {
		t.Fatalf("short importer search returned %+v, want no matches", shortQueryImporters.GetDatas())
	}

	title := "audit invitation"
	exported, err := client.ChatExportChatInvite(auditInviteContext(t, creatorID), &chatpb.TLChatExportChatInvite{
		ChatId:        chatID,
		AdminId:       creatorID,
		RequestNeeded: true,
		UsageLimit:    wrapperspb.Int32(3),
		Title:         wrapperspb.String(title),
	})
	if err != nil {
		t.Fatal("creator export invite:", err)
	}
	if exported.GetLink() == "" {
		t.Fatal("creator export returned an empty invite link")
	}

	var (
		storedAdminID int64
		storedLink    string
		storedTitle   string
		storedRequest bool
		storedLimit   int32
	)
	if err := adminDB.QueryRow(`SELECT admin_id, link, title, request_needed, usage_limit
		FROM chat_invites WHERE chat_id = ? AND admin_id = ? AND revoked = 0`, chatID, creatorID).
		Scan(&storedAdminID, &storedLink, &storedTitle, &storedRequest, &storedLimit); err != nil {
		t.Fatal("read persisted invite row:", err)
	}
	if storedAdminID != creatorID || storedTitle != title || !storedRequest || storedLimit != 3 {
		t.Fatalf("persisted invite = admin:%d title:%q request:%t limit:%d", storedAdminID, storedTitle, storedRequest, storedLimit)
	}

	listed, err := client.ChatGetExportedChatInvites(auditInviteContext(t, creatorID), &chatpb.TLChatGetExportedChatInvites{
		ChatId:  chatID,
		AdminId: creatorID,
		Limit:   10,
	})
	if err != nil {
		t.Fatal("creator list exported invites:", err)
	}
	if len(listed.GetDatas()) != 1 {
		t.Fatalf("list returned %d invite(s), want 1", len(listed.GetDatas()))
	}
	if got := listed.GetDatas()[0]; got.GetLink() != exported.GetLink() || got.GetTitle().GetValue() != title {
		t.Fatalf("list readback = link:%q title:%q, want the exported link and title %q", got.GetLink(), got.GetTitle().GetValue(), title)
	}
	if storedLink == "" || !strings.HasSuffix(exported.GetLink(), "+"+storedLink) {
		t.Fatalf("returned link %q does not correspond to persisted hash", exported.GetLink())
	}

	_, err = client.ChatExportChatInvite(auditInviteContext(t, memberID), &chatpb.TLChatExportChatInvite{
		ChatId:  chatID,
		AdminId: memberID,
	})
	assertInviteRPCError(t, err, "CHAT_ADMIN_REQUIRED")
	_, err = client.ChatExportChatInvite(auditInviteContext(t, outsiderID), &chatpb.TLChatExportChatInvite{
		ChatId:  chatID,
		AdminId: creatorID,
	})
	assertInviteRPCError(t, err, "USER_ID_INVALID")
	_, err = client.ChatExportChatInvite(context.Background(), &chatpb.TLChatExportChatInvite{
		ChatId:  chatID,
		AdminId: creatorID,
	})
	assertInviteRPCError(t, err, "USER_ID_INVALID")
	_, err = client.ChatGetExportedChatInvites(auditInviteContext(t, memberID), &chatpb.TLChatGetExportedChatInvites{
		ChatId:  chatID,
		AdminId: creatorID,
	})
	assertInviteRPCError(t, err, "CHAT_ADMIN_REQUIRED")
	_, err = client.ChatGetExportedChatInvites(auditInviteContext(t, adminID), &chatpb.TLChatGetExportedChatInvites{
		ChatId:  chatID,
		AdminId: creatorID,
	})
	assertInviteRPCError(t, err, "CHAT_ADMIN_REQUIRED")
	_, err = client.ChatGetExportedChatInvites(auditInviteContext(t, outsiderID), &chatpb.TLChatGetExportedChatInvites{
		ChatId:  chatID,
		AdminId: creatorID,
	})
	assertInviteRPCError(t, err, "USER_NOT_PARTICIPANT")
	_, err = client.ChatGetExportedChatInvites(context.Background(), &chatpb.TLChatGetExportedChatInvites{
		ChatId:  chatID,
		AdminId: creatorID,
	})
	assertInviteRPCError(t, err, "USER_ID_INVALID")

	checked, err := client.ChatCheckChatInvite(auditInviteContext(t, outsiderID), &chatpb.TLChatCheckChatInvite{
		SelfId: outsiderID,
		Hash:   storedLink,
	})
	if err != nil {
		t.Fatal("check exported invite:", err)
	}
	if !checked.GetRequestNeeded() {
		t.Fatal("check exported invite did not preserve request_needed")
	}
	_, err = client.ChatCheckChatInvite(auditInviteContext(t, memberID), &chatpb.TLChatCheckChatInvite{
		SelfId: outsiderID,
		Hash:   storedLink,
	})
	assertInviteRPCError(t, err, "USER_ID_INVALID")

	_, err = client.ChatGetExportedChatInvite(auditInviteContext(t, memberID), &chatpb.TLChatGetExportedChatInvite{
		ChatId: chatID,
		Link:   exported.GetLink(),
	})
	assertInviteRPCError(t, err, "CHAT_ADMIN_REQUIRED")
	_, err = client.ChatGetExportedChatInvite(auditInviteContext(t, outsiderID), &chatpb.TLChatGetExportedChatInvite{
		ChatId: chatID,
		Link:   exported.GetLink(),
	})
	assertInviteRPCError(t, err, "USER_NOT_PARTICIPANT")
	readInvite, err := client.ChatGetExportedChatInvite(auditInviteContext(t, creatorID), &chatpb.TLChatGetExportedChatInvite{
		ChatId: chatID,
		Link:   exported.GetLink(),
	})
	if err != nil {
		t.Fatal("creator read exported invite:", err)
	}
	if readInvite.GetLink() != exported.GetLink() {
		t.Fatalf("single invite readback link = %q, want %q", readInvite.GetLink(), exported.GetLink())
	}
	_, err = client.ChatGetExportedChatInvite(auditInviteContext(t, creatorID), &chatpb.TLChatGetExportedChatInvite{
		ChatId: chatID,
		Link:   "https://t.me/+abcdefghijklmnopqrst",
	})
	assertInviteRPCError(t, err, "INVITE_HASH_INVALID")
	_, err = client.ChatGetExportedChatInvites(auditInviteContext(t, creatorID), &chatpb.TLChatGetExportedChatInvites{
		ChatId:  chatID,
		AdminId: creatorID,
		Limit:   -1,
	})
	assertInviteRPCError(t, err, "LIMIT_INVALID")
	_, err = client.ChatGetChatInviteImporters(auditInviteContext(t, creatorID), &chatpb.TLChatGetChatInviteImporters{
		SelfId: creatorID,
		ChatId: chatID,
		Limit:  -1,
	})
	assertInviteRPCError(t, err, "LIMIT_INVALID")

	_, err = client.ChatImportChatInvite2(auditInviteContext(t, memberID), &chatpb.TLChatImportChatInvite2{
		SelfId: outsiderID,
		Hash:   storedLink,
	})
	assertInviteRPCError(t, err, "USER_ID_INVALID")

	requested, err := client.ChatImportChatInvite2(auditInviteContext(t, outsiderID), &chatpb.TLChatImportChatInvite2{
		SelfId: outsiderID,
		Hash:   storedLink,
	})
	if err != nil {
		t.Fatal("request join through invite:", err)
	}
	if !containsInviteRequester(requested.GetRequesters(), outsiderID) {
		t.Fatalf("join request readback = %+v, want requester %d", requested.GetRequesters(), outsiderID)
	}
	retried, err := client.ChatImportChatInvite2(auditInviteContext(t, outsiderID), &chatpb.TLChatImportChatInvite2{
		SelfId: outsiderID,
		Hash:   storedLink,
	})
	if err != nil {
		t.Fatal("retry join request through invite:", err)
	}
	if countInviteRequester(retried.GetRequesters(), outsiderID) != 1 {
		t.Fatalf("retried join request readback = %+v, want one requester %d", retried.GetRequesters(), outsiderID)
	}

	importers, err = client.ChatGetChatInviteImporters(auditInviteContext(t, creatorID), &chatpb.TLChatGetChatInviteImporters{
		SelfId:    creatorID,
		ChatId:    chatID,
		Requested: true,
		Link:      wrapperspb.String(exported.GetLink()),
		Limit:     10,
	})
	if err != nil {
		t.Fatal("read requested importer:", err)
	}
	if !containsInviteImporter(importers.GetDatas(), outsiderID) {
		t.Fatalf("requested importers = %+v, want requester %d", importers.GetDatas(), outsiderID)
	}
	_, err = client.ChatGetChatInviteImporters(auditInviteContext(t, memberID), &chatpb.TLChatGetChatInviteImporters{
		SelfId:    creatorID,
		ChatId:    chatID,
		Requested: true,
		Link:      wrapperspb.String(exported.GetLink()),
		Limit:     10,
	})
	assertInviteRPCError(t, err, "USER_ID_INVALID")

	remaining, err := client.ChatHideChatJoinRequests(auditInviteContext(t, creatorID), &chatpb.TLChatHideChatJoinRequests{
		SelfId:   creatorID,
		ChatId:   chatID,
		Approved: false,
		Link:     wrapperspb.String(exported.GetLink()),
		UserId:   wrapperspb.Int64(outsiderID),
	})
	if err != nil {
		t.Fatal("reject join request:", err)
	}
	if containsInviteRequester(remaining, outsiderID) {
		t.Fatalf("rejected requester remained pending: %+v", remaining)
	}
	var outsiderParticipantCount int
	if err := adminDB.QueryRow(`SELECT COUNT(*) FROM chat_participants
		WHERE chat_id = ? AND user_id = ? AND state = 0`, chatID, outsiderID).Scan(&outsiderParticipantCount); err != nil {
		t.Fatal("read rejected requester membership:", err)
	}
	if outsiderParticipantCount != 0 {
		t.Fatalf("rejected requester became a participant %d time(s)", outsiderParticipantCount)
	}

	remaining, err = client.ChatHideChatJoinRequests(auditInviteContext(t, creatorID), &chatpb.TLChatHideChatJoinRequests{
		SelfId:   creatorID,
		ChatId:   chatID,
		Approved: true,
		Link:     wrapperspb.String(exported.GetLink()),
		UserId:   wrapperspb.Int64(outsiderID),
	})
	if err != nil {
		t.Fatal("ignore missing join request:", err)
	}
	if containsInviteRequester(remaining, outsiderID) {
		t.Fatalf("missing requester appeared pending: %+v", remaining)
	}
	if err := adminDB.QueryRow(`SELECT COUNT(*) FROM chat_participants
		WHERE chat_id = ? AND user_id = ? AND state = 0`, chatID, outsiderID).Scan(&outsiderParticipantCount); err != nil {
		t.Fatal("read missing requester membership:", err)
	}
	if outsiderParticipantCount != 0 {
		t.Fatalf("missing requester became a participant %d time(s)", outsiderParticipantCount)
	}

	if _, err = client.ChatEditExportedChatInvite(auditInviteContext(t, creatorID), &chatpb.TLChatEditExportedChatInvite{
		SelfId:  creatorID,
		ChatId:  chatID,
		Link:    exported.GetLink(),
		Revoked: true,
	}); err != nil {
		t.Fatal("revoke exported invite:", err)
	}
	_, err = client.ChatImportChatInvite(auditInviteContext(t, outsiderID), &chatpb.TLChatImportChatInvite{
		SelfId: outsiderID,
		Hash:   storedLink,
	})
	assertInviteRPCError(t, err, "INVITE_HASH_EXPIRED")

	var inviteCount int
	if err := adminDB.QueryRow("SELECT COUNT(*) FROM chat_invites WHERE chat_id = ?", chatID).Scan(&inviteCount); err != nil {
		t.Fatal("read invite count after rejected operations:", err)
	}
	if inviteCount != 1 {
		t.Fatalf("rejected operations changed invite count to %d, want 1", inviteCount)
	}
}

func containsInviteRequester(requesters *chatpb.RecentChatInviteRequesters, userID int64) bool {
	return countInviteRequester(requesters, userID) > 0
}

func countInviteRequester(requesters *chatpb.RecentChatInviteRequesters, userID int64) int {
	if requesters == nil {
		return 0
	}
	count := 0
	for _, requesterID := range requesters.GetRecentRequesters() {
		if requesterID == userID {
			count++
		}
	}
	return count
}

func containsInviteImporter(importers []*mtproto.ChatInviteImporter, userID int64) bool {
	for _, importer := range importers {
		if importer.GetUserId() == userID {
			return true
		}
	}
	return false
}

func auditInviteContext(t *testing.T, userID int64) context.Context {
	t.Helper()
	ctx, err := rpcmetadata.RpcMetadataToOutgoing(context.Background(), &rpcmetadata.RpcMetadata{UserId: userID})
	if err != nil {
		t.Fatal("encode RPC metadata:", err)
	}
	return ctx
}

func assertInviteRPCError(t *testing.T, err error, wantMessage string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected RPC error %s, got nil", wantMessage)
	}
	if got := status.Convert(err).Message(); got != wantMessage {
		t.Fatalf("RPC error = %q, want %q (full error: %v)", got, wantMessage, err)
	}
}

var _ marmota_cache.BatchCache = (*chatInviteAuditCache)(nil)
