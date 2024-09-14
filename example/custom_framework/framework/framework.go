package framework

import (
	"bytes"
	"context"
	_ "embed"
	"io"
	"log/slog"
	"os"
	"sync"

	"github.com/z5labs/bedrock/example/custom_framework/framework/internal"
	"github.com/z5labs/bedrock/example/custom_framework/framework/internal/global"

	"github.com/z5labs/bedrock"
)

//go:embed default_config.yaml
var configBytes []byte

func init() {
	global.RegisterConfigSource(internal.ConfigSource(bytes.NewReader(configBytes)))
}

type OTelConfig struct {
	ServiceName    string `config:"service_name"`
	ServiceVersion string `config:"service_version"`
	OTLP           struct {
		Target string `config:"target"`
	} `config:"otlp"`
}

type LoggingConfig struct {
	Level slog.Level `config:"level"`
}

type Config struct {
	OTel    OTelConfig    `config:"otel"`
	Logging LoggingConfig `config:"logging"`
}

var logger *slog.Logger
var initLoggerOnce sync.Once

func Logger(cfg LoggingConfig) *slog.Logger {
	initLoggerOnce.Do(func() {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			AddSource: true,
			Level:     cfg.Level,
		}))
	})
	return logger
}

type App bedrock.App

func Run[T any](r io.Reader, build func(context.Context, T) (App, error)) {
	err := bedrock.Run(
		context.Background(),
		bedrock.AppBuilderFunc[T](func(ctx context.Context, cfg T) (bedrock.App, error) {
			return build(ctx, cfg)
		}),
		global.ConfigSources...,
	)
	if err == nil {
		return
	}

	// there's a chance Run failed on config parsing/unmarshalling
	// thus the logging config is most likely unusable and we should
	// instead create our own logger here for logging this error
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
	}))
	log.Error("failed while running application", slog.String("error", err.Error()))
}
