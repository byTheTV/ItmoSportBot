package schedule

import (
	"context"
	"sort"
	"sync"

	"itmosportbot/internal/myitmo"
)

// FetchBuildingSchedules параллельно запрашивает GET sign/schedule по каждому building_id.
func FetchBuildingSchedules(ctx context.Context, client *myitmo.Client, date string, buildingIDs []int64, concurrency int) (parts []BuildingPart, failed []int64) {
	ids := uniqueSortedInts(buildingIDs)
	if concurrency <= 0 {
		concurrency = 10
	}

	type res struct {
		bid int64
		raw []byte
		err error
	}
	jobs := make(chan int64)
	out := make(chan res, len(ids))

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for bid := range jobs {
				raw, err := client.ScheduleAvailable(ctx, date, date, bid)
				out <- res{bid: bid, raw: raw, err: err}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, bid := range ids {
			select {
			case <-ctx.Done():
				return
			case jobs <- bid:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(out)
	}()

	for r := range out {
		if r.err != nil {
			failed = append(failed, r.bid)
			continue
		}
		parts = append(parts, BuildingPart{BuildingID: r.bid, Raw: r.raw})
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].BuildingID < parts[j].BuildingID })
	sort.Slice(failed, func(i, j int) bool { return failed[i] < failed[j] })
	return parts, failed
}

// FetchBuildingSchedulesRange — то же для интервала date_start…date_end (один запрос на корпус).
func FetchBuildingSchedulesRange(ctx context.Context, client *myitmo.Client, dateStart, dateEnd string, buildingIDs []int64, concurrency int) (parts []BuildingPart, failed []int64) {
	ids := uniqueSortedInts(buildingIDs)
	if concurrency <= 0 {
		concurrency = 10
	}
	type res struct {
		bid int64
		raw []byte
		err error
	}
	jobs := make(chan int64)
	out := make(chan res, len(ids))
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for bid := range jobs {
				raw, err := client.ScheduleAvailable(ctx, dateStart, dateEnd, bid)
				out <- res{bid: bid, raw: raw, err: err}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, bid := range ids {
			select {
			case <-ctx.Done():
				return
			case jobs <- bid:
			}
		}
	}()
	go func() {
		wg.Wait()
		close(out)
	}()
	for r := range out {
		if r.err != nil {
			failed = append(failed, r.bid)
			continue
		}
		parts = append(parts, BuildingPart{BuildingID: r.bid, Raw: r.raw})
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].BuildingID < parts[j].BuildingID })
	sort.Slice(failed, func(i, j int) bool { return failed[i] < failed[j] })
	return parts, failed
}

// FetchBuildingLimits параллельно запрашивает limits по каждому building_id (ошибки пропускаются).
func FetchBuildingLimits(ctx context.Context, client *myitmo.Client, buildingIDs []int64, concurrency int) [][]byte {
	ids := uniqueSortedInts(buildingIDs)
	if concurrency <= 0 {
		concurrency = 10
	}
	var mu sync.Mutex
	var chunks [][]byte

	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)
	for _, bid := range ids {
		bid := bid
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			lim, err := client.ScheduleLimits(ctx, bid)
			if err != nil || len(lim) == 0 {
				return
			}
			mu.Lock()
			chunks = append(chunks, lim)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return chunks
}

func uniqueSortedInts(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		seen[id] = struct{}{}
	}
	out := make([]int64, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
