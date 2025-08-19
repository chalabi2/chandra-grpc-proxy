package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v2"
)

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

type ProxyServer struct {
	config   Config
	server   *grpc.Server
	upstream *grpc.ClientConn
}

func NewProxyServer(config Config) (*ProxyServer, error) {
	// Create upstream connection
	var conn *grpc.ClientConn
	var err error

	if config.UseTLS {
		conn, err = grpc.Dial(config.RemoteAddress,
			grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
	} else {
		conn, err = grpc.Dial(config.RemoteAddress,
			grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to upstream: %v", err)
	}

	return &ProxyServer{
		config:   config,
		upstream: conn,
	}, nil
}

func (p *ProxyServer) Start() error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", p.config.LocalPort))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %v", p.config.LocalPort, err)
	}

	log.Printf("Starting gRPC proxy for %s on port %d -> %s",
		p.config.Name, p.config.LocalPort, p.config.RemoteAddress)

	// Create gRPC server with transparent proxy
	p.server = grpc.NewServer(
		grpc.UnknownServiceHandler(p.transparentHandler),
	)

	return p.server.Serve(lis)
}

func (p *ProxyServer) transparentHandler(srv interface{}, serverStream grpc.ServerStream) error {
	// Get the method name
	fullMethodName, ok := grpc.MethodFromServerStream(serverStream)
	if !ok {
		return status.Errorf(codes.Internal, "lowlevel gRPC error: cannot get method name")
	}

	// Get incoming metadata and add auth header
	md, _ := metadata.FromIncomingContext(serverStream.Context())
	outCtx := metadata.NewOutgoingContext(serverStream.Context(), metadata.Join(md, metadata.Pairs(
		"authorization", fmt.Sprintf("Bearer %s", p.config.JWTToken),
	)))

	// Create client stream
	clientStream, err := p.upstream.NewStream(outCtx, &grpc.StreamDesc{
		StreamName:    fullMethodName,
		ClientStreams: true,
		ServerStreams: true,
	}, fullMethodName)

	if err != nil {
		return err
	}

	// Bi-directional streaming proxy
	s2cErrChan := p.forwardServerToClient(serverStream, clientStream)
	c2sErrChan := p.forwardClientToServer(clientStream, serverStream)

	// Wait for one of the streams to close
	for i := 0; i < 2; i++ {
		select {
		case s2cErr := <-s2cErrChan:
			if s2cErr == io.EOF {
				// Server ended normally, close client stream
				clientStream.CloseSend()
				break
			} else if s2cErr != nil {
				return s2cErr
			}
		case c2sErr := <-c2sErrChan:
			// Client stream ended
			if c2sErr != io.EOF && c2sErr != nil {
				return c2sErr
			}
		}
	}
	return nil
}

func (p *ProxyServer) forwardClientToServer(src grpc.ClientStream, dst grpc.ServerStream) chan error {
	ret := make(chan error, 1)
	go func() {
		f := &frame{}
		for i := 0; ; i++ {
			if err := src.RecvMsg(f); err != nil {
				ret <- err
				break
			}
			if err := dst.SendMsg(f); err != nil {
				ret <- err
				break
			}
		}
	}()
	return ret
}

func (p *ProxyServer) forwardServerToClient(src grpc.ServerStream, dst grpc.ClientStream) chan error {
	ret := make(chan error, 1)
	go func() {
		f := &frame{}
		for i := 0; ; i++ {
			if err := src.RecvMsg(f); err != nil {
				ret <- err
				break
			}
			if err := dst.SendMsg(f); err != nil {
				ret <- err
				break
			}
		}
	}()
	return ret
}

// frame is used to hold arbitrary gRPC message data
type frame struct {
	payload []byte
}

func (f *frame) Reset() {
	f.payload = f.payload[:0]
}

func (f *frame) String() string {
	return fmt.Sprintf("frame{len=%d}", len(f.payload))
}

func (f *frame) ProtoMessage() {}

func (f *frame) Marshal() ([]byte, error) {
	return f.payload, nil
}

func (f *frame) Unmarshal(data []byte) error {
	f.payload = append(f.payload[:0], data...)
	return nil
}

func loadConfig(filename string) (*ProxyConfig, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	var config ProxyConfig
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file: %v", err)
	}

	return &config, nil
}

func main() {
	// Load configuration from YAML file
	config, err := loadConfig("config.yaml")
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	if len(config.Endpoints) == 0 {
		log.Fatal("No endpoints configured in config.yaml")
	}

	log.Printf("Loaded configuration with %d endpoints", len(config.Endpoints))

	var wg sync.WaitGroup
	for _, endpoint := range config.Endpoints {
		// Validate endpoint configuration
		if endpoint.JWTToken == "" || endpoint.JWTToken == "your_cosmos_jwt_token_here" || endpoint.JWTToken == "your_osmosis_jwt_token_here" {
			log.Fatalf("Please set a valid JWT token for endpoint '%s' in config.yaml", endpoint.Name)
		}

		wg.Add(1)
		go func(cfg Config) {
			defer wg.Done()
			proxy, err := NewProxyServer(cfg)
			if err != nil {
				log.Printf("Failed to create proxy server %s: %v", cfg.Name, err)
				return
			}
			if err := proxy.Start(); err != nil {
				log.Printf("Proxy server %s error: %v", cfg.Name, err)
			}
		}(endpoint)
	}

	log.Println("All proxy servers started")
	wg.Wait()
}
