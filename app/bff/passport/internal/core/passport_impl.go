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
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/code"
	"github.com/teamgram/teamgram-server/pkg/phonenumber"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// One JSON document per user. The key is exactly passport:<userId>:
// (the store has no delete, so removals rewrite this blob).

type passportPhone struct {
	Hash string `json:"hash"`
	Code string `json:"code"`
}

type passportAccept struct {
	BotID       int64                               `json:"bot_id"`
	Scope       string                              `json:"scope"`
	PublicKey   string                              `json:"public_key"`
	Nonce       string                              `json:"nonce,omitempty"`
	ValueHashes []*mtproto.SecureValueHash          `json:"value_hashes,omitempty"`
	Credentials *mtproto.SecureCredentialsEncrypted `json:"credentials,omitempty"`
}

type passportDoc struct {
	Values         map[string]*mtproto.SecureValue `json:"values,omitempty"`
	Errors         []*mtproto.SecureValueError     `json:"errors,omitempty"`
	Accepts        []passportAccept                `json:"accepts,omitempty"`
	Phones         map[string]passportPhone        `json:"phones,omitempty"`
	SecureSecretID int64                           `json:"secure_secret_id,omitempty"`
}

const passportPhoneChallengePurpose = "passport.verify_phone"

type passportUserVerifier interface {
	UserGetImmutableUser(context.Context, *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error)
}

func passportKey(uid int64) string {
	return fmt.Sprintf("passport:%d:", uid)
}

func (c *PassportCore) passportUser() (int64, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return 0, mtproto.ErrAuthKeyUnregistered
	}
	return c.MD.UserId, nil
}

func (c *PassportCore) passportVerifier() passportUserVerifier {
	if c == nil {
		return nil
	}
	if c.userVerifier != nil {
		return c.userVerifier
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil
	}
	return c.svcCtx.Dao.UserClient
}

func (c *PassportCore) passportLookupUser(id int64) (*mtproto.ImmutableUser, error) {
	if id <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	verifier := c.passportVerifier()
	if verifier == nil {
		return nil, mtproto.ErrUserIdInvalid
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	user, err := verifier.UserGetImmutableUser(ctx, &userpb.TLUserGetImmutableUser{Id: id})
	if err != nil {
		return nil, err
	}
	if user == nil || user.GetUser() == nil || user.Id() != id || user.Deleted() {
		return nil, mtproto.ErrUserIdInvalid
	}
	return user, nil
}

func (c *PassportCore) passportBot(id int64) (*mtproto.ImmutableUser, error) {
	if id <= 0 {
		return nil, mtproto.ErrBotInvalid
	}
	user, err := c.passportLookupUser(id)
	if err != nil {
		if err == mtproto.ErrUserIdInvalid {
			return nil, mtproto.ErrBotInvalid
		}
		return nil, err
	}
	if !user.IsBot() {
		return nil, mtproto.ErrBotInvalid
	}
	return user, nil
}

func (c *PassportCore) passportBotCaller(id int64) error {
	if c == nil || c.MD == nil || !c.MD.IsBot {
		return mtproto.ErrUserBotInvalid
	}
	if _, err := c.passportBot(id); err != nil {
		if err == mtproto.ErrBotInvalid {
			return mtproto.ErrUserBotInvalid
		}
		return err
	}
	return nil
}

func passportLoad(uid int64) (*passportDoc, error) {
	raw, err := persist.Default.Get(passportKey(uid))
	if err != nil {
		return nil, err
	}
	doc := &passportDoc{
		Values: map[string]*mtproto.SecureValue{},
		Phones: map[string]passportPhone{},
	}
	if raw == "" {
		return doc, nil
	}
	if err = json.Unmarshal([]byte(raw), doc); err != nil {
		return nil, err
	}
	if doc.Values == nil {
		doc.Values = map[string]*mtproto.SecureValue{}
	}
	if doc.Phones == nil {
		doc.Phones = map[string]passportPhone{}
	}
	for k, v := range doc.Values {
		nv := normalizeSecureValue(v)
		if nv == nil {
			delete(doc.Values, k)
			continue
		}
		nk := nv.GetType().GetPredicateName()
		if nk != k {
			delete(doc.Values, k)
		}
		doc.Values[nk] = nv
	}
	return doc, nil
}

func passportSave(uid int64, doc *passportDoc) error {
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	return persist.Default.Set(passportKey(uid), string(raw))
}

func (d *passportDoc) allValues() []*mtproto.SecureValue {
	keys := make([]string, 0, len(d.Values))
	for k := range d.Values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]*mtproto.SecureValue, 0, len(keys))
	for _, k := range keys {
		if v := d.Values[k]; v != nil {
			out = append(out, v)
		}
	}
	return out
}

func (d *passportDoc) valuesByTypes(types []*mtproto.SecureValueType) []*mtproto.SecureValue {
	out := make([]*mtproto.SecureValue, 0)
	seen := map[string]struct{}{}
	for _, t := range types {
		nt := normalizeType(t)
		if nt == nil {
			continue
		}
		k := nt.GetPredicateName()
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		if v := d.Values[k]; v != nil {
			out = append(out, v)
		}
	}
	return out
}

var secureTypeByPred = map[string]mtproto.TLConstructor{
	mtproto.Predicate_secureValueTypePersonalDetails:       mtproto.CRC32_secureValueTypePersonalDetails,
	mtproto.Predicate_secureValueTypePassport:              mtproto.CRC32_secureValueTypePassport,
	mtproto.Predicate_secureValueTypeDriverLicense:         mtproto.CRC32_secureValueTypeDriverLicense,
	mtproto.Predicate_secureValueTypeIdentityCard:          mtproto.CRC32_secureValueTypeIdentityCard,
	mtproto.Predicate_secureValueTypeInternalPassport:      mtproto.CRC32_secureValueTypeInternalPassport,
	mtproto.Predicate_secureValueTypeAddress:               mtproto.CRC32_secureValueTypeAddress,
	mtproto.Predicate_secureValueTypeUtilityBill:           mtproto.CRC32_secureValueTypeUtilityBill,
	mtproto.Predicate_secureValueTypeBankStatement:         mtproto.CRC32_secureValueTypeBankStatement,
	mtproto.Predicate_secureValueTypeRentalAgreement:       mtproto.CRC32_secureValueTypeRentalAgreement,
	mtproto.Predicate_secureValueTypePassportRegistration:  mtproto.CRC32_secureValueTypePassportRegistration,
	mtproto.Predicate_secureValueTypeTemporaryRegistration: mtproto.CRC32_secureValueTypeTemporaryRegistration,
	mtproto.Predicate_secureValueTypePhone:                 mtproto.CRC32_secureValueTypePhone,
	mtproto.Predicate_secureValueTypeEmail:                 mtproto.CRC32_secureValueTypeEmail,
}

var secureTypeByCtor = map[mtproto.TLConstructor]string{
	mtproto.CRC32_secureValueTypePersonalDetails:       mtproto.Predicate_secureValueTypePersonalDetails,
	mtproto.CRC32_secureValueTypePassport:              mtproto.Predicate_secureValueTypePassport,
	mtproto.CRC32_secureValueTypeDriverLicense:         mtproto.Predicate_secureValueTypeDriverLicense,
	mtproto.CRC32_secureValueTypeIdentityCard:          mtproto.Predicate_secureValueTypeIdentityCard,
	mtproto.CRC32_secureValueTypeInternalPassport:      mtproto.Predicate_secureValueTypeInternalPassport,
	mtproto.CRC32_secureValueTypeAddress:               mtproto.Predicate_secureValueTypeAddress,
	mtproto.CRC32_secureValueTypeUtilityBill:           mtproto.Predicate_secureValueTypeUtilityBill,
	mtproto.CRC32_secureValueTypeBankStatement:         mtproto.Predicate_secureValueTypeBankStatement,
	mtproto.CRC32_secureValueTypeRentalAgreement:       mtproto.Predicate_secureValueTypeRentalAgreement,
	mtproto.CRC32_secureValueTypePassportRegistration:  mtproto.Predicate_secureValueTypePassportRegistration,
	mtproto.CRC32_secureValueTypeTemporaryRegistration: mtproto.Predicate_secureValueTypeTemporaryRegistration,
	mtproto.CRC32_secureValueTypePhone:                 mtproto.Predicate_secureValueTypePhone,
	mtproto.CRC32_secureValueTypeEmail:                 mtproto.Predicate_secureValueTypeEmail,
}

var scopeAliases = map[string]string{
	"personal_details":       mtproto.Predicate_secureValueTypePersonalDetails,
	"passport":               mtproto.Predicate_secureValueTypePassport,
	"driver_license":         mtproto.Predicate_secureValueTypeDriverLicense,
	"identity_card":          mtproto.Predicate_secureValueTypeIdentityCard,
	"internal_passport":      mtproto.Predicate_secureValueTypeInternalPassport,
	"address":                mtproto.Predicate_secureValueTypeAddress,
	"utility_bill":           mtproto.Predicate_secureValueTypeUtilityBill,
	"bank_statement":         mtproto.Predicate_secureValueTypeBankStatement,
	"rental_agreement":       mtproto.Predicate_secureValueTypeRentalAgreement,
	"passport_registration":  mtproto.Predicate_secureValueTypePassportRegistration,
	"temporary_registration": mtproto.Predicate_secureValueTypeTemporaryRegistration,
	"phone":                  mtproto.Predicate_secureValueTypePhone,
	"email":                  mtproto.Predicate_secureValueTypeEmail,
}

func normalizeType(t *mtproto.SecureValueType) *mtproto.SecureValueType {
	if t == nil {
		return nil
	}
	pred := t.GetPredicateName()
	if pred == "" {
		pred = secureTypeByCtor[t.GetConstructor()]
	}
	ctor, ok := secureTypeByPred[pred]
	if !ok {
		return nil
	}
	return &mtproto.SecureValueType{PredicateName: pred, Constructor: ctor}
}

func bytesOrEmpty(b []byte) []byte {
	if len(b) == 0 {
		return []byte{}
	}
	return append([]byte(nil), b...)
}

func normalizeFile(f *mtproto.SecureFile) *mtproto.SecureFile {
	if f == nil {
		return nil
	}
	pred := f.GetPredicateName()
	ctor := f.GetConstructor()
	if pred == "" {
		switch ctor {
		case mtproto.CRC32_secureFileEmpty:
			pred = mtproto.Predicate_secureFileEmpty
		case mtproto.CRC32_secureFile_7d09c27e, mtproto.CRC32_secureFile_e0277a62:
			pred = mtproto.Predicate_secureFile
		}
	}
	if pred == mtproto.Predicate_secureFileEmpty {
		return mtproto.MakeTLSecureFileEmpty(&mtproto.SecureFile{
			Constructor: mtproto.CRC32_secureFileEmpty,
		}).To_SecureFile()
	}
	if pred != mtproto.Predicate_secureFile {
		if f.GetId() == 0 && f.GetAccessHash() == 0 && len(f.GetFileHash()) == 0 && len(f.GetSecret()) == 0 {
			return mtproto.MakeTLSecureFileEmpty(&mtproto.SecureFile{
				Constructor: mtproto.CRC32_secureFileEmpty,
			}).To_SecureFile()
		}
		pred = mtproto.Predicate_secureFile
	}
	f.PredicateName = pred
	f.Constructor = mtproto.CRC32_secureFile_7d09c27e
	f.FileHash = bytesOrEmpty(f.GetFileHash())
	f.Secret = bytesOrEmpty(f.GetSecret())
	return mtproto.MakeTLSecureFile(f).To_SecureFile()
}

func normalizePlain(p *mtproto.SecurePlainData) *mtproto.SecurePlainData {
	if p == nil {
		return nil
	}
	pred := p.GetPredicateName()
	if pred == "" {
		switch p.GetConstructor() {
		case mtproto.CRC32_securePlainEmail:
			pred = mtproto.Predicate_securePlainEmail
		case mtproto.CRC32_securePlainPhone:
			pred = mtproto.Predicate_securePlainPhone
		default:
			if p.GetEmail() != "" && p.GetPhone() == "" {
				pred = mtproto.Predicate_securePlainEmail
			} else {
				pred = mtproto.Predicate_securePlainPhone
			}
		}
	}
	out := &mtproto.SecurePlainData{Phone: p.GetPhone(), Email: p.GetEmail()}
	if pred == mtproto.Predicate_securePlainEmail {
		out.Constructor = mtproto.CRC32_securePlainEmail
		return mtproto.MakeTLSecurePlainEmail(out).To_SecurePlainData()
	}
	out.Constructor = mtproto.CRC32_securePlainPhone
	return mtproto.MakeTLSecurePlainPhone(out).To_SecurePlainData()
}

func normalizeData(d *mtproto.SecureData) *mtproto.SecureData {
	if d == nil {
		return nil
	}
	out := &mtproto.SecureData{
		Constructor: mtproto.CRC32_secureData,
		Data:        bytesOrEmpty(d.GetData()),
		DataHash:    bytesOrEmpty(d.GetDataHash()),
		Secret:      bytesOrEmpty(d.GetSecret()),
	}
	return mtproto.MakeTLSecureData(out).To_SecureData()
}

func normalizeFiles(list []*mtproto.SecureFile) []*mtproto.SecureFile {
	if len(list) == 0 {
		return nil
	}
	out := make([]*mtproto.SecureFile, 0, len(list))
	for _, f := range list {
		if nf := normalizeFile(f); nf != nil {
			out = append(out, nf)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func secureValueDigest(v *mtproto.SecureValue) []byte {
	h := sha256.New()
	if t := v.GetType(); t != nil {
		_, _ = h.Write([]byte(t.GetPredicateName()))
	}
	if d := v.GetData(); d != nil {
		_, _ = h.Write(d.GetData())
		_, _ = h.Write(d.GetDataHash())
		_, _ = h.Write(d.GetSecret())
	}
	writeFile := func(f *mtproto.SecureFile) {
		if f == nil {
			return
		}
		var id [8]byte
		binary.BigEndian.PutUint64(id[:], uint64(f.GetId()))
		_, _ = h.Write(id[:])
		_, _ = h.Write(f.GetFileHash())
		_, _ = h.Write(f.GetSecret())
	}
	writeFile(v.GetFrontSide())
	writeFile(v.GetReverseSide())
	writeFile(v.GetSelfie())
	for _, f := range v.GetTranslation() {
		writeFile(f)
	}
	for _, f := range v.GetFiles() {
		writeFile(f)
	}
	if p := v.GetPlainData(); p != nil {
		_, _ = h.Write([]byte(p.GetPredicateName()))
		_, _ = h.Write([]byte(p.GetPhone()))
		_, _ = h.Write([]byte(p.GetEmail()))
	}
	return h.Sum(nil)
}

func normalizeSecureValue(v *mtproto.SecureValue) *mtproto.SecureValue {
	if v == nil {
		return nil
	}
	t := normalizeType(v.GetType())
	if t == nil {
		return nil
	}
	v.Type = t
	v.Data = normalizeData(v.GetData())
	v.FrontSide = normalizeFile(v.GetFrontSide())
	v.ReverseSide = normalizeFile(v.GetReverseSide())
	v.Selfie = normalizeFile(v.GetSelfie())
	v.Translation = normalizeFiles(v.GetTranslation())
	v.Files = normalizeFiles(v.GetFiles())
	v.PlainData = normalizePlain(v.GetPlainData())
	if len(v.GetHash()) == 0 {
		v.Hash = secureValueDigest(v)
	}
	v.Constructor = mtproto.CRC32_secureValue
	return mtproto.MakeTLSecureValue(v).To_SecureValue()
}

func inputSecureFileToSecure(in *mtproto.InputSecureFile) *mtproto.SecureFile {
	if in == nil {
		return nil
	}
	pred := in.GetPredicateName()
	if pred == "" {
		switch in.GetConstructor() {
		case mtproto.CRC32_inputSecureFile:
			pred = mtproto.Predicate_inputSecureFile
		case mtproto.CRC32_inputSecureFileUploaded:
			pred = mtproto.Predicate_inputSecureFileUploaded
		default:
			if in.GetAccessHash() != 0 && len(in.GetFileHash()) == 0 && len(in.GetSecret()) == 0 && in.GetParts() == 0 {
				pred = mtproto.Predicate_inputSecureFile
			} else {
				pred = mtproto.Predicate_inputSecureFileUploaded
			}
		}
	}
	out := &mtproto.SecureFile{
		Id:         in.GetId(),
		AccessHash: in.GetAccessHash(),
		FileHash:   bytesOrEmpty(in.GetFileHash()),
		Secret:     bytesOrEmpty(in.GetSecret()),
	}
	if pred == mtproto.Predicate_inputSecureFileUploaded {
		out.Size2_INT32 = in.GetParts()
	}
	if pred != mtproto.Predicate_inputSecureFile && pred != mtproto.Predicate_inputSecureFileUploaded &&
		out.Id == 0 && out.AccessHash == 0 && len(in.GetFileHash()) == 0 && len(in.GetSecret()) == 0 {
		return mtproto.MakeTLSecureFileEmpty(&mtproto.SecureFile{
			Constructor: mtproto.CRC32_secureFileEmpty,
		}).To_SecureFile()
	}
	return normalizeFile(out)
}

func inputFilesToSecure(in []*mtproto.InputSecureFile) []*mtproto.SecureFile {
	if len(in) == 0 {
		return nil
	}
	out := make([]*mtproto.SecureFile, 0, len(in))
	for _, f := range in {
		if sf := inputSecureFileToSecure(f); sf != nil {
			out = append(out, sf)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func inputToSecureValue(in *mtproto.InputSecureValue) (*mtproto.SecureValue, error) {
	if in == nil {
		return nil, mtproto.ErrTypesEmpty
	}
	t := normalizeType(in.GetType())
	if t == nil {
		return nil, mtproto.ErrTypesEmpty
	}
	v := &mtproto.SecureValue{
		Type:        t,
		Data:        normalizeData(in.GetData()),
		FrontSide:   inputSecureFileToSecure(in.GetFrontSide()),
		ReverseSide: inputSecureFileToSecure(in.GetReverseSide()),
		Selfie:      inputSecureFileToSecure(in.GetSelfie()),
		Translation: inputFilesToSecure(in.GetTranslation()),
		Files:       inputFilesToSecure(in.GetFiles()),
		PlainData:   normalizePlain(in.GetPlainData()),
	}
	v.Hash = secureValueDigest(v)
	v = normalizeSecureValue(v)
	if v == nil {
		return nil, mtproto.ErrTypesEmpty
	}
	return v, nil
}

type scopeElem struct {
	Type                string      `json:"type"`
	NativeNames         bool        `json:"native_names"`
	Selfie              bool        `json:"selfie"`
	SelfieRequired      bool        `json:"selfie_required"`
	Translation         bool        `json:"translation"`
	TranslationRequired bool        `json:"translation_required"`
	OneOf               []scopeElem `json:"one_of"`
}

func typeFromScopeName(name string) *mtproto.SecureValueType {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	if pred, ok := scopeAliases[name]; ok {
		return normalizeType(&mtproto.SecureValueType{PredicateName: pred})
	}
	return normalizeType(&mtproto.SecureValueType{PredicateName: name})
}

func scopeRequired(el scopeElem) (*mtproto.SecureRequiredType, []string) {
	if len(el.OneOf) > 0 {
		types := make([]*mtproto.SecureRequiredType, 0, len(el.OneOf))
		var keys []string
		for _, sub := range el.OneOf {
			r, ks := scopeRequired(sub)
			if r == nil {
				continue
			}
			types = append(types, r)
			keys = append(keys, ks...)
		}
		if len(types) == 0 {
			return nil, nil
		}
		return mtproto.MakeTLSecureRequiredTypeOneOf(&mtproto.SecureRequiredType{
			Types: types,
		}).To_SecureRequiredType(), keys
	}
	t := typeFromScopeName(el.Type)
	if t == nil {
		return nil, nil
	}
	return mtproto.MakeTLSecureRequiredType(&mtproto.SecureRequiredType{
		NativeNames:         el.NativeNames,
		SelfieRequired:      el.Selfie || el.SelfieRequired,
		TranslationRequired: el.Translation || el.TranslationRequired,
		Type:                t,
	}).To_SecureRequiredType(), []string{t.GetPredicateName()}
}

// parseScope reads a Telegram Passport scope JSON. ok is false when scope is not that object.
// nonce is the trimmed "nonce" field (Layer 229 has no nonce argument; it lives in the scope).
func parseScope(scope string) (nonce string, required []*mtproto.SecureRequiredType, wanted []string, ok bool) {
	scope = strings.TrimSpace(scope)
	if scope == "" || scope[0] != '{' {
		return "", nil, nil, false
	}
	var wrap struct {
		Data  []scopeElem `json:"data"`
		Nonce string      `json:"nonce"`
	}
	if err := json.Unmarshal([]byte(scope), &wrap); err != nil {
		return "", nil, nil, false
	}
	for _, el := range wrap.Data {
		r, keys := scopeRequired(el)
		if r == nil {
			continue
		}
		required = append(required, r)
		wanted = append(wanted, keys...)
	}
	if required == nil {
		required = []*mtproto.SecureRequiredType{}
	}
	return strings.TrimSpace(wrap.Nonce), required, wanted, true
}

func authorizationNonce(scope, publicKey string) (string, []*mtproto.SecureRequiredType, []string, error) {
	nonce, required, wanted, ok := parseScope(scope)
	if !ok || nonce == "" {
		nonce = strings.TrimSpace(publicKey)
	}
	if nonce == "" {
		return "", nil, nil, mtproto.ErrPublicKeyRequired
	}
	if required == nil {
		required = []*mtproto.SecureRequiredType{}
	}
	return nonce, required, wanted, nil
}

func (d *passportDoc) valuesForWanted(wanted []string) []*mtproto.SecureValue {
	if len(wanted) == 0 {
		return d.allValues()
	}
	out := make([]*mtproto.SecureValue, 0)
	seen := map[string]struct{}{}
	for _, k := range wanted {
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		if v := d.Values[k]; v != nil {
			out = append(out, v)
		}
	}
	return out
}

func passportTarget(self int64, id *mtproto.InputUser) (int64, error) {
	if id == nil {
		return 0, mtproto.ErrUserIdInvalid
	}
	predicate := id.GetPredicateName()
	constructor := id.GetConstructor()
	if predicate == "" {
		switch constructor {
		case mtproto.CRC32_inputUserSelf:
			predicate = mtproto.Predicate_inputUserSelf
		case mtproto.CRC32_inputUser:
			predicate = mtproto.Predicate_inputUser
		}
	}
	switch predicate {
	case mtproto.Predicate_inputUserSelf:
		if (constructor != 0 && constructor != mtproto.CRC32_inputUserSelf) || id.GetUserId() != 0 || id.GetAccessHash() != 0 {
			return 0, mtproto.ErrUserIdInvalid
		}
		if self <= 0 {
			return 0, mtproto.ErrUserIdInvalid
		}
		return self, nil
	case mtproto.Predicate_inputUser:
		if (constructor != 0 && constructor != mtproto.CRC32_inputUser) || id.GetUserId() <= 0 || id.GetAccessHash() == 0 {
			return 0, mtproto.ErrUserIdInvalid
		}
		return id.GetUserId(), nil
	default:
		return 0, mtproto.ErrUserIdInvalid
	}
}

// AccountGetAllSecureValues
// account.getAllSecureValues#b288bc7d = Vector<SecureValue>;
func (c *PassportCore) AccountGetAllSecureValues(in *mtproto.TLAccountGetAllSecureValues) (*mtproto.Vector_SecureValue, error) {
	uid, err := c.passportUser()
	if err != nil {
		return nil, err
	}
	_ = in
	doc, err := passportLoad(uid)
	if err != nil {
		return nil, err
	}
	return &mtproto.Vector_SecureValue{Datas: doc.allValues()}, nil
}

// AccountGetSecureValue
// account.getSecureValue#73665bc2 types:Vector<SecureValueType> = Vector<SecureValue>;
func (c *PassportCore) AccountGetSecureValue(in *mtproto.TLAccountGetSecureValue) (*mtproto.Vector_SecureValue, error) {
	uid, err := c.passportUser()
	if err != nil {
		return nil, err
	}
	doc, err := passportLoad(uid)
	if err != nil {
		return nil, err
	}
	var types []*mtproto.SecureValueType
	if in != nil {
		types = in.GetTypes()
	}
	return &mtproto.Vector_SecureValue{Datas: doc.valuesByTypes(types)}, nil
}

// AccountSaveSecureValue
// account.saveSecureValue#899fe31d value:InputSecureValue secure_secret_id:long = SecureValue;
func (c *PassportCore) AccountSaveSecureValue(in *mtproto.TLAccountSaveSecureValue) (*mtproto.SecureValue, error) {
	uid, err := c.passportUser()
	if err != nil {
		return nil, err
	}
	var input *mtproto.InputSecureValue
	var secretID int64
	if in != nil {
		input = in.GetValue()
		secretID = in.GetSecureSecretId()
	}
	stored, err := inputToSecureValue(input)
	if err != nil {
		return nil, err
	}
	doc, err := passportLoad(uid)
	if err != nil {
		return nil, err
	}
	doc.Values[stored.GetType().GetPredicateName()] = stored
	if secretID != 0 {
		doc.SecureSecretID = secretID
	}
	if err = passportSave(uid, doc); err != nil {
		return nil, err
	}
	return stored, nil
}

// AccountDeleteSecureValue
// account.deleteSecureValue#b880bc4b types:Vector<SecureValueType> = Bool;
func (c *PassportCore) AccountDeleteSecureValue(in *mtproto.TLAccountDeleteSecureValue) (*mtproto.Bool, error) {
	uid, err := c.passportUser()
	if err != nil {
		return nil, err
	}
	doc, err := passportLoad(uid)
	if err != nil {
		return nil, err
	}
	if in != nil {
		for _, t := range in.GetTypes() {
			nt := normalizeType(t)
			if nt == nil {
				continue
			}
			delete(doc.Values, nt.GetPredicateName())
		}
	}
	if err = passportSave(uid, doc); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

// AccountGetAuthorizationForm
// account.getAuthorizationForm#a929597a bot_id:long scope:string public_key:string = account.AuthorizationForm;
func (c *PassportCore) AccountGetAuthorizationForm(in *mtproto.TLAccountGetAuthorizationForm) (*mtproto.Account_AuthorizationForm, error) {
	uid, err := c.passportUser()
	if err != nil {
		return nil, err
	}
	var botID int64
	var scope, publicKey string
	if in != nil {
		botID = in.GetBotId()
		scope = in.GetScope()
		publicKey = in.GetPublicKey()
	}
	if botID == 0 {
		return nil, mtproto.ErrBotInvalid
	}
	if _, err = c.passportBot(botID); err != nil {
		return nil, err
	}
	_, required, wanted, err := authorizationNonce(scope, publicKey)
	if err != nil {
		return nil, err
	}
	doc, err := passportLoad(uid)
	if err != nil {
		return nil, err
	}
	errs := doc.Errors
	if errs == nil {
		errs = []*mtproto.SecureValueError{}
	}
	return mtproto.MakeTLAccountAuthorizationForm(&mtproto.Account_AuthorizationForm{
		RequiredTypes: required,
		Values:        doc.valuesForWanted(wanted),
		Errors:        errs,
		Users:         []*mtproto.User{},
	}).To_Account_AuthorizationForm(), nil
}

// AccountAcceptAuthorization
// account.acceptAuthorization#f3ed4c73 bot_id:long scope:string public_key:string value_hashes:Vector<SecureValueHash> credentials:SecureCredentialsEncrypted = Bool;
func (c *PassportCore) AccountAcceptAuthorization(in *mtproto.TLAccountAcceptAuthorization) (*mtproto.Bool, error) {
	if _, err := c.passportUser(); err != nil {
		return nil, err
	}
	_ = in
	return nil, mtproto.ErrMethodNotImpl
}

// AccountSendVerifyPhoneCode
// account.sendVerifyPhoneCode#a5a356f9 phone_number:string settings:CodeSettings = auth.SentCode;
func (c *PassportCore) AccountSendVerifyPhoneCode(in *mtproto.TLAccountSendVerifyPhoneCode) (*mtproto.Auth_SentCode, error) {
	uid, err := c.passportUser()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	rawPhone := strings.TrimSpace(in.GetPhoneNumber())
	if rawPhone == "" {
		return nil, mtproto.ErrPhoneNumberInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Challenges == nil {
		return nil, mtproto.ErrSendCodeUnavailable
	}
	_, phone, err := phonenumber.CheckPhoneNumberInvalid(rawPhone)
	if err != nil {
		return nil, mtproto.ErrPhoneNumberInvalid
	}
	issued, err := c.svcCtx.Challenges.Issue(c.ctx, code.IssueRequest{
		Channel:    code.ChannelSMS,
		Purpose:    passportPhoneChallengePurpose,
		Subject:    phone,
		Scope:      code.ScopeID(uid),
		CodeLength: 5,
	})
	if err != nil {
		switch {
		case errors.Is(err, code.ErrRateLimited):
			return nil, mtproto.ErrPhoneNumberFlood
		case code.ProviderError(err):
			return nil, mtproto.ErrSendCodeUnavailable
		default:
			return nil, mtproto.ErrInternalServerError
		}
	}
	timeout := int32(time.Until(issued.ExpiresAt).Seconds())
	if timeout < 1 {
		timeout = 1
	}
	return mtproto.MakeTLAuthSentCode(&mtproto.Auth_SentCode{
		Type: mtproto.MakeTLAuthSentCodeTypeSms(&mtproto.Auth_SentCodeType{
			Length: int32(len(issued.Code)),
		}).To_Auth_SentCodeType(),
		PhoneCodeHash: issued.ID,
		Timeout:       &wrapperspb.Int32Value{Value: timeout},
	}).To_Auth_SentCode(), nil
}

// AccountVerifyPhone
// account.verifyPhone#4dd3a7f6 phone_number:string phone_code_hash:string phone_code:string = Bool;
func (c *PassportCore) AccountVerifyPhone(in *mtproto.TLAccountVerifyPhone) (*mtproto.Bool, error) {
	uid, err := c.passportUser()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	rawPhone := strings.TrimSpace(in.GetPhoneNumber())
	if rawPhone == "" {
		return nil, mtproto.ErrPhoneNumberInvalid
	}
	hash := strings.TrimSpace(in.GetPhoneCodeHash())
	value := strings.TrimSpace(in.GetPhoneCode())
	if hash == "" {
		return nil, mtproto.ErrPhoneCodeHashEmpty
	}
	if value == "" {
		return nil, mtproto.ErrPhoneCodeEmpty
	}
	if c.svcCtx == nil || c.svcCtx.Challenges == nil {
		return nil, mtproto.ErrPhoneCodeExpired
	}
	_, phone, err := phonenumber.CheckPhoneNumberInvalid(rawPhone)
	if err != nil {
		return nil, mtproto.ErrPhoneNumberInvalid
	}
	request := code.VerifyRequest{
		Channel:     code.ChannelSMS,
		Purpose:     passportPhoneChallengePurpose,
		Scope:       code.ScopeID(uid),
		ChallengeID: hash,
		Code:        value,
	}
	challenge, err := c.svcCtx.Challenges.Check(c.ctx, request)
	if err != nil {
		return nil, mapPassportPhoneChallengeError(err)
	}
	if challenge == nil || challenge.Subject != phone {
		return nil, mtproto.ErrPhoneCodeInvalid
	}
	if _, err = c.svcCtx.Challenges.Consume(c.ctx, request); err != nil {
		return nil, mapPassportPhoneChallengeError(err)
	}
	doc, err := passportLoad(uid)
	if err != nil {
		return nil, err
	}
	doc.Phones[phone] = passportPhone{Hash: hash}
	if err = passportSave(uid, doc); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func mapPassportPhoneChallengeError(err error) error {
	switch {
	case errors.Is(err, code.ErrChallengeInvalid):
		return mtproto.ErrPhoneCodeInvalid
	case errors.Is(err, code.ErrChallengeNotFound), errors.Is(err, code.ErrChallengeExpired):
		return mtproto.ErrPhoneCodeExpired
	default:
		return mtproto.ErrInternalServerError
	}
}

// UsersSetSecureValueErrors
// users.setSecureValueErrors#90c894b5 id:InputUser errors:Vector<SecureValueError> = Bool;
func (c *PassportCore) UsersSetSecureValueErrors(in *mtproto.TLUsersSetSecureValueErrors) (*mtproto.Bool, error) {
	uid, err := c.passportUser()
	if err != nil {
		return nil, err
	}
	if err = c.passportBotCaller(uid); err != nil {
		return nil, err
	}
	var id *mtproto.InputUser
	var errsIn []*mtproto.SecureValueError
	if in != nil {
		id = in.GetId()
		errsIn = in.GetErrors()
	}
	target, err := passportTarget(uid, id)
	if err != nil {
		return nil, err
	}
	if id.GetPredicateName() == mtproto.Predicate_inputUser || id.GetConstructor() == mtproto.CRC32_inputUser {
		user, lookupErr := c.passportLookupUser(target)
		if lookupErr != nil {
			return nil, lookupErr
		}
		if user.AccessHash() != id.GetAccessHash() {
			return nil, mtproto.ErrUserIdInvalid
		}
	}
	doc, err := passportLoad(target)
	if err != nil {
		return nil, err
	}
	errs := make([]*mtproto.SecureValueError, 0, len(errsIn))
	for _, e := range errsIn {
		if e != nil {
			errs = append(errs, e)
		}
	}
	doc.Errors = errs
	if err = passportSave(target, doc); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

// HelpGetPassportConfig
// help.getPassportConfig#c661ad08 hash:int = help.PassportConfig;
func (c *PassportCore) HelpGetPassportConfig(in *mtproto.TLHelpGetPassportConfig) (*mtproto.Help_PassportConfig, error) {
	_ = in
	return mtproto.MakeTLHelpPassportConfig(&mtproto.Help_PassportConfig{
		Hash: 0,
		CountriesLangs: mtproto.MakeTLDataJSON(&mtproto.DataJSON{
			Data: "{}",
		}).To_DataJSON(),
	}).To_Help_PassportConfig(), nil
}
