package xmr_nanopool_org

import (
	"encoding/json"
	"fmt"
	"io"
	"monero-blocks/pool"
	"net/http"
	"time"
)

type Pool struct {
	throttler <-chan time.Time
}

type pagingToken struct {
	page   uint64
	id     pool.Hash
	height uint64
}

type blocksJson struct {
	Data []blockJson `json:"data"`
}

type blockJson struct {
	Ts     uint64    `json:"date"`
	Hash   pool.Hash `json:"hash"`
	Height uint64    `json:"block_number"`
	Status int       `json:"status"`
	Value  float64   `json:"value"`
	Miner  string    `json:"miner"`
}

func New() *Pool {
	return &Pool{
		throttler: time.Tick(time.Second * 5), //One request every five seconds
	}
}

func (p *Pool) Name() string {
	return "xmr.nanopool.org"
}

func (p *Pool) GetBlocks(token pool.Token) ([]pool.Block, pool.Token) {
	var t *pagingToken
	var ok bool

	var page uint64

	if t, ok = token.(*pagingToken); token != nil && ok {
		page = t.page
	} else {
		t = &pagingToken{}
	}

	<-p.throttler
	response, err := http.DefaultClient.Get(fmt.Sprintf("https://xmr.nanopool.org/api/v1/pool/blocks/%d/%d", page*500, 500))
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

	start := t.id == pool.ZeroHash
	for _, b := range blockData.Data {
		if b.Height < t.height {
			start = true
		}
		if start {
			blocks = append(blocks, pool.Block{
				Id:     b.Hash,
				Height: b.Height,
				Reward: uint64(b.Value * 1000000000000),
				// Nanopool 'date' is a unix timestamp in seconds already.
				Timestamp: b.Ts,
				Valid:     b.Status != 1,
				Miner:     b.Miner,
			})
		}
		if b.Hash == t.id {
			start = true
		}
	}

	if len(blocks) == 0 {
		return nil, nil
	}

	return blocks, &pagingToken{
		id:     blocks[len(blocks)-1].Id,
		page:   page + 1,
		height: blocks[len(blocks)-1].Height,
	}
}
