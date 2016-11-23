package client

import (
	"io/ioutil"

	"github.com/nu7hatch/gouuid"
	"gopkg.in/yaml.v2"

	"github.com/root-gg/wsp/common"
)

// Config configures an Proxy
type Config struct {
	ID              string
	Targets         []string
	PoolMinSize     int
	PoolMinIdleSize int
	PoolMaxSize     int
	Whitelist       []*common.Rule
	Blacklist       []*common.Rule
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
	config.PoolMinSize = 10
	config.PoolMinIdleSize = 5
	config.PoolMaxSize = 100

	config.Whitelist = make([]*common.Rule, 0)
	config.Blacklist = make([]*common.Rule, 0)

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
