package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/big"
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
	Data             string
}

type TestCaseOutput struct {
	testCase        TestCaseInput
	blobs           types.Blobs
	commitments     []types.KZGCommitment
	versionedHashes []common.Hash
	aggregatedProof types.KZGProof
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

	testCaseInputs := []TestCaseInput{
		{"aa3a09289747a62b7b8190af3a75544cbe8c3b4a58f7b11d8d3b12ad17300a59", "0x45Ae5777c9b35Eb16280e423b0d7c91C06C66B58", 1, "1", int64(100000), "1000", "50", "0x1234"},
		{"67f45650acc5dc426fc424348f8d9f07032c439f03797c4beba096a6e23e666b", "0x549A51956bd364D8bB2Efb1F1eA4436e8D7764Ff", 2, "1", int64(50000), "1234", "100", "0xABCDEF"},
		{"2e2b5c749eab38b8eca6b788c70429580ffa8eb79ab9635b95af00c6f6cba661", "0xa39c4e1B259473fbcC5213a0613eB53a8C50bf76", 3, "1", int64(70000), "999", "10", "0x9988"},
		{"ee15ba623c2a495eefc9b8dc7447ff70bee325cc1f75f5170a71dc4dd3227f13", "0xd59399657A78bb69dEE83C416C13Be711e02fA23", 4, "1", int64(21000), "1001", "35", "0x01"},
	}

	testCaseOutputs := []TestCaseOutput{}
	for _, testCaseInput := range testCaseInputs {
		testCaseOutput, err := generateTestVector(chainId, signer, testCaseInput)
		assert.Nil(t, err)
		if testCaseOutput != nil {
			testCaseOutputs = append(testCaseOutputs, *testCaseOutput)
		}
	}

	outputFilePath := "/tmp/eip4844_test_vectors.txt"
	outputFile, _ := os.Create(outputFilePath)
	summary := []TestCaseResult{}
	for _, testCaseOutput := range testCaseOutputs {
		txAsRawData, _ := testCaseOutput.transaction.MarshalBinary()
		txRawHex := hexutil.Encode(txAsRawData)
		result := TestCaseResult{testCaseOutput.testCase, txRawHex}
		summary = append(summary, result)
		/*outputFile.WriteString(fmt.Sprintf("Test case [%d]\n", i))
		outputFile.WriteString("Input: \n")
		outputFile.WriteString(fmt.Sprintf("%v\n", testCaseOutput.testCase))
		outputFile.WriteString("Output: \n")
		outputFile.WriteString(hexutil.Encode(txAsRawData))
		outputFile.WriteString("\n\n")*/
	}
	summaryAsJson, _ := json.Marshal(summary)
	outputFile.WriteString(string(summaryAsJson))

}

func generateTestVector(chainId *big.Int, signer types.Signer, testCaseInput TestCaseInput) (*TestCaseOutput, error) {
	blobs := EncodeBlobs(hexutil.MustDecode(testCaseInput.Data))
	commitments, versionedHashes, aggregatedProof, err := blobs.ComputeCommitmentsAndAggregatedProof()
	if err != nil {
		log.Fatalf("failed to compute commitments: %v", err)
	}

	// Encode fields
	gasPrice256, _ := DecodeUint256String(testCaseInput.GasPrice)
	priorityGasPrice256, _ := DecodeUint256String(testCaseInput.PriorityGasPrice)
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
			Value:               view.Uint256View(*value256),
			To:                  types.AddressOptionalSSZ{Address: (*types.AddressSSZ)(&to)},
			BlobVersionedHashes: versionedHashes,
		},
	}

	// Generate wrap transaction data
	wrapData := types.BlobTxWrapData{
		BlobKzgs:           commitments,
		Blobs:              blobs,
		KzgAggregatedProof: aggregatedProof,
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
		aggregatedProof: aggregatedProof,
		txData:          txData,
		wrapData:        wrapData,
		transaction:     tx,
	}
	return testCaseOutput, nil
}
