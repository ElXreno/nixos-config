package power

import (
	"context"
	"errors"
	"time"
)

var ErrNoBattery = errors.New("this machine has no battery")

func Wait(ctx context.Context, read func() State, ac bool, every time.Duration) (State, error) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		s := read()
		if !ac && !s.Battery {
			return s, ErrNoBattery
		}
		if s.OnAC == ac {
			return s, nil
		}
		select {
		case <-ctx.Done():
			return s, ctx.Err()
		case <-t.C:
		}
	}
}
