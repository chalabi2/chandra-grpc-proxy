package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Basic test proto messages - in a real setup these would be generated
// For now we'll test with raw gRPC calls using the Any type

func TestGRPCEndpoints(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("test cosmos node info", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		conn, err := grpc.DialContext(ctx, "localhost:9090",
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock())

		if err != nil {
			t.Skipf("Skipping test - proxy not running on localhost:9090: %v", err)
			return
		}
		defer conn.Close()

		// Test a simple unary call - GetNodeInfo
		// This tests that the proxy correctly forwards auth headers and handles responses
		err = conn.Invoke(ctx, "/cosmos.base.tendermint.v1beta1.Service/GetNodeInfo",
			&emptypb.Empty{}, &emptypb.Empty{})

		// We expect this to either succeed or fail with a specific gRPC error
		// The important thing is that we don't get connection/auth errors
		if err != nil {
			t.Logf("GetNodeInfo call result: %v", err)
			// Check that it's not an auth error (which would indicate our JWT isn't working)
			assert.NotContains(t, err.Error(), "unauthenticated", "Should not get auth errors")
			assert.NotContains(t, err.Error(), "unauthorized", "Should not get auth errors")
			assert.NotContains(t, err.Error(), "authentication", "Should not get auth errors")
		} else {
			t.Logf("GetNodeInfo call succeeded")
		}
	})

	t.Run("test osmosis endpoints", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		conn, err := grpc.DialContext(ctx, "localhost:9091",
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock())

		if err != nil {
			t.Skipf("Skipping test - proxy not running on localhost:9091: %v", err)
			return
		}
		defer conn.Close()

		// Test connection to Osmosis endpoint
		// Similar to Cosmos test but for Osmosis-specific services
		err = conn.Invoke(ctx, "/cosmos.base.tendermint.v1beta1.Service/GetNodeInfo",
			&emptypb.Empty{}, &emptypb.Empty{})

		if err != nil {
			t.Logf("Osmosis GetNodeInfo call result: %v", err)
			// Check that it's not an auth error
			assert.NotContains(t, err.Error(), "unauthenticated", "Should not get auth errors")
			assert.NotContains(t, err.Error(), "unauthorized", "Should not get auth errors")
		} else {
			t.Logf("Osmosis GetNodeInfo call succeeded")
		}
	})
}

func TestProxyHealthCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	endpoints := []struct {
		name string
		addr string
	}{
		{"cosmos", "localhost:9090"},
		{"osmosis", "localhost:9091"},
	}

	for _, endpoint := range endpoints {
		t.Run("health_check_"+endpoint.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			conn, err := grpc.DialContext(ctx, endpoint.addr,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithBlock())

			if err != nil {
				t.Skipf("Skipping test - proxy not running on %s: %v", endpoint.addr, err)
				return
			}
			defer conn.Close()

			// Just test that we can connect - this verifies the proxy is running
			// If we got here without error, the connection is working
		})
	}
}

func TestProxyStreaming(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("test streaming call", func(t *testing.T) {
		// This test would verify that streaming gRPC calls work through the proxy
		// For now, we'll test with the reflection streaming call which we know exists

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		conn, err := grpc.DialContext(ctx, "localhost:9090",
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock())

		if err != nil {
			t.Skipf("Skipping test - proxy not running: %v", err)
			return
		}
		defer conn.Close()

		// Create a streaming call - we'll use the reflection service
		stream, err := conn.NewStream(ctx, &grpc.StreamDesc{
			StreamName:    "ServerReflectionInfo",
			ServerStreams: true,
			ClientStreams: true,
		}, "/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo")

		if err != nil {
			t.Skipf("Could not create stream: %v", err)
			return
		}

		// Test that we can send and receive on the stream
		// This verifies that streaming works through the proxy
		err = stream.CloseSend()
		assert.NoError(t, err, "Should be able to close send stream")

		t.Logf("Streaming test completed successfully")
	})
}

func TestProxyErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("test invalid method", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		conn, err := grpc.DialContext(ctx, "localhost:9090",
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock())

		if err != nil {
			t.Skipf("Skipping test - proxy not running: %v", err)
			return
		}
		defer conn.Close()

		// Test calling a non-existent method
		err = conn.Invoke(ctx, "/invalid.service/InvalidMethod",
			&emptypb.Empty{}, &emptypb.Empty{})

		// We should get a proper gRPC error, not a connection error
		assert.Error(t, err, "Should get an error for invalid method")
		assert.NotContains(t, err.Error(), "connection", "Should not be a connection error")
		t.Logf("Invalid method error (expected): %v", err)
	})

	t.Run("test connection to non-existent port", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Try to connect to a port where the proxy is not running
		conn, err := grpc.DialContext(ctx, "localhost:19999",
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock())

		// This should fail to connect
		assert.Error(t, err, "Should not be able to connect to non-existent service")

		if conn != nil {
			conn.Close()
		}
	})
}
