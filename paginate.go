package main

import (
	"context"
	"math"
	"slices"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

func paginateWithParams(
	interval time.Duration,
	globalLimit uint64,
) func(ctx context.Context, urls []string, filters nostr.Filters) chan nostr.RelayEvent {
	return func(ctx context.Context, urls []string, filters nostr.Filters) chan nostr.RelayEvent {
		// filters will always be just one
		filter := filters[0]

		nextUntil := nostr.Now()
		if filter.Until != nil {
			nextUntil = *filter.Until
		}

		if globalLimit == 0 {
			globalLimit = uint64(filter.Limit)
			if globalLimit == 0 && !filter.LimitZero {
				globalLimit = math.MaxUint64
			}
		}
		var globalCount uint64 = 0
		globalCh := make(chan nostr.RelayEvent)

		repeatedCache := make([]string, 0, 300)
		nextRepeatedCache := make([]string, 0, 300)

		go func() {
			defer close(globalCh)

			for {
				filter.Until = &nextUntil
				time.Sleep(interval)

				keepGoing := false
				for evt := range sys.Pool.SubManyEose(ctx, urls, nostr.Filters{filter}) {
					if slices.Contains(repeatedCache, evt.ID) {
						continue
					}

					keepGoing = true // if we get one that isn't repeated, then keep trying to get more
					nextRepeatedCache = append(nextRepeatedCache, evt.ID)

					globalCh <- evt

					globalCount++
					if globalCount >= globalLimit {
						return
					}

					if evt.CreatedAt < *filter.Until {
						nextUntil = evt.CreatedAt
					}
				}

				if !keepGoing {
					return
				}

				repeatedCache = nextRepeatedCache
				nextRepeatedCache = nextRepeatedCache[:0]
			}
		}()

		return globalCh
	}
}
