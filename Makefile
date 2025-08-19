# gRPC Auth Proxy Makefile

# Variables
BINARY_NAME=grpc-auth-proxy
CONFIG_FILE=config.yaml
EXAMPLE_CONFIG=config.example.yaml
TEST_CONFIG=config.test.yaml

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

.PHONY: all build clean test deps config run help test-auth test-reflection test-endpoints test-integration

# Default target
all: deps build test

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	$(GOBUILD) -o $(BINARY_NAME) -v main.go

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(TEST_CONFIG)

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Run all tests
test: deps test-unit test-integration

# Run unit tests
test-unit:
	@echo "Running unit tests..."
	$(GOTEST) -v ./...

# Check if config exists and help create it
config:
	@if [ ! -f $(CONFIG_FILE) ]; then \
		echo "$(CONFIG_FILE) not found!"; \
		if [ -f $(EXAMPLE_CONFIG) ]; then \
			echo "Creating $(CONFIG_FILE) from example..."; \
			cp $(EXAMPLE_CONFIG) $(CONFIG_FILE); \
			echo "Please edit $(CONFIG_FILE) and set your JWT tokens"; \
		else \
			echo "No example config found. Please create $(CONFIG_FILE)"; \
		fi; \
		exit 1; \
	fi
	@echo "Config file $(CONFIG_FILE) exists ✓"

# Create test config with dummy tokens
test-config:
	@echo "Creating test configuration..."
	@echo "endpoints:" > $(TEST_CONFIG)
	@echo "  - name: \"test-cosmos\"" >> $(TEST_CONFIG)
	@echo "    local_port: 19090" >> $(TEST_CONFIG)
	@echo "    remote_address: \"cosmos-grpc-api.chandrastation.com:443\"" >> $(TEST_CONFIG)
	@echo "    use_tls: true" >> $(TEST_CONFIG)
	@echo "    jwt_token: \"test_cosmos_token_12345\"" >> $(TEST_CONFIG)
	@echo "  - name: \"test-osmosis\"" >> $(TEST_CONFIG)
	@echo "    local_port: 19091" >> $(TEST_CONFIG)
	@echo "    remote_address: \"osmosis-grpc-api.chandrastation.com:443\"" >> $(TEST_CONFIG)
	@echo "    use_tls: true" >> $(TEST_CONFIG)
	@echo "    jwt_token: \"test_osmosis_token_67890\"" >> $(TEST_CONFIG)

# Run the proxy
run: config build
	@echo "Starting gRPC Auth Proxy..."
	@echo "Press Ctrl+C to stop"
	./$(BINARY_NAME)

# Test auth header injection
test-auth: test-config build
	@echo "Testing auth header injection..."
	$(GOTEST) -v -run TestAuthHeaderInjection ./tests/

# Test gRPC reflection
test-reflection: test-config build
	@echo "Testing gRPC reflection..."
	$(GOTEST) -v -run TestGRPCReflection ./tests/

# Test main gRPC endpoints
test-endpoints: test-config build  
	@echo "Testing gRPC endpoints..."
	$(GOTEST) -v -run TestGRPCEndpoints ./tests/

# Run integration tests (requires actual JWT tokens)
test-integration: config build
	@echo "Running integration tests..."
	@echo "Note: This requires valid JWT tokens in $(CONFIG_FILE)"
	$(GOTEST) -v -run TestIntegration ./tests/

# Test with grpcurl (requires running proxy)
test-grpcurl: 
	@echo "Testing with grpcurl..."
	@echo "Testing Cosmos Hub reflection:"
	grpcurl -plaintext localhost:9090 list || echo "Failed - is proxy running?"
	@echo "Testing Osmosis reflection:"
	grpcurl -plaintext localhost:9091 list || echo "Failed - is proxy running?"

# Development mode - run with file watching
dev: config build
	@echo "Starting in development mode..."
	@which fswatch > /dev/null || (echo "Install fswatch for file watching: brew install fswatch" && exit 1)
	@echo "Watching for changes... Press Ctrl+C to stop"
	@fswatch -o . | xargs -n1 -I{} make build run &
	./$(BINARY_NAME)

# Install grpcurl for testing
install-grpcurl:
	@which grpcurl > /dev/null || (echo "Installing grpcurl..." && go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest)

# Install test dependencies
install-test-deps: install-grpcurl
	@echo "Installing test dependencies..."
	$(GOGET) github.com/stretchr/testify/assert
	$(GOGET) github.com/stretchr/testify/require
	$(GOGET) google.golang.org/grpc/reflection/grpc_reflection_v1alpha

# Help
help:
	@echo "gRPC Auth Proxy Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all              - Build and test everything"
	@echo "  build            - Build the binary"
	@echo "  clean            - Clean build artifacts"
	@echo "  deps             - Download Go dependencies"
	@echo "  config           - Check/create config file"
	@echo "  run              - Build and run the proxy"
	@echo "  test             - Run all tests"
	@echo "  test-unit        - Run unit tests only"
	@echo "  test-auth        - Test auth header injection"
	@echo "  test-reflection  - Test gRPC reflection"
	@echo "  test-endpoints   - Test gRPC endpoints"
	@echo "  test-integration - Run integration tests"
	@echo "  test-grpcurl     - Test with grpcurl (requires running proxy)"
	@echo "  dev              - Run in development mode with file watching"
	@echo "  install-grpcurl  - Install grpcurl for testing"
	@echo "  install-test-deps- Install test dependencies"
	@echo "  help             - Show this help"