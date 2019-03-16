
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:32</date>
//</624342611083137024>


package dpos

import (
	"encoding/binary"
	"errors"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/trie"
	"math/rand"
	"sort"
	"fmt"

	"math/big"
)

//API是面向用户的RPC API，允许控制委托和投票
//委托股权证明机制
type API struct {
	chain consensus.ChainReader
	dpos  *Dpos
}

//getvalidators检索指定块上的验证程序列表
func (api *API) GetValidators(number *rpc.BlockNumber) ([]common.Address, error) {
	var header *types.Header
	if number == nil || *number == rpc.LatestBlockNumber {
		header = api.chain.CurrentHeader()
	} else {
		header = api.chain.GetHeaderByNumber(uint64(number.Int64()))
	}
	if header == nil {
		return nil, errUnknownBlock
	}

	trieDB := trie.NewDatabase(api.dpos.db)
	epochTrie, err := types.NewEpochTrie(header.DposContext.EpochHash, trieDB)

	if err != nil {
		return nil, err
	}
	dposContext := types.DposContext{}
	dposContext.SetEpoch(epochTrie)
	validators, err := dposContext.GetValidators()
	if err != nil {
		return nil, err
	}
	return validators, nil
}

//getconfirmedBlockNumber检索最新的不可逆块
func (api *API) GetConfirmedBlockNumber() (*big.Int, error) {
	var err error
	header := api.dpos.confirmedBlockHeader
	if header == nil {
		header, err = api.dpos.loadConfirmedBlockHeader(api.chain)
		if err != nil {
			return nil, err
		}
	}
	return header.Number, nil
}
func (ec *EpochContext) tryElect(genesis, parent *types.Header) error {

genesisEpoch := genesis.Time.Int64() / epochInterval   //GenesEpoch为0
	prevEpoch := parent.Time.Int64() / epochInterval
	currentEpoch := ec.TimeStamp / epochInterval

prevEpochIsGenesis := prevEpoch == genesisEpoch  		//布尔类型
	if prevEpochIsGenesis && prevEpoch < currentEpoch {
		prevEpoch = currentEpoch - 1
	}

	prevEpochBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(prevEpochBytes, uint64(prevEpoch))
	iter := trie.NewIterator(ec.DposContext.MintCntTrie().PrefixIterator(prevEpochBytes))
	from_genesis_maxsize :=  genesis.MaxValidatorSize
	fmt.Print("+++++++++++++++++++6666666666666666666++++++++++++++++++++++++++++\n")
	fmt.Print(from_genesis_maxsize)
//根据当前块和上一块的时间计算当前块和上一块是否属于同一个周期，
//如果是同一个周期，意味着当前块不是周期的第一块，不需要联系选举
//如果不是同一个周期，说明当前块是该周期的第一块，则联系投票
	fmt.Print("+++++++++++++++++++8888888++++++++++++++++++++++++++++\n")
	fmt.Print("Genesis init get maxvalidatorsize to kickoutValidator")
	for i := prevEpoch; i < currentEpoch; i++ {
//如果前一个世纪不是创世记，则启动非活动候选
//如果前一个周期不是创世周期，接触发奖金候选人规则
//出局规则主要是看上一周是否存在选人出局块少于规定值（50%），如果存在则出局
		if !prevEpochIsGenesis && iter.Next() {
			if err := ec.kickoutValidator(prevEpoch,genesis); err != nil {
				return err
			}
		}
//对候选人进行计票后按票数由高到低排序，选出前n个
//这里需要注意的是目前对于成为候选人没有门槛限制很容易被恶意攻击
		votes, err := ec.countVotes()
		if err != nil {
			return err
		}
//添加
		maxValidatorSize := int(genesis.MaxValidatorSize)
		safeSize := maxValidatorSize*2/3+1
		candidates := sortableAddresses{}
		for candidate, cnt := range votes {
			candidates = append(candidates, &sortableAddress{candidate, cnt})
		}
		if len(candidates) < safeSize {
//fmt.打印（“whteaaa！！！！！安全保险
			return errors.New("too few candidates")
		}
		sort.Sort(candidates)
		if len(candidates) > maxValidatorSize {
			candidates = candidates[:maxValidatorSize]
		}

//洗牌候选人
//乱验证人列表，由于使用seed是由父块的hash以及当前周编号组成，
//所以每个节点计算出来的验证人员列表也会一致
		seed := int64(binary.LittleEndian.Uint32(crypto.Keccak512(parent.Hash().Bytes()))) + i
		r := rand.New(rand.NewSource(seed))
		for i := len(candidates) - 1; i > 0; i-- {
			j := int(r.Int31n(int32(i + 1)))
			candidates[i], candidates[j] = candidates[j], candidates[i]
		}
		sortedValidators := make([]common.Address, 0)
		for _, candidate := range candidates {
			sortedValidators = append(sortedValidators, candidate.address)
		}

		epochTrie, _ := types.NewEpochTrie(common.Hash{}, ec.DposContext.DB())
		ec.DposContext.SetEpoch(epochTrie)
		ec.DposContext.SetValidators(sortedValidators)
		log.Info("Come to new epoch", "prevEpoch", i, "nextEpoch", i+1)
	}
	return nil
}





