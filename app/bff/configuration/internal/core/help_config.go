package core

import (
	"encoding/json"
	"fmt"

	"github.com/teamgram/proto/mtproto"
	sharedpersist "github.com/teamgram/teamgram-server/app/bff/apifull/persist"
	configuration "github.com/teamgram/teamgram-server/app/bff/configuration/internal/config"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const helpConfigKey = "configuration:help"

// helpConfig reads the deployment configuration first from the service
// config, then from the PostgreSQL-backed shared KV row. The shared row is a
// partial JSON patch, so unrelated service defaults remain effective.
func (c *ConfigurationCore) helpConfig() (configuration.HelpConfig, error) {
	var cfg configuration.HelpConfig
	if c != nil && c.svcCtx != nil {
		cfg = c.svcCtx.Config.Help
	}
	raw, err := sharedpersist.Default.Get(helpConfigKey)
	if err != nil {
		return configuration.HelpConfig{}, fmt.Errorf("read %s: %w", helpConfigKey, err)
	}
	if raw == "" {
		return cfg, nil
	}
	var patch helpConfigPatch
	if err := json.Unmarshal([]byte(raw), &patch); err != nil {
		return configuration.HelpConfig{}, fmt.Errorf("decode %s: %w", helpConfigKey, err)
	}
	patch.apply(&cfg)
	return cfg, nil
}

// helpConfigPatch preserves deployment defaults when the shared row contains
// only a subset of help fields. Pointers also let an explicit false or zero
// override a non-zero value from the service configuration.
type helpConfigPatch struct {
	InviteText            *string `json:"inviteText"`
	SupportPhoneNumber    *string `json:"supportPhoneNumber"`
	SupportUserID         *int64  `json:"supportUserId"`
	SupportUserAccessHash *int64  `json:"supportUserAccessHash"`
	SupportUserFirstName  *string `json:"supportUserFirstName"`
	SupportUserLastName   *string `json:"supportUserLastName"`
	SupportUserUsername   *string `json:"supportUserUsername"`
	AppUpdateCanNotSkip   *bool   `json:"appUpdateCanNotSkip"`
	AppUpdateID           *int32  `json:"appUpdateId"`
	AppUpdateVersion      *string `json:"appUpdateVersion"`
	AppUpdateText         *string `json:"appUpdateText"`
	AppUpdateURL          *string `json:"appUpdateUrl"`
}

func (p helpConfigPatch) apply(cfg *configuration.HelpConfig) {
	if p.InviteText != nil {
		cfg.InviteText = *p.InviteText
	}
	if p.SupportPhoneNumber != nil {
		cfg.SupportPhoneNumber = *p.SupportPhoneNumber
	}
	if p.SupportUserID != nil {
		cfg.SupportUserID = *p.SupportUserID
	}
	if p.SupportUserAccessHash != nil {
		cfg.SupportUserAccessHash = *p.SupportUserAccessHash
	}
	if p.SupportUserFirstName != nil {
		cfg.SupportUserFirstName = *p.SupportUserFirstName
	}
	if p.SupportUserLastName != nil {
		cfg.SupportUserLastName = *p.SupportUserLastName
	}
	if p.SupportUserUsername != nil {
		cfg.SupportUserUsername = *p.SupportUserUsername
	}
	if p.AppUpdateCanNotSkip != nil {
		cfg.AppUpdateCanNotSkip = *p.AppUpdateCanNotSkip
	}
	if p.AppUpdateID != nil {
		cfg.AppUpdateID = *p.AppUpdateID
	}
	if p.AppUpdateVersion != nil {
		cfg.AppUpdateVersion = *p.AppUpdateVersion
	}
	if p.AppUpdateText != nil {
		cfg.AppUpdateText = *p.AppUpdateText
	}
	if p.AppUpdateURL != nil {
		cfg.AppUpdateURL = *p.AppUpdateURL
	}
}

func supportUser(cfg configuration.HelpConfig) *mtproto.User {
	if cfg.SupportUserID <= 0 {
		return mtproto.MakeTLUserEmpty(&mtproto.User{Id: 0}).To_User()
	}
	user := &mtproto.User{
		Id:      cfg.SupportUserID,
		Support: true,
	}
	if cfg.SupportUserAccessHash != 0 {
		user.AccessHash = wrapperspb.Int64(cfg.SupportUserAccessHash)
	}
	if cfg.SupportUserFirstName != "" {
		user.FirstName = wrapperspb.String(cfg.SupportUserFirstName)
	}
	if cfg.SupportUserLastName != "" {
		user.LastName = wrapperspb.String(cfg.SupportUserLastName)
	}
	if cfg.SupportUserUsername != "" {
		user.Username = wrapperspb.String(cfg.SupportUserUsername)
	}
	return mtproto.MakeTLUser(user).To_User()
}
