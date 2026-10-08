package channelview

import (
	"context"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/zeromicro/go-zero/core/logx"
)

type ChannelDeliveryPush func(context.Context, int64, []int64, *mtproto.Updates) error

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
	release, acquired, err := domain.LockChannelDelivery(ctx, key, wait)
	if err != nil || !acquired {
		return false, err
	}
	defer release()

	payload, found, err := domain.LoadChannelDelivery(ctx, key)
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
	recipients, err := domain.ListChannelDeliveryRecipients(ctx, key, dueOnly)
	if err != nil {
		return true, err
	}
	var firstErr error
	for _, recipient := range recipients {
		if err = ctx.Err(); err != nil {
			scheduleChannelDeliveryRetry(key, recipient.UserID, err)
			if firstErr == nil {
				firstErr = err
			}
			break
		}
		canReceive, checkErr := domain.ChannelDeliveryRecipientCanReceive(ctx, key.ChannelID, recipient.UserID, messageIDs)
		if checkErr != nil {
			scheduleChannelDeliveryRetry(key, recipient.UserID, checkErr)
			if firstErr == nil {
				firstErr = checkErr
			}
			continue
		}
		if !canReceive {
			if err = completeChannelDeliveryRecipient(key, recipient.UserID, "skipped"); err != nil && firstErr == nil {
				firstErr = err
			}
			continue
		}
		chat, chatErr := ChatForUpdates(recipient.UserID, key.ChannelID)
		if chatErr != nil {
			scheduleChannelDeliveryRetry(key, recipient.UserID, chatErr)
			if firstErr == nil {
				firstErr = chatErr
			}
			continue
		}
		updates, updatesErr := channelDeliveryUpdates(payload, key, recipient.UserID)
		if updatesErr != nil {
			scheduleChannelDeliveryRetry(key, recipient.UserID, updatesErr)
			if firstErr == nil {
				firstErr = updatesErr
			}
			continue
		}
		updates.Chats = []*mtproto.Chat{chat}
		var excludes []int64
		if payload.ExcludeAuthKeyID != 0 {
			excludes = []int64{payload.ExcludeAuthKeyID}
		}
		if pushErr := push(ctx, recipient.UserID, excludes, updates); pushErr != nil {
			scheduleChannelDeliveryRetry(key, recipient.UserID, pushErr)
			if firstErr == nil {
				firstErr = pushErr
			}
			continue
		}
		if err = completeChannelDeliveryRecipient(key, recipient.UserID, "delivered"); err != nil {
			scheduleChannelDeliveryRetry(key, recipient.UserID, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return true, firstErr
}

func completeChannelDeliveryRecipient(key domain.ChannelDeliveryKey, userID int64, state string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return domain.CompleteChannelDeliveryRecipient(ctx, key, userID, state)
}

func scheduleChannelDeliveryRetry(key domain.ChannelDeliveryKey, userID int64, cause error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := domain.RetryChannelDeliveryRecipient(ctx, key, userID, cause); err != nil {
		logx.Errorf("channel delivery retry scheduling failed for channel %d pts %d-%d user %d: %v", key.ChannelID, key.PTSFrom, key.PTSTo, userID, err)
	}
}
