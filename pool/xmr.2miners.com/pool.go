package xmr_2miners_com

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

type blocksJson struct {
	Matured []blockJson `json:"matured"`
}

type blockJson struct {
	Ts     uint64    `json:"timestamp"`
	Hash   pool.Hash `json:"hash"`
	Height uint64    `json:"height"`
	Value  uint64    `json:"reward"`
}

func New() *Pool {
	return &Pool{
		throttler: time.Tick(time.Second * 5), //One request every five seconds
	}
}

func (p *Pool) Name() string {
	return "xmr.2miners.com"
}

// GetBlocks does not support paging
func (p *Pool) GetBlocks(token pool.Token) ([]pool.Block, pool.Token) {

	<-p.throttler
	response, err := http.DefaultClient.Get("https://xmr.2miners.com/api/blocks")
	if err != nil {
		return nil, nil
	}
	defer response.Body.Close()

	var blockData blocksJson

	if data, err := io.ReadAll(response.Body); err != nil {
		return nil, nil
	} else {
		if err = json.Unmarshal(data, &blockData); err != nil {
			return nil, nil
		}
	}

	var blocks []pool.Block

	for _, b := range blockData.Matured {
		blocks = append(blocks, pool.Block{
			Id:        b.Hash,
			Height:    b.Height,
			Reward:    b.Value,
			Timestamp: b.Ts * 1000,
		})
	}

	if len(blocks) == 0 {
		return nil, nil
	}

	return blocks, nil
}
