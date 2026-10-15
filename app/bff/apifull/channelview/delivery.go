package channelview

import (
	"context"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

type ChannelDeliveryPush func(context.Context, int64, []int64, *mtproto.Updates) error

const (
	// Keep recipient pushes concurrent so a large channel does not make the
	// sender RPC wait behind a long serial tail. Per-recipient completion is
	// still persisted in bounded batches and the outbox remains retryable.
	channelDeliveryFanoutConcurrency = 16
	channelDeliveryStateBatchSize    = 64
)

func DeliveryKeyFromUpdates(updates *mtproto.Updates) (domain.ChannelDeliveryKey, error) {
	var key domain.ChannelDeliveryKey
	if updates == nil || len(updates.GetUpdates()) == 0 {
		return key, mtproto.ErrInputRequestInvalid
	}
	for _, update := range updates.GetUpdates() {
		message := update.GetMessage_MESSAGE()
		if message == nil || message.GetPeerId() == nil || message.GetPeerId().GetChannelId() <= 0 || update.GetPts_INT32() <= 0 {
			return key, mtproto.ErrInputRequestInvalid
		}
		channelID, pts := message.GetPeerId().GetChannelId(), update.GetPts_INT32()
		if key.ChannelID == 0 {
			key.ChannelID = channelID
			key.PTSFrom = pts
			key.PTSTo = pts
			continue
		}
		if key.ChannelID != channelID {
			return domain.ChannelDeliveryKey{}, mtproto.ErrInputRequestInvalid
		}
		if pts < key.PTSFrom {
			key.PTSFrom = pts
		}
		if pts > key.PTSTo {
			key.PTSTo = pts
		}
	}
	return key, nil
}

func channelDeliveryUpdates(payload domain.ChannelDeliveryPayload, key domain.ChannelDeliveryKey, userID int64) (*mtproto.Updates, error) {
	eventType := payload.EventType
	if eventType == "" {
		eventType = "new"
	}
	ptsCount := key.PTSTo - key.PTSFrom + 1
	if ptsCount <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	messageIDs := append([]int32(nil), payload.MessageIDs...)
	if len(messageIDs) == 0 {
		for _, row := range payload.Rows {
			messageIDs = append(messageIDs, row.MessageID)
		}
	}
	switch eventType {
	case "new":
		if len(payload.Rows) == 0 {
			return nil, mtproto.ErrInputRequestInvalid
		}
		return updatesNewForUser(payload.Rows, userID), nil
	case "edit":
		if len(payload.Rows) != 1 || len(messageIDs) != 1 {
			return nil, mtproto.ErrInputRequestInvalid
		}
		return mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateEditChannelMessage(&mtproto.Update{
			Message_MESSAGE: messageOfForUser(payload.Rows[0], 0, userID),
			Pts_INT32:       key.PTSTo,
			PtsCount:        ptsCount,
		}).To_Update()), nil
	case "delete":
		if len(messageIDs) == 0 {
			return nil, mtproto.ErrInputRequestInvalid
		}
		return mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateDeleteChannelMessages(&mtproto.Update{
			ChannelId: key.ChannelID,
			Messages:  messageIDs,
			Pts_INT32: key.PTSTo,
			PtsCount:  ptsCount,
		}).To_Update()), nil
	case "pin":
		if len(messageIDs) == 0 {
			return nil, mtproto.ErrInputRequestInvalid
		}
		return mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdatePinnedChannelMessages(&mtproto.Update{
			Pinned:    payload.Pinned,
			ChannelId: key.ChannelID,
			Messages:  messageIDs,
			Pts_INT32: key.PTSTo,
			PtsCount:  ptsCount,
		}).To_Update()), nil
	default:
		return nil, mtproto.ErrInputRequestInvalid
	}
}

func DeliverPendingChannelUpdates(ctx context.Context, key domain.ChannelDeliveryKey, dueOnly bool, push ChannelDeliveryPush) (bool, error) {
	if push == nil {
		return false, mtproto.ErrMethodNotImpl
	}
	wait := 2 * time.Second
	if dueOnly {
		wait = 0
	}
	stageStarted := time.Now()
	release, acquired, err := domain.LockChannelDelivery(ctx, key, wait)
	observeChannelDeliveryStage("lock", stageStarted, err)
	if err != nil || !acquired {
		return false, err
	}
	defer release()

	stageStarted = time.Now()
	payload, found, err := domain.LoadChannelDelivery(ctx, key)
	observeChannelDeliveryStage("event_load", stageStarted, err)
	if err != nil || !found {
		return found, err
	}
	if payload.Complete {
		return true, nil
	}
	messageIDs := append([]int32(nil), payload.MessageIDs...)
	if len(messageIDs) == 0 {
		for _, row := range payload.Rows {
			messageIDs = append(messageIDs, row.MessageID)
		}
	}
	if len(messageIDs) == 0 {
		return true, mtproto.ErrInputRequestInvalid
	}
	stageStarted = time.Now()
	recipients, err := domain.ListChannelDeliveryRecipients(ctx, key, dueOnly)
	observeChannelDeliveryStage("recipient_load", stageStarted, err)
	if err != nil {
		return true, err
	}
	stageStarted = time.Now()
	channel, channelFound, err := domain.LoadChannel(key.ChannelID)
	observeChannelDeliveryStage("channel_load", stageStarted, err)
	if err != nil {
		return true, err
	}
	if !channelFound {
		for offset := 0; offset < len(recipients); offset += channelDeliveryStateBatchSize {
			end := min(offset+channelDeliveryStateBatchSize, len(recipients))
			batch := make([]domain.ChannelDeliveryRecipientResult, 0, end-offset)
			for _, recipient := range recipients[offset:end] {
				batch = append(batch, domain.ChannelDeliveryRecipientResult{UserID: recipient.UserID, State: domain.ChannelDeliveryRecipientSkipped})
			}
			if err = recordChannelDeliveryRecipientResults(ctx, key, batch); err != nil {
				return true, err
			}
		}
		return true, nil
	}
	if len(recipients) == 0 {
		return true, nil
	}
	dispatchCtx, cancelDispatch := context.WithCancel(ctx)
	defer cancelDispatch()
	workerCount := min(len(recipients), channelDeliveryFanoutConcurrency)
	jobs := make(chan domain.ChannelDeliveryRecipient, workerCount)
	results := make(chan domain.ChannelDeliveryRecipientResult, workerCount)
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go func() {
			defer workers.Done()
			for recipient := range jobs {
				results <- deliverChannelDeliveryRecipient(dispatchCtx, key, payload, channel, messageIDs, recipient, push)
			}
		}()
	}
	go func() {
		for offset := 0; offset < len(recipients); offset += channelDeliveryStateBatchSize {
			if dispatchCtx.Err() != nil {
				break
			}
			end := min(offset+channelDeliveryStateBatchSize, len(recipients))
			batch := recipients[offset:end]
			stageStarted := time.Now()
			canReceive, checkErr := domain.ChannelDeliveryRecipientsCanReceive(dispatchCtx, key.ChannelID, channel.Creator, batch, messageIDs)
			observeChannelDeliveryStage("eligibility_query", stageStarted, checkErr)
			if checkErr != nil {
				for _, recipient := range recipients[offset:] {
					select {
					case results <- domain.ChannelDeliveryRecipientResult{UserID: recipient.UserID, State: domain.ChannelDeliveryRecipientRetry, Err: checkErr}:
					case <-dispatchCtx.Done():
						close(jobs)
						workers.Wait()
						close(results)
						return
					}
				}
				break
			}
			for _, recipient := range batch {
				if !canReceive[recipient.UserID] {
					select {
					case results <- domain.ChannelDeliveryRecipientResult{UserID: recipient.UserID, State: domain.ChannelDeliveryRecipientSkipped}:
					case <-dispatchCtx.Done():
						close(jobs)
						workers.Wait()
						close(results)
						return
					}
					continue
				}
				select {
				case jobs <- recipient:
				case <-dispatchCtx.Done():
					close(jobs)
					workers.Wait()
					close(results)
					return
				}
			}
		}
		close(jobs)
		workers.Wait()
		close(results)
	}()

	var firstErr error
	var persistErr error
	batch := make([]domain.ChannelDeliveryRecipientResult, 0, channelDeliveryStateBatchSize)
	for result := range results {
		if result.Err != nil && firstErr == nil {
			firstErr = result.Err
		}
		if persistErr != nil {
			continue
		}
		batch = append(batch, result)
		if len(batch) == channelDeliveryStateBatchSize {
			persistErr = recordChannelDeliveryRecipientResults(ctx, key, batch)
			if persistErr != nil {
				firstErr = persistErr
				cancelDispatch()
				batch = nil
			} else {
				batch = batch[:0]
			}
		}
	}
	if persistErr == nil && len(batch) > 0 {
		if persistErr = recordChannelDeliveryRecipientResults(ctx, key, batch); persistErr != nil {
			firstErr = persistErr
		}
	}
	if firstErr == nil {
		firstErr = ctx.Err()
	}
	return true, firstErr
}

func recordChannelDeliveryRecipientResults(ctx context.Context, key domain.ChannelDeliveryKey, results []domain.ChannelDeliveryRecipientResult) error {
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	started := time.Now()
	err := domain.RecordChannelDeliveryRecipientResults(writeCtx, key, results)
	observeChannelDeliveryStage("receipt_persist", started, err)
	return err
}

func deliverChannelDeliveryRecipient(ctx context.Context, key domain.ChannelDeliveryKey, payload domain.ChannelDeliveryPayload, channel domain.Channel, messageIDs []int32, recipient domain.ChannelDeliveryRecipient, push ChannelDeliveryPush) domain.ChannelDeliveryRecipientResult {
	result := domain.ChannelDeliveryRecipientResult{UserID: recipient.UserID, State: domain.ChannelDeliveryRecipientRetry}
	if err := ctx.Err(); err != nil {
		result.Err = err
		return result
	}
	updates, err := channelDeliveryUpdates(payload, key, recipient.UserID)
	if err != nil {
		result.Err = err
		return result
	}
	updates.Chats = []*mtproto.Chat{Chat(channel, channel.Creator == recipient.UserID)}
	var excludes []int64
	if payload.ExcludeAuthKeyID != 0 {
		excludes = []int64{payload.ExcludeAuthKeyID}
	}
	pushStarted := time.Now()
	if err = push(ctx, recipient.UserID, excludes, updates); err != nil {
		observeChannelDeliveryStage("sync_rpc", pushStarted, err)
		result.Err = err
		return result
	}
	observeChannelDeliveryStage("sync_rpc", pushStarted, nil)
	result.State = domain.ChannelDeliveryRecipientDelivered
	return result
}
