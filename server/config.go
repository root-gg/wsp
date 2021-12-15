package server

import (
	"io/ioutil"
	"strconv"

	"gopkg.in/yaml.v2"
)

// Config configures an Server
type Config struct {
	Host        string
	Port        int
	Timeout     int
	IdleTimeout int
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
