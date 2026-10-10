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
// config, then from the PostgreSQL-backed shared KV row. Keeping the row as a
// complete JSON object makes changes atomic and avoids partially applied help
// responses when multiple BFF instances are serving requests.
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
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return configuration.HelpConfig{}, fmt.Errorf("decode %s: %w", helpConfigKey, err)
	}
	return cfg, nil
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
