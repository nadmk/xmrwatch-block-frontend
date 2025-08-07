package xmrpool_eu

import (
	cryptonote_pool "monero-blocks/pool/cryptonote-pool"
)

func New() *cryptonote_pool.Pool {
	return cryptonote_pool.New("https://web.xmrpool.eu:8119", "xmrpool.eu")
}
