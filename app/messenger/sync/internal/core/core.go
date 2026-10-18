/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"context"
	stdsync "sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/interface/session/session"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/dao"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/svc"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/status/status"

	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type SyncType int

const (
	syncTypeUser      SyncType = 1 // 该用户所有设备
	syncTypeUserNotMe SyncType = 2 // 该用户除了某个设备
	syncTypeUserMe    SyncType = 3 // 该用户指定某个设备
)

const maxConcurrentSessionPushes = 8

type sessionPushTarget struct {
	serverID  string
	authKeyID int64
	updates   *mtproto.Updates
}

func pushSessionTargets(ctx context.Context, targets []sessionPushTarget, concurrency int, push func(context.Context, sessionPushTarget) error) error {
	if len(targets) == 0 {
		return nil
	}
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > len(targets) {
		concurrency = len(targets)
	}

	jobs := make(chan sessionPushTarget, len(targets))
	for _, target := range targets {
		jobs <- target
	}
	close(jobs)

	errChan := make(chan error, 1)
	var wg stdsync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for target := range jobs {
				if err := push(ctx, target); err != nil {
					select {
					case errChan <- err:
					default:
					}
				}
			}
		}()
	}
	wg.Wait()
	select {
	case err := <-errChan:
		return err
	default:
		return nil
	}
}

type SyncCore struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
	MD *metadata.RpcMetadata
}

func New(ctx context.Context, svcCtx *svc.ServiceContext) *SyncCore {
	return &SyncCore{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
		MD:     metadata.RpcMetadataFromIncoming(ctx),
	}
}

func (c *SyncCore) processUpdates(syncType SyncType, userId int64, isBot bool, ups *mtproto.Updates) (needPush bool, err error) {
	if userId <= 0 || ups == nil || (ups.GetPredicateName() == mtproto.Predicate_updateShort && ups.GetUpdate() == nil) {
		return false, mtproto.ErrInputRequestInvalid
	}
	mtproto.VisitUpdates(userId, ups, map[string]mtproto.UpdateVisitedFunc{
		mtproto.Predicate_updateNewEncryptedMessage: func(
			userId int64,
			update *mtproto.Update,
			users []*mtproto.User,
			chats []*mtproto.Chat,
			date int32,
		) {
			needPush = true
		},
		mtproto.Predicate_updateNewMessage: func(
			userId int64,
			update *mtproto.Update,
			users []*mtproto.User,
			chats []*mtproto.Chat,
			date int32,
		) {
			needPush = true
		},
		mtproto.Predicate_updateDeleteMessages: func(
			userId int64,
			update *mtproto.Update,
			users []*mtproto.User,
			chats []*mtproto.Chat,
			date int32,
		) {
			needPush = true
		},
		mtproto.Predicate_updateReadHistoryInbox: func(
			userId int64,
			update *mtproto.Update,
			users []*mtproto.User,
			chats []*mtproto.Chat,
			date int32,
		) {
			needPush = true
		},
		mtproto.Predicate_updateReadHistoryOutbox: func(
			userId int64,
			update *mtproto.Update,
			users []*mtproto.User,
			chats []*mtproto.Chat,
			date int32,
		) {
			needPush = true
		},
		mtproto.Predicate_updateWebPage: func(
			userId int64,
			update *mtproto.Update,
			users []*mtproto.User,
			chats []*mtproto.Chat,
			date int32,
		) {
			needPush = true
		},
		mtproto.Predicate_updateReadMessagesContents: func(
			userId int64,
			update *mtproto.Update,
			users []*mtproto.User,
			chats []*mtproto.Chat,
			date int32,
		) {
			needPush = true
		},
		mtproto.Predicate_updateEditMessage: func(
			userId int64,
			update *mtproto.Update,
			users []*mtproto.User,
			chats []*mtproto.Chat,
			date int32,
		) {
			needPush = true
		},
		mtproto.Predicate_updatePhoneCall: func(
			userId int64,
			update *mtproto.Update,
			users []*mtproto.User,
			chats []*mtproto.Chat,
			date int32,
		) {
			if update.GetPhoneCall().GetPredicateName() == mtproto.Predicate_phoneCallRequested {
				// log.Debugf("recv phoneCallRequested")
				needPush = true
			}
		},
		mtproto.Predicate_updatePeerSettings: func(
			userId int64,
			update *mtproto.Update,
			users []*mtproto.User,
			chats []*mtproto.Chat,
			date int32,
		) {
			needPush = true
		},
	})

	return needPush, nil
}

func (c *SyncCore) pushUpdatesToSession(syncType SyncType, userId, permAuthKeyId int64, hasServerId *wrapperspb.StringValue, authKeyId, sessionId *wrapperspb.Int64Value, pushData *mtproto.Updates, notification bool) (err error) {
	started := time.Now()
	defer func() { dao.ObserveDeliveryStage("fanout", started, err) }()
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil {
		return mtproto.ErrInternalServerError
	}
	onlyAuthID := int64(0)
	var excludes []int64
	if syncType == syncTypeUserMe {
		onlyAuthID = permAuthKeyId
		if onlyAuthID == 0 {
			return mtproto.ErrAuthKeyUnregistered
		}
	} else if syncType == syncTypeUserNotMe {
		excludes = []int64{permAuthKeyId}
	}
	prepareStarted := time.Now()
	prepared, err := c.svcCtx.Dao.PrepareUpdates(c.ctx, userId, pushData, onlyAuthID, excludes)
	dao.ObserveDeliveryStage("prepare", prepareStarted, err)
	if err != nil {
		return err
	}
	if prepared.Completed {
		return nil
	}
	if notification {
		// Persist the notification before network fan-out. A later
		// messages.receivedMessages call can then acknowledge it atomically,
		// while retries remain idempotent through the notification key.
		if err := c.svcCtx.Dao.RecordReceivedNotifications(c.ctx, userId, pushData); err != nil {
			return err
		}
	}
	if syncType == syncTypeUserMe && hasServerId != nil {
		pushData = prepared.AuthUpdates[permAuthKeyId]
		c.Logger.Debugf("pushUpdatesToSession - pushData: {server_id: %v, auth_key_id: %v}", hasServerId, authKeyId)
		if sessionId != nil {
			err = c.svcCtx.Dao.PushSessionUpdatesToSession(
				c.ctx,
				hasServerId.GetValue(),
				&session.TLSessionPushSessionUpdatesData{
					PermAuthKeyId: permAuthKeyId,
					AuthKeyId:     authKeyId.GetValue(),
					SessionId:     sessionId.GetValue(),
					Updates:       pushData,
				})
		} else {
			err = c.svcCtx.Dao.PushUpdatesToSession(
				c.ctx,
				hasServerId.GetValue(),
				&session.TLSessionPushUpdatesData{
					PermAuthKeyId: permAuthKeyId,
					Notification:  notification,
					Updates:       pushData,
				})
		}
		if err != nil {
			return err
		}
	} else {
		var (
			pushExcludeList   = make([]int64, 0)
			pushExcludeSet    = make(map[int64]struct{})
			serverIdKeyIdList = make(map[string][]int64)
		)

		if c.svcCtx.Dao.StatusClient == nil {
			return mtproto.ErrInternalServerError
		}
		statusStarted := time.Now()
		statusList, err := c.svcCtx.Dao.StatusClient.StatusGetUserOnlineSessions(c.ctx, &status.TLStatusGetUserOnlineSessions{
			UserId: userId,
		})
		dao.ObserveDeliveryStage("status_lookup", statusStarted, err)
		if err != nil {
			return err
		}
		if statusList == nil {
			return mtproto.ErrInternalServerError
		}
		c.Logger.Debugf("statusList - #%v", statusList)
		for _, sess := range statusList.GetUserSessions() {
			if sess == nil {
				continue
			}
			keyID := sess.GetPermAuthKeyId()
			if keyID == 0 {
				keyID = sess.GetAuthKeyId()
			}
			if _, ok := prepared.AuthUpdates[keyID]; !ok {
				continue
			}
			pushExcludeList = append(pushExcludeList, keyID)
			pushExcludeSet[keyID] = struct{}{}
			if keyIdList, ok := serverIdKeyIdList[sess.Gateway]; ok {
				keyIdList = append(keyIdList, keyID)
				serverIdKeyIdList[sess.Gateway] = keyIdList
			} else {
				serverIdKeyIdList[sess.Gateway] = []int64{keyID}
			}
		}

		c.Logger.Debugf("serverIdKeyIdList - #%v", serverIdKeyIdList)
		pushTargets := make([]sessionPushTarget, 0, len(pushExcludeList))
		for serverId, keyIdList := range serverIdKeyIdList {
			for _, keyID := range keyIdList {
				pushTargets = append(pushTargets, sessionPushTarget{
					serverID:  serverId,
					authKeyID: keyID,
					updates:   prepared.AuthUpdates[keyID],
				})
			}
		}
		if err := pushSessionTargets(c.ctx, pushTargets, maxConcurrentSessionPushes, func(ctx context.Context, target sessionPushTarget) error {
			return c.svcCtx.Dao.PushUpdatesToSession(ctx, target.serverID, &session.TLSessionPushUpdatesData{
				PermAuthKeyId: target.authKeyID,
				Notification:  notification,
				Updates:       target.updates,
			})
		}); err != nil {
			return err
		}

		if syncType == syncTypeUser {
			if c.svcCtx.Dao.PushClient != nil {
				c.Logger.Debugf("push PushClient...")
				for authID, updates := range prepared.AuthUpdates {
					if _, alreadyPushed := pushExcludeSet[authID]; alreadyPushed {
						continue
					}
					forward := proto.Clone(updates).(*mtproto.Updates)
					forward.AuthKeyId = authID
					if _, err := c.svcCtx.Dao.PushClient.SyncPushUpdatesIfNot(c.ctx, &syncpb.TLSyncPushUpdatesIfNot{
						UserId: userId, Excludes: pushExcludeList, Updates: forward,
					}); err != nil {
						return err
					}
				}
			}
		}
	}
	completeStarted := time.Now()
	err = c.svcCtx.Dao.MarkDeliveryComplete(c.ctx, userId)
	dao.ObserveDeliveryStage("mark_complete", completeStarted, err)
	return err
}
