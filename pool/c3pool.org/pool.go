package c3pool_org

import (
	nodejs_pool "monero-blocks/pool/nodejs-pool"
)

func New() *nodejs_pool.Pool {
	return nodejs_pool.New("https://api.c3pool.org", "c3pool.org")
}
