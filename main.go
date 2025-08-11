package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"monero-blocks/pool"
	cryptonote_pool "monero-blocks/pool/cryptonote-pool"
	kryptex_com "monero-blocks/pool/kryptex.com"
	monero_hashvault_pro "monero-blocks/pool/monero.hashvault.pro"
	nodejs_pool "monero-blocks/pool/nodejs-pool"
	"monero-blocks/pool/p2pool"
	rplant_xyz "monero-blocks/pool/rplant.xyz"
	xmr_nanopool_org "monero-blocks/pool/xmr.nanopool.org"
	xmr_solopool_org "monero-blocks/pool/xmr.solopool.org"
)

// appState holds blocks in-memory for API/server mode.
type appState struct {
	mu        sync.RWMutex
	pools     []pool.Pool
	allBlocks [][]pool.Block // per pool index, sorted desc by height
}

// normalizeTimestamp converts mixed timestamp units to seconds since epoch.
// Many upstream APIs return seconds, milliseconds, or microseconds. We standardize on seconds.
func normalizeTimestamp(ts uint64) uint64 {
	if ts == 0 {
		return 0
	}
	// microseconds (e.g., 1_700_000_000_000_000)
	if ts > 1_000_000_000_000_000 {
		return ts / 1_000_000
	}
	// milliseconds (e.g., 1_700_000_000_000)
	if ts > 1_000_000_000_000 {
		return ts / 1_000
	}
	// assume seconds already
	return ts
}

// findIndexBlock finds index of a block by predicate, returns -1 if not found.
func findIndexBlock(ss []pool.Block, pred func(pool.Block) bool) int {
	for i := range ss {
		if pred(ss[i]) {
			return i
		}
	}
	return -1
}

// latestCombined returns up to limit latest blocks across all pools, sorted by height desc.
// It deduplicates by height and fills missing heights with synthetic "Unknown" entries.
func (a *appState) latestCombined(limit int, onlyValid bool, since uint64) []map[string]any {
	a.mu.RLock()
	defer a.mu.RUnlock()
	// Establish the earliest height we reasonably know about across all pools
	minKnown := uint64(0)
	for i := range a.allBlocks {
		if len(a.allBlocks[i]) == 0 {
			continue
		}
		// find tail height (smallest) in this pool's slice (sorted desc)
		h := a.allBlocks[i][len(a.allBlocks[i])-1].Height
		if minKnown == 0 || h < minKnown {
			minKnown = h
		}
	}
	// Make a copy of first element iterators
	idx := make([]int, len(a.allBlocks))
	res := make([]map[string]any, 0, limit)
	heightsSeen := make(map[uint64]bool)
	var prevHeight uint64
	var havePrev bool
	for len(res) < limit {
		smallIndex := -1
		smallValue := uint64(0)
		for i := range a.allBlocks {
			if idx[i] < len(a.allBlocks[i]) {
				b := a.allBlocks[i][idx[i]]
				tnorm := normalizeTimestamp(b.Timestamp)
				// honor filters for choosing candidate
				if (since == 0 || tnorm >= since) && (!onlyValid || b.Valid) && b.Height >= smallValue {
					smallValue = b.Height
					smallIndex = i
				}
			}
		}
		if smallIndex == -1 {
			break
		}
		b := a.allBlocks[smallIndex][idx[smallIndex]]
		idx[smallIndex]++
		// Fill unknown gaps between prevHeight and current b.Height, but never below earliest known height
		if havePrev && prevHeight > b.Height+1 {
			for h := prevHeight - 1; h > b.Height && len(res) < limit; h-- {
				if minKnown != 0 && h < minKnown { // don't invent unknowns before we have any data
					break
				}
				if heightsSeen[h] {
					continue
				}
				// Try to enrich unknown via cached header if available in server mode
				var ts uint64
				var rw uint64
				// in server mode we may have a getHeader closure in scope; if not, leave zeros
				// NOTE: we cannot call it here directly; enrichment is also exposed via /api/block_header for the UI
				res = append(res, map[string]any{
					"height":    h,
					"id":        pool.ZeroHash,
					"timestamp": ts,
					"reward":    rw,
					"pool":      "Unknown",
					"valid":     true,
					"miner":     "",
				})
				heightsSeen[h] = true
			}
		}
		// Deduplicate per height
		if heightsSeen[b.Height] {
			// skip duplicates of same height
			continue
		}
		res = append(res, map[string]any{
			"height":    b.Height,
			"id":        b.Id,
			"timestamp": normalizeTimestamp(b.Timestamp),
			"reward":    b.Reward,
			"pool":      a.pools[smallIndex].Name(),
			"valid":     b.Valid,
			"miner":     b.Miner,
		})
		heightsSeen[b.Height] = true
		prevHeight = b.Height
		havePrev = true
	}
	return res
}

// ownership computes share of blocks per pool in the given window.
// If sinceUnix > 0, count blocks with timestamp >= sinceUnix. Otherwise, use lastN blocks.
// ownership computes share of blocks per pool in the given window, filling missing heights as "Unknown".
func (a *appState) ownership(lastN int, sinceUnix uint64, onlyValid bool) []map[string]any {
	a.mu.RLock()
	defer a.mu.RUnlock()
	type stat struct{ count int }
	stats := make([]stat, len(a.pools))
	unknown := 0
	total := 0
	// Earliest known height boundary across all pools
	minKnown := uint64(0)
	for i := range a.allBlocks {
		if len(a.allBlocks[i]) == 0 {
			continue
		}
		h := a.allBlocks[i][len(a.allBlocks[i])-1].Height
		if minKnown == 0 || h < minKnown {
			minKnown = h
		}
	}
	// Iterate across combined list but stop after lastN or time window
	idx := make([]int, len(a.allBlocks))
	heightsSeen := make(map[uint64]bool)
	var prevHeight uint64
	var havePrev bool
	for {
		smallIndex := -1
		smallValue := uint64(0)
		for i := range a.allBlocks {
			if idx[i] < len(a.allBlocks[i]) {
				b := a.allBlocks[i][idx[i]]
				tnorm := normalizeTimestamp(b.Timestamp)
				if (sinceUnix == 0 || tnorm >= sinceUnix) && (!onlyValid || b.Valid) && b.Height >= smallValue {
					smallValue = b.Height
					smallIndex = i
				}
			}
		}
		if smallIndex == -1 {
			break
		}
		b := a.allBlocks[smallIndex][idx[smallIndex]]
		idx[smallIndex]++
		// Fill unknown gaps conservatively:
		// - never below earliest known height across pools
		// - do not synthesize unknowns for time-window (sinceUnix>0) ownership queries
		if sinceUnix == 0 && havePrev && prevHeight > b.Height+1 {
			for h := prevHeight - 1; h > b.Height; h-- {
				if minKnown != 0 && h < minKnown {
					break
				}
				if heightsSeen[h] {
					continue
				}
				unknown++
				total++
				heightsSeen[h] = true
				if lastN > 0 && total >= lastN {
					break
				}
			}
			if lastN > 0 && total >= lastN {
				break
			}
		}
		if heightsSeen[b.Height] {
			// already counted this height
			continue
		}
		stats[smallIndex] = stat{count: stats[smallIndex].count + 1}
		total++
		heightsSeen[b.Height] = true
		prevHeight = b.Height
		havePrev = true
		if sinceUnix == 0 && lastN > 0 && total >= lastN {
			break
		}
	}

	out := make([]map[string]any, 0, len(a.pools))
	for i, p := range a.pools {
		cnt := stats[i].count
		if cnt == 0 {
			continue
		}
		out = append(out, map[string]any{
			"pool":       p.Name(),
			"count":      cnt,
			"percentage": float64(cnt) / float64(max(1, total)) * 100.0,
		})
	}
	if unknown > 0 {
		out = append(out, map[string]any{
			"pool":       "Unknown",
			"count":      unknown,
			"percentage": float64(unknown) / float64(max(1, total)) * 100.0,
		})
	}
	// Sort by count desc
	sort.Slice(out, func(i, j int) bool { return out[i]["count"].(int) > out[j]["count"].(int) })
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func main() {
	scanDownToHeight := flag.Uint64("height", 3475361, "Height at which scans will stop from the tip. Defaults to v15 upgrade.")
	csvOutput := flag.String("output", "blocks.csv", "CSV blocks output file")
	hideNotValid := flag.Bool("only-valid", false, "Do not output the blocks that are marked not valid by pools")
	serve := flag.Bool("serve", false, "Run HTTP server instead of writing CSV")
	addr := flag.String("addr", ":8080", "Address for HTTP server in serve mode")
	webDir := flag.String("web", "web/dist", "Directory from which to serve the frontend (in serve mode)")
	// TLS options
	tlsCert := flag.String("tls-cert", "", "Path to TLS certificate (PEM)")
	tlsKey := flag.String("tls-key", "", "Path to TLS private key (PEM)")
	tlsAddr := flag.String("tls-addr", ":443", "Address for HTTPS server (when --tls-cert and --tls-key are set)")
	httpRedirect := flag.Bool("http-redirect", false, "If true and TLS enabled, start an HTTP server on --addr that redirects to HTTPS")

	flag.Parse()

	pools := []pool.Pool{
		// custom implementations
		monero_hashvault_pro.New(),
		xmr_nanopool_org.New(),
		kryptex_com.New(),
		xmr_solopool_org.New(),

		// rplant.xyz
		rplant_xyz.New(),

		// TODO: mining-dutch.nl
		// https://www.mining-dutch.nl/pools/monero.php?page=api&action=getdashboarddata

		// TODO: zergpool.com
		// https://zergpool.com/api/blocks?pageIndex=0&pageSize=10&coin=XMR

		// TODO: dxpool.com
		// https://www.dxpool.com/api/pools/xmr/blocks?page_size=500&offset=0

		// nodejs-pool based ones
		nodejs_pool.New("https://supportxmr.com/api", "supportxmr.com"),
		nodejs_pool.New("https://api.c3pool.org", "c3pool.org"),
		nodejs_pool.New("https://api.moneroocean.stream", "moneroocean.stream"),
		nodejs_pool.New("https://api.skypool.xyz", "skypool.org"),
		nodejs_pool.New("https://np-api.monerod.org", "monerod.org"),
		nodejs_pool.New("https://pool.xmr.pt/api", "pool.xmr.pt"),
		nodejs_pool.New("https://bohemianpool.com/api", "bohemianpool.com"),
		nodejs_pool.New("https://xmr.gntl.uk/api", "xmr.gntl.uk"),

		// cryptonote-universal-pool based ones
		cryptonote_pool.New("https://web.xmrpool.eu:8119", "xmrpool.eu", nil),
		cryptonote_pool.New(
			"https://monero.herominers.com/api", "monero.herominers.com",
			map[string]int{"hash": 0, "ts": 1, "reward": 7, "miner": 8},
		),
		cryptonote_pool.New("https://monerohash.com/api", "monerohash.com", nil),
		cryptonote_pool.New("https://fastpool.xyz/api-xmr", "fastpool.xyz",
			map[string]int{"hash": 2, "ts": 3, "orphaned": 6, "reward": 7, "miner": 1},
		),
		cryptonote_pool.New("https://xmr.zeropool.io:8119", "xmr.zeropool.io",
			map[string]int{"hash": 2, "ts": 3, "orphaned": 6, "reward": 7, "miner": 1},
		),
		cryptonote_pool.New("https://monero.fairhash.org/api", "monero.fairhash.org", nil),

		// p2pool interfaces
		// main
		p2pool.New("https://p2pool.observer"),
		p2pool.New("https://old.p2pool.observer"),
		p2pool.New("https://old-old.p2pool.observer"),

		// mini
		p2pool.New("https://mini.p2pool.observer"),
		p2pool.New("https://old-mini.p2pool.observer"),

		// nano
		p2pool.New("https://nano.p2pool.observer"),
	}

	allBlocks := make([][]pool.Block, len(pools))

	if s, err := os.Stat(*csvOutput); err == nil && s.Size() > 0 {
		nameToIx := make(map[string]int)
		for i, p := range pools {
			nameToIx[p.Name()] = i
		}
		func() {
			f, err := os.Open(*csvOutput)
			if err != nil {
				return
			}
			defer f.Close()
			csvr := csv.NewReader(f)

			for {
				// "Height", "Id", "Timestamp", "Reward", "Pool", "Valid", "Miner"
				r, err := csvr.Read()
				if errors.Is(err, io.EOF) {
					break
				}

				if len(r) < 5 {
					continue
				}

				if i, ok := nameToIx[r[4]]; ok {
					height, err := strconv.ParseUint(r[0], 10, 64)
					if err != nil {
						continue
					}
					id, err := pool.HashFromString(r[1])
					if err != nil {
						continue
					}
					timestamp, err := strconv.ParseUint(r[2], 10, 64)
					if err != nil {
						continue
					}
					timestamp = normalizeTimestamp(timestamp)
					reward, err := strconv.ParseUint(r[3], 10, 64)
					if err != nil {
						continue
					}

					valid := true

					if len(r) > 5 {
						valid, _ = strconv.ParseBool(r[5])
					}

					miner := ""

					if len(r) > 6 {
						miner = r[6]
					}

					allBlocks[i] = append(allBlocks[i], pool.Block{
						Height:    height,
						Id:        id,
						Timestamp: timestamp,
						Reward:    reward,
						Valid:     valid,
						Miner:     miner,
					})
				}
			}

			for i := range allBlocks {
				sort.Slice(allBlocks[i], func(x, y int) bool { return allBlocks[i][x].Height > allBlocks[i][y].Height })
			}
		}()
	}

	// Shared fetch function usable for CSV mode.
	fetchAll := func(stopAtHeight uint64) {
		var wg sync.WaitGroup
		lowerHeight := stopAtHeight
		for i, p := range pools {
			wg.Add(1)
			go func(pIndex int, p pool.Pool) {
				defer wg.Done()
				var token pool.Token
				var tempBlocks []pool.Block
				var lastBlock uint64
				var stopHeight uint64
				if len(allBlocks[pIndex]) > 0 {
					// pick top block
					stopHeight = allBlocks[pIndex][0].Height
				} else {
					stopHeight = lowerHeight
				}
				for {
					tempBlocks, token = p.GetBlocks(token)
					var finished bool
					for _, b := range tempBlocks {
						lastBlock = b.Height
						if b.Height < stopHeight && !finished {
							log.Printf("[%s] Finished: reached height %d\n", p.Name(), stopHeight)
							finished = true
						}
						// normalize ts
						b.Timestamp = normalizeTimestamp(b.Timestamp)
						if ii := findIndexBlock(allBlocks[pIndex], func(p pool.Block) bool { return p.Id == b.Id }); ii != -1 {
							// already added
							allBlocks[pIndex][ii] = b
						} else {
							allBlocks[pIndex] = append(allBlocks[pIndex], b)
						}
					}
					if finished {
						return
					}
					log.Printf("[%s] at %d/%d\n", p.Name(), lastBlock, stopHeight)
					if token == nil {
						log.Printf("[%s] Finished: no more blocks\n", p.Name())
						return
					}
				}
			}(i, p)
		}
		wg.Wait()
	}

	if *serve {
		// State for server mode
		state := &appState{pools: pools, allBlocks: make([][]pool.Block, len(pools))}

		// Header cache for unknown blocks enrichment
		type headerItem struct {
			ts      uint64
			reward  uint64
			hash    string
			fetched time.Time
		}
		var headerMu sync.RWMutex
		headers := make(map[uint64]headerItem)
		httpClient := &http.Client{Timeout: 8 * time.Second}
		getHeader := func(height uint64) (uint64, uint64, string, error) {
			headerMu.RLock()
			if it, ok := headers[height]; ok {
				headerMu.RUnlock()
				return it.ts, it.reward, it.hash, nil
			}
			headerMu.RUnlock()
			url := fmt.Sprintf("https://localmonero.co/blocks/api/get_block_header/%d", height)
			req, _ := http.NewRequest(http.MethodGet, url, nil)
			req.Header.Set("User-Agent", "monero-blocks/serve")
			resp, err := httpClient.Do(req)
			if err != nil {
				return 0, 0, "", err
			}
			defer resp.Body.Close()
			var j struct {
				BlockHeader struct {
					Height    uint64 `json:"height"`
					Timestamp uint64 `json:"timestamp"`
					Reward    uint64 `json:"reward"`
					Hash      string `json:"hash"`
				} `json:"block_header"`
				Status string `json:"status"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
				return 0, 0, "", err
			}
			if j.Status != "OK" {
				return 0, 0, "", fmt.Errorf("bad status: %s", j.Status)
			}
			headerMu.Lock()
			headers[height] = headerItem{ts: j.BlockHeader.Timestamp, reward: j.BlockHeader.Reward, hash: j.BlockHeader.Hash, fetched: time.Now()}
			headerMu.Unlock()
			return j.BlockHeader.Timestamp, j.BlockHeader.Reward, j.BlockHeader.Hash, nil
		}

		// Preload from CSV if present
		if s, err := os.Stat(*csvOutput); err == nil && s.Size() > 0 {
			nameToIx := make(map[string]int)
			for i, p := range pools {
				nameToIx[p.Name()] = i
			}
			func() {
				f, err := os.Open(*csvOutput)
				if err != nil {
					return
				}
				defer f.Close()
				csvr := csv.NewReader(f)
				for {
					r, err := csvr.Read()
					if errors.Is(err, io.EOF) {
						break
					}
					if len(r) < 5 {
						continue
					}
					if i, ok := nameToIx[r[4]]; ok {
						height, err := strconv.ParseUint(r[0], 10, 64)
						if err != nil {
							continue
						}
						id, err := pool.HashFromString(r[1])
						if err != nil {
							continue
						}
						timestamp, err := strconv.ParseUint(r[2], 10, 64)
						if err != nil {
							continue
						}
						reward, err := strconv.ParseUint(r[3], 10, 64)
						if err != nil {
							continue
						}
						valid := true
						if len(r) > 5 {
							valid, _ = strconv.ParseBool(r[5])
						}
						miner := ""
						if len(r) > 6 {
							miner = r[6]
						}
						state.mu.Lock()
						state.allBlocks[i] = append(state.allBlocks[i], pool.Block{Height: height, Id: id, Timestamp: timestamp, Reward: reward, Valid: valid, Miner: miner})
						state.mu.Unlock()
					}
				}
				state.mu.Lock()
				for i := range state.allBlocks {
					sort.Slice(state.allBlocks[i], func(x, y int) bool { return state.allBlocks[i][x].Height > state.allBlocks[i][y].Height })
				}
				state.mu.Unlock()
			}()
		}

		// Serve-mode fetch with locking
		fetchAllServe := func(stopAtHeight uint64) {
			var wg sync.WaitGroup
			lowerHeight := stopAtHeight
			for i, p := range pools {
				wg.Add(1)
				go func(pIndex int, p pool.Pool) {
					defer wg.Done()
					var token pool.Token
					var tempBlocks []pool.Block
					var lastBlock uint64
					var stopHeight uint64
					state.mu.RLock()
					if len(state.allBlocks[pIndex]) > 0 {
						stopHeight = state.allBlocks[pIndex][0].Height
					} else {
						stopHeight = lowerHeight
					}
					state.mu.RUnlock()
					for {
						tempBlocks, token = p.GetBlocks(token)
						var finished bool
						for _, b := range tempBlocks {
							lastBlock = b.Height
							if b.Height < stopHeight && !finished {
								log.Printf("[%s] Finished: reached height %d\n", p.Name(), stopHeight)
								finished = true
							}
							// normalize timestamp before storing
							b.Timestamp = normalizeTimestamp(b.Timestamp)
							state.mu.Lock()
							if ii := findIndexBlock(state.allBlocks[pIndex], func(p pool.Block) bool { return p.Id == b.Id }); ii != -1 {
								state.allBlocks[pIndex][ii] = b
							} else {
								state.allBlocks[pIndex] = append(state.allBlocks[pIndex], b)
							}
							state.mu.Unlock()
						}
						if finished {
							return
						}
						log.Printf("[%s] at %d/%d\n", p.Name(), lastBlock, stopHeight)
						if token == nil {
							log.Printf("[%s] Finished: no more blocks\n", p.Name())
							return
						}
					}
				}(i, p)
			}
			wg.Wait()
			state.mu.Lock()
			for i := range state.allBlocks {
				sort.Slice(state.allBlocks[i], func(x, y int) bool { return state.allBlocks[i][x].Height > state.allBlocks[i][y].Height })
			}
			state.mu.Unlock()
		}

		// Initial fetch down to desired height
		fetchAllServe(*scanDownToHeight)

		mux := http.NewServeMux()

		// CORS middleware wrapper for dev convenience
		withCORS := func(h http.HandlerFunc) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Access-Control-Allow-Origin", "*")
				if r.Method == http.MethodOptions {
					w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
					w.WriteHeader(http.StatusNoContent)
					return
				}
				h(w, r)
			}
		}

		mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"ok"}`))
		})

		mux.HandleFunc("/api/pools", withCORS(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			names := make([]string, len(pools))
			for i, p := range pools {
				names[i] = p.Name()
			}
			json.NewEncoder(w).Encode(map[string]any{"pools": names})
		}))

		mux.HandleFunc("/api/blocks", withCORS(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			limit := 200
			if v := r.URL.Query().Get("limit"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 10000 {
					limit = n
				}
			}
			onlyValid := r.URL.Query().Get("onlyValid") == "true"
			var since uint64
			if v := r.URL.Query().Get("since"); v != "" {
				if n, err := strconv.ParseUint(v, 10, 64); err == nil {
					since = n
				}
			}
			out := state.latestCombined(limit, onlyValid, since)
			json.NewEncoder(w).Encode(map[string]any{"blocks": out})
		}))

		mux.HandleFunc("/api/ownership", withCORS(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			lastN := 1000
			if v := r.URL.Query().Get("lastN"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100000 {
					lastN = n
				}
			}
			var since uint64
			if v := r.URL.Query().Get("since"); v != "" {
				if n, err := strconv.ParseUint(v, 10, 64); err == nil {
					since = n
				}
			}
			onlyValid := r.URL.Query().Get("onlyValid") == "true"
			out := state.ownership(lastN, since, onlyValid)
			json.NewEncoder(w).Encode(map[string]any{"ownership": out})
		}))

		// Fetch minimal block header for a specific height (used to enrich unknown blocks)
		mux.HandleFunc("/api/block_header", withCORS(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			v := r.URL.Query().Get("height")
			if v == "" {
				http.Error(w, `{"error":"height required"}`, http.StatusBadRequest)
				return
			}
			h, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				http.Error(w, `{"error":"invalid height"}`, http.StatusBadRequest)
				return
			}
			ts, rew, hash, err := getHeader(h)
			if err != nil {
				w.WriteHeader(http.StatusBadGateway)
				json.NewEncoder(w).Encode(map[string]any{"status": "error", "height": h, "error": err.Error()})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"status":    "OK",
				"height":    h,
				"timestamp": ts,
				"reward":    rew,
				"hash":      hash,
			})
		}))

		// Static files (frontend build)
		// Resolve absolute path for clarity
		absWeb := *webDir
		if !filepath.IsAbs(absWeb) {
			cwd, _ := os.Getwd()
			absWeb = filepath.Join(cwd, absWeb)
		}
		fs := http.FileServer(http.Dir(absWeb))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// Serve index.html for SPA routes
			path := filepath.Join(absWeb, filepath.Clean(r.URL.Path))
			if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
				fs.ServeHTTP(w, r)
				return
			}
			http.ServeFile(w, r, filepath.Join(absWeb, "index.html"))
		})

		// Background light refresh: periodically try to get more blocks at the tip
		go func() {
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				log.Printf("Refreshing latest blocks...")
				fetchAllServe(*scanDownToHeight)
			}
		}()

		// Start HTTPS if cert/key provided, otherwise HTTP only
		if *tlsCert != "" && *tlsKey != "" {
			if *httpRedirect {
				go func() {
					redir := http.NewServeMux()
					redir.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
						// Build https URL preserving host and path
						target := "https://" + r.Host + r.URL.RequestURI()
						http.Redirect(w, r, target, http.StatusMovedPermanently)
					})
					log.Printf("HTTP redirect listening on %s -> %s", *addr, *tlsAddr)
					if err := http.ListenAndServe(*addr, redir); err != nil {
						log.Printf("HTTP redirect server stopped: %v", err)
					}
				}()
			}
			log.Printf("Serving HTTPS on %s (frontend: %s)", *tlsAddr, absWeb)
			if err := http.ListenAndServeTLS(*tlsAddr, *tlsCert, *tlsKey, mux); err != nil {
				log.Fatal(err)
			}
		} else {
			log.Printf("Serving HTTP on %s (frontend: %s)", *addr, absWeb)
			if err := http.ListenAndServe(*addr, mux); err != nil {
				log.Fatal(err)
			}
		}
		return
	}

	// CSV mode (default)
	fetchAll(*scanDownToHeight)

	f, err := os.Create(*csvOutput)
	if err != nil {
		log.Panic(err)
	}
	defer f.Close()
	csvFile := csv.NewWriter(f)
	defer csvFile.Flush()

	csvFile.Write([]string{"Height", "Id", "Timestamp", "Reward", "Pool", "Valid", "Miner"})

	for i := range allBlocks {
		sort.Slice(allBlocks[i], func(x, y int) bool { return allBlocks[i][x].Height > allBlocks[i][y].Height })
	}

	for {
		smallIndex := -1
		smallValue := uint64(0)
		for i, s := range allBlocks {
			if len(s) > 0 {
				if s[0].Height >= smallValue {
					smallValue = s[0].Height
					smallIndex = i
				}
			}
		}
		if smallIndex == -1 {
			break
		}

		b := allBlocks[smallIndex][0]

		if !*hideNotValid || b.Valid {
			csvFile.Write([]string{
				strconv.FormatUint(b.Height, 10),
				b.Id.String(),
				strconv.FormatUint(b.Timestamp, 10),
				strconv.FormatUint(b.Reward, 10),
				pools[smallIndex].Name(),
				strconv.FormatBool(b.Valid),
				b.Miner,
			})
		}

		allBlocks[smallIndex] = allBlocks[smallIndex][1:]
	}
	csvFile.Flush()

}
