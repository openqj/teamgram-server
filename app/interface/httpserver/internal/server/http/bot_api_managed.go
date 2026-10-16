// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func getManagedBotToken(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.BotsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: managed bot service is not configured"
	}
	inputBot, err := managedBotInputUser(ctx, r, botID, request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	revoke := false
	if request.has("revoke") {
		revoke, err = request.bool("revoke")
		if err != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: revoke is invalid"
		}
	}
	result, err := ctx.BotsClient.BotsExportBotToken(botRPCContext(r.Context(), botID), &mtproto.TLBotsExportBotToken{
		Bot: inputBot, Revoke: mtproto.ToBool(revoke),
	})
	if err != nil || result == nil || strings.TrimSpace(result.GetToken()) == "" {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return result.GetToken(), http.StatusOK, 0, ""
}

func replaceManagedBotToken(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.BotsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: managed bot service is not configured"
	}
	inputBot, err := managedBotInputUser(ctx, r, botID, request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	result, err := ctx.BotsClient.BotsExportBotToken(botRPCContext(r.Context(), botID), &mtproto.TLBotsExportBotToken{
		Bot: inputBot, Revoke: mtproto.BoolTrue,
	})
	if err != nil || result == nil || strings.TrimSpace(result.GetToken()) == "" {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return result.GetToken(), http.StatusOK, 0, ""
}

func getManagedBotAccessSettings(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.BotsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: managed bot service is not configured"
	}
	inputBot, err := managedBotInputUser(ctx, r, botID, request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	result, err := ctx.BotsClient.BotsGetAccessSettings(botRPCContext(r.Context(), botID), &mtproto.TLBotsGetAccessSettings{Bot: inputBot})
	if err != nil || result == nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	userIDs := make([]int64, 0, len(result.GetAddUsers()))
	for _, user := range result.GetAddUsers() {
		if user != nil && user.GetId() > 0 {
			userIDs = append(userIDs, user.GetId())
		}
	}
	return &botapi.BotAccessSettings{IsAccessRestricted: result.GetRestricted(), AddedUserIDs: userIDs}, http.StatusOK, 0, ""
}

func setManagedBotAccessSettings(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.BotsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: managed bot service is not configured"
	}
	inputBot, err := managedBotInputUser(ctx, r, botID, request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if !request.has("is_access_restricted") {
		return nil, http.StatusBadRequest, 400, "Bad Request: is_access_restricted is required"
	}
	restricted, err := request.bool("is_access_restricted")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: is_access_restricted is invalid"
	}
	var userIDs []int64
	field := "user_ids"
	if !request.has(field) {
		field = "added_user_ids"
	}
	if request.has(field) && request.json(field, &userIDs) != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_ids is invalid"
	}
	if len(userIDs) > 100 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_ids is invalid"
	}
	addUsers := make([]*mtproto.InputUser, 0, len(userIDs))
	for _, userID := range userIDs {
		if userID <= 0 || userID == botID {
			return nil, http.StatusBadRequest, 400, "Bad Request: user_ids contains an invalid user"
		}
		inputUser, inputErr := botAPIProfilePhotoInputUser(ctx, r.Context(), botID, userID)
		if inputErr != nil {
			return nil, http.StatusBadRequest, 400, rpcDescription(inputErr)
		}
		addUsers = append(addUsers, inputUser)
	}
	ok, err := ctx.BotsClient.BotsEditAccessSettings(botRPCContext(r.Context(), botID), &mtproto.TLBotsEditAccessSettings{
		Restricted: restricted, Bot: inputBot, AddUsers: addUsers,
	})
	if err != nil || ok == nil || !mtproto.FromBool(ok) {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func managedBotInputUser(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (*mtproto.InputUser, error) {
	if request == nil {
		return nil, errors.New("user_id is required")
	}
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, errors.New("user_id is invalid")
	}
	return botAPIProfilePhotoInputUser(ctx, requestContext(r), botID, userID)
}

func verifyUser(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	peer, err := botAPIUserPeer(ctx, requestContext(r), botID, userID)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return setCustomVerification(ctx, r, botID, request, peer, true)
}

func verifyChat(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(requestContext(r), ctx, botID, request.string("chat_id"))
	if err != nil || peerID <= 0 || peerType == mtproto.PEER_USER {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	return setCustomVerification(ctx, r, botID, request, botAPIInputPeer(botID, peerType, peerID), true)
}

func removeUserVerification(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	peer, err := botAPIUserPeer(ctx, requestContext(r), botID, userID)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return setCustomVerification(ctx, r, botID, request, peer, false)
}

func removeChatVerification(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(requestContext(r), ctx, botID, request.string("chat_id"))
	if err != nil || peerID <= 0 || peerType == mtproto.PEER_USER {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	return setCustomVerification(ctx, r, botID, request, botAPIInputPeer(botID, peerType, peerID), false)
}

func setCustomVerification(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest, peer *mtproto.InputPeer, enabled bool) (any, int, int, string) {
	if ctx == nil || ctx.BotVerificationIconsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: bot verification service is not configured"
	}
	var description *wrapperspb.StringValue
	if enabled && request.has("custom_description") {
		description = wrapperspb.String(request.string("custom_description"))
	}
	ok, err := ctx.BotVerificationIconsClient.BotsSetCustomVerification(botRPCContext(r.Context(), botID), &mtproto.TLBotsSetCustomVerification{
		Enabled: enabled, Peer: peer, CustomDescription: description,
	})
	if err != nil || ok == nil || !mtproto.FromBool(ok) {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func botAPIUserPeer(ctx *svc.ServiceContext, requestCtx context.Context, botID, userID int64) (*mtproto.InputPeer, error) {
	input, err := botAPIProfilePhotoInputUser(ctx, requestCtx, botID, userID)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: input.GetUserId(), AccessHash: input.GetAccessHash()}).To_InputPeer(), nil
}

func getUserChatBoosts(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.BoostsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: boost service is not configured"
	}
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	_, peerType, peerID, err := resolveBotAPIChatID(requestContext(r), ctx, botID, request.string("chat_id"))
	if err != nil || peerID <= 0 || peerType == mtproto.PEER_USER {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	user, err := botAPIProfilePhotoInputUser(ctx, requestContext(r), botID, userID)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	result, err := ctx.BoostsClient.PremiumGetUserBoosts(botRPCContext(r.Context(), botID), &mtproto.TLPremiumGetUserBoosts{
		Peer: botAPIInputPeer(botID, peerType, peerID), UserId: user,
	})
	if err != nil || result == nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return botapi.ChatBoostsFromMTProto(result), http.StatusOK, 0, ""
}

type botAPIPassportElementError struct {
	Source     string   `json:"source"`
	Type       string   `json:"type"`
	Field      string   `json:"field"`
	DataHash   string   `json:"data_hash"`
	FileHash   string   `json:"file_hash"`
	FileHashes []string `json:"file_hashes"`
	Message    string   `json:"message"`
}

func setPassportDataErrors(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.PassportClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: Passport service is not configured"
	}
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	var inputErrors []botAPIPassportElementError
	if err = request.json("errors", &inputErrors); err != nil || len(inputErrors) == 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: errors is invalid"
	}
	passportErrors := make([]*mtproto.SecureValueError, 0, len(inputErrors))
	for _, input := range inputErrors {
		converted, convertErr := botAPIPassportError(input)
		if convertErr != nil {
			return nil, http.StatusBadRequest, 400, "Bad Request: " + convertErr.Error()
		}
		passportErrors = append(passportErrors, converted)
	}
	inputUser, err := botAPIProfilePhotoInputUser(ctx, r.Context(), botID, userID)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	ok, err := ctx.PassportClient.UsersSetSecureValueErrors(botRPCContext(r.Context(), botID), &mtproto.TLUsersSetSecureValueErrors{
		Id: inputUser, Errors: passportErrors,
	})
	if err != nil || ok == nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return mtproto.FromBool(ok), http.StatusOK, 0, ""
}

func botAPIPassportError(input botAPIPassportElementError) (*mtproto.SecureValueError, error) {
	typ, err := botAPISecureValueType(input.Type)
	if err != nil {
		return nil, err
	}
	base := &mtproto.SecureValueError{Type: typ, Field: input.Field, Text: input.Message}
	source := strings.TrimSpace(input.Source)
	decode := func(value string) ([]byte, error) {
		if strings.TrimSpace(value) == "" {
			return nil, errors.New("passport hash is required")
		}
		decoded, decodeErr := base64.StdEncoding.DecodeString(value)
		if decodeErr != nil {
			decoded, decodeErr = base64.RawStdEncoding.DecodeString(value)
		}
		if decodeErr != nil || len(decoded) == 0 {
			return nil, errors.New("passport hash is invalid")
		}
		return decoded, nil
	}
	switch source {
	case "data":
		base.DataHash, err = decode(input.DataHash)
		if err != nil {
			return nil, err
		}
		return mtproto.MakeTLSecureValueErrorData(base).To_SecureValueError(), nil
	case "front_side":
		base.FileHash_BYTES, err = decode(input.FileHash)
		if err != nil {
			return nil, err
		}
		return mtproto.MakeTLSecureValueErrorFrontSide(base).To_SecureValueError(), nil
	case "reverse_side":
		base.FileHash_BYTES, err = decode(input.FileHash)
		if err != nil {
			return nil, err
		}
		return mtproto.MakeTLSecureValueErrorReverseSide(base).To_SecureValueError(), nil
	case "selfie":
		base.FileHash_BYTES, err = decode(input.FileHash)
		if err != nil {
			return nil, err
		}
		return mtproto.MakeTLSecureValueErrorSelfie(base).To_SecureValueError(), nil
	case "file":
		base.FileHash_BYTES, err = decode(input.FileHash)
		if err != nil {
			return nil, err
		}
		return mtproto.MakeTLSecureValueErrorFile(base).To_SecureValueError(), nil
	case "files":
		if len(input.FileHashes) == 0 {
			return nil, errors.New("file_hashes is required")
		}
		base.FileHash_VECTORBYTES = make([][]byte, 0, len(input.FileHashes))
		for _, hash := range input.FileHashes {
			decoded, decodeErr := decode(hash)
			if decodeErr != nil {
				return nil, decodeErr
			}
			base.FileHash_VECTORBYTES = append(base.FileHash_VECTORBYTES, decoded)
		}
		return mtproto.MakeTLSecureValueErrorFiles(base).To_SecureValueError(), nil
	case "unspecified":
		base.Hash, err = decode(input.DataHash)
		if err != nil {
			return nil, err
		}
		return mtproto.MakeTLSecureValueError(base).To_SecureValueError(), nil
	case "translation_file":
		base.FileHash_BYTES, err = decode(input.FileHash)
		if err != nil {
			return nil, err
		}
		return mtproto.MakeTLSecureValueErrorTranslationFile(base).To_SecureValueError(), nil
	case "translation_files":
		if len(input.FileHashes) == 0 {
			return nil, errors.New("file_hashes is required")
		}
		base.FileHash_VECTORBYTES = make([][]byte, 0, len(input.FileHashes))
		for _, hash := range input.FileHashes {
			decoded, decodeErr := decode(hash)
			if decodeErr != nil {
				return nil, decodeErr
			}
			base.FileHash_VECTORBYTES = append(base.FileHash_VECTORBYTES, decoded)
		}
		return mtproto.MakeTLSecureValueErrorTranslationFiles(base).To_SecureValueError(), nil
	default:
		return nil, errors.New("passport source is invalid")
	}
}

func botAPISecureValueType(value string) (*mtproto.SecureValueType, error) {
	value = strings.TrimSpace(value)
	predicate := ""
	var constructor mtproto.TLConstructor
	switch value {
	case "personal_details":
		predicate, constructor = mtproto.Predicate_secureValueTypePersonalDetails, mtproto.CRC32_secureValueTypePersonalDetails
	case "passport":
		predicate, constructor = mtproto.Predicate_secureValueTypePassport, mtproto.CRC32_secureValueTypePassport
	case "driver_license":
		predicate, constructor = mtproto.Predicate_secureValueTypeDriverLicense, mtproto.CRC32_secureValueTypeDriverLicense
	case "identity_card":
		predicate, constructor = mtproto.Predicate_secureValueTypeIdentityCard, mtproto.CRC32_secureValueTypeIdentityCard
	case "internal_passport":
		predicate, constructor = mtproto.Predicate_secureValueTypeInternalPassport, mtproto.CRC32_secureValueTypeInternalPassport
	case "address":
		predicate, constructor = mtproto.Predicate_secureValueTypeAddress, mtproto.CRC32_secureValueTypeAddress
	case "utility_bill":
		predicate, constructor = mtproto.Predicate_secureValueTypeUtilityBill, mtproto.CRC32_secureValueTypeUtilityBill
	case "bank_statement":
		predicate, constructor = mtproto.Predicate_secureValueTypeBankStatement, mtproto.CRC32_secureValueTypeBankStatement
	case "rental_agreement":
		predicate, constructor = mtproto.Predicate_secureValueTypeRentalAgreement, mtproto.CRC32_secureValueTypeRentalAgreement
	case "passport_registration":
		predicate, constructor = mtproto.Predicate_secureValueTypePassportRegistration, mtproto.CRC32_secureValueTypePassportRegistration
	case "temporary_registration":
		predicate, constructor = mtproto.Predicate_secureValueTypeTemporaryRegistration, mtproto.CRC32_secureValueTypeTemporaryRegistration
	case "phone":
		predicate, constructor = mtproto.Predicate_secureValueTypePhone, mtproto.CRC32_secureValueTypePhone
	case "email":
		predicate, constructor = mtproto.Predicate_secureValueTypeEmail, mtproto.CRC32_secureValueTypeEmail
	default:
		return nil, errors.New("passport element type is invalid")
	}
	return &mtproto.SecureValueType{PredicateName: predicate, Constructor: constructor}, nil
}
