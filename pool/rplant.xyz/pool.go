package rplant_xyz

import (
	"encoding/json"
	"io"
	"monero-blocks/pool"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Pool implements fetching recent Monero blocks from rplant.xyz API.
// API: https://pool.rplant.xyz/api2/poolminer2/monero/0/0
type Pool struct {
	throttler <-chan time.Time
	client    *http.Client
}

func New() *Pool {
	return &Pool{
		throttler: time.NewTicker(5 * time.Second).C,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *Pool) Name() string { return "pool.rplant.xyz" }

func (p *Pool) GetBlocks(token pool.Token) ([]pool.Block, pool.Token) {
	// no paging supported; always fetch recent list
	// Non-blocking throttle: don't wait on the first call.
	select {
	case <-p.throttler:
	default:
	}
	req, _ := http.NewRequest(http.MethodGet, "https://pool.rplant.xyz/api2/poolminer2/monero/0/0", nil)
	req.Header.Set("User-Agent", "monero-blocks/1.0")
	req.Header.Set("Accept", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil
	}

	var payload struct {
		Blocks []string `json:"blocks"`
	}

	if data, err := io.ReadAll(resp.Body); err != nil {
		return nil, nil
	} else {
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, nil
		}
	}

	if len(payload.Blocks) == 0 {
		return nil, nil
	}

	var blocks []pool.Block
	for _, rec := range payload.Blocks {
		parts := strings.Split(rec, ":")
		// Expected format (indices):
		// 0:hash 1:? 2:height 3:miner 4:timestamp 5:status 6:reward ...
		if len(parts) < 7 {
			continue
		}
		hash, err := pool.HashFromString(parts[0])
		if err != nil {
			continue
		}
		height, _ := strconv.ParseUint(parts[2], 10, 64)
		miner := parts[3]
		ts, _ := strconv.ParseUint(parts[4], 10, 64)
		status := strings.ToUpper(parts[5])
		reward, _ := strconv.ParseUint(parts[6], 10, 64)
		// Consider block invalid only if status mentions ORPHAN
		valid := !strings.Contains(status, "ORPHAN")

		blocks = append(blocks, pool.Block{
			Id:        hash,
			Height:    height,
			Reward:    reward,
			Timestamp: ts, // seconds
			Valid:     valid,
			Miner:     miner,
		})
	}

	if len(blocks) == 0 {
		return nil, nil
	}
	return blocks, nil
}
