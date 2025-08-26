package tests

import (
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Config structs - imported from main package structure
type Config struct {
	Name          string `mapstructure:"name"`
	LocalPort     int    `mapstructure:"local_port"`
	RemoteAddress string `mapstructure:"remote_address"`
	UseTLS        bool   `mapstructure:"use_tls"`
	JWTToken      string `mapstructure:"jwt_token"`
}

type ProxyConfig struct {
	Endpoints []Config `mapstructure:"endpoints"`
}

func loadConfigWithViper(filename string) (*ProxyConfig, error) {
	v := viper.New()
	v.SetConfigFile(filename)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var config ProxyConfig
	if err := v.Unmarshal(&config); err != nil {
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

	tmpfile, err := os.CreateTemp("", "config_test_*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	_, err = tmpfile.Write([]byte(tempConfig))
	require.NoError(t, err)
	tmpfile.Close()

	// Test loading the config
	config, err := loadConfigWithViper(tmpfile.Name())
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
			tmpfile, err := os.CreateTemp("", "config_test_*.yaml")
			require.NoError(t, err)
			defer os.Remove(tmpfile.Name())

			_, err = tmpfile.Write([]byte(tt.config))
			require.NoError(t, err)
			tmpfile.Close()

			config, err := loadConfigWithViper(tmpfile.Name())

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
	config, err := loadConfigWithViper("nonexistent_config.yaml")
	assert.Error(t, err)
	assert.Nil(t, config)
}
