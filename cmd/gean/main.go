package main

import (
	"fmt"

	"github.com/developerlongs/gean/types"
)

func main() {
	fmt.Println("Gean - Go Lean Ethereum Client")

	checkpoint := &types.Checkpoint{
		Root: types.Root{0xab, 0xcd},
		Slot: 100,
	}

	root, _ := checkpoint.HashTreeRoot()
	fmt.Printf("Checkpoint HashTreeRoot: %x...\n", root[:8])
}
