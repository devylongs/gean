package specfixtures

import (
	"fmt"

	"github.com/geanlabs/gean/internal/types"
)

func (sb *FixtureSignedBlock) ToSignedBlock() (*types.SignedBlock, error) {
	if sb == nil {
		return nil, fmt.Errorf("signed block fixture is nil")
	}
	block, err := sb.Block.ToBlock()
	if err != nil {
		return nil, fmt.Errorf("signedBlock.block: %w", err)
	}

	proof, err := ParseHexBytes(sb.Proof.Proof.Data)
	if err != nil {
		return nil, fmt.Errorf("signedBlock.proof.proof: %w", err)
	}

	return &types.SignedBlock{
		Block: block,
		Proof: &types.MultiMessageAggregate{Proof: proof},
	}, nil
}
