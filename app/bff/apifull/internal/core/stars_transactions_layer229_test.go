package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

func TestStarsTransactionsResponsesEncodeLayer229(t *testing.T) {
	transaction := makeStarsTransaction(domain.StarsTransaction{Amount: 37, Idem: "probe-stars-layer229"})
	if transaction == nil || transaction.GetId() != "probe-stars-layer229" || transaction.GetStars_INT64() != 37 {
		t.Fatalf("transaction mapping = %#v, want ledger id and amount", transaction)
	}
	if transaction.GetPeer() == nil || transaction.GetPeer().GetPredicateName() != mtproto.Predicate_starsTransactionPeerAPI {
		t.Fatalf("transaction peer = %#v, want API peer constructor", transaction.GetPeer())
	}

	if err := transaction.Encode(mtproto.NewEncodeBuf(256), 229); err != nil {
		t.Fatalf("stars transaction did not encode at layer 229: %v", err)
	}

	status := makeStarsStatus(37, []*mtproto.StarsTransaction{transaction}, nil)
	if status == nil || len(status.GetHistory()) != 1 || status.GetHistory()[0].GetId() != transaction.GetId() {
		t.Fatalf("stars status history = %#v, want mapped transaction", status)
	}
	if err := status.Encode(mtproto.NewEncodeBuf(1024), 229); err != nil {
		t.Fatalf("stars status did not encode at layer 229: %v", err)
	}
}
