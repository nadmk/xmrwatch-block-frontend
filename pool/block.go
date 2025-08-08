package pool

type Block struct {
	Id        Hash
	Height    uint64
	Reward    uint64
	Timestamp uint64
	Valid     bool
	Miner     string
}
