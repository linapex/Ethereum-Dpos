
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:32</date>
//</624342611016028160>


//套餐共识实现不同的以太坊共识引擎。
package consensus

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
)

//ChainReader定义访问本地
//
type ChainReader interface {
//config检索区块链的链配置。
	Config() *params.ChainConfig
//当前头从本地链中检索当前头。
	CurrentHeader() *types.Header
//GetHeader按哈希和数字从数据库中检索块头。
	GetHeader(hash common.Hash, number uint64) *types.Header
//GetHeaderByNumber按编号从数据库中检索块头。
	GetHeaderByNumber(number uint64) *types.Header
//GetHeaderByHash通过其哈希从数据库中检索块头。
	GetHeaderByHash(hash common.Hash) *types.Header
//GetBlock按哈希和数字从数据库中检索块。
	GetBlock(hash common.Hash, number uint64) *types.Block
}

//引擎是一个算法不可知的共识引擎。
type Engine interface {
//作者检索创建给定帐户的以太坊地址
//块，如果达成一致，则可能不同于标题的coinbase
//引擎基于签名。
	Author(header *types.Header) (common.Address, error)

//验证标题检查标题是否符合
//给定发动机。可在此处选择或明确地验证密封件。
//通过VerifySeal方法。
//verifyheader（链式读卡器，header*types.header，seal bool）错误
	VerifyHeader(chain ChainReader,header  *types.Header, seal  bool,  interval uint64) error
//VerifyHeaders类似于VerifyHeader，但会验证一批头
//同时地。该方法返回退出通道以中止操作，并且
//用于检索异步验证的结果通道（顺序为
//输入切片）。
//verifyheaders（chain chainreader，headers[]*types.header，seals[]bool，blockinterval uint64）（chan<-struct，<-chan error）
	VerifyHeaders(chain ChainReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error)

//验证叔父验证给定区块的叔父是否符合共识
//给定引擎的规则。
	VerifyUncles(chain ChainReader, block *types.Block) error

//根据
//给定引擎的共识规则。
//verifyseal（链条阅读器，header*types.header）错误
	VerifySeal(chain ChainReader, header *types.Header, genesisheader *types.Header) error

//Prepare根据
//特定引擎的规则。更改是以内联方式执行的。
	Prepare(chain ChainReader, header *types.Header) error

//Finalize运行任何交易后状态修改（例如块奖励）
//组装最后一块。
//注意：块头和状态数据库可能会更新以反映
//在最终确定时达成共识的规则（例如集体奖励）。
	Finalize(chain ChainReader, header *types.Header, state *state.StateDB, txs []*types.Transaction,
		uncles []*types.Header, receipts []*types.Receipt,dposContext *types.DposContext) (*types.Block, error)

//Seal使用本地矿工的
//密封顶部。
	Seal(chain ChainReader, block *types.Block, stop <-chan struct{}) (*types.Block, error)
//Seal（链条阅读器，block*types.block，stop<-chan struct）（*types.block，错误）

//计算难度是难度调整算法。它又回到了困难中
//一个新的街区应该有。
	CalcDifficulty(chain ChainReader, time uint64, parent *types.Header) *big.Int

//API返回此共识引擎提供的RPC API。
	APIs(chain ChainReader) []rpc.API

//CLOSE终止由共识引擎维护的任何后台线程。
	Close() error
}

//POW是基于工作证明的共识引擎。
type PoW interface {
	Engine

//hashRate返回POW共识引擎的当前挖掘hashRate。
	Hashrate() float64
}

