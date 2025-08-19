package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

func TestGRPCReflection(t *testing.T) {
	// Skip integration test if no valid config
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("test reflection against cosmos endpoint", func(t *testing.T) {
		// This test assumes the proxy is running on localhost:9090
		// In a real CI environment, we'd start the proxy programmatically

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		conn, err := grpc.DialContext(ctx, "localhost:9090",
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock())

		if err != nil {
			t.Skipf("Skipping test - proxy not running on localhost:9090: %v", err)
			return
		}
		defer conn.Close()

		client := grpc_reflection_v1alpha.NewServerReflectionClient(conn)
		stream, err := client.ServerReflectionInfo(ctx)
		require.NoError(t, err)

		// Test list services reflection
		err = stream.Send(&grpc_reflection_v1alpha.ServerReflectionRequest{
			Host: "localhost",
			MessageRequest: &grpc_reflection_v1alpha.ServerReflectionRequest_ListServices{
				ListServices: "*",
			},
		})
		require.NoError(t, err)

		resp, err := stream.Recv()
		require.NoError(t, err)
		assert.NotNil(t, resp)

		// Verify we get a list services response
		if listResp := resp.GetListServicesResponse(); listResp != nil {
			assert.NotEmpty(t, listResp.Service, "Should have at least one service")

			// Look for common Cosmos services
			serviceNames := make([]string, len(listResp.Service))
			for i, svc := range listResp.Service {
				serviceNames[i] = svc.Name
			}

			t.Logf("Available services: %v", serviceNames)

			// Check for some expected Cosmos services
			hasCosmosService := false
			for _, name := range serviceNames {
				if name == "cosmos.base.tendermint.v1beta1.Service" ||
					name == "cosmos.bank.v1beta1.Query" ||
					name == "cosmos.staking.v1beta1.Query" {
					hasCosmosService = true
					break
				}
			}
			assert.True(t, hasCosmosService, "Should have at least one Cosmos service")
		}

		stream.CloseSend()
	})

	t.Run("test reflection against osmosis endpoint", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		conn, err := grpc.DialContext(ctx, "localhost:9091",
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock())

		if err != nil {
			t.Skipf("Skipping test - proxy not running on localhost:9091: %v", err)
			return
		}
		defer conn.Close()

		client := grpc_reflection_v1alpha.NewServerReflectionClient(conn)
		stream, err := client.ServerReflectionInfo(ctx)
		require.NoError(t, err)

		// Test list services reflection
		err = stream.Send(&grpc_reflection_v1alpha.ServerReflectionRequest{
			Host: "localhost",
			MessageRequest: &grpc_reflection_v1alpha.ServerReflectionRequest_ListServices{
				ListServices: "*",
			},
		})
		require.NoError(t, err)

		resp, err := stream.Recv()
		require.NoError(t, err)
		assert.NotNil(t, resp)

		// Verify we get a list services response
		if listResp := resp.GetListServicesResponse(); listResp != nil {
			assert.NotEmpty(t, listResp.Service, "Should have at least one service")

			serviceNames := make([]string, len(listResp.Service))
			for i, svc := range listResp.Service {
				serviceNames[i] = svc.Name
			}

			t.Logf("Available Osmosis services: %v", serviceNames)
		}

		stream.CloseSend()
	})
}

func TestServiceDiscovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("discover cosmos services", func(t *testing.T) {
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

		client := grpc_reflection_v1alpha.NewServerReflectionClient(conn)
		stream, err := client.ServerReflectionInfo(ctx)
		require.NoError(t, err)

		// Get service descriptor for a known Cosmos service
		err = stream.Send(&grpc_reflection_v1alpha.ServerReflectionRequest{
			Host: "localhost",
			MessageRequest: &grpc_reflection_v1alpha.ServerReflectionRequest_FileContainingSymbol{
				FileContainingSymbol: "cosmos.base.tendermint.v1beta1.Service",
			},
		})

		if err != nil {
			t.Logf("Could not request service descriptor: %v", err)
			stream.CloseSend()
			return
		}

		resp, err := stream.Recv()
		if err != nil {
			t.Logf("Could not receive service descriptor: %v", err)
			stream.CloseSend()
			return
		}

		// Verify we get a file descriptor response
		if fileResp := resp.GetFileDescriptorResponse(); fileResp != nil {
			assert.NotEmpty(t, fileResp.FileDescriptorProto, "Should have file descriptor")
			t.Logf("Received file descriptor for cosmos.base.tendermint.v1beta1.Service")
		}

		stream.CloseSend()
	})
}
