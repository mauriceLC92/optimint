package mock

import (
	"encoding/binary"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/celestiaorg/optimint/da"
	"github.com/celestiaorg/optimint/log"
	"github.com/celestiaorg/optimint/store"
	"github.com/celestiaorg/optimint/types"
)

// MockDataAvailabilityLayerClient is intended only for usage in tests.
// It does actually ensures DA - it stores data in-memory.
type MockDataAvailabilityLayerClient struct {
	logger   log.Logger
	dalcKV   store.KVStore
	daHeight uint64
	config   Config
}

const DefaultBlockTime = 3 * time.Second

type Config struct {
	BlockTime time.Duration
}

var _ da.DataAvailabilityLayerClient = &MockDataAvailabilityLayerClient{}
var _ da.BlockRetriever = &MockDataAvailabilityLayerClient{}

// Init is called once to allow DA client to read configuration and initialize resources.
func (m *MockDataAvailabilityLayerClient) Init(config []byte, dalcKV store.KVStore, logger log.Logger) error {
	m.logger = logger
	m.dalcKV = dalcKV
	m.daHeight = 1
	if len(config) > 0 {
		var err error
		m.config.BlockTime, err = time.ParseDuration(string(config))
		if err != nil {
			return err
		}
	} else {
		m.config.BlockTime = DefaultBlockTime
	}
	return nil
}

// Start implements DataAvailabilityLayerClient interface.
func (m *MockDataAvailabilityLayerClient) Start() error {
	m.logger.Debug("Mock Data Availability Layer Client starting")
	go func() {
		for {
			time.Sleep(m.config.BlockTime)
			m.updateDAHeight()
		}
	}()
	return nil
}

// Stop implements DataAvailabilityLayerClient interface.
func (m *MockDataAvailabilityLayerClient) Stop() error {
	m.logger.Debug("Mock Data Availability Layer Client stopped")
	return nil
}

// SubmitBlock submits the passed in block to the DA layer.
// This should create a transaction which (potentially)
// triggers a state transition in the DA layer.
func (m *MockDataAvailabilityLayerClient) SubmitBlock(block *types.Block) da.ResultSubmitBlock {
	daHeight := atomic.LoadUint64(&m.daHeight)
	m.logger.Debug("Submitting block to DA layer!", "height", block.Header.Height, "dataLayerHeight", daHeight)

	hash := block.Header.Hash()
	blob, err := block.MarshalBinary()
	if err != nil {
		return da.ResultSubmitBlock{DAResult: da.DAResult{Code: da.StatusError, Message: err.Error()}}
	}

	err = m.dalcKV.Set(getKey(daHeight, block.Header.Height), hash[:])
	if err != nil {
		return da.ResultSubmitBlock{DAResult: da.DAResult{Code: da.StatusError, Message: err.Error()}}
	}
	err = m.dalcKV.Set(hash[:], blob)
	if err != nil {
		return da.ResultSubmitBlock{DAResult: da.DAResult{Code: da.StatusError, Message: err.Error()}}
	}

	return da.ResultSubmitBlock{
		DAResult: da.DAResult{
			Code:     da.StatusSuccess,
			Message:  "OK",
			DAHeight: daHeight,
		},
	}
}

// CheckBlockAvailability queries DA layer to check data availability of block corresponding to given header.
func (m *MockDataAvailabilityLayerClient) CheckBlockAvailability(dataLayerHeight uint64) da.ResultCheckBlock {
	blocksRes := m.RetrieveBlocks(dataLayerHeight)
	return da.ResultCheckBlock{DAResult: da.DAResult{Code: blocksRes.Code}, DataAvailable: len(blocksRes.Blocks) > 0}
}

// RetrieveBlocks returns block at given height from data availability layer.
func (m *MockDataAvailabilityLayerClient) RetrieveBlocks(dataLayerHeight uint64) da.ResultRetrieveBlocks {
	if dataLayerHeight >= atomic.LoadUint64(&m.daHeight) {
		return da.ResultRetrieveBlocks{DAResult: da.DAResult{Code: da.StatusError, Message: "block not found"}}
	}

	iter := m.dalcKV.PrefixIterator(getPrefix(dataLayerHeight))
	defer iter.Discard()

	var blocks []*types.Block
	for iter.Valid() {
		hash := iter.Value()

		blob, err := m.dalcKV.Get(hash)
		if err != nil {
			return da.ResultRetrieveBlocks{DAResult: da.DAResult{Code: da.StatusError, Message: err.Error()}}
		}

		block := &types.Block{}
		err = block.UnmarshalBinary(blob)
		if err != nil {
			return da.ResultRetrieveBlocks{DAResult: da.DAResult{Code: da.StatusError, Message: err.Error()}}
		}
		blocks = append(blocks, block)

		iter.Next()
	}

	return da.ResultRetrieveBlocks{DAResult: da.DAResult{Code: da.StatusSuccess}, Blocks: blocks}
}

func getPrefix(daHeight uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, daHeight)
	return b
}

func getKey(daHeight uint64, height uint64) []byte {
	b := make([]byte, 16)
	binary.BigEndian.PutUint64(b, daHeight)
	binary.BigEndian.PutUint64(b[8:], height)
	return b
}

func (m *MockDataAvailabilityLayerClient) updateDAHeight() {
	blockStep := rand.Uint64()%10 + 1
	atomic.AddUint64(&m.daHeight, blockStep)
}
