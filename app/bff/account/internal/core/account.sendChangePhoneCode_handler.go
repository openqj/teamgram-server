// Copyright 2022 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/authorization/model"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	verification "github.com/teamgram/teamgram-server/pkg/code"
	"github.com/teamgram/teamgram-server/pkg/phonenumber"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

/*
   } else if (request instanceof TLRPC.TL_account_sendChangePhoneCode) {
       if (error.text.contains("PHONE_NUMBER_INVALID")) {
           showSimpleAlert(fragment, LocaleController.getString("InvalidPhoneNumber", R.string.InvalidPhoneNumber));
       } else if (error.text.contains("PHONE_CODE_EMPTY") || error.text.contains("PHONE_CODE_INVALID")) {
           showSimpleAlert(fragment, LocaleController.getString("InvalidCode", R.string.InvalidCode));
       } else if (error.text.contains("PHONE_CODE_EXPIRED")) {
           showSimpleAlert(fragment, LocaleController.getString("CodeExpired", R.string.CodeExpired));
       } else if (error.text.startsWith("FLOOD_WAIT")) {
           showSimpleAlert(fragment, LocaleController.getString("FloodWait", R.string.FloodWait));
       } else if (error.text.startsWith("PHONE_NUMBER_OCCUPIED")) {
           showSimpleAlert(fragment, LocaleController.formatString("ChangePhoneNumberOccupied", R.string.ChangePhoneNumberOccupied, args[0]));
       } else {
           showSimpleAlert(fragment, LocaleController.getString("ErrorOccurred", R.string.ErrorOccurred));
       }
*/

// AccountSendChangePhoneCode
// account.sendChangePhoneCode#82574ae5 phone_number:string settings:CodeSettings = auth.SentCode;
func (c *AccountCore) AccountSendChangePhoneCode(in *mtproto.TLAccountSendChangePhoneCode) (*mtproto.Auth_SentCode, error) {
	if in == nil || in.GetSettings() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	settings := in.GetSettings()
	// ## Possible errors
	// Code	Type	Description
	// 406	FRESH_CHANGE_PHONE_FORBIDDEN	You can't change phone number right after logging in, please wait at least 24 hours.
	// 400	PHONE_NUMBER_BANNED	The provided phone number is banned from telegram.
	// 406	PHONE_NUMBER_INVALID	The phone number is invalid.
	// 400	PHONE_NUMBER_OCCUPIED	The phone number is already in use.

	// 3. check number

	// client phone number format: "+86 111 1111 1111"
	_, phoneNumber, err := phonenumber.CheckPhoneNumberInvalid(in.PhoneNumber)
	if err != nil {
		c.Logger.Errorf("check phone_number(%s) error - %v", in.PhoneNumber, err)
		err = mtproto.ErrPhoneNumberInvalid
		return nil, err
	}

	// 5. banned phone number
	if c.svcCtx.Plugin != nil {
		banned, _ := c.svcCtx.Plugin.CheckPhoneNumberBanned(c.ctx, phoneNumber)
		if banned {
			c.Logger.Errorf("{phone_number: %s} banned: %v", phoneNumber, err)
			return nil, mtproto.ErrPhoneNumberBanned
		}
	}

	// logic
	// Always crated new phoneCode
	var (
		user *mtproto.ImmutableUser
	)

	if user, err = c.svcCtx.Dao.UserClient.UserGetImmutableUserByPhone(c.ctx, &userpb.TLUserGetImmutableUserByPhone{
		Phone: phoneNumber,
	}); err != nil {
		nErr, ok := status.FromError(err)
		if ok && (nErr.Code() == codes.NotFound ||
			(nErr.Code() == status.Code(mtproto.ErrPhoneNumberUnoccupied) && nErr.Message() == status.Convert(mtproto.ErrPhoneNumberUnoccupied).Message())) {
			err = nil
		} else {
			c.Logger.Errorf("checkPhoneNumberExist error: %v", err)
			return nil, err
		}
	} else {
		c.Logger.Errorf("checkPhoneNumberExist - user: %s", user)
		return nil, mtproto.ErrPhoneNumberOccupied
	}

	//sentCodeType, nextCodeType := code.MakeCodeType(
	//	false,
	//	in.GetSettings().GetAllowFlashcall(),
	//	in.GetSettings().GetCurrentNumber())
	//
	//var (
	//	codeData *code.PhoneCodeTransaction
	//)
	//
	//codeData, err = c.svcCtx.Dao.CodeClient.CodeCreatePhoneCode(c.ctx, &code.TLCodeCreatePhoneCode{
	//	AuthKeyId:             c.MD.PermAuthKeyId,
	//	SessionId:             c.MD.SessionId,
	//	Phone:                 phoneNumber,
	//	PhoneNumberRegistered: false,
	//	SentCodeType:          int32(sentCodeType),
	//	NextCodeType:          int32(nextCodeType),
	//	State:                 code.CodeStateReSent,
	//})
	//if err != nil {
	//	c.Logger.Errorf("account.sendChangePhoneCode error: %v", err)
	//	return nil, err
	//}
	//
	//if codeData.State == model.CodeStateSent {
	//	c.Logger.Debugf("codeSent")
	//	// return nil
	//}
	//
	//c.Logger.Debugf("send code by sms")
	//if extraData, err := s.VerifyCodeInterface.SendSmsVerifyCode(
	//	context.Background(),
	//	phoneNumber,
	//	codeData.PhoneCode,
	//	codeData.PhoneCodeHash); err != nil {
	//	return nil, err
	//} else {
	//	codeData.SentCodeType = model.CodeTypeSms
	//	codeData.PhoneCodeExtraData = extraData
	//}
	//
	//codeData.NextCodeType = model.CodeTypeSms
	//codeData.State = model.CodeStateSent

	codeData, err2 := c.svcCtx.AuthLogic.DoAuthSendCode(
		c.ctx,
		c.MD.PermAuthKeyId,
		c.MD.SessionId,
		phoneNumber,
		settings.GetAllowFlashcall(),
		settings.GetCurrentNumber(),
		func(codeData2 *model.PhoneCodeTransaction) error {
			return c.issueSMSChallenge(codeData2, challengePurposeChangePhone)
		}, func(codeData2 *model.PhoneCodeTransaction) {
			if c.svcCtx.Challenges == nil || codeData2 == nil {
				return
			}
			_ = c.svcCtx.Challenges.Revoke(c.ctx, verification.VerifyRequest{
				Channel: verification.ChannelSMS, Purpose: challengePurposeChangePhone,
				Scope: c.challengeScope(), ChallengeID: codeData2.PhoneCodeHash,
			})
		})

	if err2 != nil {
		c.Logger.Errorf("auth.sendCode - error: %v", err2)
		return nil, err2
	}

	return codeData.ToAuthSentCode(), nil
}
