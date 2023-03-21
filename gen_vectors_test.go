package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"math/rand"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
	"github.com/protolambda/ztyp/view"
	"github.com/stretchr/testify/assert"
)

type TestCaseInput struct {
	PrivateKey       string
	To               string
	Nonce            int64
	Value            string
	GasLimit         int64
	GasPrice         string
	PriorityGasPrice string
	MaxFeePerDataGas string
	Data             string
}

type TestCaseOutput struct {
	testCase        TestCaseInput
	blobs           types.Blobs
	commitments     []types.KZGCommitment
	versionedHashes []common.Hash
	proofs          []types.KZGProof
	txData          types.SignedBlobTx
	wrapData        types.BlobTxWrapData
	transaction     *types.Transaction
}

type TestCaseResult struct {
	Input                 TestCaseInput
	RawEncodedTransaction string
}

func TestGenerateTestVectors(t *testing.T) {
	fmt.Println("Generating test vectors...")

	chainId := big.NewInt(1331)
	signer := types.NewDankSigner(chainId)

	data8Bytes := make([]byte, 8)
	data128Bytes := make([]byte, 128)
	data256Bytes := make([]byte, 256)
	data1024Bytes := make([]byte, 1024)
	rand.Read(data8Bytes)
	rand.Read(data128Bytes)
	rand.Read(data256Bytes)
	rand.Read(data1024Bytes)

	// Define test cases
	testCaseInputs := []TestCaseInput{
		{"aa3a09289747a62b7b8190af3a75544cbe8c3b4a58f7b11d8d3b12ad17300a59", "0x45Ae5777c9b35Eb16280e423b0d7c91C06C66B58", 1, "1", int64(100000), "1000", "50", "100", hexutil.Encode(data8Bytes)},
		{"67f45650acc5dc426fc424348f8d9f07032c439f03797c4beba096a6e23e666b", "0x549A51956bd364D8bB2Efb1F1eA4436e8D7764Ff", 2, "1", int64(50000), "1234", "100", "200", hexutil.Encode(data128Bytes)},
		{"2e2b5c749eab38b8eca6b788c70429580ffa8eb79ab9635b95af00c6f6cba661", "0xa39c4e1B259473fbcC5213a0613eB53a8C50bf76", 3, "1", int64(70000), "999", "10", "20", hexutil.Encode(data256Bytes)},
		{"ee15ba623c2a495eefc9b8dc7447ff70bee325cc1f75f5170a71dc4dd3227f13", "0xd59399657A78bb69dEE83C416C13Be711e02fA23", 4, "1", int64(21000), "1001", "35", "70", hexutil.Encode(data1024Bytes)},
	}

	summary := []TestCaseResult{}

	for _, testCaseInput := range testCaseInputs {
		testCaseOutput, err := generateTestVector(chainId, signer, testCaseInput)
		assert.Nil(t, err)
		txAsRawData, _ := testCaseOutput.transaction.MarshalBinary()
		txRawHex := hexutil.Encode(txAsRawData)
		result := TestCaseResult{testCaseOutput.testCase, txRawHex}
		summary = append(summary, result)
	}

	// Generate JSON summary
	summaryAsJson, _ := json.MarshalIndent(summary, "", "\t")
	// Write summary in output file
	outputFilePath := "/tmp/eip4844_test_vectors.json"
	outputFile, err := os.Create(outputFilePath)
	assert.Nil(t, err)
	outputFile.WriteString(string(summaryAsJson))
}

func generateTestVector(chainId *big.Int, signer types.Signer, testCaseInput TestCaseInput) (*TestCaseOutput, error) {
	blobs := EncodeBlobs(hexutil.MustDecode(testCaseInput.Data))
	commitments, versionedHashes, proofs, err := blobs.ComputeCommitmentsAndProofs()
	if err != nil {
		log.Fatalf("failed to compute commitments: %v", err)
	}

	// Encode fields
	gasPrice256, _ := DecodeUint256String(testCaseInput.GasPrice)
	priorityGasPrice256, _ := DecodeUint256String(testCaseInput.PriorityGasPrice)
	maxFeePerDataGas256, _ := DecodeUint256String(testCaseInput.MaxFeePerDataGas)
	value256, _ := DecodeUint256String(testCaseInput.Value)
	to := common.HexToAddress(testCaseInput.To)

	// Generate signed blob transaction
	txData := types.SignedBlobTx{
		Message: types.BlobTxMessage{
			ChainID:             view.Uint256View(*uint256.NewInt(chainId.Uint64())),
			Nonce:               view.Uint64View(testCaseInput.Nonce),
			Gas:                 view.Uint64View(testCaseInput.GasLimit),
			GasFeeCap:           view.Uint256View(*gasPrice256),
			GasTipCap:           view.Uint256View(*priorityGasPrice256),
			MaxFeePerDataGas:    view.Uint256View(*maxFeePerDataGas256),
			Value:               view.Uint256View(*value256),
			To:                  types.AddressOptionalSSZ{Address: (*types.AddressSSZ)(&to)},
			BlobVersionedHashes: versionedHashes,
		},
	}

	// Generate wrap transaction data
	wrapData := types.BlobTxWrapData{
		BlobKzgs: commitments,
		Blobs:    blobs,
		Proofs:   proofs,
	}

	// Generate key from private key string
	key, err := crypto.HexToECDSA(testCaseInput.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid private key", err)
	}

	tx := types.NewTx(&txData, types.WithTxWrapData(&wrapData))

	tx, err = types.SignTx(tx, signer, key)
	if err != nil {
		log.Fatalf("Error signing tx: %v", err)
	}
	testCaseOutput := &TestCaseOutput{
		testCase:        testCaseInput,
		blobs:           blobs,
		commitments:     commitments,
		versionedHashes: versionedHashes,
		proofs:          proofs,
		txData:          txData,
		wrapData:        wrapData,
		transaction:     tx,
	}
	return testCaseOutput, nil
}
