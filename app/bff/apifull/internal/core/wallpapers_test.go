package core

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	dfs_client "github.com/teamgram/teamgram-server/app/service/dfs/client"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type wallpaperUploadDfsClient struct {
	dfs_client.DfsClient
	doc *mtproto.Document
	err error
	got *dfs.TLDfsUploadWallPaperFile
}

func (c *wallpaperUploadDfsClient) DfsUploadWallPaperFile(_ context.Context, in *dfs.TLDfsUploadWallPaperFile) (*mtproto.Document, error) {
	c.got = in
	return c.doc, c.err
}

func newWallpaperUploadCore(userID int64, client dfs_client.DfsClient) *ApiFullCore {
	return &ApiFullCore{
		ctx: context.Background(),
		MD:  &metadata.RpcMetadata{UserId: userID},
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{
			DfsClient: client,
		}},
	}
}

func wallpaperUploadRequest() *mtproto.TLAccountUploadWallPaper {
	return &mtproto.TLAccountUploadWallPaper{
		File: mtproto.MakeTLInputFile(&mtproto.InputFile{
			Id_INT64: 11,
			Parts:    2,
			Name:     "wallpaper.jpg",
		}).To_InputFile(),
		Settings: mtproto.MakeTLWallPaperSettings(&mtproto.WallPaperSettings{
			Blur: true,
		}).To_WallPaperSettings(),
	}
}

func TestAccountUploadWallPaperUsesDFSAndPersistsUpload(t *testing.T) {
	userID := time.Now().UnixNano()
	dfsClient := &wallpaperUploadDfsClient{
		doc: mtproto.MakeTLDocument(&mtproto.Document{
			Id:         42,
			AccessHash: 99,
			MimeType:   "image/jpeg",
		}).To_Document(),
	}

	request := wallpaperUploadRequest()
	got, err := newWallpaperUploadCore(userID, dfsClient).AccountUploadWallPaper(request)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.GetId() != 42 || got.GetAccessHash() != 99 || !got.GetCreator() || got.GetDocument() == nil || got.GetDocument().GetId() != dfsClient.doc.GetId() || got.GetDocument().GetAccessHash() != dfsClient.doc.GetAccessHash() {
		t.Fatalf("wallpaper = %#v", got)
	}
	if got.GetSettings() == nil || !got.GetSettings().GetBlur() {
		t.Fatalf("wallpaper settings = %#v", got.GetSettings())
	}
	if dfsClient.got == nil || dfsClient.got.GetCreator() != userID || dfsClient.got.GetFile() == nil || dfsClient.got.GetFile().GetId_INT64() != 11 || dfsClient.got.GetFile().GetParts() != 2 || dfsClient.got.GetMimeType() != "image/jpeg" || mtproto.FromBool(dfsClient.got.GetAdmin()) {
		t.Fatalf("DFS request = %#v", dfsClient.got)
	}

	stored, err := loadUploadedWallpapers(userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].GetId() != 42 || stored[0].GetAccessHash() != 99 {
		t.Fatalf("stored wallpapers = %#v", stored)
	}
	readBack, err := newWallpaperUploadCore(userID, dfsClient).AccountGetWallPaper(&mtproto.TLAccountGetWallPaper{
		Wallpaper: mtproto.MakeTLInputWallPaper(&mtproto.InputWallPaper{Id: 42, AccessHash: 99}).To_InputWallPaper(),
	})
	if err != nil || readBack == nil || readBack.GetDocument() == nil {
		t.Fatalf("read uploaded wallpaper = (%#v, %v)", readBack, err)
	}
}

func TestAccountUploadWallPaperPropagatesDFSError(t *testing.T) {
	want := errors.New("dfs unavailable")
	dfsClient := &wallpaperUploadDfsClient{err: want}
	got, err := newWallpaperUploadCore(time.Now().UnixNano(), dfsClient).AccountUploadWallPaper(wallpaperUploadRequest())
	if got != nil || !errors.Is(err, want) {
		t.Fatalf("result=(%#v, %v), want DFS error", got, err)
	}
}

func TestAccountUploadWallPaperRejectsInvalidDFSResult(t *testing.T) {
	for _, id := range []int64{0, -42} {
		t.Run(fmt.Sprintf("id_%d", id), func(t *testing.T) {
			dfsClient := &wallpaperUploadDfsClient{
				doc: mtproto.MakeTLDocument(&mtproto.Document{Id: id, AccessHash: 99}).To_Document(),
			}
			got, err := newWallpaperUploadCore(time.Now().UnixNano(), dfsClient).AccountUploadWallPaper(wallpaperUploadRequest())
			if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
				t.Fatalf("result=(%#v, %v), want INTERNAL_SERVER_ERROR", got, err)
			}
		})
	}
}

func TestWallpaperSaveInstallReadOwnedUpload(t *testing.T) {
	userID := time.Now().UnixNano()
	document := mtproto.MakeTLDocument(&mtproto.Document{
		Id:         userID + 1,
		AccessHash: userID + 2,
		MimeType:   "image/jpeg",
	}).To_Document()
	wallpaper := mtproto.MakeTLWallPaper(&mtproto.WallPaper{
		Id:         document.GetId(),
		AccessHash: document.GetAccessHash(),
		Creator:    true,
		Document:   document,
	}).To_WallPaper()
	if err := storeUploadedWallpapers(userID, []*mtproto.WallPaper{wallpaper}); err != nil {
		t.Fatal(err)
	}
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}
	inWallpaper := mtproto.MakeTLInputWallPaper(&mtproto.InputWallPaper{
		Id:         document.GetId(),
		AccessHash: document.GetAccessHash(),
	}).To_InputWallPaper()
	settings := mtproto.MakeTLWallPaperSettings(&mtproto.WallPaperSettings{
		Blur:      true,
		Intensity: wrapperspb.Int32(75),
	}).To_WallPaperSettings()
	if result, err := c.AccountSaveWallPaper(&mtproto.TLAccountSaveWallPaper{Wallpaper: inWallpaper, Settings: settings}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("save result=%v err=%v", result, err)
	}
	if result, err := c.AccountInstallWallPaper(&mtproto.TLAccountInstallWallPaper{Wallpaper: inWallpaper, Settings: settings}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("install result=%v err=%v", result, err)
	}
	got, err := c.AccountGetWallPaper(&mtproto.TLAccountGetWallPaper{Wallpaper: inWallpaper})
	if err != nil || got.GetDocument() == nil || got.GetSettings().GetIntensity().GetValue() != 75 {
		t.Fatalf("get wallpaper=%v err=%v", got, err)
	}
	list, err := c.AccountGetWallPapers(&mtproto.TLAccountGetWallPapers{})
	if err != nil || len(list.GetWallpapers()) != 1 || list.GetWallpapers()[0].GetDocument() == nil {
		t.Fatalf("wallpapers=%v err=%v", list, err)
	}
	multi, err := c.AccountGetMultiWallPapers(&mtproto.TLAccountGetMultiWallPapers{Wallpapers: []*mtproto.InputWallPaper{inWallpaper}})
	if err != nil || len(multi.GetDatas()) != 1 || multi.GetDatas()[0].GetDocument() == nil {
		t.Fatalf("multi=%v err=%v", multi, err)
	}
}

func TestWallpaperSaveAndInstallRejectUnownedInput(t *testing.T) {
	userID := time.Now().UnixNano()
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}
	inWallpaper := mtproto.MakeTLInputWallPaper(&mtproto.InputWallPaper{Id: userID + 1, AccessHash: userID + 2}).To_InputWallPaper()
	if result, err := c.AccountSaveWallPaper(&mtproto.TLAccountSaveWallPaper{Wallpaper: inWallpaper}); result != nil || err != mtproto.ErrWallpaperInvalid {
		t.Fatalf("save result=%v err=%v, want WALLPAPER_INVALID", result, err)
	}
	if result, err := c.AccountInstallWallPaper(&mtproto.TLAccountInstallWallPaper{Wallpaper: inWallpaper}); result != nil || err != mtproto.ErrWallpaperInvalid {
		t.Fatalf("install result=%v err=%v, want WALLPAPER_INVALID", result, err)
	}
}
