package core

import (
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestThemeMethodsRejectInvalidUploadWithoutThemeBackend(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: time.Now().UnixNano()}}
	cases := []struct {
		name string
		call func() (bool, error)
	}{
		{"upload", func() (bool, error) {
			result, err := c.AccountUploadTheme(nil)
			return result != nil, err
		}},
		{"create", func() (bool, error) {
			result, err := c.AccountCreateTheme(nil)
			return result != nil, err
		}},
		{"update", func() (bool, error) {
			result, err := c.AccountUpdateTheme(nil)
			return result != nil, err
		}},
		{"chat themes", func() (bool, error) {
			result, err := c.AccountGetChatThemes(nil)
			return result != nil, err
		}},
		{"unique gift chat themes", func() (bool, error) {
			result, err := c.AccountGetUniqueGiftChatThemes(nil)
			return result != nil, err
		}},
		{"set chat theme", func() (bool, error) {
			result, err := c.MessagesSetChatTheme(nil)
			return result != nil, err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.call()
			if tc.name == "upload" {
				if result || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
					t.Fatalf("result=%v err=%v, want nil result and INPUT_REQUEST_INVALID", result, err)
				}
				return
			}
			if tc.name == "set chat theme" {
				if result || !errors.Is(err, mtproto.ErrPeerIdInvalid) {
					t.Fatalf("result=%v err=%v, want nil result and PEER_ID_INVALID", result, err)
				}
				return
			}
			if tc.name == "create" || tc.name == "update" {
				if result || !errors.Is(err, mtproto.ErrThemeInvalid) {
					t.Fatalf("result=%v err=%v, want nil result and THEME_INVALID", result, err)
				}
				return
			}
			if tc.name == "chat themes" {
				if !result || err != nil {
					t.Fatalf("result=%v err=%v, want persisted empty chat-theme catalogue", result, err)
				}
				return
			}
			if result || !errors.Is(err, mtproto.ErrMethodNotImpl) {
				t.Fatalf("result=%v err=%v, want nil result and METHOD_NOT_IMPL", result, err)
			}
		})
	}
}

func TestThemeGetRejectsMissingTheme(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: time.Now().UnixNano()}}
	if result, err := c.AccountGetTheme(nil); result != nil || !errors.Is(err, mtproto.ErrThemeInvalid) {
		t.Fatalf("result=%v err=%v, want nil result and THEME_INVALID", result, err)
	}
}

func TestWallpaperMethodsRejectInvalidUploadWithoutMediaBackend(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: time.Now().UnixNano()}}
	cases := []struct {
		name string
		call func() (bool, error)
	}{
		{"upload", func() (bool, error) {
			result, err := c.AccountUploadWallPaper(nil)
			return result != nil, err
		}},
		{"set chat wallpaper", func() (bool, error) {
			result, err := c.MessagesSetChatWallPaper(nil)
			return result != nil, err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.call()
			if tc.name == "upload" {
				if result || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
					t.Fatalf("result=%v err=%v, want nil result and INPUT_REQUEST_INVALID", result, err)
				}
				return
			}
			if tc.name == "set chat wallpaper" {
				if result || !errors.Is(err, mtproto.ErrPeerIdInvalid) {
					t.Fatalf("result=%v err=%v, want nil result and PEER_ID_INVALID", result, err)
				}
				return
			}
			if result || !errors.Is(err, mtproto.ErrMethodNotImpl) {
				t.Fatalf("result=%v err=%v, want nil result and METHOD_NOT_IMPL", result, err)
			}
		})
	}
}

func TestWallpaperMethodsRejectMissingWallpaper(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: time.Now().UnixNano()}}
	if result, err := c.AccountGetWallPaper(nil); result != nil || !errors.Is(err, mtproto.ErrWallpaperInvalid) {
		t.Fatalf("get result=%v err=%v, want nil result and WALLPAPER_INVALID", result, err)
	}
	if result, err := c.AccountSaveWallPaper(nil); result != nil || !errors.Is(err, mtproto.ErrWallpaperInvalid) {
		t.Fatalf("save result=%v err=%v, want nil result and WALLPAPER_INVALID", result, err)
	}
	if result, err := c.AccountInstallWallPaper(nil); result != nil || !errors.Is(err, mtproto.ErrWallpaperInvalid) {
		t.Fatalf("install result=%v err=%v, want nil result and WALLPAPER_INVALID", result, err)
	}
}
