package core

import (
	"context"
	"math"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"github.com/zeromicro/go-zero/core/logx"
)

type savedPeerSearchMessageClient struct {
	messageclient.MessageClient
	pages []*mtproto.MessageBoxList
	calls int
}

func (c *savedPeerSearchMessageClient) MessageGetSavedHistoryMessages(_ context.Context, _ *messagepb.TLMessageGetSavedHistoryMessages) (*mtproto.MessageBoxList, error) {
	page := c.pages[c.calls]
	c.calls++
	return page, nil
}

func newSavedPeerSearchCore(client messageclient.MessageClient) *MessagesCore {
	ctx := context.Background()
	return &MessagesCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{MessageClient: client}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 41},
	}
}

func savedPeerSearchPage(startID int32, filterType int32, text string, count int32) *mtproto.MessageBoxList {
	boxes := make([]*mtproto.MessageBox, 0, savedHistoryPage)
	for i := int32(0); i < savedHistoryPage; i++ {
		id := startID - i
		message := mtproto.MakeTLMessage(&mtproto.Message{Id: id, Message: text}).To_Message()
		boxes = append(boxes, mtproto.MakeTLMessageBox(&mtproto.MessageBox{
			MessageId:         id,
			Message:           message,
			MessageFilterType: filterType,
		}).To_MessageBox())
	}
	return mtproto.MakeTLMessageBoxList(&mtproto.MessageBoxList{BoxList: boxes, Count: count}).To_MessageBoxList()
}

func savedPeerSearchSelf() *mtproto.InputPeer {
	return mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
}

func TestSearchSavedPeerFailsClosedAfterUnboundedProviderPages(t *testing.T) {
	pages := make([]*mtproto.MessageBoxList, 10)
	for i := range pages {
		// Count is deliberately non-zero but not authoritative. The provider
		// may retain a legacy value after the service stopped exposing SQL totals.
		pages[i] = savedPeerSearchPage(10000-int32(i)*savedHistoryPage, 0, "other", 1)
	}
	client := &savedPeerSearchMessageClient{pages: pages}
	core := newSavedPeerSearchCore(client)
	request := &mtproto.TLMessagesSearch{
		SavedPeerId: savedPeerSearchSelf(),
		Filter:      mtproto.MakeTLInputMessagesFilterEmpty(&mtproto.MessagesFilter{}).To_MessagesFilter(),
		Q:           "needle",
		Limit:       1,
	}
	result := mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{}).To_Messages_Messages()

	got, err := core.searchSavedPeer(request, result, nil, math.MaxInt32, 1)
	if got != nil || err != mtproto.ErrMethodNotImpl {
		t.Fatalf("searchSavedPeer() = (%v, %v), want METHOD_NOT_IMPL after scan cap", got, err)
	}
	if client.calls != len(pages) {
		t.Fatalf("provider calls = %d, want %d pages before fail-closed", client.calls, len(pages))
	}
}

func TestSavedCountersMarksUnboundedProviderPagesInexact(t *testing.T) {
	pages := make([]*mtproto.MessageBoxList, 10)
	for i := range pages {
		// A stale Count must not make a full scan appear complete or exact.
		pages[i] = savedPeerSearchPage(20000-int32(i)*savedHistoryPage, mtproto.MEDIA_PHOTOS_ONLY, "", 1)
	}
	client := &savedPeerSearchMessageClient{pages: pages}
	core := newSavedPeerSearchCore(client)
	filter := mtproto.MakeTLInputMessagesFilterPhotos(&mtproto.MessagesFilter{}).To_MessagesFilter()
	missingFilter := mtproto.MakeTLInputMessagesFilterVideo(&mtproto.MessagesFilter{}).To_MessagesFilter()

	got, err := core.savedCounters(savedPeerSearchSelf(), []*mtproto.MessagesFilter{filter, missingFilter})
	if err != nil {
		t.Fatalf("savedCounters() error = %v", err)
	}
	if got == nil || len(got.GetDatas()) != 2 {
		t.Fatalf("savedCounters() = %v, want two counters", got)
	}
	counter := got.GetDatas()[0]
	if !counter.GetInexact() || counter.GetCount() != int32(savedHistoryPage*int32(len(pages))) {
		t.Fatalf("counter = %v, want inexact count %d", counter, savedHistoryPage*int32(len(pages)))
	}
	missing := got.GetDatas()[1]
	if !missing.GetInexact() || missing.GetCount() != 0 {
		t.Fatalf("missing counter = %v, want inexact zero count", missing)
	}
	if client.calls != len(pages) {
		t.Fatalf("provider calls = %d, want %d pages before marking inexact", client.calls, len(pages))
	}
}
