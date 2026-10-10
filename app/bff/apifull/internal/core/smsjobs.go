// Copyright 2026 Teamgram Authors
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
	"encoding/json"
	"errors"
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// smsPut is retained for unrelated local audit records used by translation and
// transcription handlers. SMS jobs themselves do not call it without a provider.
func smsPut(userID int64, op string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return persist.Default.Set(fmt.Sprintf("sms:%d:%s", userID, op), string(raw))
}

func (c *ApiFullCore) SmsjobsIsEligibleToJoin(in *mtproto.TLSmsjobsIsEligibleToJoin) (*mtproto.Smsjobs_EligibilityToJoin, error) {
	uid, err := c.smsjobsUser()
	if err != nil {
		return nil, err
	}
	monthly, err := domain.SmsjobsEligibility(uid)
	if err != nil {
		return nil, smsJobsError(err)
	}
	return mtproto.MakeTLSmsjobsEligibleToJoin(&mtproto.Smsjobs_EligibilityToJoin{
		TermsUrl: SmsjobsTermsURL(), MonthlySentSms: monthly,
	}).To_Smsjobs_EligibilityToJoin(), nil
}

func (c *ApiFullCore) SmsjobsJoin(in *mtproto.TLSmsjobsJoin) (*mtproto.Bool, error) {
	uid, err := c.smsjobsUser()
	if err != nil {
		return nil, err
	}
	if err = domain.SmsjobsJoin(uid); err != nil {
		return nil, smsJobsError(err)
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) SmsjobsLeave(in *mtproto.TLSmsjobsLeave) (*mtproto.Bool, error) {
	uid, err := c.smsjobsUser()
	if err != nil {
		return nil, err
	}
	if err = domain.SmsjobsLeave(uid); err != nil {
		return nil, smsJobsError(err)
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) SmsjobsUpdateSettings(in *mtproto.TLSmsjobsUpdateSettings) (*mtproto.Bool, error) {
	uid, err := c.smsjobsUser()
	if err != nil {
		return nil, err
	}
	allowInternational := in != nil && in.GetAllowInternational()
	if err = domain.SmsjobsUpdateSettings(uid, allowInternational); err != nil {
		return nil, smsJobsError(err)
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) SmsjobsGetStatus(in *mtproto.TLSmsjobsGetStatus) (*mtproto.Smsjobs_Status, error) {
	uid, err := c.smsjobsUser()
	if err != nil {
		return nil, err
	}
	status, err := domain.SmsjobsStatus(uid)
	if err != nil {
		return nil, smsJobsError(err)
	}
	wire := &mtproto.Smsjobs_Status{
		AllowInternational: status.AllowInternational,
		RecentSent:         status.RecentSent,
		RecentSince:        status.RecentSince,
		RecentRemains:      status.RecentRemains,
		TotalSent:          status.TotalSent,
		TotalSince:         status.TotalSince,
		TermsUrl:           SmsjobsTermsURL(),
	}
	if status.LastGiftSlug != "" {
		wire.LastGiftSlug = wrapperspb.String(status.LastGiftSlug)
	}
	return mtproto.MakeTLSmsjobsStatus(wire).To_Smsjobs_Status(), nil
}

func (c *ApiFullCore) SmsjobsGetSmsJob(in *mtproto.TLSmsjobsGetSmsJob) (*mtproto.SmsJob, error) {
	uid, err := c.smsjobsUser()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetJobId() == "" {
		return nil, mtproto.ErrSmsjobIdInvalid
	}
	job, err := domain.GetSmsJob(uid, in.GetJobId())
	if err != nil {
		return nil, smsJobsError(err)
	}
	return mtproto.MakeTLSmsJob(&mtproto.SmsJob{
		JobId: job.JobID, PhoneNumber: job.PhoneNumber, Text: job.Text,
	}).To_SmsJob(), nil
}

func (c *ApiFullCore) SmsjobsFinishJob(in *mtproto.TLSmsjobsFinishJob) (*mtproto.Bool, error) {
	uid, err := c.smsjobsUser()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetJobId() == "" {
		return nil, mtproto.ErrSmsjobIdInvalid
	}
	failure := ""
	if in.GetError() != nil {
		failure = in.GetError().GetValue()
	}
	if err = domain.FinishSmsJob(uid, in.GetJobId(), failure); err != nil {
		return nil, smsJobsError(err)
	}
	return mtproto.BoolTrue, nil
}

func SmsjobsTermsURL() string { return domain.SmsjobsTermsURL() }

func (c *ApiFullCore) smsjobsUser() (int64, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return 0, err
	}
	if !domain.Ready() {
		return 0, mtproto.ErrMethodNotImpl
	}
	return uid, nil
}

func smsJobsError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrSmsjobsNotJoined):
		return mtproto.ErrNotJoined
	case errors.Is(err, domain.ErrSmsJobIDInvalid):
		return mtproto.ErrSmsjobIdInvalid
	case errors.Is(err, domain.ErrSmsJobNotFound):
		return mtproto.ErrSmsjobIdInvalid
	default:
		return mtproto.ErrInternalServerError
	}
}
