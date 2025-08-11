package p2pool

import (
	"encoding/json"
	"io"
	"monero-blocks/pool"
	"net/http"
	"net/url"
	"time"
)

type Pool struct {
	observerUrl string
	throttler   <-chan time.Time
}

type blockJson struct {
	MainBlock struct {
		Height    uint64    `json:"height"`
		Id        pool.Hash `json:"id"`
		Timestamp uint64    `json:"timestamp"`
		Reward    uint64    `json:"reward"`
	} `json:"main_block"`
	MinerAddress string `json:"miner_address"`
}

func New(observerUrl string) *Pool {
	return &Pool{
		observerUrl: observerUrl,
		throttler:   time.Tick(time.Second * 5), //One request every five seconds
	}
}

func (p *Pool) Name() string {
	u, err := url.Parse(p.observerUrl)
	if err != nil {
		return ""
	}
	return u.Host
}

func (p *Pool) GetBlocks(token pool.Token) ([]pool.Block, pool.Token) {

	<-p.throttler
	response, err := http.DefaultClient.Get(p.observerUrl + "/api/found_blocks?limit=1000")
	if err != nil {
		return nil, nil
	}
	defer response.Body.Close()

	blockData := make([]blockJson, 0, 1000)

	if data, err := io.ReadAll(response.Body); err != nil {
		return nil, nil
	} else {
		if err = json.Unmarshal(data, &blockData); err != nil {
			return nil, nil
		}
	}

	var blocks []pool.Block

	for _, b := range blockData {
		blocks = append(blocks, pool.Block{
			Id:     b.MainBlock.Id,
			Height: b.MainBlock.Height,
			Reward: b.MainBlock.Reward,
			// p2pool observer returns seconds.
			Timestamp: b.MainBlock.Timestamp,
			Valid:     true,
			Miner:     b.MinerAddress,
		})
	}

	if len(blocks) == 0 {
		return nil, nil
	}

	return blocks, nil
}
