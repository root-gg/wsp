package common

import (
	"encoding/json"
	"fmt"
)

// ClientSettings are sent to the server by the WSP Client to WSP Server
// The poolSize ( number of idle connection ) is ensured by the server which is the
// only one allowed to close connections
type ClientSettings struct {
	ID           string // Instance ID
	Name         string // Hostname ( can be override in the config )
	PoolSize     int    // Number of idle connection to maintain
	ConnectionId uint64 // ID of this specific connection ( should be transmitted in a ConnectionSetting object ? )
}

// Unserialize JSON to a new ClientSettings instance
func ClientSettingsFromJson(bytes []byte) (settings *ClientSettings, err error) {
	// Deserialize request
	settings = new(ClientSettings)
	err = json.Unmarshal(bytes, settings)
	if err != nil {
		return nil, err
	}
	return settings, nil
}

// Serialize the ClientSettings to JSON
func (settings *ClientSettings) ToJson() (bytes []byte, err error) {
	bytes, err = json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("Unable to serialize request : %s", err)
	}
	return bytes, nil
}
