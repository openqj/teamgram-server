// Copyright 2026 Teamgram Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package paymentinvoice

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/proto"
)

const metadataVersion = 1

var (
	ErrInvalidMedia    = errors.New("payment invoice media is invalid")
	ErrMissingPayload  = errors.New("payment invoice payload is missing")
	ErrMissingProvider = errors.New("payment invoice provider is missing")
	ErrInvalidMetadata = errors.New("payment invoice metadata is invalid")
)

// Metadata is stored in BizDataRaw because the MTProto messageMediaInvoice
// constructor intentionally does not expose payload/provider fields.
type Metadata struct {
	Version      int    `json:"version"`
	Payload      []byte `json:"payload"`
	Provider     string `json:"provider"`
	ProviderData string `json:"provider_data"`
	Invoice      []byte `json:"invoice"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	StartParam   string `json:"start_param,omitempty"`
}

// FromInputMedia validates a bot invoice and converts it to the canonical
// message media constructor without changing the Layer 229 wire contract.
func FromInputMedia(media *mtproto.InputMedia) (*mtproto.MessageMedia, error) {
	if media == nil || media.GetPredicateName() != mtproto.Predicate_inputMediaInvoice {
		return nil, ErrInvalidMedia
	}
	if strings.TrimSpace(media.GetTitle()) == "" || strings.TrimSpace(media.GetDescription()) == "" {
		return nil, ErrInvalidMedia
	}
	if len(media.GetPayload_BYTES()) == 0 || len(media.GetPayload_BYTES()) > 4096 {
		return nil, ErrMissingPayload
	}
	invoice := media.GetInvoice()
	if !validInvoice(invoice) {
		return nil, ErrInvalidMedia
	}
	if invoice.GetPredicateName() == "" {
		mtproto.MakeTLInvoice(invoice).To_Invoice()
	}
	for _, price := range invoice.GetPrices() {
		if price.GetPredicateName() == "" {
			mtproto.MakeTLLabeledPrice(price).To_LabeledPrice()
		}
	}
	provider := strings.TrimSpace(media.GetProvider_STRING())
	if provider == "" && media.GetProvider_FLAGSTRING() != nil {
		provider = strings.TrimSpace(media.GetProvider_FLAGSTRING().GetValue())
	}
	if provider == "" && invoice.GetCurrency() == "XTR" {
		// Stars do not use a provider token on the public Bot API surface. Keep
		// an internal marker in BizDataRaw so payment reconciliation can still
		// distinguish a trusted Stars invoice from a missing provider.
		provider = "XTR"
	}
	if provider == "" {
		return nil, ErrMissingProvider
	}
	providerData := media.GetProviderData()
	if providerData == nil || strings.TrimSpace(providerData.GetData()) == "" || !json.Valid([]byte(providerData.GetData())) {
		return nil, ErrInvalidMedia
	}
	if providerData.GetPredicateName() == "" {
		mtproto.MakeTLDataJSON(providerData).To_DataJSON()
	}
	invoiceBytes, err := proto.Marshal(invoice)
	if err != nil {
		return nil, ErrInvalidMedia
	}
	metadata := Metadata{
		Version:      metadataVersion,
		Payload:      append([]byte(nil), media.GetPayload_BYTES()...),
		Provider:     provider,
		ProviderData: providerData.GetData(),
		Invoice:      invoiceBytes,
		Title:        media.GetTitle(),
		Description:  media.GetDescription(),
	}
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return nil, ErrInvalidMedia
	}

	result := &mtproto.MessageMedia{
		ShippingAddressRequested: invoice.GetShippingAddressRequested(),
		Test:                     invoice.GetTest(),
		Title:                    media.GetTitle(),
		Description:              media.GetDescription(),
		Currency:                 invoice.GetCurrency(),
		TotalAmount:              invoiceTotal(invoice),
		BizData: &mtproto.BizDataRaw{
			PredicateName: mtproto.Predicate_bizDataRaw,
			Data:          metadataBytes,
		},
	}
	if start := media.GetStartParam(); start != nil {
		result.StartParam = start.GetValue()
		metadata.StartParam = start.GetValue()
		metadataBytes, err = json.Marshal(metadata)
		if err != nil {
			return nil, ErrInvalidMedia
		}
		result.BizData.Data = metadataBytes
	}
	if photo := media.GetPhoto(); photo != nil {
		result.Photo_FLAGWEBDOCUMENT = mtproto.MakeTLWebDocument(&mtproto.WebDocument{
			Url:        photo.GetUrl(),
			Size2:      photo.GetSize2(),
			MimeType:   photo.GetMimeType(),
			Attributes: photo.GetAttributes(),
		}).To_WebDocument()
	}
	return mtproto.MakeTLMessageMediaInvoice(result).To_MessageMedia(), nil
}

// ParseMetadata returns the trusted internal invoice data and revalidates it
// against the public message media fields before a payment flow can use it.
func ParseMetadata(media *mtproto.MessageMedia) (Metadata, *mtproto.Invoice, error) {
	if media == nil || media.GetPredicateName() != mtproto.Predicate_messageMediaInvoice || media.GetBizData() == nil {
		return Metadata{}, nil, ErrInvalidMetadata
	}
	if media.GetBizData().GetPredicateName() != mtproto.Predicate_bizDataRaw {
		return Metadata{}, nil, ErrInvalidMetadata
	}
	var metadata Metadata
	if err := json.Unmarshal(media.GetBizData().GetData(), &metadata); err != nil || metadata.Version != metadataVersion ||
		len(metadata.Payload) == 0 || len(metadata.Payload) > 4096 || strings.TrimSpace(metadata.Provider) == "" ||
		strings.TrimSpace(metadata.ProviderData) == "" || !json.Valid([]byte(metadata.ProviderData)) || len(metadata.Invoice) == 0 {
		return Metadata{}, nil, ErrInvalidMetadata
	}
	invoice := &mtproto.Invoice{}
	if err := proto.Unmarshal(metadata.Invoice, invoice); err != nil || !validInvoice(invoice) {
		return Metadata{}, nil, ErrInvalidMetadata
	}
	if invoice.GetPredicateName() == "" {
		mtproto.MakeTLInvoice(invoice).To_Invoice()
	}
	for _, price := range invoice.GetPrices() {
		if price.GetPredicateName() == "" {
			mtproto.MakeTLLabeledPrice(price).To_LabeledPrice()
		}
	}
	if media.GetTitle() != metadata.Title || media.GetDescription() != metadata.Description ||
		media.GetStartParam() != metadata.StartParam ||
		media.GetCurrency() != invoice.GetCurrency() || media.GetTotalAmount() != invoiceTotal(invoice) ||
		media.GetShippingAddressRequested() != invoice.GetShippingAddressRequested() || media.GetTest() != invoice.GetTest() {
		return Metadata{}, nil, ErrInvalidMetadata
	}
	return metadata, invoice, nil
}

func validInvoice(invoice *mtproto.Invoice) bool {
	if invoice == nil || strings.TrimSpace(invoice.GetCurrency()) == "" || len(invoice.GetCurrency()) != 3 || len(invoice.GetPrices()) == 0 {
		return false
	}
	if invoice.GetRecurring() {
		period := invoice.GetSubscriptionPeriod()
		if invoice.GetCurrency() != "XTR" || period == nil || period.GetValue() != 2592000 {
			return false
		}
	} else if invoice.GetSubscriptionPeriod() != nil {
		return false
	}
	var total int64
	for _, price := range invoice.GetPrices() {
		if price == nil || strings.TrimSpace(price.GetLabel()) == "" || price.GetAmount() < 0 || total > int64(^uint64(0)>>1)-price.GetAmount() {
			return false
		}
		total += price.GetAmount()
	}
	maxTip := int64(0)
	if value := invoice.GetMaxTipAmount(); value != nil {
		maxTip = value.GetValue()
		if maxTip <= 0 {
			return false
		}
	}
	if len(invoice.GetSuggestedTipAmounts()) > 4 || (maxTip == 0 && len(invoice.GetSuggestedTipAmounts()) > 0) {
		return false
	}
	var previousTip int64
	for _, tip := range invoice.GetSuggestedTipAmounts() {
		if tip <= 0 || tip > maxTip || (previousTip > 0 && tip <= previousTip) {
			return false
		}
		previousTip = tip
	}
	return true
}

func invoiceTotal(invoice *mtproto.Invoice) int64 {
	var total int64
	for _, price := range invoice.GetPrices() {
		if price != nil {
			total += price.GetAmount()
		}
	}
	return total
}
