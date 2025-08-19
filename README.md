# gRPC Authentication Proxy (Go)

A simple gRPC proxy that adds JWT authentication headers to requests, solving the problem where Hermes cannot add auth headers to gRPC requests but your gRPC servers require authentication.

## Why Go?

After struggling with gRPC trailer handling in Rust's hyper ecosystem, Go was chosen because:

- **Native gRPC Support**: Go has first-class gRPC support with perfect trailer handling
- **Cosmos Ecosystem**: Since Cosmos SDK is Go-based, there's perfect compatibility
- **Simplicity**: A Go gRPC proxy for header injection is ~150 lines vs complex Rust implementation
- **Battle-tested**: Go's gRPC reflection implementation is what Cosmos chains use

## Problem Solved

- **Issue**: Hermes doesn't support adding auth headers to gRPC requests
- **Solution**: Transparent gRPC proxy that injects JWT auth headers
- **Benefit**: Works with any gRPC client (grpcurl, Hermes, etc.)

## Usage

1. **Configure your endpoints**:
   Copy the example config and edit it:

   ```bash
   cp config.example.yaml config.yaml
   ```

   Then edit `config.yaml` and replace the placeholder JWT tokens:

   ```yaml
   endpoints:
     - name: "cosmos-hub"
       local_port: 9090
       remote_address: "cosmos-grpc-api.chandrastation.com:443"
       use_tls: true
       jwt_token: "your_actual_cosmos_jwt_token_here"

     - name: "osmosis"
       local_port: 9091
       remote_address: "osmosis-grpc-api.chandrastation.com:443"
       use_tls: true
       jwt_token: "your_actual_osmosis_jwt_token_here"
   ```

2. **Run the proxy**:

   ```bash
   chmod +x start.sh
   ./start.sh
   ```

3. **Test with grpcurl**:

   ```bash
   # Should now work without trailer errors
   grpcurl -plaintext localhost:9090 list
   grpcurl -plaintext localhost:9091 list

   # Test actual gRPC calls
   grpcurl -plaintext -d '{}' localhost:9090 cosmos.base.tendermint.v1beta1.Service/GetNodeInfo
   ```

4. **Use with Hermes**:
   Update your Hermes config to point to `localhost:9090` and `localhost:9091` instead of the remote endpoints.

## Configuration

The proxy is configured via `config.yaml`. You can add as many endpoints as needed:

```yaml
endpoints:
  - name: "cosmos-hub"
    local_port: 9090
    remote_address: "cosmos-grpc-api.chandrastation.com:443"
    use_tls: true
    jwt_token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."

  - name: "juno"
    local_port: 9092
    remote_address: "juno-grpc-api.chandrastation.com:443"
    use_tls: true
    jwt_token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
```

**⚠️ Important**: The `config.yaml` file is gitignored to prevent accidentally committing JWT tokens.

## How It Works

1. **Transparent Proxy**: Uses gRPC's `UnknownServiceHandler` to proxy any method
2. **Header Injection**: Adds `Authorization: Bearer <token>` to all requests
3. **Perfect Streaming**: Preserves all gRPC semantics including trailers and streaming
4. **No Proto Files**: Works with any gRPC service without needing protocol definitions

## Comparison with Alternatives

| Solution     | Complexity | gRPC Support | Trailer Handling | Cosmos Compatibility |
| ------------ | ---------- | ------------ | ---------------- | -------------------- |
| **Go Proxy** | ✅ Simple  | ✅ Native    | ✅ Perfect       | ✅ Excellent         |
| Rust Proxy   | ❌ Complex | ⚠️ Limited   | ❌ Difficult     | ⚠️ Good              |
| nginx Proxy  | ⚠️ Medium  | ⚠️ Basic     | ⚠️ Limited       | ⚠️ Basic             |

The Go solution is clearly superior for this specific use case of adding auth headers to gRPC requests in the Cosmos ecosystem.
