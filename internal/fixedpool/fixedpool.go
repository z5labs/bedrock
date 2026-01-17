// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package fixedpool

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type Task func(context.Context) error

func Wait(ctx context.Context, tasks ...Task) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	var wg sync.WaitGroup
	errCh := make(chan error, len(tasks))

	for _, task := range tasks {
		wg.Add(1)
		go func(t Task) {
			defer wg.Done()

			var err error
			defer func() {
				// Must be called directly in defer for recover() to work
				r := recover()
				if r != nil {
					rerr, ok := r.(error)
					if !ok {
						rerr = fmt.Errorf("recovered from panic: %v", r)
					}
					err = errors.Join(err, rerr)
				}
				if err != nil {
					errCh <- err
					cancel(err)
				}
			}()

			err = t(ctx)
		}(task)
	}

	wg.Wait()
	close(errCh)

	var jerr error
	for err := range errCh {
		jerr = errors.Join(jerr, err)
	}
	return jerr
}
