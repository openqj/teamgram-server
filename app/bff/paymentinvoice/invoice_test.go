package paymentinvoice

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func validInputInvoiceMedia() *mtproto.InputMedia {
	invoice := mtproto.MakeTLInvoice(&mtproto.Invoice{
		ShippingAddressRequested: true,
		Currency:                 "USD",
		Prices: []*mtproto.LabeledPrice{
			mtproto.MakeTLLabeledPrice(&mtproto.LabeledPrice{Label: "item", Amount: 125}).To_LabeledPrice(),
		},
	}).To_Invoice()
	return mtproto.MakeTLInputMediaInvoice(&mtproto.InputMedia{
		Title:           "Item",
		Description:     "A paid item",
		Invoice:         invoice,
		Payload_BYTES:   []byte("signed-payload"),
		Provider_STRING: "provider",
		ProviderData:    mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: `{}`}).To_DataJSON(),
		StartParam:      nil,
	}).To_InputMedia()
}

func TestFromInputMediaRoundTrip(t *testing.T) {
	media, err := FromInputMedia(validInputInvoiceMedia())
	if err != nil {
		t.Fatalf("FromInputMedia() error = %v", err)
	}
	if media.GetPredicateName() != mtproto.Predicate_messageMediaInvoice || media.GetCurrency() != "USD" || media.GetTotalAmount() != 125 || !media.GetShippingAddressRequested() {
		t.Fatalf("message media = %+v", media)
	}
	metadata, invoice, err := ParseMetadata(media)
	if err != nil {
		t.Fatalf("ParseMetadata() error = %v", err)
	}
	if metadata.Provider != "provider" || string(metadata.Payload) != "signed-payload" || invoice.GetCurrency() != "USD" || invoice.GetPrices()[0].GetAmount() != 125 {
		t.Fatalf("metadata = %+v invoice = %+v", metadata, invoice)
	}
	if err := media.Encode(mtproto.NewEncodeBuf(1024), 229); err != nil {
		t.Fatalf("messageMediaInvoice layer 229 encode: %v", err)
	}
}

func TestFromInputMediaRejectsMissingTrustMaterial(t *testing.T) {
	cases := []struct {
		name string
		edit func(*mtproto.InputMedia)
		want error
	}{
		{name: "payload", edit: func(media *mtproto.InputMedia) { media.Payload_BYTES = nil }, want: ErrMissingPayload},
		{name: "provider", edit: func(media *mtproto.InputMedia) { media.Provider_STRING = "" }, want: ErrMissingProvider},
		{name: "provider data", edit: func(media *mtproto.InputMedia) { media.ProviderData = &mtproto.DataJSON{Data: "not-json"} }, want: ErrInvalidMedia},
		{name: "price", edit: func(media *mtproto.InputMedia) { media.Invoice.Prices[0].Amount = -1 }, want: ErrInvalidMedia},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			media := validInputInvoiceMedia()
			tc.edit(media)
			_, err := FromInputMedia(media)
			if !errors.Is(err, tc.want) {
				t.Fatalf("FromInputMedia() error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestParseMetadataRejectsTampering(t *testing.T) {
	media, err := FromInputMedia(validInputInvoiceMedia())
	if err != nil {
		t.Fatal(err)
	}
	media.Title = "tampered"
	if _, _, err = ParseMetadata(media); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("ParseMetadata() error = %v, want ErrInvalidMetadata", err)
	}
}
