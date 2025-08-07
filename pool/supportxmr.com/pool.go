package supportxmr_com

import (
	nodejs_pool "monero-blocks/pool/nodejs-pool"
)

func New() *nodejs_pool.Pool {
	return nodejs_pool.New("https://supportxmr.com/api", "supportxmr.com")
}
