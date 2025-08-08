package main

import (
	"encoding/csv"
	"errors"
	"flag"
	"io"
	"log"
	"monero-blocks/pool"
	cryptonote_pool "monero-blocks/pool/cryptonote-pool"
	kryptex_com "monero-blocks/pool/kryptex.com"
	monero_hashvault_pro "monero-blocks/pool/monero.hashvault.pro"
	nodejs_pool "monero-blocks/pool/nodejs-pool"
	"monero-blocks/pool/p2pool"
	xmr_nanopool_org "monero-blocks/pool/xmr.nanopool.org"
	"os"
	"slices"
	"strconv"
	"sync"
)

func main() {
	scanDownToHeight := flag.Uint64("height", 2688888, "Height at which scans will stop from the tip. Defaults to v15 upgrade.")
	csvOutput := flag.String("output", "blocks.csv", "CSV blocks output file")
	hideNotValid := flag.Bool("only-valid", false, "Do not output the blocks that are marked not valid by pools")

	flag.Parse()

	pools := []pool.Pool{
		monero_hashvault_pro.New(),
		xmr_nanopool_org.New(),
		kryptex_com.New(),

		// nodejs-pool based ones
		nodejs_pool.New("https://supportxmr.com/api", "supportxmr.com"),
		nodejs_pool.New("https://api.c3pool.org", "c3pool.org"),
		nodejs_pool.New("https://api.moneroocean.stream", "moneroocean.stream"),
		nodejs_pool.New("https://api.skypool.xyz", "skypool.xyz"),
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

		// TODO: pool.rplant.xyz
		// https://pool.rplant.xyz/api2/poolminer2/monero/0/0

		// TODO: xmr.solopool.org + miner attribution
		// https://xmr.solopool.org/api/blocks

		// TODO: mining-dutch.nl
		// https://www.mining-dutch.nl/pools/monero.php?page=api&action=getdashboarddata

		// TODO: zergpool.com
		// https://zergpool.com/api/blocks?pageIndex=0&pageSize=10&coin=XMR

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
				slices.SortFunc(allBlocks[i], func(a, b pool.Block) int {
					return int(b.Height) - int(a.Height)
				})
			}
		}()
	}

	var wg sync.WaitGroup

	lowerHeight := *scanDownToHeight

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
					if ii := slices.IndexFunc(allBlocks[pIndex], func(p pool.Block) bool {
						return p.Id == b.Id
					}); ii != -1 {
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

	f, err := os.Create(*csvOutput)
	if err != nil {
		log.Panic(err)
	}
	defer f.Close()
	csvFile := csv.NewWriter(f)
	defer csvFile.Flush()

	csvFile.Write([]string{"Height", "Id", "Timestamp", "Reward", "Pool", "Valid", "Miner"})

	for i := range allBlocks {
		slices.SortFunc(allBlocks[i], func(a, b pool.Block) int {
			return int(b.Height) - int(a.Height)
		})
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
