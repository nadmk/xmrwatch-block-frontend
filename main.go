package main

import (
	"encoding/csv"
	"errors"
	"flag"
	"io"
	"log"
	"monero-blocks/pool"
	c3pool_org "monero-blocks/pool/c3pool.org"
	monero_hashvault_pro "monero-blocks/pool/monero.hashvault.pro"
	moneroocean_stream "monero-blocks/pool/moneroocean.stream"
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

	flag.Parse()

	pools := []pool.Pool{
		supportxmr_com.New(),
		monero_hashvault_pro.New(),
		xmr_nanopool_org.New(),
		xmrpool_eu.New(),
		c3pool_org.New(),
		moneroocean_stream.New(),

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
				// "Height", "Id", "Timestamp", "Reward", "Pool"
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

					allBlocks[i] = append(allBlocks[i], pool.Block{
						Height:    height,
						Id:        id,
						Timestamp: timestamp,
						Reward:    reward,
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

	csvFile.Write([]string{"Height", "Id", "Timestamp", "Reward", "Pool"})

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
		csvFile.Write([]string{strconv.FormatUint(allBlocks[smallIndex][0].Height, 10), allBlocks[smallIndex][0].Id.String(), strconv.FormatUint(allBlocks[smallIndex][0].Timestamp, 10), strconv.FormatUint(allBlocks[smallIndex][0].Reward, 10), pools[smallIndex].Name()})
		allBlocks[smallIndex] = allBlocks[smallIndex][1:]
	}
	csvFile.Flush()

}
