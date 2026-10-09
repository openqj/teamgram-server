package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	bffdao "github.com/teamgram/teamgram-server/app/bff/savedmessagedialogs/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/savedmessagedialogs/internal/svc"
	dialogclient "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

type reorderPinnedSavedDialogsDialogClient struct {
	dialogclient.DialogClient
	calls    int
	request  *dialog.TLDialogReorderPinnedSavedDialogs
	response *mtproto.Bool
}

func (d *reorderPinnedSavedDialogsDialogClient) DialogReorderPinnedSavedDialogs(_ context.Context, in *dialog.TLDialogReorderPinnedSavedDialogs) (*mtproto.Bool, error) {
	d.calls++
	d.request = in
	return d.response, nil
}

func TestMessagesReorderPinnedSavedDialogsEmptyOrder(t *testing.T) {
	for _, tc := range []struct {
		name      string
		force     bool
		wantCalls int
	}{
		{name: "force clears saved pins", force: true, wantCalls: 1},
		{name: "non-force remains a no-op", wantCalls: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dialogs := &reorderPinnedSavedDialogsDialogClient{response: mtproto.BoolTrue}
			c := New(context.Background(), &svc.ServiceContext{Dao: &bffdao.Dao{DialogClient: dialogs}})
			c.MD = &metadata.RpcMetadata{UserId: 42}

			got, err := c.MessagesReorderPinnedSavedDialogs(&mtproto.TLMessagesReorderPinnedSavedDialogs{Force: tc.force})
			if err != nil || !mtproto.FromBool(got) {
				t.Fatalf("MessagesReorderPinnedSavedDialogs() = (%v, %v), want true", got, err)
			}
			if dialogs.calls != tc.wantCalls {
				t.Fatalf("dialog RPC calls = %d, want %d", dialogs.calls, tc.wantCalls)
			}
			if tc.force {
				if dialogs.request == nil || dialogs.request.GetUserId() != 42 || !mtproto.FromBool(dialogs.request.GetForce()) || len(dialogs.request.GetOrder()) != 0 {
					t.Fatalf("dialog RPC request = %v, want force=true, user=42, empty order", dialogs.request)
				}
			}
		})
	}
}
