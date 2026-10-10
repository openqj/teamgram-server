package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	bffdao "github.com/teamgram/teamgram-server/app/bff/savedmessagedialogs/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/savedmessagedialogs/internal/svc"
	msgclient "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type savedHistoryDeleteMessageClient struct {
	messageclient.MessageClient
	page *mtproto.MessageBoxList
	seen []*messagepb.TLMessageGetSavedHistoryMessages
}

func (c *savedHistoryDeleteMessageClient) MessageGetSavedHistoryMessages(_ context.Context, in *messagepb.TLMessageGetSavedHistoryMessages) (*mtproto.MessageBoxList, error) {
	c.seen = append(c.seen, in)
	if len(c.seen) == 1 {
		return c.page, nil
	}
	return &mtproto.MessageBoxList{}, nil
}

type savedHistoryDeleteMsgClient struct {
	msgclient.MsgClient
	request *msgpb.TLMsgDeleteMessages
}

func (c *savedHistoryDeleteMsgClient) MsgDeleteMessages(_ context.Context, in *msgpb.TLMsgDeleteMessages) (*mtproto.Messages_AffectedMessages, error) {
	c.request = in
	return mtproto.MakeTLMessagesAffectedMessages(&mtproto.Messages_AffectedMessages{Pts: 71, PtsCount: int32(len(in.GetId()))}).To_Messages_AffectedMessages(), nil
}

func newDeleteSavedHistoryCore(messages messageclient.MessageClient, deleter msgclient.MsgClient, uid int64) *SavedMessageDialogsCore {
	ctx := context.Background()
	return &SavedMessageDialogsCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &bffdao.Dao{MessageClient: messages, MsgClient: deleter}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: uid, PermAuthKeyId: 19},
	}
}

func TestMessagesDeleteSavedHistoryCoordinatesPostgresMessageDeletion(t *testing.T) {
	messages := &savedHistoryDeleteMessageClient{page: &mtproto.MessageBoxList{BoxList: []*mtproto.MessageBox{
		{MessageId: 11, Message: &mtproto.Message{Id: 11, Date: 120}},
		{MessageId: 12, Message: &mtproto.Message{Id: 12, Date: 121}},
	}}}
	deleter := &savedHistoryDeleteMsgClient{}
	core := newDeleteSavedHistoryCore(messages, deleter, 42)
	result, err := core.MessagesDeleteSavedHistory(&mtproto.TLMessagesDeleteSavedHistory{
		Peer:    mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 99, AccessHash: 7}).To_InputPeer(),
		MaxId:   100,
		MinDate: wrapperspb.Int32(121),
		MaxDate: wrapperspb.Int32(130),
	})
	if err != nil || result == nil || result.GetPts() != 71 || result.GetPtsCount() != 1 {
		t.Fatalf("delete result = (%v, %v), want pts 71/count 1", result, err)
	}
	if deleter.request == nil || len(deleter.request.GetId()) != 1 || deleter.request.GetId()[0] != 12 || deleter.request.GetRevoke() || deleter.request.GetPeerType() != mtproto.PEER_EMPTY {
		t.Fatalf("delete request = %+v", deleter.request)
	}
	if len(messages.seen) != 1 || messages.seen[0].GetMaxId() != 100 || messages.seen[0].GetOffsetDate() != 0 {
		t.Fatalf("saved history requests = %+v", messages.seen)
	}
}

func TestMessagesDeleteSavedHistoryRequiresPostgresProviders(t *testing.T) {
	core := newDeleteSavedHistoryCore(nil, nil, 42)
	result, err := core.MessagesDeleteSavedHistory(&mtproto.TLMessagesDeleteSavedHistory{
		Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 99, AccessHash: 7}).To_InputPeer(),
	})
	if result != nil || err != mtproto.ErrMethodNotImpl {
		t.Fatalf("missing providers = (%v, %v), want METHOD_NOT_IMPL", result, err)
	}
}

func TestMessagesDeleteSavedHistoryValidatesParentAndDateRange(t *testing.T) {
	core := newDeleteSavedHistoryCore(nil, nil, 42)
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 99, AccessHash: 7}).To_InputPeer()
	for name, request := range map[string]*mtproto.TLMessagesDeleteSavedHistory{
		"foreign parent": {ParentPeer: peer, Peer: peer},
		"reversed dates": {Peer: peer, MinDate: wrapperspb.Int32(20), MaxDate: wrapperspb.Int32(10)},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := core.MessagesDeleteSavedHistory(request)
			if result != nil || err != mtproto.ErrPeerIdInvalid && err != mtproto.ErrInputRequestInvalid {
				t.Fatalf("request = (%v, %v), want validation error", result, err)
			}
		})
	}
}
