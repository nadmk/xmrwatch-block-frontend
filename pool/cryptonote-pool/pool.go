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
}

type pagingToken struct {
	height uint64
	id     pool.Hash
}

type blockJson struct {
	Ts     uint64    `json:"ts"`
	Hash   pool.Hash `json:"hash"`
	Height uint64    `json:"height"`
	Valid  bool      `json:"valid"`
	Value  uint64    `json:"value"`
}

func New(apiUrl, name string) *Pool {
	return &Pool{
		throttler: time.Tick(time.Second * 5), //One request every five seconds
		name:      name,
		apiUrl:    apiUrl,
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
		if len(pieces) < 6 {
			break
		}
		hash, _ := pool.HashFromString(pieces[0])
		ts, _ := strconv.ParseUint(pieces[1], 10, 0)
		blockHeight, _ := strconv.ParseUint(blockData[i+1], 10, 0)
		orphaned := pieces[4] != "0"
		reward, _ := strconv.ParseUint(pieces[5], 10, 0)
		blocks = append(blocks, pool.Block{
			Id:        hash,
			Height:    blockHeight,
			Reward:    reward,
			Timestamp: ts * 1000,
			Valid:     orphaned,
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
