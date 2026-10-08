package core

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestChannelsDeleteChannelOwnerTransactionRoundTrip(t *testing.T) {
	db, err := sql.Open("mysql", isolatedAuditDSN(t))
	if err != nil {
		t.Fatal("open isolated audit MySQL:", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS chat_invites (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			chat_id BIGINT NOT NULL,
			admin_id BIGINT NOT NULL,
			link VARCHAR(64) NOT NULL,
			permanent TINYINT NOT NULL DEFAULT 0,
			revoked TINYINT NOT NULL DEFAULT 0,
			request_needed TINYINT NOT NULL DEFAULT 0,
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
			requested TINYINT NOT NULL DEFAULT 0,
			approved_by BIGINT NOT NULL DEFAULT 0,
			date2 BIGINT NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_channel_message_request (
			channel_id BIGINT NOT NULL,
			sender_user_id BIGINT NOT NULL,
			random_id BIGINT NOT NULL,
			message_id INT NOT NULL,
			pts INT NOT NULL,
			request_hash BINARY(32) NOT NULL,
			created_at INT NOT NULL,
			PRIMARY KEY (channel_id, sender_user_id, random_id),
			KEY idx_apifull_channel_message_request_message (channel_id, message_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	} {
		if _, err = db.Exec(statement); err != nil {
			t.Fatal("create shared invite test table:", err)
		}
	}

	channelID := time.Now().UnixNano()
	const owner int64 = 92041
	const member int64 = 92042
	callID := channelID + 1
	legacyKey := chanDataKey(owner, channelID)
	t.Cleanup(func() {
		for _, query := range []string{
			`DELETE FROM apifull_channel_message_request WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message_hidden WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message_seq WHERE channel_id=?`,
			`DELETE FROM apifull_channel_read_state WHERE channel_id=?`,
			`DELETE FROM chat_invite_participants WHERE chat_id=?`,
			`DELETE FROM chat_invites WHERE chat_id=?`,
			`DELETE FROM apifull_channel_member WHERE channel_id=?`,
			`DELETE FROM apifull_group_call WHERE channel_id=?`,
			`DELETE FROM apifull_channel WHERE id=?`,
		} {
			if _, cleanupErr := db.Exec(query, channelID); cleanupErr != nil {
				t.Errorf("clean up delete-channel fixture: %v", cleanupErr)
			}
		}
		_ = persist.Default.Set(legacyKey, "")
	})

	if err = domain.SaveChannel(domain.Channel{ID: channelID, AccessHash: channelID, Creator: owner, Title: "delete-channel-test", Broadcast: true}); err != nil {
		t.Fatal("save channel:", err)
	}
	if err = domain.JoinChannel(channelID, member); err != nil {
		t.Fatal("save member:", err)
	}
	if _, err = channelview.Post(owner, channelID, "delete me", time.Now().Unix()); err != nil {
		t.Fatal("save channel message:", err)
	}
	if _, err = db.Exec(`INSERT INTO apifull_channel_message_request
		(channel_id, sender_user_id, random_id, message_id, pts, request_hash, created_at)
		VALUES (?,?,?,?,?,?,?)`, channelID, owner, 91024001, 1, 1, make([]byte, 32), time.Now().Unix()); err != nil {
		t.Fatal("save channel message request mapping:", err)
	}
	if _, err = db.Exec(`INSERT INTO apifull_channel_message_hidden (user_id, channel_id, message_id) VALUES (?,?,?)`, member, channelID, 1); err != nil {
		t.Fatal("save hidden-message row:", err)
	}
	if _, err = db.Exec(`INSERT INTO apifull_channel_read_state (user_id, channel_id, read_max_id) VALUES (?,?,?)`, member, channelID, 1); err != nil {
		t.Fatal("save read-state row:", err)
	}
	if err = domain.SaveGroupCall(callID, callID, owner, channelID, "", "{}"); err != nil {
		t.Fatal("save group-call row:", err)
	}
	if err = persist.Default.Set(legacyKey, "legacy-channel"); err != nil {
		t.Fatal("save legacy channel key:", err)
	}
	if _, err = db.Exec(`INSERT INTO chat_invites
		(chat_id, admin_id, link, permanent, revoked, request_needed, start_date, expire_date,
		usage_limit, usage2, requested, title, date2)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		channelID, owner, "delete-channel-invite", 0, 0, 0, 0, 0, 0, 0, 0, "", time.Now().Unix()); err != nil {
		t.Fatal("save channel invite:", err)
	}
	if _, err = db.Exec(`INSERT INTO chat_invite_participants
		(chat_id, link, user_id, requested, approved_by, date2) VALUES (?,?,?,?,?,?)`,
		channelID, "delete-channel-invite", member, 0, 0, time.Now().Unix()); err != nil {
		t.Fatal("save channel invite participant:", err)
	}

	input := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID}).To_InputChannel()
	outsiderCore := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: member}}
	if _, err = outsiderCore.ChannelsDeleteChannel(&mtproto.TLChannelsDeleteChannel{Channel: input}); !errors.Is(err, mtproto.ErrChatAdminRequired) {
		t.Fatalf("non-owner delete = %v, want CHAT_ADMIN_REQUIRED", err)
	}
	if _, ok, loadErr := domain.LoadChannel(channelID); loadErr != nil || !ok {
		t.Fatalf("non-owner delete changed channel: ok=%v err=%v", ok, loadErr)
	}

	badHash := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID + 1}).To_InputChannel()
	ownerCore := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: owner}}
	if _, err = ownerCore.ChannelsDeleteChannel(&mtproto.TLChannelsDeleteChannel{Channel: badHash}); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("wrong-hash delete = %v, want CHANNEL_INVALID", err)
	}

	updates, err := ownerCore.ChannelsDeleteChannel(&mtproto.TLChannelsDeleteChannel{Channel: input})
	if err != nil || updates == nil {
		t.Fatalf("owner delete = (%+v, %v), want empty Updates", updates, err)
	}
	if err = updates.To_Updates().Encode(mtproto.NewEncodeBuf(256), 229); err != nil {
		t.Fatal("encode delete Updates:", err)
	}
	if _, ok, loadErr := domain.LoadChannel(channelID); loadErr != nil || ok {
		t.Fatalf("deleted channel readback: ok=%v err=%v", ok, loadErr)
	}
	for name, query := range map[string]string{
		"members":            `SELECT COUNT(*) FROM apifull_channel_member WHERE channel_id=?`,
		"messages":           `SELECT COUNT(*) FROM apifull_channel_message WHERE channel_id=?`,
		"message_requests":   `SELECT COUNT(*) FROM apifull_channel_message_request WHERE channel_id=?`,
		"hidden":             `SELECT COUNT(*) FROM apifull_channel_message_hidden WHERE channel_id=?`,
		"sequence":           `SELECT COUNT(*) FROM apifull_channel_message_seq WHERE channel_id=?`,
		"read":               `SELECT COUNT(*) FROM apifull_channel_read_state WHERE channel_id=?`,
		"invites":            `SELECT COUNT(*) FROM chat_invites WHERE chat_id=?`,
		"participants":       `SELECT COUNT(*) FROM chat_invite_participants WHERE chat_id=?`,
		"calls":              `SELECT COUNT(*) FROM apifull_group_call WHERE channel_id=?`,
		"call_settings":      `SELECT COUNT(*) FROM apifull_group_call_settings WHERE call_id=?`,
		"call_participants":  `SELECT COUNT(*) FROM apifull_group_call_participant WHERE call_id=?`,
		"call_subscriptions": `SELECT COUNT(*) FROM apifull_group_call_subscription WHERE call_id=?`,
		"call_send_as":       `SELECT COUNT(*) FROM apifull_group_call_send_as WHERE call_id=?`,
		"call_messages":      `SELECT COUNT(*) FROM apifull_group_call_message WHERE call_id=?`,
	} {
		var count int
		queryArg := channelID
		if name != "calls" {
			queryArg = callID
		}
		if err = db.QueryRow(query, queryArg).Scan(&count); err != nil {
			t.Fatalf("count %s rows: %v", name, err)
		}
		if count != 0 {
			t.Errorf("%s rows after delete = %d, want 0", name, count)
		}
	}
	if raw, getErr := persist.Default.Get(legacyKey); getErr != nil || raw != "" {
		t.Fatalf("legacy channel key after delete = %q, err=%v", raw, getErr)
	}
}
