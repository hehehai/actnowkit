package licence

import (
	"context"
	"time"
)

// Timing of Run (docs/licensing.md §15).
var (
	// CheckEvery: how often Run looks at the state (a refresh happens only
	// when it is due: 7 days after the last one).
	CheckEvery = 6 * time.Hour
	// RetryAfter: the waits after failed refreshes, then the last one again.
	RetryAfter = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour}
)

// Run checks at once and then on schedule until ctx ends, calling onChange
// with every status whose State, Reason or Usable changed (and the first).
// Failed refreshes are retried after RetryAfter; a refusal (removed,
// revoked, expired) is final until the user activates again. Wake asks for
// a check now (after sleep, or when the network comes back).
func (c *Client) Run(ctx context.Context, onChange func(Status)) (wake func()) {
	kick := make(chan struct{}, 1)
	wake = func() {
		select {
		case kick <- struct{}{}:
		default:
		}
	}
	go func() {
		var last *Status
		failures := 0
		for {
			st := c.Check(ctx)
			if last == nil || st.State != last.State || st.Reason != last.Reason || st.Usable != last.Usable {
				onChange(st)
			}
			last = &st
			wait := CheckEvery
			if st.LastError != "" && (st.State == Grace || st.State == NeedsRefresh) {
				wait = RetryAfter[min(failures, len(RetryAfter)-1)]
				failures++
			} else {
				failures = 0
				// Wake up when the token becomes due, if that is sooner.
				if st.State == Valid {
					if d := st.RefreshAfter.Sub(c.cfg.Now()); d > 0 && d < wait {
						wait = d + time.Minute
					}
				}
			}
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-kick:
				timer.Stop()
			case <-timer.C:
			}
		}
	}()
	return wake
}
