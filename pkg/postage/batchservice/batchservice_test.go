// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchservice_test

import (
	"bytes"
	"errors"
	"hash"
	"io/ioutil"
	"math/big"
	"math/rand"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchservice"
	"github.com/ethersphere/bee/pkg/postage/batchstore/mock"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
	mocks "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
)

var (
	testLog    = logging.New(ioutil.Discard, 0)
	errTest    = errors.New("fails")
	testTxHash = make([]byte, 32)
)

type mockListener struct {
}

func (*mockListener) Listen(from uint64, updater postage.EventUpdater) <-chan struct{} { return nil }
func (*mockListener) Close() error                                                     { return nil }

func newMockListener() *mockListener {
	return &mockListener{}
}

type mockBatchCreationHandler struct {
	count int
}

func (m *mockBatchCreationHandler) Handle(b *postage.Batch) {
	m.count++
}

func TestBatchServiceCreate(t *testing.T) {
	testChainState := postagetesting.NewChainState()

	t.Run("expect put create put error", func(t *testing.T) {
		testBatch := postagetesting.MustNewBatch()
		testBatchListener := &mockBatchCreationHandler{}
		svc, _, _ := newTestStoreAndServiceWithListener(
			t,
			testBatch.Owner,
			testBatchListener,
			mock.WithChainState(testChainState),
			mock.WithPutErr(errTest, 0),
		)

		if err := svc.Create(
			testBatch.ID,
			testBatch.Owner,
			testBatch.Value,
			testBatch.Depth,
			testBatch.BucketDepth,
			testBatch.Immutable,
			testTxHash,
		); err == nil {
			t.Fatalf("expected error")
		}
		if testBatchListener.count != 0 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 0, testBatchListener.count)
		}
	})

	validateBatch := func(t *testing.T, testBatch *postage.Batch, st *mock.BatchStore) {
		got, err := st.Get(testBatch.ID)
		if err != nil {
			t.Fatalf("batch store get: %v", err)
		}

		if !bytes.Equal(got.ID, testBatch.ID) {
			t.Fatalf("batch id: want %v, got %v", testBatch.ID, got.ID)
		}
		if !bytes.Equal(got.Owner, testBatch.Owner) {
			t.Fatalf("batch owner: want %v, got %v", testBatch.Owner, got.Owner)
		}
		if got.Value.Cmp(testBatch.Value) != 0 {
			t.Fatalf("batch value: want %v, got %v", testBatch.Value.String(), got.Value.String())
		}
		if got.BucketDepth != testBatch.BucketDepth {
			t.Fatalf("bucket depth: want %v, got %v", got.BucketDepth, testBatch.BucketDepth)
		}
		if got.Depth != testBatch.Depth {
			t.Fatalf("batch depth: want %v, got %v", got.Depth, testBatch.Depth)
		}
		if got.Immutable != testBatch.Immutable {
			t.Fatalf("immutable: want %v, got %v", got.Immutable, testBatch.Immutable)
		}
		if got.Start != testChainState.Block {
			t.Fatalf("batch start block different form chain state: want %v, got %v", got.Start, testChainState.Block)
		}
	}

	t.Run("passes", func(t *testing.T) {
		testBatch := postagetesting.MustNewBatch()
		testBatchListener := &mockBatchCreationHandler{}
		svc, batchStore, _ := newTestStoreAndServiceWithListener(
			t,
			testBatch.Owner,
			testBatchListener,
			mock.WithChainState(testChainState),
		)

		if err := svc.Create(
			testBatch.ID,
			testBatch.Owner,
			testBatch.Value,
			testBatch.Depth,
			testBatch.BucketDepth,
			testBatch.Immutable,
			testTxHash,
		); err != nil {
			t.Fatalf("got error %v", err)
		}
		if testBatchListener.count != 1 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 1, testBatchListener.count)
		}

		validateBatch(t, testBatch, batchStore)
	})

	t.Run("passes without recovery", func(t *testing.T) {
		testBatch := postagetesting.MustNewBatch()
		testBatchListener := &mockBatchCreationHandler{}
		// create a owner different from the batch owner
		owner := make([]byte, 32)
		rand.Read(owner)

		svc, batchStore, _ := newTestStoreAndServiceWithListener(
			t,
			owner,
			testBatchListener,
			mock.WithChainState(testChainState),
		)

		if err := svc.Create(
			testBatch.ID,
			testBatch.Owner,
			testBatch.Value,
			testBatch.Depth,
			testBatch.BucketDepth,
			testBatch.Immutable,
			testTxHash,
		); err != nil {
			t.Fatalf("got error %v", err)
		}
		if testBatchListener.count != 0 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 1, testBatchListener.count)
		}

		validateBatch(t, testBatch, batchStore)
	})
}

func TestBatchServiceTopUp(t *testing.T) {
	testBatch := postagetesting.MustNewBatch()
	testNormalisedBalance := big.NewInt(2000000000000)

	t.Run("expect get error", func(t *testing.T) {
		svc, _, _ := newTestStoreAndService(
			t,
			mock.WithGetErr(errTest, 0),
		)

		if err := svc.TopUp(testBatch.ID, testNormalisedBalance, testTxHash); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("expect put error", func(t *testing.T) {
		svc, batchStore, _ := newTestStoreAndService(
			t,
			mock.WithPutErr(errTest, 1),
		)
		putBatch(t, batchStore, testBatch)

		if err := svc.TopUp(testBatch.ID, testNormalisedBalance, testTxHash); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("passes", func(t *testing.T) {
		svc, batchStore, _ := newTestStoreAndService(t)
		putBatch(t, batchStore, testBatch)

		want := testNormalisedBalance

		if err := svc.TopUp(testBatch.ID, testNormalisedBalance, testTxHash); err != nil {
			t.Fatalf("top up: %v", err)
		}

		got, err := batchStore.Get(testBatch.ID)
		if err != nil {
			t.Fatalf("batch store get: %v", err)
		}

		if got.Value.Cmp(want) != 0 {
			t.Fatalf("topped up amount: got %v, want %v", got.Value, want)
		}
	})
}

func TestBatchServiceUpdateDepth(t *testing.T) {
	const testNewDepth = 30
	testNormalisedBalance := big.NewInt(2000000000000)
	testBatch := postagetesting.MustNewBatch()

	t.Run("expect get error", func(t *testing.T) {
		svc, _, _ := newTestStoreAndService(
			t,
			mock.WithGetErr(errTest, 0),
		)

		if err := svc.UpdateDepth(testBatch.ID, testNewDepth, testNormalisedBalance, testTxHash); err == nil {
			t.Fatal("expected get error")
		}
	})

	t.Run("expect put error", func(t *testing.T) {
		svc, batchStore, _ := newTestStoreAndService(
			t,
			mock.WithPutErr(errTest, 1),
		)
		putBatch(t, batchStore, testBatch)

		if err := svc.UpdateDepth(testBatch.ID, testNewDepth, testNormalisedBalance, testTxHash); err == nil {
			t.Fatal("expected put error")
		}
	})

	t.Run("passes", func(t *testing.T) {
		svc, batchStore, _ := newTestStoreAndService(t)
		putBatch(t, batchStore, testBatch)

		if err := svc.UpdateDepth(testBatch.ID, testNewDepth, testNormalisedBalance, testTxHash); err != nil {
			t.Fatalf("update depth: %v", err)
		}

		val, err := batchStore.Get(testBatch.ID)
		if err != nil {
			t.Fatalf("batch store get: %v", err)
		}

		if val.Depth != testNewDepth {
			t.Fatalf("wrong batch depth set: want %v, got %v", testNewDepth, val.Depth)
		}
	})
}

func TestBatchServiceUpdatePrice(t *testing.T) {
	testChainState := postagetesting.NewChainState()
	testChainState.CurrentPrice = big.NewInt(100000)
	testNewPrice := big.NewInt(20000000)

	t.Run("expect put error", func(t *testing.T) {
		svc, batchStore, _ := newTestStoreAndService(
			t,
			mock.WithChainState(testChainState),
			mock.WithPutErr(errTest, 1),
		)
		putChainState(t, batchStore, testChainState)

		if err := svc.UpdatePrice(testNewPrice, testTxHash); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("passes", func(t *testing.T) {
		svc, batchStore, _ := newTestStoreAndService(
			t,
			mock.WithChainState(testChainState),
		)

		if err := svc.UpdatePrice(testNewPrice, testTxHash); err != nil {
			t.Fatalf("update price: %v", err)
		}

		cs := batchStore.GetChainState()
		if cs.CurrentPrice.Cmp(testNewPrice) != 0 {
			t.Fatalf("bad price: want %v, got %v", cs.CurrentPrice, testNewPrice)
		}
	})
}
func TestBatchServiceUpdateBlockNumber(t *testing.T) {
	testChainState := &postage.ChainState{
		Block:        1,
		CurrentPrice: big.NewInt(100),
		TotalAmount:  big.NewInt(100),
	}
	svc, batchStore, _ := newTestStoreAndService(
		t,
		mock.WithChainState(testChainState),
	)

	// advance the block number and expect total cumulative payout to update
	nextBlock := uint64(4)

	if err := svc.UpdateBlockNumber(nextBlock); err != nil {
		t.Fatalf("update price: %v", err)
	}
	nn := big.NewInt(400)
	cs := batchStore.GetChainState()
	if cs.TotalAmount.Cmp(nn) != 0 {
		t.Fatalf("bad price: want %v, got %v", nn, cs.TotalAmount)
	}
}

func TestTransactionOk(t *testing.T) {
	svc, store, s := newTestStoreAndService(t)
	if _, err := svc.Start(10); err != nil {
		t.Fatal(err)
	}

	if err := svc.TransactionStart(); err != nil {
		t.Fatal(err)
	}

	if err := svc.TransactionEnd(); err != nil {
		t.Fatal(err)
	}

	svc2, err := batchservice.New(s, store, testLog, newMockListener(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc2.Start(10); err != nil {
		t.Fatal(err)
	}

	if c := store.ResetCalls(); c != 0 {
		t.Fatalf("expect %d reset calls got %d", 0, c)
	}
}

func TestTransactionFail(t *testing.T) {
	svc, store, s := newTestStoreAndService(t)
	if _, err := svc.Start(10); err != nil {
		t.Fatal(err)
	}

	if err := svc.TransactionStart(); err != nil {
		t.Fatal(err)
	}

	svc2, err := batchservice.New(s, store, testLog, newMockListener(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc2.Start(10); err != nil {
		t.Fatal(err)
	}

	if c := store.ResetCalls(); c != 1 {
		t.Fatalf("expect %d reset calls got %d", 1, c)
	}
}

func TestChecksum(t *testing.T) {
	s := mocks.NewStateStore()
	store := mock.New()
	mockHash := &hs{}
	svc, err := batchservice.New(s, store, testLog, newMockListener(), nil, nil, func() hash.Hash { return mockHash }, false)
	if err != nil {
		t.Fatal(err)
	}
	testNormalisedBalance := big.NewInt(2000000000000)
	testBatch := postagetesting.MustNewBatch()
	putBatch(t, store, testBatch)

	if err := svc.TopUp(testBatch.ID, testNormalisedBalance, testTxHash); err != nil {
		t.Fatalf("top up: %v", err)
	}
	if m := mockHash.ctr; m != 2 {
		t.Fatalf("expected %d calls got %d", 2, m)
	}
}

func TestChecksumResync(t *testing.T) {
	s := mocks.NewStateStore()
	store := mock.New()
	mockHash := &hs{}
	svc, err := batchservice.New(s, store, testLog, newMockListener(), nil, nil, func() hash.Hash { return mockHash }, true)
	if err != nil {
		t.Fatal(err)
	}
	testNormalisedBalance := big.NewInt(2000000000000)
	testBatch := postagetesting.MustNewBatch()
	putBatch(t, store, testBatch)

	if err := svc.TopUp(testBatch.ID, testNormalisedBalance, testTxHash); err != nil {
		t.Fatalf("top up: %v", err)
	}
	if m := mockHash.ctr; m != 2 {
		t.Fatalf("expected %d calls got %d", 2, m)
	}

	// now start a new instance and check that the value gets read from statestore
	store2 := mock.New()
	mockHash2 := &hs{}
	_, err = batchservice.New(s, store2, testLog, newMockListener(), nil, nil, func() hash.Hash { return mockHash2 }, false)
	if err != nil {
		t.Fatal(err)
	}
	if m := mockHash2.ctr; m != 1 {
		t.Fatalf("expected %d calls got %d", 1, m)
	}

	// now start a new instance and check that the value does not get written into the hasher
	// when resyncing
	store3 := mock.New()
	mockHash3 := &hs{}
	_, err = batchservice.New(s, store3, testLog, newMockListener(), nil, nil, func() hash.Hash { return mockHash3 }, true)
	if err != nil {
		t.Fatal(err)
	}
	if m := mockHash3.ctr; m != 0 {
		t.Fatalf("expected %d calls got %d", 0, m)
	}
}

func newTestStoreAndServiceWithListener(
	t *testing.T,
	owner []byte,
	batchListener postage.BatchCreationListener,
	opts ...mock.Option,
) (postage.EventUpdater, *mock.BatchStore, storage.StateStorer) {
	t.Helper()
	s := mocks.NewStateStore()
	store := mock.New(opts...)
	svc, err := batchservice.New(s, store, testLog, newMockListener(), owner, batchListener, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	return svc, store, s
}

func newTestStoreAndService(t *testing.T, opts ...mock.Option) (postage.EventUpdater, *mock.BatchStore, storage.StateStorer) {
	t.Helper()
	return newTestStoreAndServiceWithListener(t, nil, nil, opts...)
}

func putBatch(t *testing.T, store postage.Storer, b *postage.Batch) {
	t.Helper()

	if err := store.Put(b, big.NewInt(0), 0); err != nil {
		t.Fatalf("store put batch: %v", err)
	}
}

func putChainState(t *testing.T, store postage.Storer, cs *postage.ChainState) {
	t.Helper()

	if err := store.PutChainState(cs); err != nil {
		t.Fatalf("store put chain state: %v", err)
	}
}

type hs struct{ ctr uint8 }

func (h *hs) Write(p []byte) (n int, err error) { h.ctr++; return len(p), nil }
func (h *hs) Sum(b []byte) []byte               { return []byte{h.ctr} }
func (h *hs) Reset()                            {}
func (h *hs) Size() int                         { panic("not implemented") }
func (h *hs) BlockSize() int                    { panic("not implemented") }
