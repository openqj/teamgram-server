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

package core

import (
	"errors"
	"reflect"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestUsersGetSavedMusicSelfPaginatesAndSupportsHash(t *testing.T) {
	users := newSavedMusicByIDUserClient()
	users.users[42] = mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User: mtproto.MakeTLUserData(&mtproto.UserData{Id: 42, AccessHash: 4200}).To_UserData(),
	}).To_ImmutableUser()
	users.savedIDs = []int64{10, 20, 30}
	mediaClient := &savedMusicByIDMediaClient{documents: []*mtproto.Document{
		{Id: 10, AccessHash: 1000},
		{Id: 20, AccessHash: 2000},
		{Id: 30, AccessHash: 3000},
	}}
	core := newSavedMusicByIDCore(users, mediaClient)

	got, err := core.UsersGetSavedMusic(&mtproto.TLUsersGetSavedMusic{
		Id:     mtproto.MakeTLInputUserSelf(nil).To_InputUser(),
		Offset: 1,
		Limit:  1,
	})
	if err != nil {
		t.Fatalf("UsersGetSavedMusic() error = %v", err)
	}
	if got.GetCount() != 3 || len(got.GetDocuments()) != 1 || got.GetDocuments()[0].GetId() != 20 {
		t.Fatalf("UsersGetSavedMusic() = %v, want count 3 and page [20]", got)
	}
	if users.privacyCalls != 0 || users.savedRequest.GetUserId() != 42 {
		t.Fatalf("self lookup/privacy/saved owner = (%d, %v, %d), want (42, no privacy, 42)", users.savedRequest.GetUserId(), users.privacyCalls, users.savedRequest.GetUserId())
	}
	if !reflect.DeepEqual(mediaClient.ids, []int64{10, 20, 30}) {
		t.Fatalf("media lookup ids = %v, want [10 20 30]", mediaClient.ids)
	}

	hash := int64(1)
	for _, id := range users.savedIDs {
		hash = hash*31 + id
	}
	got, err = core.UsersGetSavedMusic(&mtproto.TLUsersGetSavedMusic{
		Id:   mtproto.MakeTLInputUserSelf(nil).To_InputUser(),
		Hash: hash,
	})
	if err != nil {
		t.Fatalf("UsersGetSavedMusic(hash) error = %v", err)
	}
	if got.GetPredicateName() != mtproto.Predicate_users_savedMusicNotModified || got.GetCount() != 3 || len(got.GetDocuments()) != 0 {
		t.Fatalf("UsersGetSavedMusic(hash) = %v, want not-modified count 3", got)
	}
}

func TestUsersGetSavedMusicRejectsTargetAccessHashBeforeReadingSavedMusic(t *testing.T) {
	users := newSavedMusicByIDUserClient()
	core := newSavedMusicByIDCore(users, &savedMusicByIDMediaClient{})

	got, err := core.UsersGetSavedMusic(savedMusicRequest(7, 701))
	if got != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("UsersGetSavedMusic() = (%v, %v), want USER_ID_INVALID", got, err)
	}
	if users.savedCalls != 0 {
		t.Fatalf("saved-music reads after access hash mismatch = %d, want none", users.savedCalls)
	}
}

func TestUsersGetSavedMusicDeniesPrivacyBeforeReadingSavedMusic(t *testing.T) {
	users := newSavedMusicByIDUserClient()
	users.privacy = []*mtproto.PrivacyRule{mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule()}
	core := newSavedMusicByIDCore(users, &savedMusicByIDMediaClient{})

	got, err := core.UsersGetSavedMusic(savedMusicRequest(7, 700))
	if got != nil || !errors.Is(err, mtproto.ErrUserPrivacyRestricted) {
		t.Fatalf("UsersGetSavedMusic() = (%v, %v), want USER_PRIVACY_RESTRICTED", got, err)
	}
	if users.savedCalls != 0 {
		t.Fatalf("saved-music reads after privacy denial = %d, want none", users.savedCalls)
	}
}

func TestUsersGetSavedMusicFailsClosedOnNilDependencies(t *testing.T) {
	t.Run("saved list", func(t *testing.T) {
		users := newSavedMusicByIDUserClient()
		users.users[42] = mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
			User: mtproto.MakeTLUserData(&mtproto.UserData{Id: 42, AccessHash: 4200}).To_UserData(),
		}).To_ImmutableUser()
		users.nilSaved = true
		mediaClient := &savedMusicByIDMediaClient{}
		core := newSavedMusicByIDCore(users, mediaClient)

		got, err := core.UsersGetSavedMusic(savedMusicSelfRequest())
		if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
			t.Fatalf("UsersGetSavedMusic() = (%v, %v), want INTERNAL_SERVER_ERROR", got, err)
		}
		if len(mediaClient.ids) != 0 {
			t.Fatalf("media reads after nil saved list = %v, want none", mediaClient.ids)
		}
	})

	t.Run("media list", func(t *testing.T) {
		users := newSavedMusicByIDUserClient()
		users.users[42] = mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
			User: mtproto.MakeTLUserData(&mtproto.UserData{Id: 42, AccessHash: 4200}).To_UserData(),
		}).To_ImmutableUser()
		users.savedIDs = []int64{10}
		mediaClient := &savedMusicByIDMediaClient{nilResult: true}
		core := newSavedMusicByIDCore(users, mediaClient)

		got, err := core.UsersGetSavedMusic(savedMusicSelfRequest())
		if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
			t.Fatalf("UsersGetSavedMusic() = (%v, %v), want INTERNAL_SERVER_ERROR", got, err)
		}
	})
}

func savedMusicRequest(userID, accessHash int64) *mtproto.TLUsersGetSavedMusic {
	return &mtproto.TLUsersGetSavedMusic{
		Id: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: userID, AccessHash: accessHash}).To_InputUser(),
	}
}

func savedMusicSelfRequest() *mtproto.TLUsersGetSavedMusic {
	return &mtproto.TLUsersGetSavedMusic{
		Id: mtproto.MakeTLInputUserSelf(nil).To_InputUser(),
	}
}
