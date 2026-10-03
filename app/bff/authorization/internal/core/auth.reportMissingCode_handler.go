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
	"github.com/teamgram/proto/mtproto"
)

// AuthReportMissingCode
// auth.reportMissingCode#cb9deff6 phone_number:string phone_code_hash:string mnc:string = Bool;
func (c *AuthorizationCore) AuthReportMissingCode(in *mtproto.TLAuthReportMissingCode) (*mtproto.Bool, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c == nil {
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

	_ = phoneNumber
	// There is no delivery-provider incident/report sink. A local phone-code
	// row is not evidence that the report was accepted by a provider.
	c.Logger.Errorf("auth.reportMissingCode - report provider unavailable")
	return nil, mtproto.ErrMethodNotImpl
}
