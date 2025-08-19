# gRPC Authentication Proxy

A simple gRPC proxy that adds JWT authentication headers to requests, solving the problem where Hermes cannot add auth headers to gRPC requests but your gRPC servers require authentication.

**Problem**: Hermes doesn't support adding auth headers to gRPC requests  
**Solution**: Transparent gRPC proxy that injects JWT auth headers  
**Benefit**: Works with any gRPC client (grpcurl, Hermes, etc.)

## Usage

1. **Configure your endpoints**:

   ```bash
   cp config.example.yaml config.yaml
   # Edit config.yaml and replace the placeholder JWT tokens
   ```

2. **Build and run**:

   ```bash
   make run
   ```

3. **Test**:

   ```bash
   # In another terminal
   grpcurl -plaintext localhost:9090 list
   grpcurl -plaintext localhost:9091 list
   ```

4. **Use with Hermes**:
   Update your Hermes config to point to `localhost:9090` and `localhost:9091` instead of the remote endpoints.

## Commands

- `make build` - Build the proxy
- `make run` - Build and run the proxy
- `make test` - Run all tests
- `make clean` - Clean build artifacts
- `make help` - Show all available commands

## Configuration

Edit `config.yaml` to add your endpoints:

```yaml
endpoints:
  - name: "cosmos-hub"
    local_port: 9090
    remote_address: "cosmos-grpc-api.chandrastation.com:443"
    use_tls: true
    jwt_token: "your_jwt_token_here"
```

**Note**: `config.yaml` is gitignored to prevent accidentally committing JWT tokens.

## How It Works

- Transparent proxy using gRPC's `UnknownServiceHandler`
- Adds `Authorization: Bearer <token>` to all requests
- Preserves all gRPC semantics including trailers and streaming
- Works with any gRPC service without needing protocol definitions
