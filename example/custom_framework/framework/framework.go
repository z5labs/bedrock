package framework

import (
	"context"
	_ "embed"
	"io"
	"log/slog"
	"net/http"
)

//go:embed base_config.yaml
var baseCfgSrc []byte

type Config struct {
	OTel struct {
		ServiceName string `config:"serviceName"`
		OTLP        struct {
			Target string `config:"target"`
		} `config:"otlp"`
	} `config:"otel"`

	Logging struct {
		Level slog.Level `config:"level"`
	} `config:"logging"`

	Http struct {
		Port uint `config:"port"`
	} `config:"http"`
}

func Rest[T any](cfg io.Reader, f func(context.Context, T, *http.ServeMux) error) error {
	return nil
}
