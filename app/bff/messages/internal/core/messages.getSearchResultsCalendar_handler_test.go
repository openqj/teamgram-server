package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type searchCalendarMessageClientStub struct {
	messageclient.MessageClient
	result *mtproto.MessageBoxList
	err    error
}

func (s *searchCalendarMessageClientStub) MessageSearchByMediaType(context.Context, *messagepb.TLMessageSearchByMediaType) (*mtproto.MessageBoxList, error) {
	return s.result, s.err
}

type searchCalendarUserClientStub struct {
	userclient.UserClient
	users *userpb.Vector_ImmutableUser
}

func (s *searchCalendarUserClientStub) UserGetMutableUsers(context.Context, *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	return s.users, nil
}

func newSearchCalendarCore(messages messageclient.MessageClient, users userclient.UserClient) *MessagesCore {
	ctx := context.Background()
	return &MessagesCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			MessageClient: messages,
			UserClient:    users,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
}

func searchCalendarBox(id, date int32) *mtproto.MessageBox {
	message := mtproto.MakeTLMessage(&mtproto.Message{
		Id:     id,
		Date:   date,
		FromId: mtproto.MakePeerUser(42),
		PeerId: mtproto.MakePeerUser(43),
	}).To_Message()
	return mtproto.MakeTLMessageBox(&mtproto.MessageBox{
		MessageId: id,
		PeerType:  mtproto.PEER_USER,
		PeerId:    43,
		Message:   message,
	}).To_MessageBox()
}

func searchCalendarFilter() *mtproto.MessagesFilter {
	return mtproto.MakeTLInputMessagesFilterPhotoVideo(&mtproto.MessagesFilter{}).To_MessagesFilter()
}

func TestMessagesGetSearchResultsCalendarBucketsProviderMessages(t *testing.T) {
	messages := mtproto.MakeTLMessageBoxList(&mtproto.MessageBoxList{BoxList: []*mtproto.MessageBox{
		searchCalendarBox(11, 172800+120),
		searchCalendarBox(10, 172800+60),
		searchCalendarBox(9, 86400+30),
	}}).To_MessageBoxList()
	users := &searchCalendarUserClientStub{users: &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}).To_ImmutableUser(),
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{Id: 43}}).To_ImmutableUser(),
	}}}
	core := newSearchCalendarCore(&searchCalendarMessageClientStub{result: messages}, users)
	result, err := core.MessagesGetSearchResultsCalendar(&mtproto.TLMessagesGetSearchResultsCalendar{
		Peer:   mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 43, AccessHash: 143}).To_InputPeer(),
		Filter: searchCalendarFilter(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.GetCount() != 3 || len(result.GetPeriods()) != 2 || len(result.GetMessages()) != 2 {
		t.Fatalf("calendar result = %v, want 3 messages in 2 periods with 2 representative messages", result)
	}
	if result.GetPeriods()[0].GetCount() != 2 || result.GetPeriods()[1].GetCount() != 1 {
		t.Fatalf("calendar periods = %v, want counts [2 1]", result.GetPeriods())
	}
	if err := result.Encode(mtproto.NewEncodeBuf(512), 229); err != nil {
		t.Fatalf("encode calendar result: %v", err)
	}
}

func TestMessagesGetSearchResultsCalendarRejectsMissingProviderResult(t *testing.T) {
	wantErr := errors.New("search unavailable")
	core := newSearchCalendarCore(&searchCalendarMessageClientStub{err: wantErr}, &searchCalendarUserClientStub{})
	result, err := core.MessagesGetSearchResultsCalendar(&mtproto.TLMessagesGetSearchResultsCalendar{
		Peer:   mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 43, AccessHash: 143}).To_InputPeer(),
		Filter: searchCalendarFilter(),
	})
	if result != nil || !errors.Is(err, wantErr) {
		t.Fatalf("provider failure = (%v, %v), want search error", result, err)
	}
}
