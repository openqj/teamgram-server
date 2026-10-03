package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	dialogclient "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"google.golang.org/grpc/status"
)

type dialogFilterTestStore struct {
	values map[string]string
}

type folderValidationDialogClient struct {
	dialogclient.DialogClient
}

type folderArchiveDialogClient struct {
	dialogclient.DialogClient
	request *dialog.TLDialogEditPeerFolders
}

type folderTagsDialogClient struct {
	dialogclient.DialogClient
	result *mtproto.Bool
}

func (c *folderArchiveDialogClient) DialogGetDialogById(context.Context, *dialog.TLDialogGetDialogById) (*dialog.DialogExt, error) {
	return &dialog.DialogExt{Dialog: &mtproto.Dialog{}}, nil
}

func (c *folderArchiveDialogClient) DialogEditPeerFolders(_ context.Context, in *dialog.TLDialogEditPeerFolders) (*dialog.Vector_DialogPinnedExt, error) {
	c.request = in
	return &dialog.Vector_DialogPinnedExt{}, nil
}

func (c *folderTagsDialogClient) DialogToggleDialogFilterTags(context.Context, *dialog.TLDialogToggleDialogFilterTags) (*mtproto.Bool, error) {
	return c.result, nil
}

func (s *dialogFilterTestStore) Get(key string) (string, error) {
	return s.values[key], nil
}

func (s *dialogFilterTestStore) Set(key, value string) error {
	if s.values == nil {
		s.values = make(map[string]string)
	}
	s.values[key] = value
	return nil
}

func testDialogFilter(id int32) *mtproto.DialogFilter {
	return mtproto.MakeTLDialogFilter(&mtproto.DialogFilter{
		Id:           id,
		Title_STRING: "filter",
		PinnedPeers:  []*mtproto.InputPeer{},
		IncludePeers: []*mtproto.InputPeer{},
		ExcludePeers: []*mtproto.InputPeer{},
	}).To_DialogFilter()
}

func TestDialogFiltersRoundTripOrderAndIsolation(t *testing.T) {
	previous := persist.Default
	persist.Use(&dialogFilterTestStore{})
	t.Cleanup(func() { persist.Use(previous) })

	alice := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 101}}
	bob := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 202}}
	for _, id := range []int32{2, 3} {
		result, err := alice.MessagesUpdateDialogFilter(&mtproto.TLMessagesUpdateDialogFilter{
			Id: id, Filter: testDialogFilter(id),
		})
		if err != nil || !mtproto.FromBool(result) {
			t.Fatalf("save filter %d = (%v, %v), want true", id, result, err)
		}
	}

	filters, err := alice.MessagesGetDialogFiltersF19ED96D(nil)
	if err != nil || len(filters.GetDatas()) != 2 {
		t.Fatalf("Alice filters = (%+v, %v), want two filters", filters, err)
	}
	if got := filters.GetDatas()[0].GetId(); got != 2 {
		t.Fatalf("first filter ID = %d, want 2", got)
	}
	other, err := bob.MessagesGetDialogFiltersF19ED96D(nil)
	if err != nil || len(other.GetDatas()) != 0 {
		t.Fatalf("Bob filters = (%+v, %v), want empty", other, err)
	}

	result, err := alice.MessagesUpdateDialogFiltersOrder(&mtproto.TLMessagesUpdateDialogFiltersOrder{Order: []int32{3, 2}})
	if err != nil || !mtproto.FromBool(result) {
		t.Fatalf("reorder = (%v, %v), want true", result, err)
	}
	filters, err = alice.MessagesGetDialogFiltersF19ED96D(nil)
	if err != nil || len(filters.GetDatas()) != 2 || filters.GetDatas()[0].GetId() != 3 || filters.GetDatas()[1].GetId() != 2 {
		t.Fatalf("ordered filters = (%+v, %v), want [3, 2]", filters, err)
	}

	result, err = alice.MessagesUpdateDialogFilter(&mtproto.TLMessagesUpdateDialogFilter{Id: 2})
	if err != nil || !mtproto.FromBool(result) {
		t.Fatalf("delete = (%v, %v), want true", result, err)
	}
	filters, err = alice.MessagesGetDialogFiltersF19ED96D(nil)
	if err != nil || len(filters.GetDatas()) != 1 || filters.GetDatas()[0].GetId() != 3 {
		t.Fatalf("filters after delete = (%+v, %v), want [3]", filters, err)
	}
}

func TestDialogFiltersRejectReservedIDs(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 101}}
	for _, id := range []int32{-1, 0, 1} {
		result, err := c.MessagesUpdateDialogFilter(&mtproto.TLMessagesUpdateDialogFilter{
			Id: id, Filter: testDialogFilter(id),
		})
		if result != nil || status.Convert(err).Message() != "FILTER_ID_INVALID" {
			t.Fatalf("filter ID %d = (%v, %v), want FILTER_ID_INVALID", id, result, err)
		}
	}
}

func TestFoldersEditPeerFoldersRejectsInvalidFolderID(t *testing.T) {
	c := &ApiFullCore{
		MD: &metadata.RpcMetadata{UserId: 101},
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			DialogClient: folderValidationDialogClient{},
		}},
	}
	result, err := c.FoldersEditPeerFolders(&mtproto.TLFoldersEditPeerFolders{
		FolderPeers: []*mtproto.InputFolderPeer{{
			Peer:     mtproto.MakeInputPeerChat(42),
			FolderId: -1,
		}},
	})
	if result != nil || status.Convert(err).Message() != "FOLDER_ID_INVALID" {
		t.Fatalf("negative folder ID = (%v, %v), want FOLDER_ID_INVALID", result, err)
	}
}

func TestFoldersEditPeerFoldersAllowsArchive(t *testing.T) {
	previous := persist.Default
	persist.Use(&dialogFilterTestStore{})
	t.Cleanup(func() { persist.Use(previous) })

	client := &folderArchiveDialogClient{}
	c := &ApiFullCore{
		MD: &metadata.RpcMetadata{UserId: 101},
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			DialogClient: client,
		}},
	}
	result, err := c.FoldersEditPeerFolders(&mtproto.TLFoldersEditPeerFolders{
		FolderPeers: []*mtproto.InputFolderPeer{{
			Peer:     mtproto.MakeInputPeerChat(42),
			FolderId: 1,
		}},
	})
	if err != nil || result == nil {
		t.Fatalf("archive dialog = (%v, %v), want updates", result, err)
	}
	if client.request == nil || client.request.GetUserId() != 101 || client.request.GetFolderId() != 1 || len(client.request.GetPeerDialogList()) != 1 {
		t.Fatalf("archive request = %+v, want user=101 folder=1 and one dialog", client.request)
	}
}

func TestToggleDialogFilterTagsAcknowledgesSuccessfulDisable(t *testing.T) {
	c := &ApiFullCore{
		MD: &metadata.RpcMetadata{UserId: 101},
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			DialogClient: &folderTagsDialogClient{result: mtproto.BoolFalse},
		}},
	}
	result, err := c.MessagesToggleDialogFilterTags(&mtproto.TLMessagesToggleDialogFilterTags{
		Enabled: mtproto.BoolFalse,
	})
	if err != nil || !mtproto.FromBool(result) {
		t.Fatalf("toggle disable = (%v, %v), want true", result, err)
	}
}
