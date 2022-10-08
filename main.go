package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/holiman/uint256"
	"github.com/protolambda/ztyp/view"

	"github.com/urfave/cli"
)

func main() {
	app := cli.NewApp()
	app.Commands = []cli.Command{
		{
			Name:   "tx",
			Usage:  "send a blob transaction",
			Action: TxApp,
			Flags:  TxFlags,
		},
		{
			Name:   "download",
			Usage:  "download blobs from the beacon net",
			Action: DownloadApp,
			Flags:  DownloadFlags,
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatalf("App failed: %v", err)
	}
}

func TxApp(cliCtx *cli.Context) error {
	addr := cliCtx.String(TxRPCURLFlag.Name)
	to := common.HexToAddress(cliCtx.String(TxToFlag.Name))
	prv := cliCtx.String(TxPrivateKeyFlag.Name)
	file := cliCtx.String(TxBlobFileFlag.Name)
	nonce := cliCtx.Int64(TxNonceFlag.Name)
	value := cliCtx.String(TxValueFlag.Name)
	gasLimit := cliCtx.Uint64(TxGasLimitFlag.Name)
	gasPrice := cliCtx.String(TxGasPriceFlag.Name)
	priorityGasPrice := cliCtx.String(TxPriorityGasPrice.Name)
	maxFeePerDataGas := cliCtx.String(TxMaxFeePerDataGas.Name)
	chainID := cliCtx.String(TxChainID.Name)

	value256, err := uint256.FromHex(value)
	if err != nil {
		return fmt.Errorf("invalid value param: %v", err)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("error reading blob file: %v", err)
	}

	chainId, _ := new(big.Int).SetString(chainID, 0)
	signer := types.NewDankSigner(chainId)

	ctx := context.Background()
	client, err := ethclient.DialContext(ctx, addr)
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	key, err := crypto.HexToECDSA(prv)
	if err != nil {
		return fmt.Errorf("%w: invalid private key", err)
	}

	if nonce == -1 {
		pendingNonce, err := client.PendingNonceAt(ctx, crypto.PubkeyToAddress(key.PublicKey))
		if err != nil {
			log.Fatalf("Error getting nonce: %v", err)
		}
		nonce = int64(pendingNonce)
	}

	var gasPrice256 *uint256.Int
	if gasPrice == "" {
		val, err := client.SuggestGasPrice(ctx)
		if err != nil {
			log.Fatalf("Error getting suggested gas price: %v", err)
		}
		var nok bool
		gasPrice256, nok = uint256.FromBig(val)
		if nok {
			log.Fatalf("gas price is too high! got %v", val.String())
		}
	} else {
		gasPrice256, err = DecodeUint256String(gasPrice)
		if err != nil {
			return fmt.Errorf("%w: invalid gas price", err)
		}
	}

	priorityGasPrice256 := gasPrice256
	if priorityGasPrice != "" {
		priorityGasPrice256, err = DecodeUint256String(priorityGasPrice)
		if err != nil {
			return fmt.Errorf("%w: invalid priority gas price", err)
		}
	}

	maxFeePerDataGas256, err := DecodeUint256String(maxFeePerDataGas)
	if err != nil {
		return fmt.Errorf("%w: invalid max_fee_per_data_gas", err)
	}

	blobs := EncodeBlobs(data)
	commitments, versionedHashes, aggregatedProof, err := blobs.ComputeCommitmentsAndAggregatedProof()
	if err != nil {
		log.Fatalf("failed to compute commitments: %v", err)
	}

	txData := types.SignedBlobTx{
		Message: types.BlobTxMessage{
			ChainID:             view.Uint256View(*uint256.NewInt(chainId.Uint64())),
			Nonce:               view.Uint64View(nonce),
			Gas:                 view.Uint64View(gasLimit),
			GasFeeCap:           view.Uint256View(*gasPrice256),
			GasTipCap:           view.Uint256View(*priorityGasPrice256),
			MaxFeePerDataGas:    view.Uint256View(*maxFeePerDataGas256), // needs to be at least the min fee
			Value:               view.Uint256View(*value256),
			To:                  types.AddressOptionalSSZ{Address: (*types.AddressSSZ)(&to)},
			BlobVersionedHashes: versionedHashes,
		},
	}

	wrapData := types.BlobTxWrapData{
		BlobKzgs:           commitments,
		Blobs:              blobs,
		KzgAggregatedProof: aggregatedProof,
	}
	tx := types.NewTx(&txData, types.WithTxWrapData(&wrapData))
	tx, err = types.SignTx(tx, signer, key)
	if err != nil {
		log.Fatalf("Error signing tx: %v", err)
	}

	err = client.SendTransaction(ctx, tx)
	if err != nil {
		return fmt.Errorf("%w: unable to send transaction", err)
	}

	log.Printf("Transaction submitted. nonce=%d hash=%v", nonce, tx.Hash())

	return nil
}
