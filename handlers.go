package main

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"runtime/debug"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/prysmaticlabs/prysm/v4/beacon-chain/p2p"
	"github.com/prysmaticlabs/prysm/v4/beacon-chain/p2p/encoder"
	"github.com/prysmaticlabs/prysm/v4/beacon-chain/p2p/types"
	consensustypes "github.com/prysmaticlabs/prysm/v4/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v4/consensus-types/wrapper"
	ethpb "github.com/prysmaticlabs/prysm/v4/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v4/proto/prysm/v1alpha1/metadata"
)

type rpcHandler func(context.Context, interface{}, network.Stream) error

// adapted from prysm's handler router - https://github.com/prysmaticlabs/prysm/blob/4e28192541625e2e7828928430dbc72eb6c075c4/beacon-chain/sync/rpc.go#L109
func setHandler(h host.Host, baseTopic string, handler rpcHandler) {
	encoding := &encoder.SszNetworkEncoder{}
	topic := baseTopic + encoding.ProtocolSuffix()
	h.SetStreamHandler(protocol.ID(topic), func(stream network.Stream) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Panic occurred: %v", r)
				log.Printf("%s", debug.Stack())
			}
		}()

		// Resetting after closing is a no-op so defer a reset in case something goes wrong.
		// It's up to the handler to Close the stream (send an EOF) if
		// it successfully writes a response. We don't blindly call
		// Close here because we may have only written a partial
		// response.
		defer func() {
			_err := stream.Reset()
			_ = _err
		}()

		base, ok := p2p.RPCTopicMappings[baseTopic]
		if !ok {
			log.Printf("ERROR: Could not retrieve base message for topic %s", baseTopic)
			return
		}
		bb := base
		t := reflect.TypeOf(base)
		// Copy Base
		base = reflect.New(t)

		if baseTopic == p2p.RPCMetaDataTopicV1 || baseTopic == p2p.RPCMetaDataTopicV2 {
			if err := metadataHandler(context.Background(), base, stream); err != nil {
				if err != types.ErrWrongForkDigestVersion {
					log.Printf("ERROR: Could not handle p2p RPC: %v", err)
				}
			}
			return
		}

		// Given we have an input argument that can be pointer or the actual object, this gives us
		// a way to check for its reflect.Kind and based on the result, we can decode
		// accordingly.
		if t.Kind() == reflect.Ptr {
			msg, ok := reflect.New(t.Elem()).Interface().(ssz.Unmarshaler)
			if !ok {
				log.Printf("ERROR: message of %T ptr does not support marshaller interface. topic=%s", bb, baseTopic)
				return
			}
			if err := encoding.DecodeWithMaxLength(stream, msg); err != nil {
				log.Printf("ERROR: could not decode stream message: %v", err)
				return
			}
			if err := handler(context.Background(), msg, stream); err != nil {
				if err != types.ErrWrongForkDigestVersion {
					log.Printf("ERROR: Could not handle p2p RPC: %v", err)
				}
			}
		} else {
			nTyp := reflect.New(t)
			msg, ok := nTyp.Interface().(ssz.Unmarshaler)
			if !ok {
				log.Printf("ERROR: message of %T does not support marshaller interface", msg)
				return
			}
			if err := handler(context.Background(), msg, stream); err != nil {
				if err != types.ErrWrongForkDigestVersion {
					log.Printf("ERROR: Could not handle p2p RPC: %v", err)
				}
			}
		}
	})
}

func dummyMetadata() metadata.Metadata {
	metaData := &ethpb.MetaDataV1{
		SeqNumber: 0,
		Attnets:   bitfield.NewBitvector64(),
		Syncnets:  bitfield.Bitvector4{byte(0x00)},
	}
	return wrapper.WrappedMetadataV1(metaData)
}

// pingHandler reads the incoming ping rpc message from the peer.
func pingHandler(_ context.Context, _ interface{}, stream network.Stream) error {
	encoding := &encoder.SszNetworkEncoder{}
	defer closeStream(stream)
	if _, err := stream.Write([]byte{responseCodeSuccess}); err != nil {
		return err
	}
	m := dummyMetadata()
	sq := consensustypes.SSZUint64(m.SequenceNumber())
	if _, err := encoding.EncodeWithMaxLength(stream, &sq); err != nil {
		return fmt.Errorf("%w: pingHandler stream write", err)
	}
	return nil
}

// metadataHandler spoofs a valid looking metadata message
func metadataHandler(_ context.Context, _ interface{}, stream network.Stream) error {
	encoding := &encoder.SszNetworkEncoder{}
	defer closeStream(stream)
	if _, err := stream.Write([]byte{responseCodeSuccess}); err != nil {
		return err
	}

	// write a dummy metadata message to satify the client handshake
	m := dummyMetadata()
	if _, err := encoding.EncodeWithMaxLength(stream, m); err != nil {
		return fmt.Errorf("%w: metadata stream write", err)
	}
	return nil
}

func closeStream(stream network.Stream) {
	if err := stream.Close(); err != nil {
		log.Println(err)
	}
}
