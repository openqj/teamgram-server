package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

type paidProductProbeStore struct {
	reads  int
	writes int
}

func (s *paidProductProbeStore) Get(string) (string, error) {
	s.reads++
	return "", nil
}

func (s *paidProductProbeStore) Set(string, string) error {
	s.writes++
	return nil
}

func TestPaidProductMethodsFailClosedWithoutPersistence(t *testing.T) {
	store := &paidProductProbeStore{}
	previous := persist.Default
	persist.Use(store)
	t.Cleanup(func() { persist.Use(previous) })

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	calls := []struct {
		name   string
		invoke func() (bool, error)
	}{
		{"bank card data", func() (bool, error) {
			result, err := c.PaymentsGetBankCardData(&mtproto.TLPaymentsGetBankCardData{Number: "4111111111111111"})
			return result != nil, err
		}},
		{"extended media", func() (bool, error) {
			result, err := c.MessagesGetExtendedMedia(&mtproto.TLMessagesGetExtendedMedia{Id: []int32{1}})
			return result != nil, err
		}},
		{"paid messages revenue", func() (bool, error) {
			result, err := c.AccountGetPaidMessagesRevenue(nil)
			return result != nil, err
		}},
		{"paid messages exception", func() (bool, error) {
			result, err := c.AccountToggleNoPaidMessagesException(nil)
			return result != nil, err
		}},
		{"paid messages price", func() (bool, error) {
			result, err := c.ChannelsUpdatePaidMessagesPrice(nil)
			return result != nil, err
		}},
		{"paid reaction send", func() (bool, error) {
			result, err := c.MessagesSendPaidReaction(&mtproto.TLMessagesSendPaidReaction{MsgId: 1})
			return result != nil, err
		}},
		{"paid reaction privacy", func() (bool, error) {
			result, err := c.MessagesTogglePaidReactionPrivacy(&mtproto.TLMessagesTogglePaidReactionPrivacy{MsgId: 1})
			return result != nil, err
		}},
		{"paid reaction privacy read", func() (bool, error) {
			result, err := c.MessagesGetPaidReactionPrivacy(nil)
			return result != nil, err
		}},
		{"broadcast revenue stats", func() (bool, error) {
			result, err := c.StatsGetBroadcastRevenueStats(nil)
			return result != nil, err
		}},
		{"broadcast revenue transactions", func() (bool, error) {
			result, err := c.StatsGetBroadcastRevenueTransactions(nil)
			return result != nil, err
		}},
	}

	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.invoke()
			if result || !errors.Is(err, mtproto.ErrMethodNotImpl) {
				t.Fatalf("result present=%v err=%v, want nil result and METHOD_NOT_IMPL", result, err)
			}
		})
	}
	if store.reads != 0 || store.writes != 0 {
		t.Fatalf("unavailable paid-product methods accessed persistence: reads=%d writes=%d", store.reads, store.writes)
	}
}
