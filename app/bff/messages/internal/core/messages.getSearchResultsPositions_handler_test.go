package core

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"google.golang.org/protobuf/proto"
)

type searchResultsPositionsMessageClient struct {
	messageclient.MessageClient
	counter    *mtproto.Int32
	counterErr error
	search     func(*messagepb.TLMessageSearchByMediaType) (*mtproto.MessageBoxList, error)
	counterIn  []*messagepb.TLMessageGetSearchCounter
	searchIn   []*messagepb.TLMessageSearchByMediaType
}

func (c *searchResultsPositionsMessageClient) MessageGetSearchCounter(_ context.Context, in *messagepb.TLMessageGetSearchCounter) (*mtproto.Int32, error) {
	c.counterIn = append(c.counterIn, proto.Clone(in).(*messagepb.TLMessageGetSearchCounter))
	return c.counter, c.counterErr
}

func (c *searchResultsPositionsMessageClient) MessageSearchByMediaType(_ context.Context, in *messagepb.TLMessageSearchByMediaType) (*mtproto.MessageBoxList, error) {
	c.searchIn = append(c.searchIn, proto.Clone(in).(*messagepb.TLMessageSearchByMediaType))
	if c.search == nil {
		return nil, nil
	}
	return c.search(in)
}

func newSearchResultsPositionsCore(client messageclient.MessageClient, userID int64) *MessagesCore {
	return &MessagesCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{MessageClient: client}},
		MD:     &metadata.RpcMetadata{UserId: userID},
	}
}

func searchResultsPositionsBox(id, date int32) *mtproto.MessageBox {
	msg := mtproto.MakeTLMessage(&mtproto.Message{Id: id, Date: date}).To_Message()
	return mtproto.MakeTLMessageBox(&mtproto.MessageBox{MessageId: id, Message: msg}).To_MessageBox()
}

func searchResultsPositionsPages(boxes []*mtproto.MessageBox) func(*messagepb.TLMessageSearchByMediaType) (*mtproto.MessageBoxList, error) {
	return func(in *messagepb.TLMessageSearchByMediaType) (*mtproto.MessageBoxList, error) {
		page := make([]*mtproto.MessageBox, 0, in.GetLimit())
		for _, box := range boxes {
			if box.GetMessageId() >= in.GetOffset() {
				continue
			}
			page = append(page, box)
			if int32(len(page)) == in.GetLimit() {
				break
			}
		}
		return mtproto.MakeTLMessageBoxList(&mtproto.MessageBoxList{BoxList: page}).To_MessageBoxList(), nil
	}
}

func searchResultsPositionsPeer(id int64) *mtproto.InputPeer {
	return mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: id, AccessHash: id + 100}).To_InputPeer()
}

func searchResultsPositionsFilter() *mtproto.MessagesFilter {
	return mtproto.MakeTLInputMessagesFilterPhotoVideo(&mtproto.MessagesFilter{}).To_MessagesFilter()
}

func TestMessagesGetSearchResultsPositionsBuildsGlobalPhotoVideoPositions(t *testing.T) {
	boxes := make([]*mtproto.MessageBox, 0, searchResultsPositionsPageSize+1)
	for id := int32(3000); id >= 2001; id-- {
		boxes = append(boxes, searchResultsPositionsBox(id, id+1000))
	}
	boxes = append(boxes, searchResultsPositionsBox(999, 1999))
	client := &searchResultsPositionsMessageClient{
		counter: &mtproto.Int32{V: int32(len(boxes))},
		search:  searchResultsPositionsPages(boxes),
	}
	core := newSearchResultsPositionsCore(client, 41)

	got, err := core.MessagesGetSearchResultsPositions(&mtproto.TLMessagesGetSearchResultsPositions{
		Peer: searchResultsPositionsPeer(42), Filter: searchResultsPositionsFilter(), OffsetId: 1500, Limit: 2,
	})
	if err != nil {
		t.Fatalf("MessagesGetSearchResultsPositions() error = %v", err)
	}
	if got == nil || got.GetCount() != int32(len(boxes)) {
		t.Fatalf("result = %+v, want count %d", got, len(boxes))
	}
	positions := got.GetPositions()
	if len(positions) != 1 || positions[0].GetMsgId() != 999 || positions[0].GetDate() != 1999 || positions[0].GetOffset() != 1000 {
		t.Fatalf("positions = %+v, want message 999 at global offset 1000", positions)
	}
	if len(client.counterIn) != 1 || client.counterIn[0].GetMediaType() != mtproto.MEDIA_PHOTOVIDEO {
		t.Fatalf("counter request = %+v, want photo/video media type", client.counterIn)
	}
	if len(client.searchIn) != 3 {
		t.Fatalf("search calls = %d, want two rank pages and one result page", len(client.searchIn))
	}
	if client.searchIn[0].GetOffset() != math.MaxInt32 || client.searchIn[1].GetOffset() != 2001 || client.searchIn[2].GetOffset() != 1500 {
		t.Fatalf("search offsets = (%d, %d, %d), want (max, 2001, 1500)", client.searchIn[0].GetOffset(), client.searchIn[1].GetOffset(), client.searchIn[2].GetOffset())
	}
	if err = got.Encode(mtproto.NewEncodeBuf(128), 229); err != nil {
		t.Fatalf("encode search result positions: %v", err)
	}
}

func TestMessagesGetSearchResultsPositionsHandlesEmptyAndMismatchedResults(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		client := &searchResultsPositionsMessageClient{counter: &mtproto.Int32{}}
		got, err := newSearchResultsPositionsCore(client, 41).MessagesGetSearchResultsPositions(&mtproto.TLMessagesGetSearchResultsPositions{
			Peer: searchResultsPositionsPeer(42), Filter: searchResultsPositionsFilter(), Limit: 1,
		})
		if err != nil || got == nil || got.GetCount() != 0 || len(got.GetPositions()) != 0 {
			t.Fatalf("result = (%v, %v), want empty success", got, err)
		}
		if len(client.searchIn) != 0 {
			t.Fatalf("search calls = %d, want 0", len(client.searchIn))
		}
	})

	t.Run("counter mismatch", func(t *testing.T) {
		client := &searchResultsPositionsMessageClient{
			counter: &mtproto.Int32{V: 3},
			search: searchResultsPositionsPages([]*mtproto.MessageBox{
				searchResultsPositionsBox(200, 2000), searchResultsPositionsBox(100, 1000),
			}),
		}
		got, err := newSearchResultsPositionsCore(client, 41).MessagesGetSearchResultsPositions(&mtproto.TLMessagesGetSearchResultsPositions{
			Peer: searchResultsPositionsPeer(42), Filter: searchResultsPositionsFilter(), OffsetId: 150, Limit: 1,
		})
		if got != nil || err != mtproto.ErrInternalServerError {
			t.Fatalf("result = (%v, %v), want INTERNAL_SERVER_ERROR", got, err)
		}
	})
}

func TestMessagesGetSearchResultsPositionsHandlesOffsetBelowAFullResultPage(t *testing.T) {
	boxes := make([]*mtproto.MessageBox, 0, searchResultsPositionsPageSize)
	for id := int32(2000); id > 1000; id-- {
		boxes = append(boxes, searchResultsPositionsBox(id, id+1000))
	}
	client := &searchResultsPositionsMessageClient{
		counter: &mtproto.Int32{V: int32(len(boxes))},
		search:  searchResultsPositionsPages(boxes),
	}

	got, err := newSearchResultsPositionsCore(client, 41).MessagesGetSearchResultsPositions(&mtproto.TLMessagesGetSearchResultsPositions{
		Peer: searchResultsPositionsPeer(42), Filter: searchResultsPositionsFilter(), OffsetId: 1, Limit: 1,
	})
	if err != nil || got == nil || got.GetCount() != int32(len(boxes)) || len(got.GetPositions()) != 0 {
		t.Fatalf("result = (%+v, %v), want an empty page after all %d results", got, err, len(boxes))
	}
	if len(client.searchIn) != 3 || client.searchIn[1].GetOffset() != 1001 || client.searchIn[2].GetOffset() != 1 {
		t.Fatalf("search calls = %+v, want full-page confirmation then the requested empty page", client.searchIn)
	}
}

func TestMessagesGetSearchResultsPositionsPropagatesFailures(t *testing.T) {
	counterErr := errors.New("counter unavailable")
	searchErr := errors.New("search unavailable")
	tests := []struct {
		name   string
		client *searchResultsPositionsMessageClient
		want   error
	}{
		{name: "counter error", client: &searchResultsPositionsMessageClient{counterErr: counterErr}, want: counterErr},
		{name: "nil counter", client: &searchResultsPositionsMessageClient{}, want: mtproto.ErrInternalServerError},
		{name: "search error", client: &searchResultsPositionsMessageClient{counter: &mtproto.Int32{V: 1}, search: func(*messagepb.TLMessageSearchByMediaType) (*mtproto.MessageBoxList, error) { return nil, searchErr }}, want: searchErr},
		{name: "nil search result", client: &searchResultsPositionsMessageClient{counter: &mtproto.Int32{V: 1}}, want: mtproto.ErrInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newSearchResultsPositionsCore(tt.client, 41).MessagesGetSearchResultsPositions(&mtproto.TLMessagesGetSearchResultsPositions{
				Peer: searchResultsPositionsPeer(42), Filter: searchResultsPositionsFilter(), Limit: 1,
			})
			if got != nil || !errors.Is(err, tt.want) {
				t.Fatalf("result = (%v, %v), want nil and %v", got, err, tt.want)
			}
		})
	}
}

func TestMessagesGetSearchResultsPositionsRejectsInvalidRequests(t *testing.T) {
	client := &searchResultsPositionsMessageClient{counter: &mtproto.Int32{}}
	core := newSearchResultsPositionsCore(client, 41)
	valid := func() *mtproto.TLMessagesGetSearchResultsPositions {
		return &mtproto.TLMessagesGetSearchResultsPositions{Peer: searchResultsPositionsPeer(42), Filter: searchResultsPositionsFilter(), Limit: 1}
	}
	tests := []struct {
		name string
		in   *mtproto.TLMessagesGetSearchResultsPositions
		want error
	}{
		{name: "nil request", want: mtproto.ErrInputRequestInvalid},
		{name: "missing peer", in: &mtproto.TLMessagesGetSearchResultsPositions{Filter: searchResultsPositionsFilter()}, want: mtproto.ErrPeerIdInvalid},
		{name: "missing filter", in: &mtproto.TLMessagesGetSearchResultsPositions{Peer: searchResultsPositionsPeer(42)}, want: mtproto.ErrInputFilterInvalid},
		{name: "zero limit", in: func() *mtproto.TLMessagesGetSearchResultsPositions { in := valid(); in.Limit = 0; return in }(), want: mtproto.ErrLimitInvalid},
		{name: "oversized limit", in: func() *mtproto.TLMessagesGetSearchResultsPositions {
			in := valid()
			in.Limit = searchResultsPositionsMaxLimit + 1
			return in
		}(), want: mtproto.ErrLimitInvalid},
		{name: "negative offset", in: func() *mtproto.TLMessagesGetSearchResultsPositions { in := valid(); in.OffsetId = -1; return in }(), want: mtproto.ErrOffsetInvalid},
		{name: "saved peer", in: func() *mtproto.TLMessagesGetSearchResultsPositions {
			in := valid()
			in.SavedPeerId = searchResultsPositionsPeer(43)
			return in
		}(), want: mtproto.ErrMethodNotImpl},
		{name: "channel", in: &mtproto.TLMessagesGetSearchResultsPositions{Peer: mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 42, AccessHash: 142}).To_InputPeer(), Filter: searchResultsPositionsFilter(), Limit: 1}, want: mtproto.ErrMethodNotImpl},
		{name: "unsupported filter", in: &mtproto.TLMessagesGetSearchResultsPositions{Peer: searchResultsPositionsPeer(42), Filter: mtproto.MakeTLInputMessagesFilterEmpty(&mtproto.MessagesFilter{}).To_MessagesFilter(), Limit: 1}, want: mtproto.ErrFilterNotSupported},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := core.MessagesGetSearchResultsPositions(tt.in)
			if got != nil || err != tt.want {
				t.Fatalf("result = (%v, %v), want nil and %v", got, err, tt.want)
			}
		})
	}
	core.MD.IsBot = true
	got, err := core.MessagesGetSearchResultsPositions(valid())
	if got != nil || err != mtproto.ErrBotMethodInvalid {
		t.Fatalf("bot result = (%v, %v), want BOT_METHOD_INVALID", got, err)
	}
	core.MD.IsBot = false
	core.MD.UserId = 0
	got, err = core.MessagesGetSearchResultsPositions(valid())
	if got != nil || err != mtproto.ErrActiveUserRequired {
		t.Fatalf("unauthenticated result = (%v, %v), want ACTIVE_USER_REQUIRED", got, err)
	}
}
