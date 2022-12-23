package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/libp2p/go-libp2p"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"

	"github.com/libp2p/go-libp2p/p2p/protocol/identify"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/p2p"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/p2p/encoder"
	p2ptypes "github.com/prysmaticlabs/prysm/v3/beacon-chain/p2p/types"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/sync"
	types "github.com/prysmaticlabs/prysm/v3/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v3/proto/prysm/v1alpha1"
)

func init() {
	// Done to ensure that we are able to download blob chunks with larger chunk sizes (that is 10 MiB post-bellatrix)
	encoder.MaxChunkSize = 10 << 20
}

func DownloadApp(cliCtx *cli.Context) error {
	addr := cliCtx.String(DownloadBeaconP2PAddr.Name)
	slot := cliCtx.Int64(DownloadSlotFlag.Name)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := &ethpb.BlobsSidecarsByRangeRequest{
		StartSlot: types.Slot(slot),
		Count:     1,
	}

	h, err := libp2p.New(libp2p.Transport(tcp.NewTCPTransport))
	if err != nil {
		return err
	}
	defer func() {
		_ = h.Close()
	}()
	h.RemoveStreamHandler(identify.IDDelta)
	// setup enough handlers so we look like a beacon peer
	// Some clients, including lighthouse, expect a minimum set of protocols before completing
	// a libp2p connection
	setHandler(h, p2p.RPCPingTopicV1, pingHandler)
	setHandler(h, p2p.RPCGoodByeTopicV1, pingHandler)
	setHandler(h, p2p.RPCMetaDataTopicV1, pingHandler)
	setHandler(h, p2p.RPCMetaDataTopicV2, pingHandler)

	nilHandler := func(ctx context.Context, i interface{}, stream network.Stream) error {
		return nil
	}
	setHandler(h, p2p.RPCBlocksByRangeTopicV1, nilHandler)
	setHandler(h, p2p.RPCBlocksByRangeTopicV2, nilHandler)
	setHandler(h, p2p.RPCBlobsSidecarsByRangeTopicV1, nilHandler)

	multiaddr, err := getMultiaddr(ctx, h, addr)
	if err != nil {
		return fmt.Errorf("%w: unable to get multiaddr", err)
	}

	addrInfo, err := peer.AddrInfoFromP2pAddr(multiaddr)
	if err != nil {
		return fmt.Errorf("%w: unable to get addr info", err)
	}

	err = h.Connect(ctx, *addrInfo)
	if err != nil {
		return fmt.Errorf("%w: failed to connect", err)
	}

	sidecars, err := sendBlobsSidecarsByRangeRequest(ctx, h, encoder.SszNetworkEncoder{}, addrInfo.ID, req)
	if err != nil {
		return fmt.Errorf("%w: unable to send blobs RPC request", err)
	}

	anyBlobs := false
	for _, sidecar := range sidecars {
		if int64(sidecar.BeaconBlockSlot) != slot {
			break
		}

		if len(sidecar.Blobs) == 0 {
			continue
		}

		anyBlobs = true
		for _, blob := range sidecar.Blobs {
			data := DecodeBlob(blob.Data)
			_, _ = os.Stdout.Write(data)
		}

		// stop after the first sidecar with blobs:
		break
	}

	if !anyBlobs {
		return fmt.Errorf("no blobs found in requested slots, sidecar count: %d", len(sidecars))
	}
	return nil
}

func getMultiaddr(ctx context.Context, h host.Host, addr string) (ma.Multiaddr, error) {
	multiaddr, err := ma.NewMultiaddr(addr)
	if err != nil {
		return nil, err
	}
	_, id := peer.SplitAddr(multiaddr)
	if id != "" {
		return multiaddr, nil
	}
	// peer ID wasn't provided, look it up
	id, err = retrievePeerID(ctx, h, addr)
	if err != nil {
		return nil, err
	}
	return ma.NewMultiaddr(fmt.Sprintf("%s/p2p/%s", addr, string(id)))
}

// Helper for retrieving the peer ID from a security error... obviously don't use this in production!
// See https://github.com/libp2p/go-libp2p-noise/blob/v0.3.0/handshake.go#L250
func retrievePeerID(ctx context.Context, h host.Host, addr string) (peer.ID, error) {
	incorrectPeerID := "16Uiu2HAmSifdT5QutTsaET8xqjWAMPp4obrQv7LN79f2RMmBe3nY"
	addrInfo, err := peer.AddrInfoFromString(fmt.Sprintf("%s/p2p/%s", addr, incorrectPeerID))
	if err != nil {
		return "", err
	}
	err = h.Connect(ctx, *addrInfo)
	if err == nil {
		return "", errors.New("unexpected successful connection")
	}
	if strings.Contains(err.Error(), "but remote key matches") {
		split := strings.Split(err.Error(), " ")
		return peer.ID(split[len(split)-1]), nil
	}
	return "", err
}

func sendBlobsSidecarsByRangeRequest(ctx context.Context, h host.Host, encoding encoder.NetworkEncoding, pid peer.ID, req *ethpb.BlobsSidecarsByRangeRequest) ([]*ethpb.BlobsSidecar, error) {
	topic := fmt.Sprintf("%s%s", p2p.RPCBlobsSidecarsByRangeTopicV1, encoding.ProtocolSuffix())

	stream, err := h.NewStream(ctx, pid, protocol.ID(topic))
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = stream.Close()
	}()

	if _, err := encoding.EncodeWithMaxLength(stream, req); err != nil {
		_ = stream.Reset()
		return nil, err
	}

	if err := stream.CloseWrite(); err != nil {
		_ = stream.Reset()
		return nil, err
	}

	var blobSidecars []*ethpb.BlobsSidecar
	for {
		isFirstChunk := len(blobSidecars) == 0
		blobs, err := readChunkedBlobsSidecar(stream, encoding, isFirstChunk)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: readChunkedBlobsSidecar", err)
		}
		blobSidecars = append(blobSidecars, blobs)
	}
	return blobSidecars, nil
}

func readChunkedBlobsSidecar(stream libp2pcore.Stream, encoding encoder.NetworkEncoding, isFirstChunk bool) (*ethpb.BlobsSidecar, error) {
	var (
		code   uint8
		errMsg string
		err    error
	)
	if isFirstChunk {
		code, errMsg, err = sync.ReadStatusCode(stream, encoding)
	} else {
		sync.SetStreamReadDeadline(stream, time.Second*10)
		code, errMsg, err = readStatusCodeNoDeadline(stream, encoding)
	}
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, errors.New(errMsg)
	}
	// ignored: we assume we got the correct context
	b := make([]byte, 4)
	if _, err := stream.Read(b); err != nil {
		return nil, err
	}
	sidecar := new(ethpb.BlobsSidecar)
	err = encoding.DecodeWithMaxLength(stream, sidecar)
	return sidecar, err
}

func readStatusCodeNoDeadline(stream libp2pcore.Stream, encoding encoder.NetworkEncoding) (uint8, string, error) {
	b := make([]byte, 1)
	_, err := stream.Read(b)
	if err != nil {
		return 0, "", err
	}
	if b[0] == responseCodeSuccess {
		return 0, "", nil
	}
	msg := &p2ptypes.ErrorMessage{}
	if err := encoding.DecodeWithMaxLength(stream, msg); err != nil {
		return 0, "", err
	}
	return b[0], string(*msg), nil
}

var responseCodeSuccess = byte(0x00)
