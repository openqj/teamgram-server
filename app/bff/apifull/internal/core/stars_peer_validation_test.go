package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestStarsMandatoryPeersRejectMissingOrZeroAccessHash(t *testing.T) {
	const uid int64 = 904201
	zeroHashPeer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: uid}).To_InputPeer()
	transaction := mtproto.MakeTLInputStarsTransaction(&mtproto.InputStarsTransaction{Id: "tx-1"}).To_InputStarsTransaction()
	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}

	tests := []struct {
		name string
		call func() (any, error)
	}{
		{
			name: "stars status missing peer",
			call: func() (any, error) {
				return core.PaymentsGetStarsStatus(&mtproto.TLPaymentsGetStarsStatus{})
			},
		},
		{
			name: "stars status zero hash",
			call: func() (any, error) {
				return core.PaymentsGetStarsStatus(&mtproto.TLPaymentsGetStarsStatus{Peer: zeroHashPeer})
			},
		},
		{
			name: "stars transactions missing peer",
			call: func() (any, error) {
				return core.PaymentsGetStarsTransactions(&mtproto.TLPaymentsGetStarsTransactions{})
			},
		},
		{
			name: "stars transactions zero hash",
			call: func() (any, error) {
				return core.PaymentsGetStarsTransactions(&mtproto.TLPaymentsGetStarsTransactions{Peer: zeroHashPeer})
			},
		},
		{
			name: "stars transactions by id missing peer",
			call: func() (any, error) {
				return core.PaymentsGetStarsTransactionsByID(&mtproto.TLPaymentsGetStarsTransactionsByID{Id: []*mtproto.InputStarsTransaction{transaction}})
			},
		},
		{
			name: "stars transactions by id zero hash",
			call: func() (any, error) {
				return core.PaymentsGetStarsTransactionsByID(&mtproto.TLPaymentsGetStarsTransactionsByID{Peer: zeroHashPeer, Id: []*mtproto.InputStarsTransaction{transaction}})
			},
		},
		{
			name: "revenue stats missing peer",
			call: func() (any, error) {
				return core.PaymentsGetStarsRevenueStats(&mtproto.TLPaymentsGetStarsRevenueStats{})
			},
		},
		{
			name: "revenue stats zero hash",
			call: func() (any, error) {
				return core.PaymentsGetStarsRevenueStats(&mtproto.TLPaymentsGetStarsRevenueStats{Peer: zeroHashPeer})
			},
		},
		{
			name: "revenue withdrawal missing peer",
			call: func() (any, error) {
				return core.PaymentsGetStarsRevenueWithdrawalUrl(&mtproto.TLPaymentsGetStarsRevenueWithdrawalUrl{})
			},
		},
		{
			name: "revenue ads missing peer",
			call: func() (any, error) {
				return core.PaymentsGetStarsRevenueAdsAccountUrl(&mtproto.TLPaymentsGetStarsRevenueAdsAccountUrl{})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.call()
			if result != nil || !errors.Is(err, mtproto.ErrPeerIdInvalid) {
				t.Fatalf("result=(%v, %v), want nil result and PEER_ID_INVALID", result, err)
			}
		})
	}
}

func TestRevenuePeerAllowsSelfAndChatWithoutAccessHash(t *testing.T) {
	const uid int64 = 904202
	for name, peer := range map[string]*mtproto.InputPeer{
		"self": mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		"chat": mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 77}).To_InputPeer(),
	} {
		t.Run(name, func(t *testing.T) {
			if !validRevenuePeer(uid, peer) || !validRequiredRevenuePeer(uid, peer) {
				t.Fatalf("peer %#v should remain valid", peer)
			}
		})
	}
}
