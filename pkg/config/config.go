package config

import (
	"log"
	"os"
	"strings"
	"time"

	"github.com/creasty/defaults"
	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

type Config struct {
	HTTP           HTTP     `mapstructure:"http"`
	BaseURL        string   `mapstructure:"base_url" default:"http://localhost:8080" validate:"required,url"`
	DB             DB       `mapstructure:"db"`
	GitHub         GitHub   `mapstructure:"github"`
	Admins         []string `mapstructure:"admins"`
	Secrets        Secrets  `mapstructure:"secrets"`
	AllowedDomains []string `mapstructure:"allowed_domains" default:"[\"localhost\"]" validate:"min=1"`
	Session        Session  `mapstructure:"session"`
	OIDC           OIDC     `mapstructure:"oidc"`
	Build          Build    `mapstructure:"build"`
	Deploy         Deploy   `mapstructure:"deploy"`
	Agent          Agent    `mapstructure:"agent"`
}

type Deploy struct {
	Timeout time.Duration `mapstructure:"timeout" default:"30m" validate:"gt=0"`
}

type Agent struct {
	Token string `mapstructure:"token"`
}

type OIDC struct {
	Issuer   string `mapstructure:"issuer" default:"https://token.actions.githubusercontent.com" validate:"required,url"`
	Audience string `mapstructure:"audience"`
}

type Build struct {
	Timeout time.Duration `mapstructure:"timeout" default:"60m" validate:"gt=0"`
}

type HTTP struct {
	Addr string `mapstructure:"addr" default:":8080" validate:"required"`
}

type DB struct {
	DSN          string `mapstructure:"dsn" default:"root:test@tcp(127.0.0.1:3306)/platform" validate:"required"`
	MaxOpenConns int    `mapstructure:"max_open_conns" default:"10" validate:"gt=0"`
}

type GitHub struct {
	ClientID      string `mapstructure:"client_id"`
	ClientSecret  string `mapstructure:"client_secret"`
	OAuthURL      string `mapstructure:"oauth_url" default:"https://github.com" validate:"required,url"`
	APIURL        string `mapstructure:"api_url" default:"https://api.github.com" validate:"required,url"`
	AppID         int64  `mapstructure:"app_id"`
	AppPrivateKey string `mapstructure:"app_private_key"`
	WebhookSecret string `mapstructure:"webhook_secret"`
}

type Secrets struct {
	Key   string `mapstructure:"key" default:"MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE=" validate:"required,base64"`
	KeyID string `mapstructure:"key_id" default:"k1" validate:"required"`
}

type Session struct {
	TTL         time.Duration `mapstructure:"ttl" default:"168h" validate:"gt=0"`
	RenewWithin time.Duration `mapstructure:"renew_within" default:"24h" validate:"gt=0"`
}

func LoadConfig() (Config, error) {
	var config Config
	if err := defaults.Set(&config); err != nil {
		return Config{}, err
	}
	readConfigFile()
	bindEnvVars()
	if err := viper.Unmarshal(&config); err != nil {
		return Config{}, err
	}
	if err := validator.New().Struct(config); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (c Config) OIDCAudience() string {
	if c.OIDC.Audience != "" {
		return c.OIDC.Audience
	}
	return c.BaseURL
}

func (c Config) SecureCookies() bool {
	return strings.HasPrefix(c.BaseURL, "https://")
}

func readConfigFile() {
	viper.SetConfigName("config")
	viper.SetConfigType("yml")
	viper.AddConfigPath(".")
	viper.AddConfigPath(os.Getenv("PLATFORM_PATH"))
	if err := viper.ReadInConfig(); err != nil {
		log.Printf("no config file, using defaults and environment: %s", err)
	}
}

var envKeys = []string{
	"http.addr", "base_url", "db.dsn", "db.max_open_conns",
	"github.client_id", "github.client_secret", "github.oauth_url", "github.api_url",
	"admins", "secrets.key", "secrets.key_id", "allowed_domains",
	"session.ttl", "session.renew_within",
	"github.app_id", "github.app_private_key", "github.webhook_secret",
	"oidc.issuer", "oidc.audience", "build.timeout", "deploy.timeout", "agent.token",
}

func bindEnvVars() {
	viper.SetEnvPrefix("PLATFORM")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	for _, key := range envKeys {
		_ = viper.BindEnv(key)
	}
}
