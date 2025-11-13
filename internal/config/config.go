package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all runtime configuration knobs for the proxy.
type Config struct {
	HTTP struct {
		Addr         string        `mapstructure:"addr"`
		ReadTimeout  time.Duration `mapstructure:"read_timeout"`
		WriteTimeout time.Duration `mapstructure:"write_timeout"`
	} `mapstructure:"http"`
	Bark struct {
		BaseURL        string        `mapstructure:"base_url"`
		Token          string        `mapstructure:"token"`
		RequestTimeout time.Duration `mapstructure:"request_timeout"`
	} `mapstructure:"bark"`
	Storage struct {
		Path string `mapstructure:"path"`
	} `mapstructure:"storage"`
	Crypto struct {
		DefaultAlgorithm string `mapstructure:"default_algorithm"`
		DefaultMode      string `mapstructure:"default_mode"`
		DefaultPadding   string `mapstructure:"default_padding"`
		KeyBytes         int    `mapstructure:"key_bytes"`
		IVBytes          int    `mapstructure:"iv_bytes"`
	} `mapstructure:"crypto"`
	Frontend struct {
		Dir string `mapstructure:"dir"`
	} `mapstructure:"frontend"`
	Auth struct {
		Enabled   bool   `mapstructure:"enabled"`
		Username  string `mapstructure:"username"`
		Password  string `mapstructure:"password"`
		JWTSecret string `mapstructure:"jwt_secret"`
	} `mapstructure:"auth"`
}

// Load reads the configuration from disk/environment using Viper.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.SetEnvPrefix("bark_proxy")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		// v.ReadInConfig returns error if file missing; ignore if not found to allow env-only config
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("load config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("http.addr", ":8090")
	v.SetDefault("http.read_timeout", "15s")
	v.SetDefault("http.write_timeout", "30s")

	v.SetDefault("bark.base_url", "http://127.0.0.1:8080")
	v.SetDefault("bark.request_timeout", "10s")

	v.SetDefault("storage.path", "./data/devices.db")

	v.SetDefault("crypto.default_algorithm", "AES")
	v.SetDefault("crypto.default_mode", "CBC")
	v.SetDefault("crypto.default_padding", "PKCS7Padding")
	v.SetDefault("crypto.key_bytes", 32)
	v.SetDefault("crypto.iv_bytes", 16)

	v.SetDefault("frontend.dir", "./web")

	v.SetDefault("auth.enabled", true)
	v.SetDefault("auth.username", "admin")
	v.SetDefault("auth.password", "admin123")
	v.SetDefault("auth.jwt_secret", "change-me-secret")
}
