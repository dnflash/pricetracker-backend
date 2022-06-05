package configuration

import (
	"encoding/json"
	"github.com/BurntSushi/toml"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/pkg/errors"
	"pricetracker/internal/logger"
	"time"
)

type Config struct {
	ServerAddress     string        `json:"server_address"`
	DatabaseURI       string        `json:"database_uri"`
	FetchDataInterval time.Duration `json:"-"`
	LogLevel          logger.Level  `json:"-"`
	LogToFile         bool          `json:"log_to_file"`
	AuthSecretKey     jwk.Key       `json:"-"`
	FCMKey            string        `json:"-"`
}

type tomlConfig struct {
	ServerAddress     string `toml:"server_address"`
	DatabaseURI       string `toml:"database_uri"`
	FetchDataInterval string `toml:"fetch_data_interval"`
	LogLevel          string `toml:"log_level"`
	LogToFile         bool   `toml:"log_to_file"`
	AuthSecretKey     string `toml:"auth_secret_key"`
	FCMKey            string `toml:"fcm_key"`
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
		return nil, errors.Wrapf(err, "failed to parse fetch_data_interval")
	}
	if fetchDataInterval < 10*time.Second {
		return nil, errors.Errorf("fetch_data_interval too short (%v), minimum interval: 10s", fetchDataInterval)
	}

	if tc.LogLevel == "" {
		return nil, errors.New("log_level is not set")
	}
	logLevel, err := logger.ParseLevel(tc.LogLevel)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse log_level")
	}

	if tc.AuthSecretKey == "" {
		return nil, errors.New("auth_secret_key is not set")
	}

	authSecretKey, err := jwk.FromRaw([]byte(tc.AuthSecretKey))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create key from auth_secret_key")
	}

	if tc.FCMKey == "" {
		return nil, errors.New("fcm_key is not set")
	}

	return &Config{
		ServerAddress:     tc.ServerAddress,
		DatabaseURI:       tc.DatabaseURI,
		FetchDataInterval: fetchDataInterval,
		LogLevel:          logLevel,
		LogToFile:         tc.LogToFile,
		AuthSecretKey:     authSecretKey,
		FCMKey:            tc.FCMKey,
	}, nil
}

func (c Config) MarshalJSON() ([]byte, error) {
	type localConfig Config
	type myType struct {
		localConfig
		LogLevel          string `json:"log_level"`
		FetchDataInterval string `json:"fetch_data_interval"`
		AuthSecretKey     string `json:"auth_secret_key"`
		FCMKey            string `json:"fcm_key"`
	}
	mt := myType{localConfig: localConfig(c)}
	mt.LogLevel = c.LogLevel.String()
	mt.FetchDataInterval = c.FetchDataInterval.String()
	if len(c.FCMKey) > 21 {
		mt.FCMKey = c.FCMKey[:21] + "..."
	} else {
		mt.FCMKey = c.FCMKey
	}
	mt.AuthSecretKey = "SET"
	return json.Marshal(mt)
}
