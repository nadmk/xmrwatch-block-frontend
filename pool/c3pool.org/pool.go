package c3pool_org

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

type blockJson struct {
	Ts     uint64    `json:"ts"`
	Hash   pool.Hash `json:"hash"`
	Height uint64    `json:"height"`
	Valid  bool      `json:"valid"`
	Value  uint64    `json:"value"`
}

func New() *Pool {
	return &Pool{
		throttler: time.Tick(time.Second * 5), //One request every five seconds
	}
}

func (p *Pool) Name() string {
	return "c3pool.org"
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
	response, err := http.DefaultClient.Get(fmt.Sprintf("https://api.c3pool.org/pool/blocks?page=%d&limit=9999", page))
	if err != nil {
		return nil, nil
	}
	defer response.Body.Close()

	blockData := make([]blockJson, 0, 9999)

	if data, err := io.ReadAll(response.Body); err != nil {
		return nil, nil
	} else {
		if err = json.Unmarshal(data, &blockData); err != nil {
			return nil, nil
		}
	}

	var blocks []pool.Block

	start := t.id == pool.ZeroHash
	for _, b := range blockData {
		if b.Height < t.height {
			start = true
		}
		if start {
			blocks = append(blocks, pool.Block{
				Id:        b.Hash,
				Height:    b.Height,
				Reward:    b.Value,
				Timestamp: b.Ts,
				Valid:     b.Valid,
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
