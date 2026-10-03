package core

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	dialogclient "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	dialogpb "github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

type chatAppearanceDialogClient struct {
	dialogclient.DialogClient
	theme     *dialogpb.TLDialogSetChatTheme
	wallpaper *dialogpb.TLDialogSetChatWallpaper
}

func (c *chatAppearanceDialogClient) DialogSetChatTheme(_ context.Context, in *dialogpb.TLDialogSetChatTheme) (*mtproto.Bool, error) {
	c.theme = in
	return mtproto.BoolTrue, nil
}

func (c *chatAppearanceDialogClient) DialogSetChatWallpaper(_ context.Context, in *dialogpb.TLDialogSetChatWallpaper) (*mtproto.Bool, error) {
	c.wallpaper = in
	return mtproto.BoolTrue, nil
}

func newChatAppearanceCore(userID int64, client dialogclient.DialogClient) *ApiFullCore {
	return &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{DialogClient: client}},
		MD:     &metadata.RpcMetadata{UserId: userID},
	}
}

func appearancePeer(userID int64) *mtproto.InputPeer {
	return mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: userID + 1, AccessHash: userID + 2}).To_InputPeer()
}

func TestChatThemeAndWallpaperUseDialogPersistence(t *testing.T) {
	userID := time.Now().UnixNano()
	client := &chatAppearanceDialogClient{}
	c := newChatAppearanceCore(userID, client)
	peer := appearancePeer(userID)

	theme, err := c.MessagesSetChatTheme(&mtproto.TLMessagesSetChatTheme{
		Peer:     peer,
		Theme:    mtproto.MakeTLInputChatTheme(&mtproto.InputChatTheme{Emoticon: "❤️"}).To_InputChatTheme(),
		Emoticon: "❤️",
	})
	if err != nil || theme == nil || client.theme == nil {
		t.Fatalf("set chat theme = (%v, %v), dialog request=%+v", theme, err, client.theme)
	}
	if client.theme.GetUserId() != userID || client.theme.GetPeerType() != mtproto.PEER_USER || client.theme.GetPeerId() != userID+1 || client.theme.GetThemeEmoticon() != "❤️" {
		t.Fatalf("theme dialog request = %+v", client.theme)
	}

	documentID, accessHash := userID+10, userID+11
	wallpaper := mtproto.MakeTLWallPaper(&mtproto.WallPaper{Id: documentID, AccessHash: accessHash, Document: mtproto.MakeTLDocument(&mtproto.Document{Id: documentID, AccessHash: accessHash}).To_Document()}).To_WallPaper()
	if err = storeUploadedWallpapers(userID, []*mtproto.WallPaper{wallpaper}); err != nil {
		t.Fatal(err)
	}
	wallpaperUpdates, err := c.MessagesSetChatWallPaper(&mtproto.TLMessagesSetChatWallPaper{
		Peer:      peer,
		Wallpaper: mtproto.MakeTLInputWallPaper(&mtproto.InputWallPaper{Id: documentID, AccessHash: accessHash}).To_InputWallPaper(),
		ForBoth:   true,
	})
	if err != nil || wallpaperUpdates == nil || client.wallpaper == nil {
		t.Fatalf("set chat wallpaper = (%v, %v), dialog request=%+v", wallpaperUpdates, err, client.wallpaper)
	}
	if client.wallpaper.GetUserId() != userID || client.wallpaper.GetPeerType() != mtproto.PEER_USER || client.wallpaper.GetPeerId() != userID+1 || client.wallpaper.GetWallpaperId() != documentID || !client.wallpaper.GetWallpaperOverridden() {
		t.Fatalf("wallpaper dialog request = %+v", client.wallpaper)
	}

	reverted, err := c.MessagesSetChatWallPaper(&mtproto.TLMessagesSetChatWallPaper{Peer: peer, Revert: true})
	if err != nil || reverted == nil || client.wallpaper.GetWallpaperId() != 0 || client.wallpaper.GetWallpaperOverridden() {
		t.Fatalf("revert chat wallpaper = (%v, %v), dialog request=%+v", reverted, err, client.wallpaper)
	}
}
