package moneroocean_stream

import (
	nodejs_pool "monero-blocks/pool/nodejs-pool"
)

func New() *nodejs_pool.Pool {
	return nodejs_pool.New("https://api.moneroocean.stream", "moneroocean.stream")
}
