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

func Init(ctx context.Context, mux *http.ServeMux) error {
	var cfg Config
	err := framework.UnmarshalConfigFromContext(ctx, &cfg)
	if err != nil {
		return err
	}

	logger := slog.New(framework.LogHandler())

	mux.Handle("/echo", &service{
		log: logger,
	})

	return nil
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
