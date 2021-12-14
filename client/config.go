package client

import (
	"io/ioutil"

	uuid "github.com/nu7hatch/gouuid"
	"gopkg.in/yaml.v2"

	"github.com/root-gg/wsp"
)

// Config configures an Proxy
type Config struct {
	ID           string
	Targets      []string
	PoolIdleSize int
	PoolMaxSize  int
	Whitelist    []*wsp.Rule
	Blacklist    []*wsp.Rule
	SecretKey    string
}

// NewConfig creates a new ProxyConfig
func NewConfig() (config *Config) {
	config = new(Config)

	id, err := uuid.NewV4()
	if err != nil {
		panic(err)
	}
	config.ID = id.String()

	config.Targets = []string{"ws://127.0.0.1:8080/register"}
	config.PoolIdleSize = 10
	config.PoolMaxSize = 100

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
