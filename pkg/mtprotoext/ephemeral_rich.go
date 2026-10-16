// Copyright 2026 Teamgram Authors
// All rights reserved.

package mtprotoext

import (
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

// ephemeralRichMessageField is reserved for the service-to-service Bot API
// representation. The native ephemeralMessage TL codec does not encode
// unknown protobuf fields, so this sidecar does not alter the MTProto wire
// contract while preserving rich drafts across the gRPC boundary.
const ephemeralRichMessageField protowire.Number = 15

// SetEphemeralRichMessage stores rich content in the protobuf unknown-field
// area used by APIFull and the Bot API update queue. Existing sidecar values
// are replaced so draft updates remain idempotent.
func SetEphemeralRichMessage(message *mtproto.EphemeralMessage, rich *mtproto.RichMessage) error {
	if message == nil {
		return fmt.Errorf("ephemeral message is nil")
	}
	unknown, err := withoutEphemeralRichMessage(message.ProtoReflect().GetUnknown())
	if err != nil {
		return err
	}
	if rich == nil {
		message.ProtoReflect().SetUnknown(unknown)
		return nil
	}
	payload, err := proto.Marshal(rich)
	if err != nil {
		return fmt.Errorf("marshal ephemeral rich message: %w", err)
	}
	encoded := protowire.AppendTag(nil, ephemeralRichMessageField, protowire.BytesType)
	encoded = protowire.AppendBytes(encoded, payload)
	message.ProtoReflect().SetUnknown(append(unknown, encoded...))
	return nil
}

// EphemeralRichMessage reads the service-to-service rich sidecar. A missing
// sidecar is valid for ordinary ephemeral text/media messages.
func EphemeralRichMessage(message *mtproto.EphemeralMessage) (*mtproto.RichMessage, error) {
	if message == nil {
		return nil, nil
	}
	raw := message.ProtoReflect().GetUnknown()
	for len(raw) > 0 {
		number, typ, tagLen := protowire.ConsumeTag(raw)
		if tagLen < 0 {
			return nil, fmt.Errorf("decode ephemeral rich field tag: %w", protowire.ParseError(tagLen))
		}
		raw = raw[tagLen:]
		if number == ephemeralRichMessageField && typ == protowire.BytesType {
			payload, valueLen := protowire.ConsumeBytes(raw)
			if valueLen < 0 {
				return nil, fmt.Errorf("decode ephemeral rich field value: %w", protowire.ParseError(valueLen))
			}
			rich := new(mtproto.RichMessage)
			if err := proto.Unmarshal(payload, rich); err != nil {
				return nil, fmt.Errorf("unmarshal ephemeral rich message: %w", err)
			}
			return rich, nil
		}
		valueLen := protowire.ConsumeFieldValue(number, typ, raw)
		if valueLen < 0 {
			return nil, fmt.Errorf("skip ephemeral unknown field: %w", protowire.ParseError(valueLen))
		}
		raw = raw[valueLen:]
	}
	return nil, nil
}

func withoutEphemeralRichMessage(raw []byte) ([]byte, error) {
	var kept []byte
	for len(raw) > 0 {
		number, typ, tagLen := protowire.ConsumeTag(raw)
		if tagLen < 0 {
			return nil, fmt.Errorf("decode ephemeral unknown field tag: %w", protowire.ParseError(tagLen))
		}
		field := raw[:tagLen]
		raw = raw[tagLen:]
		valueLen := protowire.ConsumeFieldValue(number, typ, raw)
		if valueLen < 0 {
			return nil, fmt.Errorf("decode ephemeral unknown field value: %w", protowire.ParseError(valueLen))
		}
		if number != ephemeralRichMessageField {
			kept = append(kept, field...)
			kept = append(kept, raw[:valueLen]...)
		}
		raw = raw[valueLen:]
	}
	return kept, nil
}
