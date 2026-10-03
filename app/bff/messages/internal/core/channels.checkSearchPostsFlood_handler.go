// Copyright 2025 Teamgram Authors
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
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	searchPostsFloodTotalDaily = 10
	searchPostsFloodRemains    = 10
)

// ChannelsCheckSearchPostsFlood
// channels.checkSearchPostsFlood#22567115 flags:# query:flags.0?string = SearchPostsFlood;
func (c *MessagesCore) ChannelsCheckSearchPostsFlood(in *mtproto.TLChannelsCheckSearchPostsFlood) (*mtproto.SearchPostsFlood, error) {
	_ = in

	return mtproto.MakeTLSearchPostsFlood(&mtproto.SearchPostsFlood{
		QueryIsFree: true,
		TotalDaily:  searchPostsFloodTotalDaily,
		Remains:     searchPostsFloodRemains,
		WaitTill:    &wrapperspb.Int32Value{Value: 0},
		StarsAmount: 0,
	}).To_SearchPostsFlood(), nil
}
