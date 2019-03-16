
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:33</date>
//</624342612190433280>


package ethash

import (
	"bytes"
	crand "crypto/rand"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"math/rand"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

var (
	errNoMiningWork      = errors.New("no mining work available yet")
	errInvalidSealResult = errors.New("invalid or stale proof-of-work solution")
)

//
//块的难度要求。
func (ethash *Ethash) Seal(chain consensus.ChainReader, block *types.Block, stop <-chan struct{}) (*types.Block, error) {
//如果我们使用的是假战俘，只需立即返回一个0。
	if ethash.config.PowMode == ModeFake || ethash.config.PowMode == ModeFullFake {
		header := block.Header()
		header.Nonce, header.MixDigest = types.BlockNonce{}, common.Hash{}
		return block.WithSeal(header), nil
	}
//如果我们正在运行一个共享的POW，就委托密封它。
	if ethash.shared != nil {
		return ethash.shared.Seal(chain, block, stop)
	}
//创建一个运行程序及其所指向的多个搜索线程
	abort := make(chan struct{})

	ethash.lock.Lock()
	threads := ethash.threads
	if ethash.rand == nil {
		seed, err := crand.Int(crand.Reader, big.NewInt(math.MaxInt64))
		if err != nil {
			ethash.lock.Unlock()
			return nil, err
		}
		ethash.rand = rand.New(rand.NewSource(seed.Int64()))
	}
	ethash.lock.Unlock()
	if threads == 0 {
		threads = runtime.NumCPU()
	}
	if threads < 0 {
threads = 0 //允许禁用本地挖掘而不在本地/远程周围使用额外逻辑
	}
//将新工作推到远程密封器上
	if ethash.workCh != nil {
		ethash.workCh <- block
	}
	var pend sync.WaitGroup
	for i := 0; i < threads; i++ {
		pend.Add(1)
		go func(id int, nonce uint64) {
			defer pend.Done()
			ethash.mine(block, id, nonce, abort, ethash.resultCh)
		}(i, uint64(ethash.rand.Int63()))
	}
//等待密封终止或找到一个nonce
	var result *types.Block
	select {
	case <-stop:
//外部中止，停止所有矿工线程
		close(abort)
	case result = <-ethash.resultCh:
//其中一个线程找到一个块，中止所有其他线程
		close(abort)
	case <-ethash.update:
//线程计数已根据用户请求更改，请重新启动
		close(abort)
		pend.Wait()
		return ethash.Seal(chain, block, stop)
	}
//等待所有矿工终止并返回区块
	pend.Wait()
	return result, nil
}

//Mine是工作矿工从
//导致正确最终阻塞困难的种子。
func (ethash *Ethash) mine(block *types.Block, id int, seed uint64, abort chan struct{}, found chan *types.Block) {
//从头中提取一些数据
	var (
		header  = block.Header()
		hash    = header.HashNoNonce().Bytes()
		target  = new(big.Int).Div(two256, header.Difficulty)
		number  = header.Number.Uint64()
		dataset = ethash.dataset(number, false)
	)
//开始生成随机的nonce，直到我们中止或找到一个好的nonce
	var (
		attempts = int64(0)
		nonce    = seed
	)
	logger := log.New("miner", id)
	logger.Trace("Started ethash search for new nonces", "seed", seed)
search:
	for {
		select {
		case <-abort:
//挖掘已终止，更新状态并中止
			logger.Trace("Ethash nonce search aborted", "attempts", nonce-seed)
			ethash.hashrate.Mark(attempts)
			break search

		default:
//我们不需要每次更新哈希率，所以在2^x次之后更新
			attempts++
			if (attempts % (1 << 15)) == 0 {
				ethash.hashrate.Mark(attempts)
				attempts = 0
			}
//计算这个nonce的pow值
			digest, result := hashimotoFull(dataset.dataset, hash, nonce)
			if new(big.Int).SetBytes(result).Cmp(target) <= 0 {
//一旦找到正确的标题，就用它创建一个新的标题
				header = types.CopyHeader(header)
				header.Nonce = types.EncodeNonce(nonce)
				header.MixDigest = common.BytesToHash(digest)

//密封并返回一个块（如果仍然需要）
				select {
				case found <- block.WithSeal(header):
					logger.Trace("Ethash nonce found and reported", "attempts", nonce-seed, "nonce", nonce)
				case <-abort:
					logger.Trace("Ethash nonce found but discarded", "attempts", nonce-seed, "nonce", nonce)
				}
				break search
			}
			nonce++
		}
	}
//数据集在终结器中未映射。确保数据集保持活动状态
//
	runtime.KeepAlive(dataset)
}

//远程是一个独立的goroutine，用于处理与远程挖掘相关的内容。
func (ethash *Ethash) remote(notify []string) {
	var (
		works = make(map[common.Hash]*types.Block)
		rates = make(map[common.Hash]hashrate)

		currentBlock *types.Block
		currentWork  [3]string

		notifyTransport = &http.Transport{}
		notifyClient    = &http.Client{
			Transport: notifyTransport,
			Timeout:   time.Second,
		}
		notifyReqs = make([]*http.Request, len(notify))
	)
//notifywork通知所有指定的挖掘终结点
//要处理的新工作。
	notifyWork := func() {
		work := currentWork
		blob, _ := json.Marshal(work)

		for i, url := range notify {
//终止任何以前挂起的请求并创建新工作
			if notifyReqs[i] != nil {
				notifyTransport.CancelRequest(notifyReqs[i])
			}
			notifyReqs[i], _ = http.NewRequest("POST", url, bytes.NewReader(blob))
			notifyReqs[i].Header.Set("Content-Type", "application/json")

//同时将新工作推送到所有远程节点
			go func(req *http.Request, url string) {
				res, err := notifyClient.Do(req)
				if err != nil {
					log.Warn("Failed to notify remote miner", "err", err)
				} else {
					log.Trace("Notified remote miner", "miner", url, "hash", log.Lazy{Fn: func() common.Hash { return common.HexToHash(work[0]) }}, "target", work[2])
					res.Body.Close()
				}
			}(notifyReqs[i], url)
		}
	}
//makework为外部矿工创建工作包。
//
//工作包由3个字符串组成：
//结果[0]，32字节十六进制编码的当前块头POW哈希
//
//结果[2]，32字节十六进制编码边界条件（“目标”），2^256/难度
	makeWork := func(block *types.Block) {
		hash := block.HashNoNonce()

		currentWork[0] = hash.Hex()
		currentWork[1] = common.BytesToHash(SeedHash(block.NumberU64())).Hex()
		currentWork[2] = common.BytesToHash(new(big.Int).Div(two256, block.Difficulty()).Bytes()).Hex()

//追踪遥控封口机取出的封印工作。
		currentBlock = block
		works[hash] = block
	}
//SubNetwork验证提交的POW解决方案，返回
//解决方案是否被接受（不可能是一个坏的POW以及
//任何其他错误，例如没有挂起的工作或过时的挖掘结果）。
	submitWork := func(nonce types.BlockNonce, mixDigest common.Hash, hash common.Hash) bool {
//
		block := works[hash]
		if block == nil {
			log.Info("Work submitted but none pending", "hash", hash)
			return false
		}
//验证提交结果的正确性。
		header := block.Header()
		header.Nonce = nonce
		header.MixDigest = mixDigest

		start := time.Now()
		if err := ethash.verifySeal(nil, header, true); err != nil {
			log.Warn("Invalid proof-of-work submitted", "hash", hash, "elapsed", time.Since(start), "err", err)
			return false
		}
//确保已创建结果通道。
		if ethash.resultCh == nil {
			log.Warn("Ethash result channel is empty, submitted mining result is rejected")
			return false
		}
		log.Trace("Verified correct proof-of-work", "hash", hash, "elapsed", time.Since(start))

//解决方案似乎是有效的，返回矿工并通知接受。
		select {
		case ethash.resultCh <- block.WithSeal(header):
			delete(works, hash)
			return true
		default:
			log.Info("Work submitted is stale", "hash", hash)
			return false
		}
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case block := <-ethash.workCh:
			if currentBlock != nil && block.ParentHash() != currentBlock.ParentHash() {
//开始新一轮开采，放弃以前的工作。
				works = make(map[common.Hash]*types.Block)
			}
//使用新接收的块更新当前工作。
//注意，相同的工作可能会过去两次，发生在更改CPU线程时。
			makeWork(block)

//通知并请求新工作可用性的URL
			notifyWork()

		case work := <-ethash.fetchWorkCh:
//将当前采矿工作返回给远程矿工。
			if currentBlock == nil {
				work.errc <- errNoMiningWork
			} else {
				work.res <- currentWork
			}

		case result := <-ethash.submitWorkCh:
//根据维护的采矿块验证提交的POW解决方案。
			if submitWork(result.nonce, result.mixDigest, result.hash) {
				result.errc <- nil
			} else {
				result.errc <- errInvalidSealResult
			}

		case result := <-ethash.submitRateCh:
//按提交的值跟踪远程密封程序的哈希率。
			rates[result.id] = hashrate{rate: result.rate, ping: time.Now()}
			close(result.done)

		case req := <-ethash.fetchRateCh:
//收集远程密封程序提交的所有哈希率。
			var total uint64
			for _, rate := range rates {
//这可能会溢出
				total += rate.rate
			}
			req <- total

		case <-ticker.C:
//清除过时提交的哈希率。
			for id, rate := range rates {
				if time.Since(rate.ping) > 10*time.Second {
					delete(rates, id)
				}
			}

		case errc := <-ethash.exitCh:
//如果ethash关闭，则退出远程回路并返回相关错误。
			errc <- nil
			log.Trace("Ethash remote sealer is exiting")
			return
		}
	}
}

