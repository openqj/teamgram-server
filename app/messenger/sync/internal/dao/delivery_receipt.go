package dao

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

type DeliveryReceipt struct {
	ConsumerGroup string
	Topic         string
	Partition     int32
	Offset        int64
	RequestHash   []byte
}

type deliveryReceiptKey struct{}

func WithDeliveryReceipt(ctx context.Context, receipt DeliveryReceipt) context.Context {
	return context.WithValue(ctx, deliveryReceiptKey{}, receipt)
}

func loadDeliveryReceipt(ctx context.Context, tx pgx.Tx, userID int64) (*PreparedUpdates, error) {
	receipt, ok := ctx.Value(deliveryReceiptKey{}).(DeliveryReceipt)
	if !ok {
		return nil, nil
	}
	if receipt.ConsumerGroup == "" || receipt.Topic == "" || receipt.Partition < 0 || receipt.Offset < 0 || len(receipt.RequestHash) != 32 {
		return nil, errors.New("sync: invalid Kafka delivery identity")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO sync_delivery_receipts
 (consumer_group,topic,partition_id,message_offset,user_id,request_hash)
 VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`,
		receipt.ConsumerGroup, receipt.Topic, receipt.Partition, receipt.Offset, userID, receipt.RequestHash); err != nil {
		return nil, err
	}
	var hash, data []byte
	var delivered bool
	if err := tx.QueryRow(ctx, `SELECT request_hash,delivery_data,delivered FROM sync_delivery_receipts
 WHERE consumer_group=$1 AND topic=$2 AND partition_id=$3 AND message_offset=$4 AND user_id=$5 FOR UPDATE`,
		receipt.ConsumerGroup, receipt.Topic, receipt.Partition, receipt.Offset, userID).Scan(&hash, &data, &delivered); err != nil {
		return nil, err
	}
	if !bytes.Equal(hash, receipt.RequestHash) {
		return nil, errors.New("sync: Kafka delivery identity was reused for another request")
	}
	if data == nil {
		return nil, nil
	}
	prepared := new(PreparedUpdates)
	if err := json.Unmarshal(data, prepared); err != nil {
		return nil, err
	}
	prepared.Completed = delivered
	return prepared, nil
}

func saveDeliveryReceipt(ctx context.Context, tx pgx.Tx, userID int64, prepared *PreparedUpdates) error {
	receipt, ok := ctx.Value(deliveryReceiptKey{}).(DeliveryReceipt)
	if !ok {
		return nil
	}
	data, err := json.Marshal(prepared)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE sync_delivery_receipts SET delivery_data=$6::jsonb
 WHERE consumer_group=$1 AND topic=$2 AND partition_id=$3 AND message_offset=$4 AND user_id=$5`,
		receipt.ConsumerGroup, receipt.Topic, receipt.Partition, receipt.Offset, userID, string(data))
	return err
}

// MarkDeliveryComplete runs after network fanout. Persisted receipts remain
// retryable until every requested push has succeeded.
func (d *Dao) MarkDeliveryComplete(ctx context.Context, userID int64) error {
	receipt, ok := ctx.Value(deliveryReceiptKey{}).(DeliveryReceipt)
	if !ok {
		return nil
	}
	if err := d.updateStoreAvailable(); err != nil {
		return err
	}
	return postgres.WithTx(ctx, d.Pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE sync_delivery_receipts SET delivered=TRUE,delivered_at=clock_timestamp()
 WHERE consumer_group=$1 AND topic=$2 AND partition_id=$3 AND message_offset=$4
 AND user_id=$5 AND request_hash=$6 AND delivery_data IS NOT NULL`, receipt.ConsumerGroup, receipt.Topic, receipt.Partition, receipt.Offset, userID, receipt.RequestHash)
		return err
	})
}
