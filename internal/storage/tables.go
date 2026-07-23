package storage

type Table string

const (
	TableBlockHeaders Table = "block_headers"
	TableBlockBodies  Table = "block_bodies"
	TableSignedBlocks Table = "signed_blocks"
	TableStates       Table = "states"
	TableMetadata     Table = "metadata"
	TableLiveChain    Table = "live_chain"
)

var AllTables = []Table{
	TableBlockHeaders,
	TableBlockBodies,
	TableSignedBlocks,
	TableStates,
	TableMetadata,
	TableLiveChain,
}
