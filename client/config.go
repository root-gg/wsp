package client

import (
	"io/ioutil"
	"os"

	"github.com/nu7hatch/gouuid"
	"gopkg.in/yaml.v2"

	"github.com/root-gg/wsp/common"
)

// Config configures an Proxy
type Config struct {
	Name               string
	ID                 string `json:"-"`
	Targets            []string
	PoolIdleSize       int
	PoolMaxSize        int
	Whitelist          []*common.Rule
	Blacklist          []*common.Rule
	SecretKey          string
	InsecureSkipVerify bool
}

// NewConfig creates a new ProxyConfig
func NewConfig() (config *Config) {
	config = new(Config)

	id, err := uuid.NewV4()
	if err != nil {
		panic(err)
	}
	config.ID = id.String()

	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	config.Name = hostname

	config.Targets = []string{"ws://127.0.0.1:8080/register"}
	config.PoolIdleSize = 10
	config.PoolMaxSize = 100

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

	return
}
