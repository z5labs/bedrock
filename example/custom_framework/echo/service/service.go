package service

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/z5labs/bedrock/example/custom_framework/framework"
)

type Config struct {
	// This is completely optional since none of the base config
	// values are used by this service.
	// This simply acts as an example for how to embed a custom framework
	// config into your service config.
	framework.Config `config:",squash"`

	Custom string `config:"custom"`
}

type service struct {
	log *slog.Logger
}

func Init(ctx context.Context, cfg Config, mux *http.ServeMux) error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     cfg.Logging.Level,
		AddSource: true,
	}))

	mux.Handle("/echo", &service{
		log: logger,
	})

	return nil
}

func (s *service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	b, err := readAllAndClose(r.Body)
	if err != nil {
		s.log.ErrorContext(ctx, "failed to read request", slog.String("error", err.Error()))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	s.log.InfoContext(ctx, "echoing back the received data")

	n, err := io.Copy(w, bytes.NewReader(b))
	if err != nil {
		s.log.ErrorContext(ctx, "failed to write response body", slog.String("error", err.Error()))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if n != int64(len(b)) {
		s.log.ErrorContext(
			ctx,
			"failed to write entire response body",
			slog.Any("bytes_written", n),
			slog.Int("total_bytes", len(b)),
		)
	}
}

func readAllAndClose(rc io.ReadCloser) ([]byte, error) {
	defer rc.Close()
	return io.ReadAll(rc)
}
