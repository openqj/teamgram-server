package core

import (
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
)

func seedChatlistTestState(t *testing.T, uid int64, filterID int32, state *clState) *mtproto.InputChatlist {
	t.Helper()
	filter := mtproto.MakeTLDialogFilterChatlist(&mtproto.DialogFilter{
		Id:           filterID,
		Title_STRING: "audit chatlist",
		IncludePeers: []*mtproto.InputPeer{},
		PinnedPeers:  []*mtproto.InputPeer{},
	}).To_DialogFilter()
	if err := saveDialogFilters(uid, []*mtproto.DialogFilter{filter}); err != nil {
		t.Fatal(err)
	}
	if err := saveClist(uid, state); err != nil {
		t.Fatal(err)
	}
	return mtproto.MakeTLInputChatlistDialogFilter(&mtproto.InputChatlist{FilterId: filterID}).To_InputChatlist()
}

func TestChatlistGetUpdatesRequiresInviteSource(t *testing.T) {
	_ = isolatedAuditDSN(t)
	uid := time.Now().UnixNano()
	chatlist := seedChatlistTestState(t, uid, 2, &clState{})
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}}}

	result, err := c.ChatlistsGetChatlistUpdates(&mtproto.TLChatlistsGetChatlistUpdates{Chatlist: chatlist})
	if result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("updates without an invite source: result=%+v err=%v", result, err)
	}
}

func TestChatlistGetUpdatesReturnsInvitePeers(t *testing.T) {
	_ = isolatedAuditDSN(t)
	uid := time.Now().UnixNano()
	channelID := uid + 1
	peer := mtproto.MakeInputPeerChannel(channelID)
	state := &clState{Invites: []clInvite{{FilterID: 2, Slug: "audit-invite", Peers: []*mtproto.InputPeer{peer}}}}
	chatlist := seedChatlistTestState(t, uid, 2, state)
	if err := domain.SaveChannel(domain.Channel{ID: channelID, Creator: uid, Title: "audit channel"}); err != nil {
		t.Fatal(err)
	}
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}}}

	result, err := c.ChatlistsGetChatlistUpdates(&mtproto.TLChatlistsGetChatlistUpdates{Chatlist: chatlist})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.GetMissingPeers()) != 1 || len(result.GetChats()) != 1 {
		t.Fatalf("invite updates=%+v, want one missing channel and its chat", result)
	}
	if result.GetMissingPeers()[0].GetPredicateName() != mtproto.Predicate_peerChannel || result.GetMissingPeers()[0].GetChannelId() != channelID {
		t.Fatalf("missing peer=%+v, want channel %d", result.GetMissingPeers()[0], channelID)
	}
}

func TestChatlistHideUpdatesPersistsAndSuppresses(t *testing.T) {
	_ = isolatedAuditDSN(t)
	uid := time.Now().UnixNano()
	peer := mtproto.MakeInputPeerChannel(uid + 1)
	state := &clState{Invites: []clInvite{{FilterID: 2, Slug: "audit-invite", Peers: []*mtproto.InputPeer{peer}}}}
	chatlist := seedChatlistTestState(t, uid, 2, state)
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}

	hidden, err := c.ChatlistsHideChatlistUpdates(&mtproto.TLChatlistsHideChatlistUpdates{Chatlist: chatlist})
	if err != nil || !mtproto.FromBool(hidden) {
		t.Fatalf("hide updates: result=%v err=%v", hidden, err)
	}
	stored, err := loadClist(uid)
	if err != nil || len(stored.Hidden) != 1 || stored.Hidden[0] != 2 {
		t.Fatalf("hidden state=%+v err=%v", stored, err)
	}
	updates, err := c.ChatlistsGetChatlistUpdates(&mtproto.TLChatlistsGetChatlistUpdates{Chatlist: chatlist})
	if err != nil || updates == nil || len(updates.GetMissingPeers()) != 0 {
		t.Fatalf("updates after hide=%+v err=%v, want an empty update", updates, err)
	}
}

func TestChatlistJoinUpdatesRejectsPeersOutsideInvite(t *testing.T) {
	_ = isolatedAuditDSN(t)
	uid := time.Now().UnixNano()
	allowed := mtproto.MakeInputPeerChannel(uid + 1)
	outsider := mtproto.MakeInputPeerChannel(uid + 2)
	state := &clState{Invites: []clInvite{{FilterID: 2, Slug: "audit-invite", Peers: []*mtproto.InputPeer{allowed}}}}
	chatlist := seedChatlistTestState(t, uid, 2, state)
	before, err := persist.Default.Get(clistKey(uid))
	if err != nil {
		t.Fatal(err)
	}
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}

	result, err := c.ChatlistsJoinChatlistUpdates(&mtproto.TLChatlistsJoinChatlistUpdates{Chatlist: chatlist, Peers: []*mtproto.InputPeer{outsider}})
	if result != nil || !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("join outside invite: result=%+v err=%v", result, err)
	}
	after, err := persist.Default.Get(clistKey(uid))
	if err != nil || after != before {
		t.Fatalf("joined state changed: before=%q after=%q err=%v", before, after, err)
	}
}

func TestChatlistLeaveSuggestionsUnavailable(t *testing.T) {
	_ = isolatedAuditDSN(t)
	if result, err := (&ApiFullCore{}).ChatlistsGetLeaveChatlistSuggestions(nil); result != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("unauthenticated leave suggestions: result=%+v err=%v", result, err)
	}
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: time.Now().UnixNano()}}
	if result, err := c.ChatlistsGetLeaveChatlistSuggestions(nil); result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("leave suggestions without a suggestion source: result=%+v err=%v", result, err)
	}
}

func TestChatlistInviteLifecyclePersistsAndCleans(t *testing.T) {
	_ = isolatedAuditDSN(t)
	uid := time.Now().UnixNano()
	channelID := uid + 1
	peer := mtproto.MakeInputPeerChannel(channelID)
	chatlist := seedChatlistTestState(t, uid, 2, &clState{})
	if err := domain.SaveChannel(domain.Channel{ID: channelID, AccessHash: channelID, Creator: uid, Title: "lifecycle channel"}); err != nil {
		t.Fatal(err)
	}
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}}}
	t.Cleanup(func() {
		_ = persist.Default.Set(clistKey(uid), "")
		_ = persist.Default.Set(dialogFiltersKey(uid), "")
		_ = domain.DeleteChannel(uid, channelID)
	})

	exported, err := c.ChatlistsExportChatlistInvite(&mtproto.TLChatlistsExportChatlistInvite{
		Chatlist: chatlist,
		Title:    "shared list",
		Peers:    []*mtproto.InputPeer{peer},
	})
	if err != nil || exported == nil || exported.GetInvite() == nil || exported.GetInvite().GetUrl() == "" {
		t.Fatalf("exported invite = (%+v, %v), want persisted invite", exported, err)
	}
	slug := exported.GetInvite().GetUrl()

	listed, err := c.ChatlistsGetExportedInvites(nil)
	if err != nil || listed == nil || len(listed.GetInvites()) != 1 || len(listed.GetChats()) != 1 {
		t.Fatalf("listed invites = (%+v, %v), want one invite and channel", listed, err)
	}
	checked, err := c.ChatlistsCheckChatlistInvite(&mtproto.TLChatlistsCheckChatlistInvite{Slug: slug})
	if err != nil || checked == nil || len(checked.GetPeers()) != 1 || len(checked.GetChats()) != 1 {
		t.Fatalf("checked invite = (%+v, %v), want one peer and channel", checked, err)
	}

	title := "renamed list"
	edited, err := c.ChatlistsEditExportedInvite(&mtproto.TLChatlistsEditExportedInvite{
		Slug:  slug,
		Title: mtproto.MakeFlagsString(title),
	})
	if err != nil || edited == nil || edited.GetTitle() != title {
		t.Fatalf("edited invite = (%+v, %v), want title %q", edited, err, title)
	}
	if result, err := c.ChatlistsDeleteExportedInvite(&mtproto.TLChatlistsDeleteExportedInvite{Slug: slug}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("delete invite = (%v, %v), want BoolTrue", result, err)
	}
	listed, err = c.ChatlistsGetExportedInvites(nil)
	if err != nil || listed == nil || len(listed.GetInvites()) != 0 {
		t.Fatalf("listed after delete = (%+v, %v), want empty", listed, err)
	}
}

func TestChatlistInviteMutationsRejectMissingSlug(t *testing.T) {
	uid := time.Now().UnixNano()
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	if result, err := c.ChatlistsDeleteExportedInvite(&mtproto.TLChatlistsDeleteExportedInvite{}); result != nil || !errors.Is(err, mtproto.ErrInviteHashInvalid) {
		t.Fatalf("delete empty slug = (%+v, %v), want INVITE_HASH_INVALID", result, err)
	}
	if result, err := c.ChatlistsEditExportedInvite(&mtproto.TLChatlistsEditExportedInvite{}); result != nil || !errors.Is(err, mtproto.ErrInviteHashInvalid) {
		t.Fatalf("edit empty slug = (%+v, %v), want INVITE_HASH_INVALID", result, err)
	}
}

func TestSuggestedDialogFilters(t *testing.T) {
	_ = isolatedAuditDSN(t)
	if result, err := (&ApiFullCore{}).MessagesGetSuggestedDialogFilters(nil); result != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("unauthenticated suggested filters: result=%+v err=%v", result, err)
	}
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: time.Now().UnixNano()}}
	result, err := c.MessagesGetSuggestedDialogFilters(nil)
	if err != nil || result == nil || len(result.GetDatas()) != 3 {
		t.Fatalf("suggested filters: result=%+v err=%v", result, err)
	}
}
