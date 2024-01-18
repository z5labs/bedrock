package service

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"

	"github.com/z5labs/bedrock/example/custom_framework/framework"
	"github.com/z5labs/bedrock/pkg/slogfield"
)

type Config struct {
	framework.Config

	Custom string `config:"custom"`
}

type service struct {
	log *slog.Logger
}

func New(ctx context.Context) (http.Handler, error) {
	var cfg Config
	err := framework.UnmarshalConfigFromContext(ctx, &cfg)
	if err != nil {
		return nil, err
	}

	logger := slog.New(cfg.LogHandler())

	mux := http.NewServeMux()
	mux.Handle("/echo", &service{
		log: logger,
	})

	return mux, nil
}

func (s *service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	b, err := readAllAndClose(r.Body)
	if err != nil {
		s.log.ErrorContext(ctx, "failed to read request", slogfield.Error(err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	s.log.InfoContext(ctx, "echoing back the received data")

	n, err := io.Copy(w, bytes.NewReader(b))
	if err != nil {
		s.log.ErrorContext(ctx, "failed to write response body", slogfield.Error(err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if n != int64(len(b)) {
		s.log.ErrorContext(
			ctx,
			"failed to write entire response body",
			slogfield.Int64("bytes_written", n),
			slogfield.Int("total_bytes", len(b)),
		)
	}
}

func readAllAndClose(rc io.ReadCloser) ([]byte, error) {
	defer rc.Close()
	return io.ReadAll(rc)
}
