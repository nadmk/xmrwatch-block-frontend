package main

import (
	"encoding/csv"
	"flag"
	"log"
	"monero-blocks/pool"
	c3pool_org "monero-blocks/pool/c3pool.org"
	monero_hashvault_pro "monero-blocks/pool/monero.hashvault.pro"
	moneroocean_stream "monero-blocks/pool/moneroocean.stream"
	supportxmr_com "monero-blocks/pool/supportxmr.com"
	xmr_2miners_com "monero-blocks/pool/xmr.2miners.com"
	xmr_nanopool_org "monero-blocks/pool/xmr.nanopool.org"
	xmrpool_eu "monero-blocks/pool/xmrpool.eu"
	"os"
	"strconv"
	"sync"
)

func main() {
	scanDownToHeight := flag.Uint64("height", 2688888, "Height at which scans will stop from the tip. Defaults to v15 upgrade.")

	flag.Parse()

	pools := []pool.Pool{
		supportxmr_com.New(),
		monero_hashvault_pro.New(),
		xmr_nanopool_org.New(),
		xmr_2miners_com.New(),
		xmrpool_eu.New(),
		c3pool_org.New(),
		moneroocean_stream.New(),
	}

	var wg sync.WaitGroup

	allBlocks := make([][]pool.Block, len(pools))

	lowerHeight := *scanDownToHeight

	for i, p := range pools {
		wg.Add(1)
		go func(pIndex int, p pool.Pool) {
			defer wg.Done()
			var token pool.Token
			var tempBlocks []pool.Block
			var lastBlock uint64
			for {
				tempBlocks, token = p.GetBlocks(token)
				for _, b := range tempBlocks {
					if b.Height < lowerHeight {
						log.Printf("[%s] Finished: reached height\n", p.Name())
						return
					}
					allBlocks[pIndex] = append(allBlocks[pIndex], b)
					lastBlock = b.Height
				}
				log.Printf("[%s] at %d/%d\n", p.Name(), lastBlock, lowerHeight)
				if token == nil {
					log.Printf("[%s] Finished: no more blocks\n", p.Name())
					return
				}
			}
		}(i, p)
	}

	wg.Wait()

	f, err := os.Create("blocks.csv")
	if err != nil {
		log.Panic(err)
	}
	defer f.Close()
	csvFile := csv.NewWriter(f)
	defer csvFile.Flush()

	csvFile.Write([]string{"Height", "Id", "Timestamp", "Reward", "Pool"})
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

}
