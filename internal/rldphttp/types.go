package rldphttp

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/xssnick/tonutils-go/adnl"
	"github.com/xssnick/tonutils-go/adnl/address"
	"github.com/xssnick/tonutils-go/adnl/rldp"
	"github.com/xssnick/tonutils-go/tl"
)

const _ChunkSize = 1 << 17
const _RLDPMaxAnswerSize = uint64(2*_ChunkSize + 1024)

type DHT interface {
	StoreAddress(ctx context.Context, addresses address.List, ttl time.Duration, ownerKey ed25519.PrivateKey, copies int) (int, []byte, error)
	FindAddresses(ctx context.Context, key []byte) (*address.List, ed25519.PublicKey, error)
	Close()
}

type RLDP interface {
	Close()
	DoQuery(ctx context.Context, maxAnswerSize uint64, query, result tl.Serializable) error
	SetOnQuery(handler func(transferId []byte, query *rldp.Query) error)
	SetOnDisconnect(handler func())
	SendAnswer(ctx context.Context, maxAnswerSize uint64, timeoutAt uint32, queryId, transferId []byte, answer tl.Serializable) error
}

type rldpInfo struct {
	mx             sync.RWMutex
	ActiveClient   RLDP
	ClientLastUsed time.Time

	ID   ed25519.PublicKey
	Addr string
}

var newRLDP = func(a adnl.Peer, v2 bool) RLDP {
	if v2 {
		return rldp.NewClientV2(a)
	}
	return rldp.NewClient(a)
}

func handleGetPart(req GetNextPayloadPart, stream *payloadStream) (*PayloadPart, error) {
	stream.mx.Lock()
	defer stream.mx.Unlock()

	offset := int(req.Seqno * req.MaxChunkSize)
	if offset != stream.nextOffset {
		return nil, fmt.Errorf("failed to get part for stream %s, incorrect offset %d, should be %d", hex.EncodeToString(req.ID), offset, stream.nextOffset)
	}

	var last bool
	data := make([]byte, req.MaxChunkSize)
	n, err := stream.Data.Read(data)
	if err != nil {
		if err != io.EOF {
			return nil, fmt.Errorf("failed to read chunk %d, err: %w", req.Seqno, err)
		}
		last = true
	}
	stream.nextOffset += n

	return &PayloadPart{
		Data:    data[:n],
		Trailer: nil,
		IsLast:  last,
	}, nil
}
