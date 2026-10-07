package mq

import (
	"context"
	"strings"
	"testing"

	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/zeromicro/go-zero/core/logx/logtest"
	"google.golang.org/protobuf/proto"
)

func TestHandleMessageDispatchesPushVariants(t *testing.T) {
	collector := logtest.NewCollector(t)

	cases := []struct {
		name   string
		method string
		value  string
	}{
		{
			name:   "push updates if not",
			method: string(proto.MessageName((*sync.TLSyncPushUpdatesIfNot)(nil))),
			value:  `{"user_id":1,"updates":{}}`,
		},
		{
			name:   "push bot updates",
			method: string(proto.MessageName((*sync.TLSyncPushBotUpdates)(nil))),
			value:  `{"user_id":1,"updates":{}}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			collector.Reset()
			if !handlePushVariant(context.Background(), nil, tc.method, []byte(tc.value)) {
				t.Fatalf("message was not handled")
			}
			if strings.Contains(collector.String(), "invalid key") {
				t.Fatalf("message was routed to the invalid-key handler: %s", collector.String())
			}
		})
	}
}
