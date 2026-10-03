package core

import (
	"reflect"
	"testing"

	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
)

func TestCommonChatIDsUsesUserIdentity(t *testing.T) {
	data := &chatpb.Vector_UserChatIdList{Datas: []*chatpb.UserChatIdList{
		{UserId: 902, ChatIdList: []int64{12, 9, 4}},
		{UserId: 901, ChatIdList: []int64{7, 4, 12}},
		{UserId: 903, ChatIdList: []int64{4, 99}},
	}}
	got := commonChatIDs(data, 901, 902)
	if want := []int64{4, 12}; !reflect.DeepEqual(got, want) {
		t.Fatalf("common IDs: got %v want %v", got, want)
	}
	if got := commonChatIDs(data, 901, 904); len(got) != 0 {
		t.Fatalf("missing user should have no common IDs: %v", got)
	}
}

func TestCommonChatPageHonorsCursorAndLimit(t *testing.T) {
	ids := []int64{2, 4, 7, 12}
	if got, want := commonChatPage(ids, 4, 2), []int64{7, 12}; !reflect.DeepEqual(got, want) {
		t.Fatalf("page: got %v want %v", got, want)
	}
	if got, want := commonChatPage(ids, 0, 0), []int64{2, 4, 7, 12}; !reflect.DeepEqual(got, want) {
		t.Fatalf("default page: got %v want %v", got, want)
	}
	if got, want := commonChatPage(ids, 0, 2), []int64{2, 4}; !reflect.DeepEqual(got, want) {
		t.Fatalf("limited page: got %v want %v", got, want)
	}
}

func TestMergeCommonChatIDsIncludesChannels(t *testing.T) {
	got := mergeCommonChatIDs([]int64{12, 4}, []int64{9, 12})
	if want := []int64{4, 9, 12}; !reflect.DeepEqual(got, want) {
		t.Fatalf("merged common IDs: got %v want %v", got, want)
	}
}
