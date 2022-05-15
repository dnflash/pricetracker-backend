package configuration

import (
	"github.com/BurntSushi/toml"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/pkg/errors"
	"time"
)

type Config struct {
	ServerAddress     string
	DatabaseURI       string
	FetchDataInterval time.Duration
	LogDebugEnabled   bool
	LogInfoEnabled    bool
	LogErrorEnabled   bool
	AuthSecretKey     jwk.Key
}

type tomlConfig struct {
	ServerAddress     string `toml:"server_address"`
	DatabaseURI       string `toml:"database_uri"`
	FetchDataInterval string `toml:"fetch_data_interval"`
	LogDebugEnabled   bool   `toml:"log_debug_enabled"`
	LogInfoEnabled    bool   `toml:"log_info_enabled"`
	LogErrorEnabled   bool   `toml:"log_error_enabled"`
	AuthSecretKey     string `toml:"auth_secret_key"`
}

func GetConfig(path string) (*Config, error) {
	var tc tomlConfig
	_, err := toml.DecodeFile(path, &tc)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode toml file with path: %s", path)
	}

	if tc.ServerAddress == "" {
		tc.ServerAddress = "localhost:8888"
	}

	if tc.DatabaseURI == "" {
		tc.DatabaseURI = "mongodb://localhost:27017"
	}

	if tc.FetchDataInterval == "" {
		return nil, errors.New("fetch_data_interval is not set")
	}
	fetchDataInterval, err := time.ParseDuration(tc.FetchDataInterval)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse fetch_data_interval: %s", path)
	}
	if fetchDataInterval < 15*time.Second {
		return nil, errors.Errorf("fetch_data_interval too short (%v), minimum interval: 15s", fetchDataInterval)
	}

	if tc.AuthSecretKey == "" {
		return nil, errors.New("auth_secret_key is not set")
	}

	authSecretKey, err := jwk.FromRaw([]byte(tc.AuthSecretKey))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create key from auth_secret_key")
	}

	return &Config{
		ServerAddress:     tc.ServerAddress,
		DatabaseURI:       tc.DatabaseURI,
		FetchDataInterval: fetchDataInterval,
		LogDebugEnabled:   tc.LogDebugEnabled,
		LogInfoEnabled:    tc.LogInfoEnabled,
		LogErrorEnabled:   tc.LogErrorEnabled,
		AuthSecretKey:     authSecretKey,
	}, nil
}
