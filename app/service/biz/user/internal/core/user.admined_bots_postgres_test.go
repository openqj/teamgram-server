package core

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto/rpc/metadata"
	userdao "github.com/teamgram/teamgram-server/app/service/biz/user/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func TestUserGetAdminedBotsUsesManagerHierarchyPostgres(t *testing.T) {
	pg := userPostgresTest(t, 2)
	ctx := context.Background()
	base := time.Now().UnixNano()
	ownerID, managerID := base, base+1
	managedOneID, managedTwoID := base+2, base+3
	creatorOwnedID, unrelatedID := base+4, base+5
	otherManagerID := base + 6
	ids := []int64{ownerID, managerID, managedOneID, managedTwoID, creatorOwnedID, unrelatedID, otherManagerID}
	insertUserPostgresFixtures(t, pg, ids...)

	if _, err := pg.Pool.Exec(ctx, `UPDATE users SET user_type=$1,is_bot=TRUE WHERE id=ANY($2::bigint[])`, user.UserTypeBot, []int64{managerID, managedOneID, managedTwoID, creatorOwnedID, unrelatedID, otherManagerID}); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Pool.Exec(ctx, `UPDATE users SET deleted=TRUE WHERE id=$1`, unrelatedID); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		botID, creatorID, managerID int64
		name                        string
	}{
		{managedOneID, ownerID, managerID, "managed-one"},
		{managedTwoID, ownerID + 99, managerID, "managed-two"},
		{creatorOwnedID, managerID, 0, "creator-owned"},
		{unrelatedID, ownerID, managerID, "unrelated"},
	} {
		if _, err := pg.Pool.Exec(ctx, `INSERT INTO bots(bot_id,creator_user_id,manager_bot_id,token) VALUES($1,$2,$3,$4)`, fixture.botID, fixture.creatorID, fixture.managerID, fmt.Sprintf("admined-%s-%d", fixture.name, fixture.botID)); err != nil {
			t.Fatal(err)
		}
	}

	c := New(ctx, &svc.ServiceContext{Dao: &userdao.Dao{Postgres: pg}})
	c.MD = &metadata.RpcMetadata{UserId: managerID}
	got, err := c.UserGetAdminedBots(&user.BotRegistryGetAdminedBotsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.GetDatas()) != 2 {
		t.Fatalf("admined bots = %#v, want two managed bots", got)
	}
	if got.GetDatas()[0].GetUser().GetId() != managedOneID || got.GetDatas()[1].GetUser().GetId() != managedTwoID {
		t.Fatalf("admined bot ids = %d,%d, want %d,%d", got.GetDatas()[0].GetUser().GetId(), got.GetDatas()[1].GetUser().GetId(), managedOneID, managedTwoID)
	}
}
