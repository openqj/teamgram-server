package core

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/proto/mtproto"
	updateshelper "github.com/teamgram/teamgram-server/app/service/biz/updates"
	updatesclient "github.com/teamgram/teamgram-server/app/service/biz/updates/client"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/updates"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
	"github.com/zeromicro/go-zero/core/jsonx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type postgresUpdatesRPCClient struct {
	updatesclient.UpdatesClient
	rpc updates.RPCUpdatesClient
}

func (c *postgresUpdatesRPCClient) UpdatesGetDifferenceV2(ctx context.Context, in *updates.TLUpdatesGetDifferenceV2) (*updates.Difference, error) {
	return c.rpc.UpdatesGetDifferenceV2(ctx, in)
}

func (c *postgresUpdatesRPCClient) UpdatesGetStateV2(ctx context.Context, in *updates.TLUpdatesGetStateV2) (*mtproto.Updates_State, error) {
	return c.rpc.UpdatesGetStateV2(ctx, in)
}

func postgresDifferenceCore(t *testing.T) *UpdatesCore {
	t.Helper()
	dsn := os.Getenv("UPDATES_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("UPDATES_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := fmt.Sprintf("difference_rpc_test_%d", time.Now().UnixNano())
	sqlSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+sqlSchema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), `DROP SCHEMA `+sqlSchema+` CASCADE`); err != nil {
			t.Errorf("clean difference RPC schema: %v", err)
		}
	})
	for _, table := range []string{"user_pts_updates", "auth_seq_updates", "apifull_secret_user_state"} {
		name := pgx.Identifier{table}.Sanitize()
		if _, err := admin.Exec(ctx, `CREATE TABLE `+sqlSchema+`.`+name+` (LIKE public.`+name+` INCLUDING ALL)`); err != nil {
			t.Fatal(err)
		}
	}
	for i := int32(1); i <= 3; i++ {
		update := mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
			Pts_INT32: i, PtsCount: 1,
			Message_MESSAGE: mtproto.MakeTLMessage(&mtproto.Message{Id: i, PeerId: mtproto.MakePeerUser(202), Date: 100, Message: fmt.Sprintf("message %d", i)}).To_Message(),
		}).To_Update()
		data, err := jsonx.Marshal(update)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx, `INSERT INTO `+sqlSchema+`.user_pts_updates (user_id,pts,pts_count,update_type,update_data,date2)
	 VALUES (101,$1,1,$2,$3,100)`, i, mtproto.GetUpdateType(update), string(data)); err != nil {
			t.Fatal(err)
		}
	}
	databaseURL, err := url.Parse(dsn)
	if err != nil || databaseURL.Host == "" {
		t.Fatalf("test requires PostgreSQL URL: %v", err)
	}
	query := databaseURL.Query()
	query.Set("search_path", schema)
	databaseURL.RawQuery = query.Encode()
	service := updateshelper.New(updateshelper.Config{Postgres: postgres.Config{DSN: databaseURL.String()}})
	t.Cleanup(func() { service.GetServiceContext().Close() })
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	updates.RegisterRPCUpdatesServer(server, service)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	core := secretUpdatesCore(&secretUpdatesReaderStub{}, &postgresUpdatesRPCClient{rpc: updates.NewRPCUpdatesClient(conn)}, &authsessionClientStub{})
	core.MD.UserId = 101
	core.MD.PermAuthKeyId = 11
	return core
}

func TestUpdatesPostgresLayer229DifferenceFlags(t *testing.T) {
	core := postgresDifferenceCore(t)
	request := &mtproto.TLUpdatesGetDifference{
		Constructor: mtproto.CRC32_updates_getDifference_19c2f763,
		Pts:         0, Date: 99, PtsLimit: wrapperspb.Int32(1), PtsTotalLimit: wrapperspb.Int32(3), QtsLimit: wrapperspb.Int32(2),
	}
	buf := mtproto.NewEncodeBuf(128)
	if err := request.Encode(buf, 229); err != nil {
		t.Fatal(err)
	}
	decoder := mtproto.NewDecodeBuf(buf.GetBuf())
	decoded, ok := decoder.Object().(*mtproto.TLUpdatesGetDifference)
	if !ok || decoder.GetError() != nil {
		t.Fatalf("decode Layer 229 request: %v", decoder.GetError())
	}
	first, err := core.UpdatesGetDifference(decoded)
	if err != nil || first.GetPredicateName() != mtproto.Predicate_updates_differenceSlice || first.IntermediateState.GetPts() != 1 || len(first.NewMessages) != 1 {
		t.Fatalf("Layer 229 flags through gRPC/PostgreSQL=%v err=%v", first, err)
	}
	decoded.PtsTotalLimit = wrapperspb.Int32(2)
	tooLong, err := core.UpdatesGetDifference(decoded)
	if err != nil || tooLong.GetPredicateName() != mtproto.Predicate_updates_differenceTooLong || tooLong.GetPts() != 3 {
		t.Fatalf("Layer 229 total threshold=%v err=%v", tooLong, err)
	}
	decoded.Pts = 1
	decoded.PtsLimit = nil
	last, err := core.UpdatesGetDifference(decoded)
	if err != nil || last.GetPredicateName() != mtproto.Predicate_updates_difference || last.State.GetPts() != 3 || len(last.NewMessages) != 2 {
		t.Fatalf("Layer 229 final difference=%v err=%v", last, err)
	}
	decoded.Pts = 3
	decoded.Date = last.State.GetDate()
	empty, err := core.UpdatesGetDifference(decoded)
	if err != nil || empty.GetPredicateName() != mtproto.Predicate_updates_differenceEmpty || empty.GetDate() < last.State.GetDate() {
		t.Fatalf("Layer 229 empty state=%v err=%v", empty, err)
	}
	for _, result := range []*mtproto.Updates_Difference{first, tooLong, last, empty} {
		encoded := mtproto.NewEncodeBuf(1024)
		if err := result.Encode(encoded, 229); err != nil || len(encoded.GetBuf()) < 4 {
			t.Fatalf("encode Layer 229 difference: %v", err)
		}
	}
}
