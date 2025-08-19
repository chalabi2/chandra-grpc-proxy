package tests

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// Config structs - duplicated from main.go for testing
type Config struct {
	Name          string `yaml:"name"`
	LocalPort     int    `yaml:"local_port"`
	RemoteAddress string `yaml:"remote_address"`
	UseTLS        bool   `yaml:"use_tls"`
	JWTToken      string `yaml:"jwt_token"`
}

type ProxyConfig struct {
	Endpoints []Config `yaml:"endpoints"`
}

func loadConfig(filename string) (*ProxyConfig, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config ProxyConfig
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

func TestConfigLoading(t *testing.T) {
	// Create a temporary config file
	tempConfig := `
endpoints:
  - name: "test-cosmos"
    local_port: 9090
    remote_address: "cosmos-grpc-api.chandrastation.com:443"
    use_tls: true
    jwt_token: "test_token_123"
  - name: "test-osmosis"
    local_port: 9091
    remote_address: "osmosis-grpc-api.chandrastation.com:443"
    use_tls: true
    jwt_token: "test_token_456"
`

	tmpfile, err := ioutil.TempFile("", "config_test_*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	_, err = tmpfile.Write([]byte(tempConfig))
	require.NoError(t, err)
	tmpfile.Close()

	// Test loading the config
	config, err := loadConfig(tmpfile.Name())
	require.NoError(t, err)
	require.NotNil(t, config)

	// Validate the config
	assert.Len(t, config.Endpoints, 2)

	// Test first endpoint
	cosmos := config.Endpoints[0]
	assert.Equal(t, "test-cosmos", cosmos.Name)
	assert.Equal(t, 9090, cosmos.LocalPort)
	assert.Equal(t, "cosmos-grpc-api.chandrastation.com:443", cosmos.RemoteAddress)
	assert.True(t, cosmos.UseTLS)
	assert.Equal(t, "test_token_123", cosmos.JWTToken)

	// Test second endpoint
	osmosis := config.Endpoints[1]
	assert.Equal(t, "test-osmosis", osmosis.Name)
	assert.Equal(t, 9091, osmosis.LocalPort)
	assert.Equal(t, "osmosis-grpc-api.chandrastation.com:443", osmosis.RemoteAddress)
	assert.True(t, osmosis.UseTLS)
	assert.Equal(t, "test_token_456", osmosis.JWTToken)
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name     string
		config   string
		hasError bool
	}{
		{
			name: "valid config",
			config: `
endpoints:
  - name: "test"
    local_port: 9090
    remote_address: "example.com:443"
    use_tls: true
    jwt_token: "valid_token"
`,
			hasError: false,
		},
		{
			name: "missing name",
			config: `
endpoints:
  - local_port: 9090
    remote_address: "example.com:443"
    use_tls: true
    jwt_token: "valid_token"
`,
			hasError: false, // YAML parsing won't fail, but validation should
		},
		{
			name: "invalid YAML",
			config: `
endpoints:
  - name: "test"
    local_port: not_a_number
`,
			hasError: true,
		},
		{
			name: "empty config",
			config: `
endpoints: []
`,
			hasError: false, // Valid YAML, but empty endpoints
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpfile, err := ioutil.TempFile("", "config_test_*.yaml")
			require.NoError(t, err)
			defer os.Remove(tmpfile.Name())

			_, err = tmpfile.Write([]byte(tt.config))
			require.NoError(t, err)
			tmpfile.Close()

			config, err := loadConfig(tmpfile.Name())

			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, config)
			}
		})
	}
}

func TestConfigFileNotFound(t *testing.T) {
	config, err := loadConfig("nonexistent_config.yaml")
	assert.Error(t, err)
	assert.Nil(t, config)
}
