package main

import (
	"fmt"

	"github.com/devylongs/gean/consensus"
)

func main() {
	fmt.Println("Gean - Go Lean Ethereum Client")

	checkpoint := &consensus.Checkpoint{
		Root: consensus.Root{0xab, 0xcd},
		Slot: 100,
	}

	root, _ := checkpoint.HashTreeRoot()
	fmt.Printf("Checkpoint HashTreeRoot: %x...\n", root[:8])
}
