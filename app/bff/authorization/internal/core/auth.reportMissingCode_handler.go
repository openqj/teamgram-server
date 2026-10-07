// Copyright 2024 Teamgram Authors
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
	"errors"

	"github.com/teamgram/proto/mtproto"
	verification "github.com/teamgram/teamgram-server/pkg/code"
)

// AuthReportMissingCode
// auth.reportMissingCode#cb9deff6 phone_number:string phone_code_hash:string mnc:string = Bool;
func (c *AuthorizationCore) AuthReportMissingCode(in *mtproto.TLAuthReportMissingCode) (*mtproto.Bool, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c == nil || c.svcCtx == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	_, phoneNumber, err := checkPhoneNumberInvalid(in.GetPhoneNumber())
	if err != nil {
		c.Logger.Errorf("auth.reportMissingCode - phone: %v", err)
		return nil, mtproto.ErrPhoneNumberInvalid
	}
	if in.GetPhoneCodeHash() == "" {
		c.Logger.Errorf("auth.reportMissingCode - empty phone_code_hash")
		return nil, mtproto.ErrPhoneCodeHashEmpty
	}

	reporter := c.svcCtx.MissingCodeReporter
	if reporter == nil {
		c.Logger.Errorf("auth.reportMissingCode - report provider unavailable")
		return nil, mtproto.ErrMethodNotImpl
	}
	if err = reporter.Ready(); err != nil {
		if errors.Is(err, verification.ErrProviderUnavailable) {
			c.Logger.Errorf("auth.reportMissingCode - report provider unavailable")
			return nil, mtproto.ErrMethodNotImpl
		}
		c.Logger.Errorf("auth.reportMissingCode - report provider invalid: %v", err)
		return nil, mtproto.ErrInternalServerError
	}
	if err = reporter.Deliver(c.ctx, verification.Delivery{
		Channel: verification.ChannelSMS, Destination: phoneNumber,
		ChallengeID: in.GetPhoneCodeHash(), Purpose: "auth.reportMissingCode", MNC: in.GetMnc(),
	}); err != nil {
		if errors.Is(err, verification.ErrProviderUnavailable) {
			return nil, mtproto.ErrMethodNotImpl
		}
		c.Logger.Errorf("auth.reportMissingCode - report provider failed: %v", err)
		return nil, mtproto.ErrInternalServerError
	}
	return mtproto.BoolTrue, nil
}
