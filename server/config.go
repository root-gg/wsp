package server

import (
	"io/ioutil"
	"strconv"

	"gopkg.in/yaml.v2"

	"github.com/root-gg/wsp"
)

// Config configures an Server
type Config struct {
	Host        string
	Port        int
	Timeout     int
	IdleTimeout int
	Whitelist   []*wsp.Rule
	Blacklist   []*wsp.Rule
	SecretKey   string
}

// GetAddr returns the address to specify a HTTP server address
func (c Config) GetAddr() string {
	return c.Host + ":" + strconv.Itoa(c.Port)
}

// NewConfig creates a new ProxyConfig
func NewConfig() (config *Config) {
	config = new(Config)
	config.Host = "127.0.0.1"
	config.Port = 8080
	config.Timeout = 1000
	config.IdleTimeout = 60000
	config.Whitelist = make([]*wsp.Rule, 0)
	config.Blacklist = make([]*wsp.Rule, 0)
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
