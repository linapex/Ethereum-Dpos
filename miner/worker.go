
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:42</date>
//</624342652438974464>


package miner

import (
	"bytes"
	"fmt"
	"github.com/deckarep/golang-set"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/dpos"
	"github.com/ethereum/go-ethereum/consensus/misc"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"math/big"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const (
//结果QueueSize为监听密封结果的通道大小。
	resultQueueSize = 10

//txchanSize是侦听newtxSevent的频道的大小。
//该数字是根据Tx池的大小引用的。
	txChanSize = 4096

//ChainHeadChansize是侦听ChainHeadEvent的通道的大小。
	chainHeadChanSize = 10

//ChainsideChansize是侦听ChainsideEvent的通道的大小。
	chainSideChanSize = 10

//SubmitadjustChansize是重新提交间隔调整通道的大小。
	resubmitAdjustChanSize = 10

//MiningLogAtDepth是记录成功挖掘之前的确认数。
	miningLogAtDepth = 5

//MinRecommitInterval是重新创建挖掘块所用的最小时间间隔。
//任何新到的交易。
	minRecommitInterval = 1 * time.Second

//MaxRecommitInterval是重新创建挖掘块所用的最大时间间隔
//任何新到的交易。
	maxRecommitInterval = 15 * time.Second

//间隙调整是单个间隙调整对密封工作的影响。
//重新提交间隔。
	intervalAdjustRatio = 0.1

//在新的重新提交间隔计算期间应用IntervalAdjustBias，有利于
//增大上限或减小下限，以便可以达到上限。
	intervalAdjustBias = 200 * 1000.0 * 1000.0
)

//环境是工作人员的当前环境，保存所有当前状态信息。
type environment struct {
	signer types.Signer

state     *state.StateDB //在此应用状态更改
	dposContext *types.DposContext
ancestors mapset.Set     //祖先集（用于检查叔叔父级有效性）
family    mapset.Set     //家庭设置（用于检查叔叔的无效性）
uncles    mapset.Set     //叔叔集
tcount    int            //周期中的Tx计数
gasPool   *core.GasPool  //用于包装交易的可用气体

	header   *types.Header
	txs      []*types.Transaction
	receipts []*types.Receipt
}

//任务包含共识引擎密封和结果提交的所有信息。
type task struct {
	receipts  []*types.Receipt
	state     *state.StateDB
	block     *types.Block
	createdAt time.Time
}


//工作人员是负责向共识引擎提交新工作的主要对象。
//收集密封结果。
type worker struct {
	config *params.ChainConfig
	engine consensus.Engine
	eth    Backend
	chain  *core.BlockChain

//订阅
	mux          *event.TypeMux
	txsCh        chan core.NewTxsEvent
	txsSub       event.Subscription
	chainHeadCh  chan core.ChainHeadEvent
	chainHeadSub event.Subscription

//渠道
	taskCh             chan *task
	resultCh           chan *task
	startCh            chan struct{}
	exitCh             chan struct{}

current        *environment                 //当前运行周期的环境。
possibleUncles map[common.Hash]*types.Block //一组侧块作为可能的叔叔块。
unconfirmed    *unconfirmedBlocks           //一组本地挖掘的块，等待规范性确认。

mu       sync.RWMutex //用于保护coinbase和额外字段的锁
	coinbase common.Address
	extra    []byte

snapshotMu    sync.RWMutex //用于保护块快照和状态快照的锁
	snapshotBlock *types.Block
	snapshotState *state.StateDB

//原子状态计数器
running int32 //共识引擎是否运行的指示器。
newTxs  int32 //自上次密封工作提交以来的新到达交易记录计数。

//测试钩
newTaskHook  func(*task)                        //方法在收到新的密封任务时调用。
skipSealHook func(*task) bool                   //方法来决定是否跳过密封。
fullTaskHook func()                             //方法在执行完全密封任务之前调用。
resubmitHook func(time.Duration, time.Duration) //更新重新提交间隔时调用的方法。
	quitCh  chan struct{}
	stopper chan struct{}

}

func newWorker(config *params.ChainConfig, engine consensus.Engine, eth Backend, mux *event.TypeMux, recommit time.Duration) *worker {
	worker := &worker{
		config:             config,
		engine:             engine,
		eth:                eth,
		mux:                mux,
		chain:              eth.BlockChain(),
		possibleUncles:     make(map[common.Hash]*types.Block),
		unconfirmed:        newUnconfirmedBlocks(eth.BlockChain(), miningLogAtDepth),
		txsCh:              make(chan core.NewTxsEvent, txChanSize),
		chainHeadCh:        make(chan core.ChainHeadEvent, chainHeadChanSize),
		taskCh:             make(chan *task),
		resultCh:           make(chan *task, resultQueueSize),
		exitCh:             make(chan struct{}),
		startCh:            make(chan struct{}, 1),
		quitCh:         	make(chan struct{}, 1),
		stopper:        	make(chan struct{}, 1),
	}
//订阅Tx池的NewTxSevent
	worker.txsSub = eth.TxPool().SubscribeNewTxsEvent(worker.txsCh)
//为区块链订阅事件
	worker.chainHeadSub = eth.BlockChain().SubscribeChainHeadEvent(worker.chainHeadCh)

//如果用户指定的重新提交间隔太短，则清除重新提交间隔。
	if recommit < minRecommitInterval {
		log.Warn("Sanitizing miner recommit interval", "provided", recommit, "updated", minRecommitInterval)
		recommit = minRecommitInterval
	}

	go worker.mainLoop()
	go worker.resultLoop()
	go worker.taskLoop()
	worker.createNewWork()

	return worker
}

//setcoinbase设置用于初始化块coinbase字段的coinbase。
func (w *worker) setCoinbase(addr common.Address) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.coinbase = addr
}

//setextra设置用于初始化块额外字段的内容。
func (w *worker) setExtra(extra []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.extra = extra
}


//Pending返回Pending状态和相应的块。
func (w *worker) pending() (*types.Block, *state.StateDB) {
//返回快照以避免对当前mutex的争用
	w.snapshotMu.RLock()
	defer w.snapshotMu.RUnlock()
	if w.snapshotState == nil {
		return nil, nil
	}
	return w.snapshotBlock, w.snapshotState.Copy()
}

//PendingBlock返回PendingBlock。
func (w *worker) pendingBlock() *types.Block {
//返回快照以避免对当前mutex的争用
	w.snapshotMu.RLock()
	defer w.snapshotMu.RUnlock()
	return w.snapshotBlock
}

//Start将运行状态设置为1并触发新工作提交。
func (w *worker) start(blockInterval uint64) {
	atomic.StoreInt32(&w.running, 1)
	go w.mintLoop(blockInterval)
}

func (self *worker) mintBlock(now int64,blockInterval uint64) {
	engine, ok := self.engine.(*dpos.Dpos)
	if !ok {
		log.Error("Only the dpos engine was allowed")
		return
	}
//检查当前的validator是否为当前节点
	err := engine.CheckValidator(self.chain.CurrentBlock(), now,blockInterval)
	if err != nil {
		switch err {
		case dpos.ErrWaitForPrevBlock,
			dpos.ErrMintFutureBlock,
			dpos.ErrInvalidBlockValidator,
			dpos.ErrInvalidMintBlockTime:
			log.Debug("Failed to mint the block, while ", "err", err)
		default:
			log.Error("Failed to mint the block", "err", err)
		}
		return
	}
	self.createNewWork()
 /*
 //如果是：创建一个新的块任务
 工时，错误：=self.createnewwork（）
 如果犯错！= nIL{
  log.error（“未能创建新工作”，“err”，err）
  返回
 }
 //会对新块进行签名
 结果，错误：=self.engine.seal（self.chain，work.block，self.quitch）
 如果犯错！= nIL{
  log.error（“密封块失败”，“err”，err）
  返回
 }
 //将新块广播到相邻的节点
 self.recv<-&result工作，结果
 **/

}

func (self *worker) mintLoop(blockInterval uint64) {
	wt := time.Duration(int64(blockInterval))
//默认wt为“time.second”，accouding blockinterval获取等待时间
ticker := time.NewTicker(wt * time.Second /10).C   //香奈儿
	for {
		select {
		case now := <-ticker:
			atomic.StoreInt32(&self.newTxs, 0)
			self.mintBlock(now.Unix(),blockInterval)
		case <-self.stopper:
			close(self.quitCh)
			self.quitCh = make(chan struct{}, 1)
			self.stopper = make(chan struct{}, 1)
			return
		}
	}
}

//stop将运行状态设置为0。
func (w *worker) stop() {
	atomic.StoreInt32(&w.running, 0)
	close(w.stopper)
}

//is running返回一个指示工作者是否正在运行的指示器。
func (w *worker) isRunning() bool {
	return atomic.LoadInt32(&w.running) == 1
}

//CLOSE终止工作线程维护的所有后台线程，并清除缓冲通道。
//注意工人不支持多次关闭。
func (w *worker) close() {
	close(w.exitCh)
//清除缓冲通道
	for empty := false; !empty; {
		select {
		case <-w.resultCh:
		default:
			empty = true
		}
	}
}

//mainLoop是一个独立的goroutine，用于根据接收到的事件重新生成密封任务。
func (w *worker) mainLoop() {
	defer w.txsSub.Unsubscribe()
	defer w.chainHeadSub.Unsubscribe()

	for {
		select {
		case  <-w.chainHeadCh:
			close(w.quitCh)
			w.quitCh = make(chan struct{}, 1)

		case ev := <-w.txsCh:
//如果不挖掘，将事务应用于挂起状态。
//
//注意：收到的所有交易可能与交易不连续。
//已包含在当前挖掘块中。这些交易将
//自动消除。
			if !w.isRunning() && w.current != nil {
				w.mu.RLock()
				coinbase := w.coinbase
				w.mu.RUnlock()

				txs := make(map[common.Address]types.Transactions)
				for _, tx := range ev.Txs {
					acc, _ := types.Sender(w.current.signer, tx)
					txs[acc] = append(txs[acc], tx)
				}
				txset := types.NewTransactionsByPriceAndNonce(w.current.signer, txs)
				w.commitTransactions(txset, coinbase)
				w.updateSnapshot()
			}
			atomic.AddInt32(&w.newTxs, int32(len(ev.Txs)))

//系统停止
		case <-w.exitCh:
			return
		case <-w.txsSub.Err():
			return
		case <-w.chainHeadSub.Err():
			return
		}
	}
}

//Seal将密封任务推送到共识引擎并提交结果。
func (w *worker) seal(t *task, stop <-chan struct{}) {
	var (
		err error
		res *task
	)

	if w.skipSealHook != nil && w.skipSealHook(t) {
		return
	}


	if t.block, err = w.engine.Seal(w.chain, t.block, stop); t.block != nil {
		log.Info("Successfully sealed new block", "number", t.block.Number(), "hash", t.block.Hash(),
			"elapsed", common.PrettyDuration(time.Since(t.createdAt)))
		res = t
	} else {
		if err != nil {
			log.Warn("Block sealing failed", "err", err)
		}
		res = nil
	}
	select {
	case w.resultCh <- res:
	case <-w.exitCh:
	}
}

//taskloop是一个独立的goroutine，用于从生成器获取密封任务，并且
//把他们推到共识引擎。
func (w *worker) taskLoop() {
	var prev   common.Hash

//中断中止飞行中的密封任务。
	interrupt := func() {
		close(w.quitCh)
		w.quitCh = make(chan struct{}, 1)
	}
	for {
		select {
		case task := <-w.taskCh:
			if w.newTaskHook != nil {
				w.newTaskHook(task)
			}
//因重新提交而拒绝重复密封工作。
			if task.block.HashNoNonce() == prev {
				continue
			}
			interrupt()
			prev = task.block.HashNoNonce()
			go w.seal(task, w.quitCh)
		case <-w.exitCh:
			interrupt()
			return
		}
	}
}

//resultLoop是一个独立的goroutine，用于处理密封结果提交
//并将相关数据刷新到数据库。
func (w *worker) resultLoop() {
	for {
		select {
		case result := <-w.resultCh:
//收到空结果时短路。
			if result == nil  {
				continue
			}
//由于重新提交而收到重复结果时短路。
			block := result.block
			if w.chain.HasBlock(block.Hash(), block.NumberU64()) {
				continue
			}
//更新所有日志中的块哈希，因为它现在可用，而不是
//已创建单个交易的收据/日志。
			for _, r := range result.receipts {
				for _, l := range r.Logs {
					l.BlockHash = block.Hash()
				}
			}
			for _, log := range result.state.Logs() {
				log.BlockHash = block.Hash()
			}
//将块和状态提交到数据库。
			stat, err := w.chain.WriteBlockWithState(block, result.receipts, result.state)
			if err != nil {
				log.Error("Failed writing block to chain", "err", err)
				continue
			}
//广播块并宣布链插入事件
			w.mux.Post(core.NewMinedBlockEvent{Block: block})
			var (
				events []interface{}
				logs   = result.state.Logs()
			)
			switch stat {
			case core.CanonStatTy:
				events = append(events, core.ChainEvent{Block: block, Hash: block.Hash(), Logs: logs})
				events = append(events, core.ChainHeadEvent{Block: block})
			case core.SideStatTy:
//events=append（events，core.chainsideevent block:block）
			}
			w.chain.PostChainEvents(events, logs)

//将块插入一组挂起的结果循环以进行确认
			w.unconfirmed.Insert(block.NumberU64(), block.Hash())
			log.Info("Successfully sealed new block", "number", block.Number(), "hash", block.Hash())
		case <-w.exitCh:
			return
		}
	}
}

//makecurrent为当前循环创建新环境。
func (w *worker) makeCurrent(parent *types.Block, header *types.Header) error {
	state, err := w.chain.StateAt(parent.Root())
	if err != nil {
		return err
	}

	trieDB := state.Database().TrieDB()
//日志信息（“>>>>>>>>>>>”，“parent*types.block.header”，parent.header（），
//“types.block.header.dposContext”，parent.header（）.dposContext）
	dposContext, err := types.NewDposContextFromProto(trieDB, parent.Header().DposContext)
	if err != nil {
		return err
	}
	env := &environment{
		signer:    types.NewEIP155Signer(w.config.ChainID),
		state:     state,
		dposContext: dposContext,
		ancestors: mapset.NewSet(),
		family:    mapset.NewSet(),
		uncles:    mapset.NewSet(),
		header:    header,
	}
//处理08时，祖先包含07（快速块）
	for _, ancestor := range w.chain.GetBlocksFromHash(parent.Hash(), 7) {
		for _, uncle := range ancestor.Uncles() {
			env.family.Add(uncle.Hash())
		}
		env.family.Add(ancestor.Hash())
		env.ancestors.Add(ancestor.Hash())
	}

//跟踪返回错误的事务，以便删除它们
	env.tcount = 0
	w.current = env
	return nil
}

//commituncle将给定的块添加到叔叔块集，如果添加失败则返回错误。
func (w *worker) commitUncle(env *environment, uncle *types.Header) error {
	hash := uncle.Hash()
	if env.uncles.Contains(hash) {
		return fmt.Errorf("uncle not unique")
	}
	if !env.ancestors.Contains(uncle.ParentHash) {
		return fmt.Errorf("uncle's parent unknown (%x)", uncle.ParentHash[0:4])
	}
	if env.family.Contains(hash) {
		return fmt.Errorf("uncle already in family (%x)", hash)
	}
	env.uncles.Add(uncle.Hash())
	return nil
}

//更新快照更新挂起的快照块和状态。
//注意：此函数假定当前变量是线程安全的。
func (w *worker) updateSnapshot() {
	w.snapshotMu.Lock()
	defer w.snapshotMu.Unlock()

	var uncles []*types.Header
	w.current.uncles.Each(func(item interface{}) bool {
		hash, ok := item.(common.Hash)
		if !ok {
			return false
		}
		uncle, exist := w.possibleUncles[hash]
		if !exist {
			return false
		}
		uncles = append(uncles, uncle.Header())
		return false
	})

	w.snapshotBlock = types.NewBlock(
		w.current.header,
		w.current.txs,
		uncles,
		w.current.receipts,
	)

	w.snapshotState = w.current.state.Copy()
}


func (w *worker) commitTransaction(tx *types.Transaction, coinbase common.Address) ([]*types.Log, error) {
	snap := w.current.state.Snapshot()
	env := w.current
	dposSnap := env.dposContext.Snapshot()
	receipt, _, err := core.ApplyTransaction(w.config, env.dposContext, w.chain, &coinbase, w.current.gasPool, w.current.state, w.current.header, tx, &w.current.header.GasUsed, vm.Config{})
	if err != nil {
		w.current.state.RevertToSnapshot(snap)
		env.dposContext.RevertToSnapShot(dposSnap)
		return nil, err
	}
	w.current.txs = append(w.current.txs, tx)
	w.current.receipts = append(w.current.receipts, receipt)

	return receipt.Logs, nil
}

func (w *worker) commitTransactions(txs *types.TransactionsByPriceAndNonce, coinbase common.Address) bool {
//电流为零时短路
	if w.current == nil {
		return true
	}

	if w.current.gasPool == nil {
		w.current.gasPool = new(core.GasPool).AddGas(w.current.header.GasLimit)
	}

	var coalescedLogs []*types.Log

	for {

//如果我们没有足够的汽油进行进一步的交易，那么我们就完成了
		if w.current.gasPool.Gas() < params.TxGas {
			log.Trace("Not enough gas for further transactions", "have", w.current.gasPool, "want", params.TxGas)
			break
		}
//检索下一个事务，完成后中止
		tx := txs.Peek()
		if tx == nil {
			break
		}
//此处可以忽略错误。已检查错误
//在事务接受期间是事务池。
//
//我们使用EIP155签名者，不管当前的高频。
		from, _ := types.Sender(w.current.signer, tx)
//检查Tx是否受重播保护。如果我们不在EIP155高频
//阶段，开始忽略发送者，直到我们这样做。
		if tx.Protected() && !w.config.IsEIP155(w.current.header.Number) {
			log.Trace("Ignoring reply protected transaction", "hash", tx.Hash(), "eip155", w.config.EIP155Block)

			txs.Pop()
			continue
		}
//开始执行事务
		w.current.state.Prepare(tx.Hash(), common.Hash{}, w.current.tcount)

		logs, err := w.commitTransaction(tx, coinbase)
		switch err {
		case core.ErrGasLimitReached:
//从账户中弹出当前的天然气外交易，而不在下一个账户中移动。
			log.Trace("Gas limit exceeded for current block", "sender", from)
			txs.Pop()

		case core.ErrNonceTooLow:
//事务池和矿工之间的新头通知数据竞赛，shift
			log.Trace("Skipping transaction with low nonce", "sender", from, "nonce", tx.Nonce())
			txs.Shift()

		case core.ErrNonceTooHigh:
//交易池和矿工之间的REORG通知数据竞赛，跳过帐户=
			log.Trace("Skipping account with hight nonce", "sender", from, "nonce", tx.Nonce())
			txs.Pop()

		case nil:
//一切正常，收集日志并从同一帐户转入下一个事务
			coalescedLogs = append(coalescedLogs, logs...)
			w.current.tcount++
			txs.Shift()

		default:
//奇怪的错误，丢弃事务并将下一个事务处理到行中（注意，
//nonce-too-high子句将阻止我们无效执行）。
			log.Debug("Transaction failed, account skipped", "hash", tx.Hash(), "err", err)
			txs.Shift()
		}
	}

	if !w.isRunning() && len(coalescedLogs) > 0 {
//我们在采矿时不会推悬垂的日志。原因是
//当我们进行挖掘时，工人将每3秒重新生成一个挖掘块。
//为了避免推送重复的挂起日志，我们禁用挂起日志推送。

//复制，状态缓存日志，这些日志从挂起升级到挖掘。
//当块由当地矿工开采时，通过填充块散列进行记录。这个罐头
//如果在处理PendingLogSevent之前日志已“升级”，则会导致竞争条件。
		cpy := make([]*types.Log, len(coalescedLogs))
		for i, l := range coalescedLogs {
			cpy[i] = new(types.Log)
			*cpy[i] = *l
		}
		go w.mux.Post(core.PendingLogsEvent{Logs: cpy})
	}
//如果当前间隔较大，通知重新提交循环以减少重新提交间隔
//而不是用户指定的。
 /*中断！= nIL{
  w.重新提交adjustch<-&intervaladjust inc:false
 }*/

	return false
}

//CommitnewWork基于父块生成几个新的密封任务。
func (w *worker) createNewWork() (){
	w.mu.RLock()
	defer w.mu.RUnlock()

	tstart := time.Now()
	parent := w.chain.CurrentBlock()
	fmt.Print("+++++++++++++++++++++++++++++++++++++Genesis Block MaxvalidatorSize**********\n")
	Maxvalidatorsize  :=  w.chain.GenesisBlock().Header().MaxValidatorSize
	blockInterVal :=w.chain.GenesisBlock().Header().BlockInterval
	fmt.Printf("+++++++++++++++++++++++++++++++++++++MaxValidatorSize:%v +++++++++++++++++++++++++++++++++++++\n", int(Maxvalidatorsize))
	log.Info("Currently Set Dpos Configuration","Maxvalidatorsize", int(Maxvalidatorsize),"BlockInterval", blockInterVal)

	tstamp := tstart.Unix()
	if parent.Time().Cmp(new(big.Int).SetInt64(tstamp)) >= 0 {
		tstamp = parent.Time().Int64() + 1
	}
//这将确保我们今后不会走得太远。
	if now := time.Now().Unix(); tstamp > now+1 {
		wait := time.Duration(tstamp-now) * time.Second
		log.Info("Mining too far in the future", "wait", common.PrettyDuration(wait))
		time.Sleep(wait)
	}

	num := parent.Number()
	header := &types.Header{
		ParentHash: parent.Hash(),
		Number:     num.Add(num, common.Big1),
		GasLimit:   core.CalcGasLimit(parent),
		Extra:      w.extra,
		Time:       big.NewInt(tstamp),
MaxValidatorSize: Maxvalidatorsize,//为新的区块链生成新的标题
		BlockInterval:blockInterVal,
	}
//只有在我们的共识引擎运行时才设置coinbase（避免虚假的块奖励）
	if w.isRunning() {
		if w.coinbase == (common.Address{}) {
			log.Error("Refusing to mine without etherbase")
			return
		}
		header.Coinbase = w.coinbase
	}
	if err := w.engine.Prepare(w.chain, header); err != nil {
		log.Error("Failed to prepare header for mining", "err", err)
		return
	}
//如果我们关心DAO硬分叉，请检查是否覆盖额外的数据
	if daoBlock := w.config.DAOForkBlock; daoBlock != nil {
//检查块是否在fork额外覆盖范围内
		limit := new(big.Int).Add(daoBlock, params.DAOForkExtraRange)
		if header.Number.Cmp(daoBlock) >= 0 && header.Number.Cmp(limit) < 0 {
//根据我们是支持还是反对叉子，以不同的方式超越
			if w.config.DAOForkSupport {
				header.Extra = common.CopyBytes(params.DAOForkBlockExtra)
			} else if bytes.Equal(header.Extra, params.DAOForkBlockExtra) {
header.Extra = []byte{} //如果Miner反对，不要让它使用保留的额外数据
			}
		}
	}
//如果在一个奇怪的状态下开始挖掘，可能会发生这种情况。
	err := w.makeCurrent(parent, header)
	if err != nil {
		log.Error("Failed to create mining context", "err", err)
		return
	}
//创建当前工作任务并检查所需的任何分叉转换
	env := w.current
	if w.config.DAOForkSupport && w.config.DAOForkBlock != nil && w.config.DAOForkBlock.Cmp(header.Number) == 0 {
		misc.ApplyDAOHardFork(env.state)
	}

//计算新块的叔叔。
	var (
		uncles    []*types.Header
		badUncles []common.Hash
	)
	for hash, uncle := range w.possibleUncles {
		if len(uncles) == 2 {
			break
		}
		if err := w.commitUncle(env, uncle.Header()); err != nil {
			log.Trace("Bad uncle found and will be removed", "hash", hash)
			log.Trace(fmt.Sprint(uncle))

			badUncles = append(badUncles, hash)
		} else {
			log.Debug("Committing new uncle to block", "hash", hash)
			uncles = append(uncles, uncle.Header())
		}
	}
	for _, hash := range badUncles {
		delete(w.possibleUncles, hash)
	}
//用所有可用的挂起事务填充块。
	pending, err := w.eth.TxPool().Pending()
	if err != nil {
		log.Error("Failed to fetch pending transactions", "err", err)
		return
	}

//将挂起的事务拆分为本地和远程
	localTxs, remoteTxs := make(map[common.Address]types.Transactions), pending
	for _, account := range w.eth.TxPool().Locals() {
		if txs := remoteTxs[account]; len(txs) > 0 {
			delete(remoteTxs, account)
			localTxs[account] = txs
		}
	}
	if len(localTxs) > 0 {
		txs := types.NewTransactionsByPriceAndNonce(w.current.signer, localTxs)
		if w.commitTransactions(txs, w.coinbase) {
			return
		}
	}
	if len(remoteTxs) > 0 {
		txs := types.NewTransactionsByPriceAndNonce(w.current.signer, remoteTxs)
		if w.commitTransactions(txs, w.coinbase) {
			return
		}
	}
//
	fmt.Print("+++++++++++++++++++++++++++Judged dpos config: maxvalidatorsize++++++++++++++++++++++++\n")
	err1 := w.commit(uncles, w.fullTaskHook, tstart)
	if err1 != nil{
		log.Error(err1.Error())
		os.Exit(0)
	}
	return
}

//commit运行任何事务后状态修改，组装最终块
//如果共识引擎正在运行，则提交新的工作。
func (w *worker) commit(uncles []*types.Header, interval func(), start time.Time) error {
//在此深度复制收据以避免不同任务之间的交互。
	receipts := make([]*types.Receipt, len(w.current.receipts))
	for i, l := range w.current.receipts {
		receipts[i] = new(types.Receipt)
		*receipts[i] = *l
	}
	s := w.current.state.Copy()

	block, err := w.engine.Finalize(w.chain, w.current.header, s, w.current.txs, uncles, w.current.receipts, w.current.dposContext)
	if err != nil {
		return err
	}
	block.DposContext = w.current.dposContext
	if w.isRunning() {
		if interval != nil {
			interval()
		}
		select {
		case w.taskCh <- &task{receipts: receipts, state: s, block: block, createdAt: time.Now()}:
			w.unconfirmed.Shift(block.NumberU64() - 1)

			feesWei := new(big.Int)
			for i, tx := range block.Transactions() {
				feesWei.Add(feesWei, new(big.Int).Mul(new(big.Int).SetUint64(receipts[i].GasUsed), tx.GasPrice()))
			}
			feesEth := new(big.Float).Quo(new(big.Float).SetInt(feesWei), new(big.Float).SetInt(big.NewInt(params.Ether)))

			log.Info("Commit new mining work", "number", block.Number(), "uncles", len(uncles), "txs", w.current.tcount,
				"gas", block.GasUsed(), "fees", feesEth, "elapsed", common.PrettyDuration(time.Since(start)))

		case <-w.exitCh:
			log.Info("Worker has exited")
		}
	}

	w.updateSnapshot()

	return nil
}

