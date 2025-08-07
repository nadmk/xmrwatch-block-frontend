package main

import (
	"encoding/csv"
	"errors"
	"flag"
	"io"
	"log"
	"monero-blocks/pool"
	c3pool_org "monero-blocks/pool/c3pool.org"
	cryptonote_pool "monero-blocks/pool/cryptonote-pool"
	kryptex_com "monero-blocks/pool/kryptex.com"
	monero_hashvault_pro "monero-blocks/pool/monero.hashvault.pro"
	moneroocean_stream "monero-blocks/pool/moneroocean.stream"
	nodejs_pool "monero-blocks/pool/nodejs-pool"
	"monero-blocks/pool/p2pool"
	supportxmr_com "monero-blocks/pool/supportxmr.com"
	xmr_nanopool_org "monero-blocks/pool/xmr.nanopool.org"
	xmrpool_eu "monero-blocks/pool/xmrpool.eu"
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
		supportxmr_com.New(),
		monero_hashvault_pro.New(),
		xmr_nanopool_org.New(),
		kryptex_com.New(),

		c3pool_org.New(),
		moneroocean_stream.New(),
		nodejs_pool.New("https://api.skypool.xyz", "skypool.xyz"),
		nodejs_pool.New("https://np-api.monerod.org", "monerod.org"),
		nodejs_pool.New("https://pool.xmr.pt/api", "pool.xmr.pt"),
		nodejs_pool.New("https://bohemianpool.com/api", "bohemianpool.com"),
		nodejs_pool.New("https://xmr.gntl.uk/api", "xmr.gntl.uk"),

		xmrpool_eu.New(),
		cryptonote_pool.New("https://monero.herominers.com/api", "monero.herominers.com"),
		cryptonote_pool.New("https://monerohash.com/api", "monerohash.com"),
		cryptonote_pool.New("https://fastpool.xyz/api-xmr", "fastpool.xyz"),
		cryptonote_pool.New("https://xmr.zeropool.io:8119", "xmr.zeropool.io"),
		cryptonote_pool.New("https://monero.fairhash.org/api", "monero.fairhash.org"),

		// TODO: pool.rplant.xyz
		// https://pool.rplant.xyz/api2/poolminer2/monero/0/0

		// TODO: xmr.solopool.org + miner attribution
		// https://xmr.solopool.org/api/blocks

		// TODO: mining-dutch.nl
		// https://www.mining-dutch.nl/pools/monero.php?page=api&action=getdashboarddata

		// TODO: zergpool.com
		// https://zergpool.com/api/blocks?pageIndex=0&pageSize=10&coin=XMR

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
				// "Height", "Id", "Timestamp", "Reward", "Pool", "Valid"
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

					allBlocks[i] = append(allBlocks[i], pool.Block{
						Height:    height,
						Id:        id,
						Timestamp: timestamp,
						Reward:    reward,
						Valid:     valid,
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
				for _, b := range tempBlocks {
					lastBlock = b.Height
					if b.Height < stopHeight {
						log.Printf("[%s] Finished: reached height %d\n", p.Name(), stopHeight)
						return
					}
					if slices.ContainsFunc(allBlocks[pIndex], func(p pool.Block) bool {
						return p.Id == b.Id
					}) {
						// already added
						continue
					}
					allBlocks[pIndex] = append(allBlocks[pIndex], b)
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

	csvFile.Write([]string{"Height", "Id", "Timestamp", "Reward", "Pool", "Valid"})

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

		if !*hideNotValid || allBlocks[smallIndex][0].Valid {
			csvFile.Write([]string{
				strconv.FormatUint(allBlocks[smallIndex][0].Height, 10),
				allBlocks[smallIndex][0].Id.String(),
				strconv.FormatUint(allBlocks[smallIndex][0].Timestamp, 10),
				strconv.FormatUint(allBlocks[smallIndex][0].Reward, 10),
				pools[smallIndex].Name(),
				strconv.FormatBool(allBlocks[smallIndex][0].Valid),
			})
		}

		allBlocks[smallIndex] = allBlocks[smallIndex][1:]
	}
	csvFile.Flush()

}
