package tests

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/mwitkow/grpc-proxy/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

// MockGRPCServer for testing auth header injection
type MockGRPCServer struct {
	grpc_reflection_v1alpha.UnimplementedServerReflectionServer
	ReceivedHeaders map[string][]string
}

func (m *MockGRPCServer) ServerReflectionInfo(stream grpc_reflection_v1alpha.ServerReflection_ServerReflectionInfoServer) error {
	// Capture the incoming metadata/headers
	md, ok := metadata.FromIncomingContext(stream.Context())
	if ok {
		m.ReceivedHeaders = make(map[string][]string)
		for k, v := range md {
			m.ReceivedHeaders[k] = v
		}
	}

	// Simple reflection response
	for {
		req, err := stream.Recv()
		if err != nil {
			return err
		}

		resp := &grpc_reflection_v1alpha.ServerReflectionResponse{
			ValidHost:       req.Host,
			OriginalRequest: req,
		}

		if err := stream.Send(resp); err != nil {
			return err
		}
		return nil // End after first response for testing
	}
}

func startMockGRPCServer(port int) (*grpc.Server, *MockGRPCServer, error) {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, nil, err
	}

	server := grpc.NewServer()
	mockService := &MockGRPCServer{}

	// Register reflection service
	grpc_reflection_v1alpha.RegisterServerReflectionServer(server, mockService)

	go func() {
		server.Serve(lis)
	}()

	// Wait a moment for server to start
	time.Sleep(100 * time.Millisecond)

	return server, mockService, nil
}

// createTestProxyServer creates a proxy server for testing using the same logic as main
func createTestProxyServer(localPort int, remoteAddr string, jwtToken string) (*grpc.Server, *grpc.ClientConn, error) {
	// Create upstream connection with keep alive parameters
	var opts []grpc.DialOption

	keepAliveParams := keepalive.ClientParameters{
		Time:                10 * time.Second,
		Timeout:             time.Second,
		PermitWithoutStream: true,
	}
	opts = append(opts, grpc.WithKeepaliveParams(keepAliveParams))
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))

	upstream, err := grpc.NewClient(remoteAddr, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to upstream: %v", err)
	}

	// director function that handles JWT authentication forwarding
	director := func(ctx context.Context, fullMethodName string) (context.Context, grpc.ClientConnInterface, error) {
		inMD, _ := metadata.FromIncomingContext(ctx)
		outMD := inMD.Copy()
		outMD.Set("authorization", fmt.Sprintf("Bearer %s", jwtToken))
		ctx = metadata.NewOutgoingContext(ctx, outMD)
		return ctx, upstream, nil
	}

	// Create gRPC server with the mwitkow proxy handler
	server := grpc.NewServer(
		grpc.UnknownServiceHandler(proxy.TransparentHandler(director)),
	)

	return server, upstream, nil
}

func TestAuthHeaderInjection(t *testing.T) {
	// Start a mock gRPC server on a test port
	mockServer, mockService, err := startMockGRPCServer(19999)
	require.NoError(t, err)
	defer mockServer.Stop()

	// Create a test config that points to our mock server
	testConfig := struct {
		Name          string
		LocalPort     int
		RemoteAddress string
		UseTLS        bool
		JWTToken      string
	}{
		Name:          "test-mock",
		LocalPort:     18999,
		RemoteAddress: "localhost:19999",
		UseTLS:        false,
		JWTToken:      "test_jwt_token_12345",
	}

	t.Run("verify auth header format", func(t *testing.T) {
		expectedAuthHeader := fmt.Sprintf("Bearer %s", testConfig.JWTToken)
		assert.Equal(t, "Bearer test_jwt_token_12345", expectedAuthHeader)
	})

	t.Run("mock server receives headers", func(t *testing.T) {
		// Connect directly to mock server to test header reception
		conn, err := grpc.Dial("localhost:19999", grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer conn.Close()

		client := grpc_reflection_v1alpha.NewServerReflectionClient(conn)

		// Add auth header to context
		md := metadata.New(map[string]string{
			"authorization": "Bearer test_jwt_token_12345",
		})
		ctx := metadata.NewOutgoingContext(context.Background(), md)

		stream, err := client.ServerReflectionInfo(ctx)
		require.NoError(t, err)

		// Send a reflection request
		err = stream.Send(&grpc_reflection_v1alpha.ServerReflectionRequest{
			Host: "localhost",
			MessageRequest: &grpc_reflection_v1alpha.ServerReflectionRequest_ListServices{
				ListServices: "*",
			},
		})
		require.NoError(t, err)

		// Receive response to trigger header capture
		_, err = stream.Recv()
		require.NoError(t, err)

		stream.CloseSend()

		// Verify the mock server received the auth header
		assert.Contains(t, mockService.ReceivedHeaders, "authorization")
		assert.Equal(t, []string{"Bearer test_jwt_token_12345"}, mockService.ReceivedHeaders["authorization"])
	})

	t.Run("test proxy JWT forwarding", func(t *testing.T) {
		// Create a test proxy server
		proxyServer, upstream, err := createTestProxyServer(18999, "localhost:19999", testConfig.JWTToken)
		require.NoError(t, err)
		defer upstream.Close()

		// Start proxy server
		lis, err := net.Listen("tcp", fmt.Sprintf(":%d", testConfig.LocalPort))
		require.NoError(t, err)
		defer lis.Close()

		go func() {
			proxyServer.Serve(lis)
		}()
		defer proxyServer.Stop()

		// Wait for proxy to start
		time.Sleep(200 * time.Millisecond)

		// Connect to proxy (not directly to mock server)
		conn, err := grpc.Dial(fmt.Sprintf("localhost:%d", testConfig.LocalPort),
			grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer conn.Close()

		client := grpc_reflection_v1alpha.NewServerReflectionClient(conn)

		// Make request through proxy (without auth header - proxy should add it)
		ctx := context.Background()
		stream, err := client.ServerReflectionInfo(ctx)
		require.NoError(t, err)

		// Send a reflection request
		err = stream.Send(&grpc_reflection_v1alpha.ServerReflectionRequest{
			Host: "localhost",
			MessageRequest: &grpc_reflection_v1alpha.ServerReflectionRequest_ListServices{
				ListServices: "*",
			},
		})
		require.NoError(t, err)

		// Receive response to trigger header capture
		_, err = stream.Recv()
		require.NoError(t, err)

		stream.CloseSend()

		// Verify the mock server received the JWT auth header from the proxy
		assert.Contains(t, mockService.ReceivedHeaders, "authorization")
		assert.Equal(t, []string{"Bearer test_jwt_token_12345"}, mockService.ReceivedHeaders["authorization"])
	})
}

func TestJWTTokenValidation(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		shouldError bool
	}{
		{
			name:        "valid token",
			token:       "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test.signature",
			shouldError: false,
		},
		{
			name:        "empty token",
			token:       "",
			shouldError: true,
		},
		{
			name:        "placeholder token",
			token:       "your_cosmos_jwt_token_here",
			shouldError: true,
		},
		{
			name:        "simple test token",
			token:       "test_token_123",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the validation logic that would be in main.go
			isInvalidToken := tt.token == "" ||
				tt.token == "your_cosmos_jwt_token_here" ||
				tt.token == "your_osmosis_jwt_token_here"

			if tt.shouldError {
				assert.True(t, isInvalidToken, "Token should be considered invalid")
			} else {
				assert.False(t, isInvalidToken, "Token should be considered valid")
			}
		})
	}
}
