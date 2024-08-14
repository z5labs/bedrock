// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package main

import (
	"context"
	"embed"
	"log/slog"
	"os"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/example/simple_grpc/simple_grpc_pb"
	brgrpc "github.com/z5labs/bedrock/grpc"
	"github.com/z5labs/bedrock/pkg/config"
	"github.com/z5labs/bedrock/pkg/health"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type simpleService struct {
	simple_grpc_pb.UnimplementedSimpleServer
}

func (*simpleService) Echo(ctx context.Context, req *simple_grpc_pb.EchoRequest) (*simple_grpc_pb.EchoResponse, error) {
	resp := &simple_grpc_pb.EchoResponse{
		Message: req.Message,
	}
	return resp, nil
}

func registerSimpleService(s *grpc.Server) {
	simple_grpc_pb.RegisterSimpleServer(s, &simpleService{})
}

type Config struct {
	Logging struct {
		Level slog.Level `config:"level"`
	} `config:"logging"`
}

func initRuntime(ctx context.Context, cfg Config) (bedrock.App, error) {
	logHandler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		AddSource: true,
		Level:     cfg.Logging.Level,
	})

	rt := brgrpc.NewRuntime(
		brgrpc.ListenOnPort(9080),
		brgrpc.LogHandler(logHandler),
		brgrpc.Service(
			registerSimpleService,
			brgrpc.ServiceName("simple"),
			brgrpc.Readiness(&health.Binary{}),
		),
		// register reflection service so you can test this example
		// via Insomnia, Postman and any other API testing tool that
		// understands gRPC reflection.
		brgrpc.Service(func(s *grpc.Server) {
			reflection.Register(s)
		}),
	)
	return rt, nil
}

//go:embed config.yaml
var configDir embed.FS

func main() {
	err := bedrock.Run(
		context.Background(),
		bedrock.AppBuilderFunc[Config](initRuntime),
		config.FromYaml(
			config.NewFileReader(configDir, "config.yaml"),
		),
	)
	if err != nil {
		slog.Default().Error("failed to run", slog.String("error", err.Error()))
	}
}
