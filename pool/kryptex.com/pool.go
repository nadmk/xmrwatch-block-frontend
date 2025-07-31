package kryptex_com

import (
	"encoding/json"
	"io"
	"monero-blocks/pool"
	"net/http"
	"time"
)

type Pool struct {
	throttler <-chan time.Time
}

type blockJson struct {
	Date   uint64 `json:"date,string"`
	Hash   string `json:"hash"`
	Height uint64 `json:"height"`
	Kind   string `json:"kind"`
}

func New() *Pool {
	return &Pool{
		throttler: time.Tick(time.Second * 5), //One request every five seconds
	}
}

func (p *Pool) Name() string {
	return "kryptex.com"
}

func (p *Pool) GetBlocks(token pool.Token) ([]pool.Block, pool.Token) {

	<-p.throttler
	response, err := http.DefaultClient.Get("https://pool.kryptex.com/xmr/api/v1/pool/stats")
	if err != nil {
		return nil, nil
	}
	defer response.Body.Close()

	var stats struct {
		LastBlocksFound []blockJson `json:"last_blocks_found"`
	}

	if data, err := io.ReadAll(response.Body); err != nil {
		return nil, nil
	} else {
		if err = json.Unmarshal(data, &stats); err != nil {
			return nil, nil
		}
	}

	var blocks []pool.Block

	for _, b := range stats.LastBlocksFound {
		if b.Kind == "BLOCK" {
			hash, err := pool.HashFromString(b.Hash)
			if err != nil {
				continue
			}
			blocks = append(blocks, pool.Block{
				Id:        hash,
				Height:    b.Height,
				Reward:    0,
				Timestamp: b.Date * 1000,
			})
		}
	}

	if len(blocks) == 0 {
		return nil, nil
	}

	return blocks, nil
}
