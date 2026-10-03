package core

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

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
