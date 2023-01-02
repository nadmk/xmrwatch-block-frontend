package pool

type Pool interface {
	Name() string
	GetBlocks(token Token) ([]Block, Token)
}

// Token Used to pass paging information between calls
type Token any
