
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:34</date>
//</624342615671705600>


package core

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/misc"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/params"
)

//blockgen为测试创建块。
//有关详细说明，请参阅GenerateChain。
type BlockGen struct {
	i           int
	parent      *types.Block
	chain       []*types.Block
	chainReader consensus.ChainReader
	header      *types.Header
	statedb     *state.StateDB

	gasPool  *GasPool
	txs      []*types.Transaction
	receipts []*types.Receipt
	uncles   []*types.Header

	config *params.ChainConfig
	engine consensus.Engine
}

//setcoinbase设置生成块的coinbase。
//最多只能调用一次。
func (b *BlockGen) SetCoinbase(addr common.Address) {
	if b.gasPool != nil {
		if len(b.txs) > 0 {
			panic("coinbase must be set before adding transactions")
		}
		panic("coinbase can only be set once")
	}
	b.header.Coinbase = addr
	b.gasPool = new(GasPool).AddGas(b.header.GasLimit)
}

//setextra设置所生成块的额外数据字段。
func (b *BlockGen) SetExtra(data []byte) {
	b.header.Extra = data
}

//addtx将事务添加到生成的块中。如果没有Coinbase
//设置后，块的coinbase设置为零地址。
//
//如果无法执行事务，则addtx将暂停。除了
//协议规定的限制（气体限制等），有一些
//对交易内容的进一步限制
//补充。值得注意的是，依赖blockhash指令的合同代码
//在执行过程中会恐慌。
func (b *BlockGen) AddTx(tx *types.Transaction) {
	b.AddTxWithChain(nil, tx)
}

//addtxwithchain向生成的块添加事务。如果没有Coinbase
//设置后，块的coinbase设置为零地址。
//
//如果无法执行事务，则addtxwithchain将暂停。除了
//协议规定的限制（气体限制等），有一些
//对交易内容的进一步限制
//补充。如果合同代码依赖于blockhash指令，
//将返回链中的块。
func (b *BlockGen) AddTxWithChain(bc *BlockChain, tx *types.Transaction) {
	if b.gasPool == nil {
		b.SetCoinbase(common.Address{})
	}
	b.statedb.Prepare(tx.Hash(), common.Hash{}, len(b.txs))
	receipt, _, err := ApplyTransaction(b.config, nil, bc, &b.header.Coinbase, b.gasPool, b.statedb, b.header, tx, &b.header.GasUsed, vm.Config{})
	if err != nil {
		panic(err)
	}
	b.txs = append(b.txs, tx)
	b.receipts = append(b.receipts, receipt)
}
//number返回正在生成的块的块号。
func (b *BlockGen) Number() *big.Int {
	return new(big.Int).Set(b.header.Number)
}

//addUncheckedReceipt强制将收据添加到块，而不使用
//支持事务。
//
//添加取消选中的收据在实际使用时会导致共识失败。
//链处理。这最好与原始块插入结合使用。
func (b *BlockGen) AddUncheckedReceipt(receipt *types.Receipt) {
	b.receipts = append(b.receipts, receipt)
}

//txnonce返回的下一个有效事务nonce
//账户地址如果帐户不存在，它会恐慌。
func (b *BlockGen) TxNonce(addr common.Address) uint64 {
	if !b.statedb.Exist(addr) {
		panic("account does not exist")
	}
	return b.statedb.GetNonce(addr)
}

//AddUncle向生成的块添加一个叔叔头。
func (b *BlockGen) AddUncle(h *types.Header) {
	b.uncles = append(b.uncles, h)
}

//prevblock按数字返回以前生成的块。它恐慌，如果
//num大于或等于正在生成的块的数目。
//对于索引-1，prevblock返回给generatechain的父块。
func (b *BlockGen) PrevBlock(index int) *types.Block {
	if index >= b.i {
		panic("block index out of range")
	}
	if index == -1 {
		return b.parent
	}
	return b.chain[index]
}

//offsettime修改块的时间实例，隐式更改其
//相关难度。在没有分叉的情况下测试场景很有用
//直接与链条长度相连。
func (b *BlockGen) OffsetTime(seconds int64) {
	b.header.Time.Add(b.header.Time, new(big.Int).SetInt64(seconds))
	if b.header.Time.Cmp(b.parent.Header().Time) <= 0 {
		panic("block time out of range")
	}
	b.header.Difficulty = b.engine.CalcDifficulty(b.chainReader, b.header.Time.Uint64(), b.parent.Header())
}

//GenerateChain创建一个由n个块组成的链。第一个街区
//父级将是提供的父级。数据库用于存储
//中间状态，应包含父级的状态trie。
//
//使用新的块生成器调用generator函数
//每一个街区。任何添加到生成器的事务和叔叔
//成为区块的一部分。如果gen为零，则块将为空。
//他们的硬币库将是零地址。
//
//GenerateChain创建的块不包含有效的工作证明
//价值观。将它们插入区块链需要使用fakepow或
//类似的非验证性工作实施证明。
func GenerateChain(config *params.ChainConfig, parent *types.Block, engine consensus.Engine, db ethdb.Database, n int, gen func(int, *BlockGen)) ([]*types.Block, []types.Receipts) {
	if config == nil {
		config = params.TestChainConfig
	}
	blocks, receipts := make(types.Blocks, n), make([]types.Receipts, n)
	genblock := func(i int, parent *types.Block, statedb *state.StateDB) (*types.Block, types.Receipts) {
//托多（卡拉贝尔）：这是需要的集团，这取决于多个街区。
//不过，在这里旋转区块链还是很难看的。以某种方式摆脱它。
		blockchain, _ := NewBlockChain(db, nil, config, engine, vm.Config{})
		defer blockchain.Stop()

		b := &BlockGen{i: i, parent: parent, chain: blocks, chainReader: blockchain, statedb: statedb, config: config, engine: engine}
		b.header = makeHeader(b.chainReader, parent, statedb, b.engine)

//根据任何硬分叉规格改变状态并阻塞
		if daoBlock := config.DAOForkBlock; daoBlock != nil {
			limit := new(big.Int).Add(daoBlock, params.DAOForkExtraRange)
			if b.header.Number.Cmp(daoBlock) >= 0 && b.header.Number.Cmp(limit) < 0 {
				if config.DAOForkSupport {
					b.header.Extra = common.CopyBytes(params.DAOForkBlockExtra)
				}
			}
		}
		if config.DAOForkSupport && config.DAOForkBlock != nil && config.DAOForkBlock.Cmp(b.header.Number) == 0 {
			misc.ApplyDAOHardFork(statedb)
		}
//对块执行任何用户修改并完成它
		if gen != nil {
			gen(i, b)
		}

		if b.engine != nil {
			block, _ := b.engine.Finalize(b.chainReader, b.header, statedb, b.txs, b.uncles, b.receipts,parent.DposContext)
//将状态更改写入数据库
			root, err := statedb.Commit(config.IsEIP158(b.header.Number))
			if err != nil {
				panic(fmt.Sprintf("state write error: %v", err))
			}
			if err := statedb.Database().TrieDB().Commit(root, false); err != nil {
				panic(fmt.Sprintf("trie write error: %v", err))
			}
			return block, b.receipts
		}
		return nil, nil
	}
	for i := 0; i < n; i++ {
		statedb, err := state.New(parent.Root(), state.NewDatabase(db))
		if err != nil {
			panic(err)
		}
		block, receipt := genblock(i, parent, statedb)
		blocks[i] = block
		receipts[i] = receipt
		parent = block
	}
	return blocks, receipts
}

func makeHeader(chain consensus.ChainReader, parent *types.Block, state *state.StateDB, engine consensus.Engine) *types.Header {
	var time *big.Int
	if parent.Time() == nil {
		time = big.NewInt(10)
	} else {
time = new(big.Int).Add(parent.Time(), big.NewInt(10)) //阻塞时间固定为10秒
	}

	return &types.Header{
		Root:       state.IntermediateRoot(chain.Config().IsEIP158(parent.Number())),
		ParentHash: parent.Hash(),
		Coinbase:   parent.Coinbase(),
		Difficulty: engine.CalcDifficulty(chain, time.Uint64(), &types.Header{
			Number:     parent.Number(),
			Time:       new(big.Int).Sub(time, big.NewInt(10)),
			Difficulty: parent.Difficulty(),
			UncleHash:  parent.UncleHash(),
		}),
		GasLimit: CalcGasLimit(parent),
		Number:   new(big.Int).Add(parent.Number(), common.Big1),
		Time:     time,
	}
}

//MakeHeaderChain创建一个以父级为根的具有确定性的头链。
func makeHeaderChain(parent *types.Header, n int, engine consensus.Engine, db ethdb.Database, seed int) []*types.Header {
	blocks := makeBlockChain(types.NewBlockWithHeader(parent), n, engine, db, seed)
	headers := make([]*types.Header, len(blocks))
	for i, block := range blocks {
		headers[i] = block.Header()
	}
	return headers
}

//makeBlockchain创建了一个基于父级的确定性块链。
func makeBlockChain(parent *types.Block, n int, engine consensus.Engine, db ethdb.Database, seed int) []*types.Block {
	blocks, _ := GenerateChain(params.TestChainConfig, parent, engine, db, n, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{0: byte(seed), 19: byte(i)})
	})
	return blocks
}

