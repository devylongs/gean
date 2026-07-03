package main

type options struct {
	Validators   int
	Nodes        int
	OutputDir    string
	BasePort     int
	GenesisTime  uint64
	GenesisDelay int
}

type manifest struct {
	Validators []validatorInfo `json:"validators"`
	Nodes      []nodeInfo      `json:"nodes"`
}

type validatorInfo struct {
	Index                int    `json:"index"`
	AttestationPubkeyHex string `json:"attestation_pubkey_hex"`
	ProposalPubkeyHex    string `json:"proposal_pubkey_hex"`
	AttestationSkFile    string `json:"attestation_sk_file"`
	ProposalSkFile       string `json:"proposal_sk_file"`
}

type nodeInfo struct {
	KeyFile string `json:"key_file"`
	PeerID  string `json:"peer_id"`
}
