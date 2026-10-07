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

package conf

type SmsVerifyCodeConfig struct {
	Name                   string
	SMSProvider            string `json:",optional"`
	EmailProvider          string `json:",optional"`
	SendCodeUrl            string
	EmailSendCodeUrl       string `json:",optional"`
	ReportMissingCodeUrl   string `json:",optional"`
	VerifyCodeUrl          string
	Key                    string
	Secret                 string
	ChallengeSecret        string `json:",optional"`
	RegionId               string
	ProviderTimeoutSeconds int `json:",optional"`
	ProviderRetryCount     int `json:",optional"`
	ChallengeTTLSeconds    int `json:",optional"`
	RateLimit              int `json:",optional"`
	RateWindowSeconds      int `json:",optional"`
	MaxAttempts            int `json:",optional"`
}
