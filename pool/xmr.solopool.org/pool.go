package xmr_solopool_org

import (
	"encoding/json"
	"io"
	"monero-blocks/pool"
	"net/http"
	"sort"
	"time"
)

type Pool struct {
	throttler <-chan time.Time
}

type blocksJson struct {
	Matured    []blockJson `json:"matured"`
	Immatured  []blockJson `json:"immatured"`
	Candidates []blockJson `json:"candidates"`
}

type blockJson struct {
	Ts       uint64    `json:"timestamp"`
	Hash     pool.Hash `json:"hash"`
	Height   uint64    `json:"height"`
	Orphaned bool      `json:"orphan"`
	Value    uint64    `json:"reward,string"`
	Miner    string    `json:"miner"`
}

func New() *Pool {
	return &Pool{
		throttler: time.Tick(time.Second * 5), //One request every five seconds
	}
}

func (p *Pool) Name() string {
	return "xmr.solopool.org"
}

func (p *Pool) GetBlocks(token pool.Token) ([]pool.Block, pool.Token) {

	<-p.throttler
	response, err := http.DefaultClient.Get("https://xmr.solopool.org/api/blocks")
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

	appendB := func(b blockJson) {
		blocks = append(blocks, pool.Block{
			Id:        b.Hash,
			Height:    b.Height,
			Reward:    b.Value / 1000000,
			// API returns seconds; keep seconds
			Timestamp: b.Ts,
			Valid:     !b.Orphaned,
			Miner:     b.Miner,
		})
	}

	for _, b := range blockData.Candidates {
		appendB(b)
	}

	for _, b := range blockData.Immatured {
		appendB(b)
	}

	for _, b := range blockData.Matured {
		appendB(b)
	}

	if len(blocks) == 0 {
		return nil, nil
	}

	sort.Slice(blocks, func(i, j int) bool { return blocks[i].Height > blocks[j].Height })

	return blocks, nil
}
