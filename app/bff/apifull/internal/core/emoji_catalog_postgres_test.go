package core

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestEmojiCatalogPostgresRoundTrip(t *testing.T) {
	if !persist.StickerProviderReady() {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	db, err := persist.OpenPostgresDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	uid := int64(881000000 + os.Getpid()%100000)
	lang := fmt.Sprintf("zz-test-%d", uid)
	docID := uid + 1
	groupTitle := fmt.Sprintf("test-group-%d", uid)
	emoticon := fmt.Sprintf("emoji-test-%d", uid)
	_, err = db.ExecContext(context.Background(), `INSERT INTO apifull_emoji_language(lang_code,version,url) VALUES ($1,7,$2)`, lang, "https://example.test/emoji")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(context.Background(), `INSERT INTO apifull_emoji_keyword(lang_code,keyword,emoticons) VALUES ($1,$2,$3::jsonb)`, lang, "grinning", `["😀"]`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(context.Background(), `INSERT INTO apifull_emoji_document(id,access_hash,date,alt,featured,profile,status,group_photo) VALUES ($1,$2,123,$3,TRUE,TRUE,TRUE,TRUE)`, docID, docID+1, emoticon)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(context.Background(), `INSERT INTO apifull_emoji_group(kind,title,icon_emoji_id,emoticons) VALUES ('generic',$1,$2,'["😀"]'::jsonb),('profile',$1,$2,'["😀"]'::jsonb)`, groupTitle, docID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM apifull_emoji_group WHERE title=$1`, groupTitle)
		_, _ = db.Exec(`DELETE FROM apifull_emoji_document WHERE id=$1`, docID)
		_, _ = db.Exec(`DELETE FROM apifull_emoji_keyword WHERE lang_code=$1`, lang)
		_, _ = db.Exec(`DELETE FROM apifull_emoji_language WHERE lang_code=$1`, lang)
	})

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	keywords, err := c.MessagesGetEmojiKeywords(&mtproto.TLMessagesGetEmojiKeywords{LangCode: lang})
	if err != nil || keywords.GetVersion() != 7 || len(keywords.GetKeywords()) != 1 || keywords.GetKeywords()[0].GetKeyword() != "grinning" {
		t.Fatalf("keywords=%v err=%v", keywords, err)
	}
	diff, err := c.MessagesGetEmojiKeywordsDifference(&mtproto.TLMessagesGetEmojiKeywordsDifference{LangCode: lang, FromVersion: 7})
	if err != nil || diff.GetVersion() != 7 || len(diff.GetKeywords()) != 0 {
		t.Fatalf("difference=%v err=%v", diff, err)
	}
	languages, err := c.MessagesGetEmojiKeywordsLanguages(&mtproto.TLMessagesGetEmojiKeywordsLanguages{LangCodes: []string{lang, lang, "missing"}})
	if err != nil || len(languages.GetDatas()) != 1 || languages.GetDatas()[0].GetLangCode() != lang {
		t.Fatalf("languages=%v err=%v", languages, err)
	}
	url, err := c.MessagesGetEmojiURL(&mtproto.TLMessagesGetEmojiURL{LangCode: lang})
	if err != nil || url.GetUrl() != "https://example.test/emoji" {
		t.Fatalf("url=%v err=%v", url, err)
	}
	docs, err := c.MessagesGetCustomEmojiDocuments(&mtproto.TLMessagesGetCustomEmojiDocuments{DocumentId: []int64{docID}})
	if err != nil || len(docs.GetDatas()) != 1 || docs.GetDatas()[0].GetId() != docID {
		t.Fatalf("docs=%v err=%v", docs, err)
	}
	profile, err := c.AccountGetDefaultProfilePhotoEmojis(&mtproto.TLAccountGetDefaultProfilePhotoEmojis{})
	if err != nil || !containsInt64(profile.GetDocumentId(), docID) {
		t.Fatalf("profile photo emojis=%v err=%v", profile, err)
	}
	group, err := c.AccountGetDefaultGroupPhotoEmojis(&mtproto.TLAccountGetDefaultGroupPhotoEmojis{})
	if err != nil || !containsInt64(group.GetDocumentId(), docID) {
		t.Fatalf("group photo emojis=%v err=%v", group, err)
	}
	search, err := c.MessagesSearchCustomEmoji(&mtproto.TLMessagesSearchCustomEmoji{Emoticon: emoticon})
	if err != nil || len(search.GetDocumentId()) != 1 || search.GetDocumentId()[0] != docID {
		t.Fatalf("search=%v err=%v", search, err)
	}
	unchanged, err := c.MessagesSearchCustomEmoji(&mtproto.TLMessagesSearchCustomEmoji{Emoticon: emoticon, Hash: search.GetHash()})
	if err != nil || unchanged.GetPredicateName() != mtproto.Predicate_emojiListNotModified {
		t.Fatalf("unchanged=%v err=%v", unchanged, err)
	}
	groups, err := c.MessagesGetEmojiGroups(&mtproto.TLMessagesGetEmojiGroups{})
	foundGroup := false
	for _, group := range groups.GetGroups() {
		if group != nil && group.GetTitle() == groupTitle {
			foundGroup = true
			break
		}
	}
	if err != nil || !foundGroup {
		t.Fatalf("groups=%v err=%v", groups, err)
	}
}
