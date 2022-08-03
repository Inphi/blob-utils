package main

import (
	"github.com/urfave/cli"
)

var (
	TxRPCURLFlag = cli.StringFlag{
		Name:  "rpc-url",
		Usage: "Address of exuection node JSON-RPC endpoint",
		Value: "http://127.0.0.1:8545",
	}
	TxBlobFileFlag = cli.StringFlag{
		Name:     "blob-file",
		Usage:    "Blob file data",
		Required: true,
	}
	TxToFlag = cli.StringFlag{
		Name:     "to",
		Usage:    "tx to address",
		Required: true,
	}
	TxValueFlag = cli.StringFlag{
		Name:  "value",
		Usage: "tx value (wei deonominated)",
		Value: "0x0",
	}
	TxPrivateKeyFlag = cli.StringFlag{
		Name:     "private-key",
		Usage:    "tx private key",
		Required: true,
	}
	TxNonceFlag = cli.Int64Flag{
		Name:  "nonce",
		Usage: "tx nonce",
		Value: -1,
	}
	TxGasLimitFlag = cli.Uint64Flag{
		Name:  "gas-limit",
		Usage: "tx gas limit",
		Value: 21000,
	}
	TxGasPriceFlag = cli.StringFlag{
		Name:  "gas-price",
		Usage: "sets the tx max_fee_per_gas",
	}
	TxPriorityGasPrice = cli.StringFlag{
		Name:  "priority-gas-price",
		Usage: "Sets the priority fee per gas",
		Value: "2000000000",
	}

	DownloadBeaconP2PAddr = cli.StringFlag{
		Name:  "beacon-p2p-addr",
		Usage: "P2P multiaddr of the beacon node",
		Value: "/ip4/127.0.0.1/tcp/13000",
	}
	DownloadSlotFlag = cli.Int64Flag{
		Name:     "slot",
		Usage:    "Slot to download blob from",
		Required: true,
	}
)

var TxFlags = []cli.Flag{
	TxRPCURLFlag,
	TxBlobFileFlag,
	TxToFlag,
	TxValueFlag,
	TxPrivateKeyFlag,
	TxNonceFlag,
	TxGasLimitFlag,
	TxGasPriceFlag,
	TxPriorityGasPrice,
}

var DownloadFlags = []cli.Flag{
	DownloadBeaconP2PAddr,
	DownloadSlotFlag,
}
