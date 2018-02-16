package server

import (
	"io/ioutil"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/root-gg/wsp/common"
)

// Config configures an Server
type Config struct {
	Host        string
	Port        int
	Timeout     time.Duration
	IdleTimeout time.Duration
	Whitelist   []*common.Rule
	Blacklist   []*common.Rule
	SecretKey   string
}

// NewConfig creates a new ProxyConfig
func NewConfig() (config *Config) {
	config = new(Config)
	config.Host = "127.0.0.1"
	config.Port = 8080
	config.Timeout = time.Second
	config.IdleTimeout = 60 * time.Second
	return
}

// LoadConfiguration loads configuration from a YAML file
func LoadConfiguration(path string) (config *Config, err error) {
	config = NewConfig()

	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		return
	}

	err = yaml.Unmarshal(bytes, config)
	if err != nil {
		return
	}

	// Compile the rules

	for _, rule := range config.Whitelist {
		if err = rule.Compile(); err != nil {
			return
		}
	}

	for _, rule := range config.Blacklist {
		if err = rule.Compile(); err != nil {
			return
		}
	}

	return
}
