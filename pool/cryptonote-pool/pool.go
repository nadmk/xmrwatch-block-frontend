package cryptonote_pool

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"monero-blocks/pool"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Pool struct {
	throttler <-chan time.Time
	name      string
	apiUrl    string
	kv        map[string]int
}

type pagingToken struct {
	height uint64
	id     pool.Hash
}

func New(apiUrl, name string, kv map[string]int) *Pool {
	if kv == nil {
		//default
		kv = map[string]int{
			"hash":     0,
			"ts":       1,
			"orphaned": 4,
			"reward":   5,
		}
	}
	return &Pool{
		throttler: time.Tick(time.Second * 5), //One request every five seconds
		name:      name,
		apiUrl:    apiUrl,
		kv:        kv,
	}
}

func (p *Pool) Name() string {
	return p.name
}

func (p *Pool) GetBlocks(token pool.Token) ([]pool.Block, pool.Token) {

	var t *pagingToken
	var ok bool

	var height uint64 = math.MaxInt32
	if t, ok = token.(*pagingToken); token != nil && ok {
		height = t.height
	} else {
		t = &pagingToken{}
	}

	<-p.throttler
	response, err := http.DefaultClient.Get(fmt.Sprintf(p.apiUrl+"/get_blocks?height=%d", height))
	if err != nil {
		return nil, nil
	}
	defer response.Body.Close()

	var blockData []string

	if data, err := io.ReadAll(response.Body); err != nil {
		return nil, nil
	} else {
		if err = json.Unmarshal(data, &blockData); err != nil || (len(blockData)%2 != 0) {
			return nil, nil
		}
	}

	var blocks []pool.Block

	for i := 0; i < len(blockData); i += 2 {
		pieces := strings.Split(blockData[i], ":")

		if len(pieces) < 4 {
			return nil, nil
		}

		g := func(n string) string {
			ii, ok := p.kv[n]
			if ok && ii != -1 && len(pieces) > ii {
				return pieces[ii]
			}
			return ""
		}

		var hash pool.Hash
		var miner string
		var ts, blockHeight, reward uint64
		var orphaned bool
		if v := g("hash"); v != "" {
			hash, err = pool.HashFromString(v)
			if err != nil {
				break
			}
		}
		if v := g("orphaned"); v != "" {
			orphaned = v != "0"
		}
		if v := g("ts"); v != "" {
			ts, _ = strconv.ParseUint(v, 10, 0)
		}
		if v := g("reward"); v != "" {
			reward, _ = strconv.ParseUint(v, 10, 0)
		}
		if v := g("miner"); v != "" {
			miner = v
		}
		blockHeight, _ = strconv.ParseUint(blockData[i+1], 10, 0)

		blocks = append(blocks, pool.Block{
			Id:     hash,
			Height: blockHeight,
			Reward: reward,
			// API returns seconds.
			Timestamp: ts,
			// orphaned true means not valid; Valid should be !orphaned
			Valid: !orphaned,
			Miner: miner,
		})
	}

	start := t.id == pool.ZeroHash
	for i, b := range blocks {
		if b.Height < t.height {
			start = true
		}
		if start {
			return blocks[i:], &pagingToken{
				id:     blocks[len(blocks)-1].Id,
				height: blocks[len(blocks)-1].Height,
			}
		}
		if b.Id == t.id {
			start = true
		}
	}

	if len(blocks) == 0 {
		return nil, nil
	}

	return nil, &pagingToken{
		id:     blocks[len(blocks)-1].Id,
		height: blocks[len(blocks)-1].Height,
	}
}
