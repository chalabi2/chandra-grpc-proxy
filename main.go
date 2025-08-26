package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/mwitkow/grpc-proxy/proxy"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
)

// Config represents the configuration for a single endpoint
type Config struct {
	Name          string `mapstructure:"name"`
	LocalPort     int    `mapstructure:"local_port"`
	RemoteAddress string `mapstructure:"remote_address"`
	UseTLS        bool   `mapstructure:"use_tls"`
	JWTToken      string `mapstructure:"jwt_token"`
}

// ProxyConfig represents the entire proxy configuration
type ProxyConfig struct {
	Endpoints []Config `mapstructure:"endpoints"`
}

// ProxyServer represents a single proxy server instance
type ProxyServer struct {
	config   Config
	server   *grpc.Server
	upstream *grpc.ClientConn
	listener net.Listener
}

// NewProxyServer creates a new proxy server with the specified configuration
func NewProxyServer(config Config) (*ProxyServer, error) {
	// Create upstream connection with keep alive parameters
	var opts []grpc.DialOption

	// Add keep alive parameters as recommended
	keepAliveParams := keepalive.ClientParameters{
		Time:                10 * time.Second, // send pings every 10 seconds if there is no activity
		Timeout:             time.Second,      // wait 1 second for ping ack before considering the connection dead
		PermitWithoutStream: true,             // send pings even without active streams
	}
	opts = append(opts, grpc.WithKeepaliveParams(keepAliveParams))

	// Configure TLS or insecure credentials
	if config.UseTLS {
		tlsConfig := &tls.Config{
			MinVersion:         tls.VersionTLS12,
			ClientSessionCache: tls.NewLRUClientSessionCache(1024),
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(config.RemoteAddress, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to upstream: %v", err)
	}

	return &ProxyServer{
		config:   config,
		upstream: conn,
	}, nil
}

// director function that handles JWT authentication forwarding
func (p *ProxyServer) director(ctx context.Context, fullMethodName string) (context.Context, grpc.ClientConnInterface, error) {
	// Get incoming metadata
	inMD, _ := metadata.FromIncomingContext(ctx)

	// Copy incoming metadata and add/override authorization header with JWT token
	outMD := inMD.Copy()
	outMD.Set("authorization", fmt.Sprintf("Bearer %s", p.config.JWTToken))

	// Create outgoing context with modified metadata
	ctx = metadata.NewOutgoingContext(ctx, outMD)

	return ctx, p.upstream, nil
}

// Start starts the proxy server
func (p *ProxyServer) Start() error {
	var err error
	p.listener, err = net.Listen("tcp", fmt.Sprintf(":%d", p.config.LocalPort))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %v", p.config.LocalPort, err)
	}

	log.Printf("Starting gRPC proxy for %s on port %d -> %s",
		p.config.Name, p.config.LocalPort, p.config.RemoteAddress)

	// Create gRPC server with the mwitkow proxy handler
	p.server = grpc.NewServer(
		grpc.UnknownServiceHandler(proxy.TransparentHandler(p.director)),
	)

	return p.server.Serve(p.listener)
}

// Stop gracefully stops the proxy server
func (p *ProxyServer) Stop() {
	if p.server != nil {
		log.Printf("Stopping proxy server for %s", p.config.Name)
		p.server.GracefulStop()
	}
	if p.upstream != nil {
		p.upstream.Close()
	}
	if p.listener != nil {
		p.listener.Close()
	}
}

var (
	cfgFile     string
	proxyConfig *ProxyConfig
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "grpc-proxy",
	Short: "A gRPC proxy server with JWT authentication forwarding",
	Long: `A gRPC proxy server that forwards requests to upstream gRPC services
while automatically injecting JWT authentication tokens.

The proxy supports multiple endpoints, each with their own configuration
including different JWT tokens, TLS settings, and port mappings.`,
	Run: func(cmd *cobra.Command, args []string) {
		startProxy()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Define flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.yaml)")

	// Bind flags to viper
	viper.BindPFlag("config", rootCmd.PersistentFlags().Lookup("config"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Search for config in current directory with name "config" (without extension).
		viper.AddConfigPath(".")
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		log.Printf("Using config file: %s", viper.ConfigFileUsed())
	} else {
		log.Fatalf("Error reading config file: %v", err)
	}

	// Unmarshal the configuration
	if err := viper.Unmarshal(&proxyConfig); err != nil {
		log.Fatalf("Error unmarshaling config: %v", err)
	}
}

// startProxy starts all configured proxy servers
func startProxy() {
	if len(proxyConfig.Endpoints) == 0 {
		log.Fatalf("No endpoints configured")
	}

	log.Printf("Loaded configuration with %d endpoints", len(proxyConfig.Endpoints))

	var wg sync.WaitGroup
	var servers []*ProxyServer

	// Create and start all proxy servers
	for _, endpoint := range proxyConfig.Endpoints {
		// Validate endpoint configuration
		if endpoint.JWTToken == "" ||
			endpoint.JWTToken == "your_cosmos_jwt_token_here" ||
			endpoint.JWTToken == "your_osmosis_jwt_token_here" {
			log.Fatalf("Please set a valid JWT token for endpoint '%s'", endpoint.Name)
		}

		proxy, err := NewProxyServer(endpoint)
		if err != nil {
			log.Fatalf("Failed to create proxy server %s: %v", endpoint.Name, err)
		}
		servers = append(servers, proxy)

		wg.Add(1)
		go func(p *ProxyServer) {
			defer wg.Done()
			if err := p.Start(); err != nil {
				log.Printf("Proxy server %s error: %v", p.config.Name, err)
			}
		}(proxy)
	}

	log.Println("All proxy servers started")

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal
	<-sigChan
	log.Println("Received shutdown signal, stopping all servers...")

	// Stop all servers gracefully
	for _, server := range servers {
		go server.Stop()
	}

	// Give servers time to shutdown gracefully
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("All servers stopped gracefully")
	case <-time.After(30 * time.Second):
		log.Println("Timeout waiting for servers to stop, forcing exit")
	}
}

func main() {
	Execute()
}
