
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:33</date>
//</624342612110741504>


package ethash

import (
	"io/ioutil"
	"math/big"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

//测试ethash在测试模式下是否正常工作。
func TestTestMode(t *testing.T) {
	header := &types.Header{Number: big.NewInt(1), Difficulty: big.NewInt(100)}

	ethash := NewTester(nil)
	defer ethash.Close()

	block, err := ethash.Seal(nil, types.NewBlockWithHeader(header), nil)
	if err != nil {
		t.Fatalf("failed to seal block: %v", err)
	}
	header.Nonce = types.EncodeNonce(block.Nonce())
	header.MixDigest = block.MixDigest()
	if err := ethash.VerifySeal(nil, header); err != nil {
		t.Fatalf("unexpected verification error: %v", err)
	}
}

//此测试检查高速缓存LRU逻辑在负载下是否崩溃。
//它复制了https://github.com/ethereum/go-ethereum/issues/14943
func TestCacheFileEvict(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "ethash-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)
	e := New(Config{CachesInMem: 3, CachesOnDisk: 10, CacheDir: tmpdir, PowMode: ModeTest}, nil)
	defer e.Close()

	workers := 8
	epochs := 100
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go verifyTest(&wg, e, i, epochs)
	}
	wg.Wait()
}

func verifyTest(wg *sync.WaitGroup, e *Ethash, workerIndex, epochs int) {
	defer wg.Done()

	const wiggle = 4 * epochLength
	r := rand.New(rand.NewSource(int64(workerIndex)))
	for epoch := 0; epoch < epochs; epoch++ {
		block := int64(epoch)*epochLength - wiggle/2 + r.Int63n(wiggle)
		if block < 0 {
			block = 0
		}
		header := &types.Header{Number: big.NewInt(block), Difficulty: big.NewInt(100)}
		e.VerifySeal(nil, header)
	}
}

func TestRemoteSealer(t *testing.T) {
	ethash := NewTester(nil)
	defer ethash.Close()

	api := &API{ethash}
	if _, err := api.GetWork(); err != errNoMiningWork {
		t.Error("expect to return an error indicate there is no mining work")
	}
	header := &types.Header{Number: big.NewInt(1), Difficulty: big.NewInt(100)}
	block := types.NewBlockWithHeader(header)

//推动新的工作。
	ethash.Seal(nil, block, nil)

	var (
		work [3]string
		err  error
	)
	if work, err = api.GetWork(); err != nil || work[0] != block.HashNoNonce().Hex() {
		t.Error("expect to return a mining work has same hash")
	}

	if res := api.SubmitWork(types.BlockNonce{}, block.HashNoNonce(), common.Hash{}); res {
		t.Error("expect to return false when submit a fake solution")
	}
//
	header = &types.Header{Number: big.NewInt(1), Difficulty: big.NewInt(1000)}
	block = types.NewBlockWithHeader(header)
	ethash.Seal(nil, block, nil)

	if work, err = api.GetWork(); err != nil || work[0] != block.HashNoNonce().Hex() {
		t.Error("expect to return the latest pushed work")
	}
//推压块具有更高的块编号。
	newHead := &types.Header{Number: big.NewInt(2), Difficulty: big.NewInt(100)}
	newBlock := types.NewBlockWithHeader(newHead)
	ethash.Seal(nil, newBlock, nil)

	if res := api.SubmitWork(types.BlockNonce{}, block.HashNoNonce(), common.Hash{}); res {
		t.Error("expect to return false when submit a stale solution")
	}
}

func TestHashRate(t *testing.T) {
	var (
		hashrate = []hexutil.Uint64{100, 200, 300}
		expect   uint64
		ids      = []common.Hash{common.HexToHash("a"), common.HexToHash("b"), common.HexToHash("c")}
	)
	ethash := NewTester(nil)
	defer ethash.Close()

	if tot := ethash.Hashrate(); tot != 0 {
		t.Error("expect the result should be zero")
	}

	api := &API{ethash}
	for i := 0; i < len(hashrate); i += 1 {
		if res := api.SubmitHashRate(hashrate[i], ids[i]); !res {
			t.Error("remote miner submit hashrate failed")
		}
		expect += uint64(hashrate[i])
	}
	if tot := ethash.Hashrate(); tot != float64(expect) {
		t.Error("expect total hashrate should be same")
	}
}

func TestClosedRemoteSealer(t *testing.T) {
	ethash := NewTester(nil)
time.Sleep(1 * time.Second) //确保出口频道正在收听
	ethash.Close()

	api := &API{ethash}
	if _, err := api.GetWork(); err != errEthashStopped {
		t.Error("expect to return an error to indicate ethash is stopped")
	}

	if res := api.SubmitHashRate(hexutil.Uint64(100), common.HexToHash("a")); res {
		t.Error("expect to return false when submit hashrate to a stopped ethash")
	}
}

