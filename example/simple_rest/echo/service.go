// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package echo

import (
	"context"
	"log/slog"
)

type Option func(*Service)

func LogHandler(h slog.Handler) Option {
	return func(s *Service) {
		s.log = slog.New(h)
	}
}

type Service struct {
	log *slog.Logger
}

func NewService(opts ...Option) *Service {
	s := &Service{}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Service) Handle(ctx context.Context, req Request) (Response, error) {
	s.log.InfoContext(ctx, "echoing back to client", slog.String("msg", req.Msg))

	return Response{Msg: req.Msg}, nil
}
