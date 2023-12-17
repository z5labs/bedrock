// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/example/simple_grpc/simple_grpc_pb"
	brgrpc "github.com/z5labs/bedrock/grpc"
	"github.com/z5labs/bedrock/pkg/health"
	"github.com/z5labs/bedrock/pkg/otelconfig"

	"go.opentelemetry.io/otel"
	"google.golang.org/grpc"
)

type simpleService struct {
	simple_grpc_pb.UnimplementedSimpleServer
}

func (*simpleService) Echo(ctx context.Context, req *simple_grpc_pb.EchoRequest) (*simple_grpc_pb.EchoResponse, error) {
	_, span := otel.Tracer("main").Start(ctx, "simpleServer.Echo")
	defer span.End()
	resp := &simple_grpc_pb.EchoResponse{
		Message: req.Message,
	}
	return resp, nil
}

func registerSimpleService(s *grpc.Server) {
	simple_grpc_pb.RegisterSimpleServer(s, &simpleService{})
}

func initRuntime(bc bedrock.BuildContext) (bedrock.Runtime, error) {
	logHandler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{AddSource: true})

	rt := brgrpc.NewRuntime(
		brgrpc.ListenOnPort(0),
		brgrpc.LogHandler(logHandler),
		brgrpc.Service(
			registerSimpleService,
			brgrpc.ServiceName("simple"),
			brgrpc.Readiness(&health.Readiness{}),
		),
	)
	return rt, nil
}

func main() {
	bedrock.New(
		bedrock.InitTracerProvider(func(bc bedrock.BuildContext) (otelconfig.Initializer, error) {
			return otelconfig.Local(
				otelconfig.ServiceName("simple_grpc"),
			), nil
		}),
		bedrock.WithRuntimeBuilderFunc(initRuntime),
	).Run()
}
