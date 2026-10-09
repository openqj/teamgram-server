package mq

import (
	"context"
	"encoding/json"
	"fmt"

	kafka "github.com/teamgram/marmota/pkg/mq"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/internal/core"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/internal/svc"
	"github.com/teamgram/teamgram-server/pkg/mqconsumer"
	"google.golang.org/protobuf/proto"
)

func New(svcCtx *svc.ServiceContext, conf kafka.KafkaConsumerConf) *mqconsumer.Consumer {
	consumer, err := mqconsumer.New(conf, func(ctx context.Context, method, _ string, value []byte) error {
		c := core.New(ctx, svcCtx)
		switch method {
		case string(proto.MessageName((*inbox.TLInboxEditUserMessageToInbox)(nil))):
			return dispatch(value, new(inbox.TLInboxEditUserMessageToInbox), c.InboxEditUserMessageToInbox)
		case string(proto.MessageName((*inbox.TLInboxEditChatMessageToInbox)(nil))):
			return dispatch(value, new(inbox.TLInboxEditChatMessageToInbox), c.InboxEditChatMessageToInbox)
		case string(proto.MessageName((*inbox.TLInboxDeleteMessagesToInbox)(nil))):
			return dispatch(value, new(inbox.TLInboxDeleteMessagesToInbox), c.InboxDeleteMessagesToInbox)
		case string(proto.MessageName((*inbox.TLInboxDeleteUserHistoryToInbox)(nil))):
			return dispatch(value, new(inbox.TLInboxDeleteUserHistoryToInbox), c.InboxDeleteUserHistoryToInbox)
		case string(proto.MessageName((*inbox.TLInboxDeleteChatHistoryToInbox)(nil))):
			return dispatch(value, new(inbox.TLInboxDeleteChatHistoryToInbox), c.InboxDeleteChatHistoryToInbox)
		case string(proto.MessageName((*inbox.TLInboxReadUserMediaUnreadToInbox)(nil))):
			return dispatch(value, new(inbox.TLInboxReadUserMediaUnreadToInbox), c.InboxReadUserMediaUnreadToInbox)
		case string(proto.MessageName((*inbox.TLInboxReadChatMediaUnreadToInbox)(nil))):
			return dispatch(value, new(inbox.TLInboxReadChatMediaUnreadToInbox), c.InboxReadChatMediaUnreadToInbox)
		case string(proto.MessageName((*inbox.TLInboxUpdateHistoryReaded)(nil))):
			return dispatch(value, new(inbox.TLInboxUpdateHistoryReaded), c.InboxUpdateHistoryReaded)
		case string(proto.MessageName((*inbox.TLInboxUpdatePinnedMessage)(nil))):
			return dispatch(value, new(inbox.TLInboxUpdatePinnedMessage), c.InboxUpdatePinnedMessage)
		case string(proto.MessageName((*inbox.TLInboxUnpinAllMessages)(nil))):
			return dispatch(value, new(inbox.TLInboxUnpinAllMessages), c.InboxUnpinAllMessages)
		case string(proto.MessageName((*inbox.TLInboxSendUserMessageToInboxV2)(nil))):
			return dispatch(value, new(inbox.TLInboxSendUserMessageToInboxV2), c.InboxSendUserMessageToInboxV2)
		case string(proto.MessageName((*inbox.TLInboxEditMessageToInboxV2)(nil))):
			return dispatch(value, new(inbox.TLInboxEditMessageToInboxV2), c.InboxEditMessageToInboxV2)
		case string(proto.MessageName((*inbox.TLInboxReadInboxHistory)(nil))):
			return dispatch(value, new(inbox.TLInboxReadInboxHistory), c.InboxReadInboxHistory)
		case string(proto.MessageName((*inbox.TLInboxReadOutboxHistory)(nil))):
			return dispatch(value, new(inbox.TLInboxReadOutboxHistory), c.InboxReadOutboxHistory)
		case string(proto.MessageName((*inbox.TLInboxReadMediaUnreadToInboxV2)(nil))):
			return dispatch(value, new(inbox.TLInboxReadMediaUnreadToInboxV2), c.InboxReadMediaUnreadToInboxV2)
		case string(proto.MessageName((*inbox.TLInboxUpdatePinnedMessageV2)(nil))):
			return dispatch(value, new(inbox.TLInboxUpdatePinnedMessageV2), c.InboxUpdatePinnedMessageV2)
		default:
			return fmt.Errorf("inbox: unknown Kafka method %q", method)
		}
	})
	if err != nil {
		panic(err)
	}
	return consumer
}

func dispatch[T proto.Message](value []byte, request T, handler func(T) (*mtproto.Void, error)) error {
	if err := json.Unmarshal(value, request); err != nil {
		return err
	}
	result, err := handler(request)
	if err == nil && result == nil {
		return fmt.Errorf("inbox: handler returned no result")
	}
	return err
}
