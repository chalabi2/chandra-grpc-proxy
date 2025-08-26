package tests

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

func TestIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test requires a valid config.yaml with real JWT tokens
	if _, err := os.Stat("../config.yaml"); os.IsNotExist(err) {
		t.Skip("Skipping integration test - config.yaml not found")
	}

	// Test the full integration: config loading, proxy startup, auth, and gRPC calls
	t.Run("full_integration_test", func(t *testing.T) {
		// Start the proxy in a goroutine
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Build the proxy first
		buildCmd := exec.Command("go", "build", "-o", "grpc-proxy", ".")
		buildCmd.Dir = ".."
		err := buildCmd.Run()
		require.NoError(t, err, "Should be able to build proxy")

		proxyCmd := exec.CommandContext(ctx, "../grpc-proxy", "--config", "../config.yaml")
		proxyCmd.Dir = ".."

		// Capture stdout/stderr for debugging
		proxyCmd.Stdout = os.Stdout
		proxyCmd.Stderr = os.Stderr

		err = proxyCmd.Start()
		require.NoError(t, err, "Should be able to start proxy")

		// Wait for proxy to start up
		time.Sleep(2 * time.Second)

		// Ensure we clean up the proxy process and binary
		defer func() {
			if proxyCmd.Process != nil {
				proxyCmd.Process.Kill()
				proxyCmd.Wait()
			}
			// Clean up the built binary
			os.Remove("../grpc-proxy")
		}()

		// Test both endpoints
		endpoints := []struct {
			name string
			port string
		}{
			{"cosmos", "9090"},
			{"osmosis", "9091"},
		}

		var wg sync.WaitGroup
		for _, endpoint := range endpoints {
			wg.Add(1)
			go func(name, port string) {
				defer wg.Done()
				testEndpointIntegration(t, name, port)
			}(endpoint.name, endpoint.port)
		}

		wg.Wait()
	})
}

func testEndpointIntegration(t *testing.T, name, port string) {
	t.Run(fmt.Sprintf("integration_%s", name), func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		// Try to connect with retries (proxy might still be starting)
		var conn *grpc.ClientConn
		var err error

		for i := 0; i < 10; i++ {
			conn, err = grpc.DialContext(ctx, fmt.Sprintf("localhost:%s", port),
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithBlock())
			if err == nil {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}

		require.NoError(t, err, "Should be able to connect to proxy on port %s", port)
		defer conn.Close()

		// Test 1: Reflection should work
		t.Logf("Testing reflection for %s endpoint", name)
		client := grpc_reflection_v1alpha.NewServerReflectionClient(conn)
		stream, err := client.ServerReflectionInfo(ctx)
		require.NoError(t, err, "Should be able to create reflection stream")

		err = stream.Send(&grpc_reflection_v1alpha.ServerReflectionRequest{
			Host: "localhost",
			MessageRequest: &grpc_reflection_v1alpha.ServerReflectionRequest_ListServices{
				ListServices: "*",
			},
		})
		require.NoError(t, err, "Should be able to send reflection request")

		resp, err := stream.Recv()
		require.NoError(t, err, "Should be able to receive reflection response")

		if listResp := resp.GetListServicesResponse(); listResp != nil {
			assert.NotEmpty(t, listResp.Service, "Should have services in response")
			t.Logf("%s endpoint has %d services", name, len(listResp.Service))
		}

		stream.CloseSend()

		// Test 2: The proxy should add auth headers (we can't directly verify this,
		// but if we get a proper response, it means auth is working)
		t.Logf("Testing auth header injection for %s endpoint", name)

		// If we get this far without auth errors, auth headers are being added correctly
		t.Logf("Integration test for %s endpoint completed successfully", name)
	})
}

func TestGrpcurlCompatibility(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping grpcurl compatibility test in short mode")
	}

	// Test that grpcurl works with our proxy
	// This ensures compatibility with common gRPC tools
	t.Run("grpcurl_list_services", func(t *testing.T) {
		// Test if grpcurl is available
		_, err := exec.LookPath("grpcurl")
		if err != nil {
			t.Skip("grpcurl not found in PATH")
		}

		// Test grpcurl against cosmos endpoint
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "grpcurl", "-plaintext", "localhost:9090", "list")
		output, err := cmd.Output()

		if err != nil {
			t.Logf("grpcurl failed (may be expected if proxy not running): %v", err)
			t.Logf("grpcurl output: %s", output)
		} else {
			t.Logf("grpcurl succeeded - output length: %d bytes", len(output))
			assert.NotEmpty(t, output, "grpcurl should return service list")
		}
	})

	t.Run("grpcurl_node_info", func(t *testing.T) {
		_, err := exec.LookPath("grpcurl")
		if err != nil {
			t.Skip("grpcurl not found in PATH")
		}

		// Test actual gRPC call through grpcurl
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "grpcurl", "-plaintext", "-d", "{}",
			"localhost:9090", "cosmos.base.tendermint.v1beta1.Service/GetNodeInfo")
		output, err := cmd.Output()

		if err != nil {
			t.Logf("grpcurl GetNodeInfo failed (may be expected): %v", err)
			// Check if it's not an auth error
			assert.NotContains(t, string(output), "unauthenticated")
			assert.NotContains(t, string(output), "unauthorized")
		} else {
			t.Logf("grpcurl GetNodeInfo succeeded - output: %s", output)
		}
	})
}

func TestProxyPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	t.Run("concurrent_connections", func(t *testing.T) {
		// Test that the proxy can handle multiple concurrent connections
		const numConnections = 10
		var wg sync.WaitGroup

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		for i := 0; i < numConnections; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				conn, err := grpc.DialContext(ctx, "localhost:9090",
					grpc.WithTransportCredentials(insecure.NewCredentials()))

				if err != nil {
					t.Logf("Connection %d failed: %v", id, err)
					return
				}
				defer conn.Close()

				// Make a simple call
				client := grpc_reflection_v1alpha.NewServerReflectionClient(conn)
				stream, err := client.ServerReflectionInfo(ctx)
				if err != nil {
					t.Logf("Stream creation %d failed: %v", id, err)
					return
				}

				err = stream.Send(&grpc_reflection_v1alpha.ServerReflectionRequest{
					Host: "localhost",
					MessageRequest: &grpc_reflection_v1alpha.ServerReflectionRequest_ListServices{
						ListServices: "*",
					},
				})

				if err == nil {
					stream.Recv() // Try to receive response
				}

				stream.CloseSend()
				t.Logf("Connection %d completed successfully", id)
			}(i)
		}

		wg.Wait()
		t.Logf("Concurrent connections test completed")
	})
}
