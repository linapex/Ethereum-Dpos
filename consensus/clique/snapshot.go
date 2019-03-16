
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:32</date>
//</624342610865033216>


package clique

import (
	"bytes"
	"encoding/json"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/params"
	lru "github.com/hashicorp/golang-lru"
)

//投票代表授权签名人修改
//授权列表。
type Vote struct {
Signer    common.Address `json:"signer"`    //投票的授权签署人
Block     uint64         `json:"block"`     //投票所投的区号（过期旧票）
Address   common.Address `json:"address"`   //正在投票更改其授权的帐户
Authorize bool           `json:"authorize"` //是否授权或取消对投票帐户的授权
}

//计票是一种简单的计票方式，用来保持当前的计票结果。投票赞成
//反对这项提议并不算在内，因为它等同于不投票。
type Tally struct {
Authorize bool `json:"authorize"` //投票是授权还是踢某人
Votes     int  `json:"votes"`     //到目前为止想要通过提案的票数
}

//快照是在给定时间点上投票的授权状态。
type Snapshot struct {
config   *params.CliqueConfig //微调行为的一致引擎参数
sigcache *lru.ARCCache        //缓存最近的块签名以加快ecrecover

Number  uint64                      `json:"number"`  //创建快照的块号
Hash    common.Hash                 `json:"hash"`    //创建快照的块哈希
Signers map[common.Address]struct{} `json:"signers"` //此时的授权签名人集合
Recents map[uint64]common.Address   `json:"recents"` //垃圾邮件保护的最近签名者集
Votes   []*Vote                     `json:"votes"`   //按时间顺序投票的名单
Tally   map[common.Address]Tally    `json:"tally"`   //当前投票计数以避免重新计算
}

//签名者实现排序接口以允许对地址列表进行排序
type signers []common.Address

func (s signers) Len() int           { return len(s) }
func (s signers) Less(i, j int) bool { return bytes.Compare(s[i][:], s[j][:]) < 0 }
func (s signers) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

//
//方法不初始化最近的签名者集，因此仅当用于
//
func newSnapshot(config *params.CliqueConfig, sigcache *lru.ARCCache, number uint64, hash common.Hash, signers []common.Address) *Snapshot {
	snap := &Snapshot{
		config:   config,
		sigcache: sigcache,
		Number:   number,
		Hash:     hash,
		Signers:  make(map[common.Address]struct{}),
		Recents:  make(map[uint64]common.Address),
		Tally:    make(map[common.Address]Tally),
	}
	for _, signer := range signers {
		snap.Signers[signer] = struct{}{}
	}
	return snap
}

//LoadSnapshot从数据库加载现有快照。
func loadSnapshot(config *params.CliqueConfig, sigcache *lru.ARCCache, db ethdb.Database, hash common.Hash) (*Snapshot, error) {
	blob, err := db.Get(append([]byte("clique-"), hash[:]...))
	if err != nil {
		return nil, err
	}
	snap := new(Snapshot)
	if err := json.Unmarshal(blob, snap); err != nil {
		return nil, err
	}
	snap.config = config
	snap.sigcache = sigcache

	return snap, nil
}

//存储将快照插入数据库。
func (s *Snapshot) store(db ethdb.Database) error {
	blob, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return db.Put(append([]byte("clique-"), s.Hash[:]...), blob)
}

//
func (s *Snapshot) copy() *Snapshot {
	cpy := &Snapshot{
		config:   s.config,
		sigcache: s.sigcache,
		Number:   s.Number,
		Hash:     s.Hash,
		Signers:  make(map[common.Address]struct{}),
		Recents:  make(map[uint64]common.Address),
		Votes:    make([]*Vote, len(s.Votes)),
		Tally:    make(map[common.Address]Tally),
	}
	for signer := range s.Signers {
		cpy.Signers[signer] = struct{}{}
	}
	for block, signer := range s.Recents {
		cpy.Recents[block] = signer
	}
	for address, tally := range s.Tally {
		cpy.Tally[address] = tally
	}
	copy(cpy.Votes, s.Votes)

	return cpy
}

//validvote返回在
//给定快照上下文（例如，不要尝试添加已授权的签名者）。
func (s *Snapshot) validVote(address common.Address, authorize bool) bool {
	_, signer := s.Signers[address]
	return (signer && !authorize) || (!signer && authorize)
}

//Cast在计票中添加了新的选票。
func (s *Snapshot) cast(address common.Address, authorize bool) bool {
//确保投票有意义
	if !s.validVote(address, authorize) {
		return false
	}
//将投票投到现有或新的计票中
	if old, ok := s.Tally[address]; ok {
		old.Votes++
		s.Tally[address] = old
	} else {
		s.Tally[address] = Tally{Authorize: authorize, Votes: 1}
	}
	return true
}

//
func (s *Snapshot) uncast(address common.Address, authorize bool) bool {
//如果没有计票结果，那是悬而未决的投票，就投吧。
	tally, ok := s.Tally[address]
	if !ok {
		return false
	}
//确保我们只还原已计数的投票
	if tally.Authorize != authorize {
		return false
	}
//否则恢复投票
	if tally.Votes > 1 {
		tally.Votes--
		s.Tally[address] = tally
	} else {
		delete(s.Tally, address)
	}
	return true
}

//应用通过将给定的头应用于创建新的授权快照
//原来的那个。
func (s *Snapshot) apply(headers []*types.Header) (*Snapshot, error) {
//不允许传入清除器代码的头
	if len(headers) == 0 {
		return s, nil
	}
//
	for i := 0; i < len(headers)-1; i++ {
		if headers[i+1].Number.Uint64() != headers[i].Number.Uint64()+1 {
			return nil, errInvalidVotingChain
		}
	}
	if headers[0].Number.Uint64() != s.Number+1 {
		return nil, errInvalidVotingChain
	}
//遍历头并创建新快照
	snap := s.copy()

	for _, header := range headers {
//删除检查点块上的所有投票
		number := header.Number.Uint64()
		if number%s.config.Epoch == 0 {
			snap.Votes = nil
			snap.Tally = make(map[common.Address]Tally)
		}
//从最近列表中删除最早的签名者，以允许其再次签名。
		if limit := uint64(len(snap.Signers)/2 + 1); number >= limit {
			delete(snap.Recents, number-limit)
		}
//解析授权密钥并检查签名者
		signer, err := ecrecover(header, s.sigcache)
		if err != nil {
			return nil, err
		}
		if _, ok := snap.Signers[signer]; !ok {
			return nil, errUnauthorized
		}
		for _, recent := range snap.Recents {
			if recent == signer {
				return nil, errUnauthorized
			}
		}
		snap.Recents[number] = signer

//
		for i, vote := range snap.Votes {
			if vote.Signer == signer && vote.Address == header.Coinbase {
//从缓存的计数中取消投票
				snap.uncast(vote.Address, vote.Authorize)

//取消按时间顺序排列的投票
				snap.Votes = append(snap.Votes[:i], snap.Votes[i+1:]...)
break //
			}
		}
//统计签名者的新投票
		var authorize bool
		switch {
		case bytes.Equal(header.Nonce[:], nonceAuthVote):
			authorize = true
		case bytes.Equal(header.Nonce[:], nonceDropVote):
			authorize = false
		default:
			return nil, errInvalidVote
		}
		if snap.cast(header.Coinbase, authorize) {
			snap.Votes = append(snap.Votes, &Vote{
				Signer:    signer,
				Block:     number,
				Address:   header.Coinbase,
				Authorize: authorize,
			})
		}
//如果投票通过，则更新签名者列表
		if tally := snap.Tally[header.Coinbase]; tally.Votes > len(snap.Signers)/2 {
			if tally.Authorize {
				snap.Signers[header.Coinbase] = struct{}{}
			} else {
				delete(snap.Signers, header.Coinbase)

//签名者列表收缩，删除所有剩余的最近缓存
				if limit := uint64(len(snap.Signers)/2 + 1); number >= limit {
					delete(snap.Recents, number-limit)
				}
//放弃取消授权签名者所投的任何以前的票
				for i := 0; i < len(snap.Votes); i++ {
					if snap.Votes[i].Signer == header.Coinbase {
//从缓存的计数中取消投票
						snap.uncast(snap.Votes[i].Address, snap.Votes[i].Authorize)

//取消按时间顺序排列的投票
						snap.Votes = append(snap.Votes[:i], snap.Votes[i+1:]...)

						i--
					}
				}
			}
//放弃对刚刚更改的帐户的任何以前的投票
			for i := 0; i < len(snap.Votes); i++ {
				if snap.Votes[i].Address == header.Coinbase {
					snap.Votes = append(snap.Votes[:i], snap.Votes[i+1:]...)
					i--
				}
			}
			delete(snap.Tally, header.Coinbase)
		}
	}
	snap.Number += uint64(len(headers))
	snap.Hash = headers[len(headers)-1].Hash()

	return snap, nil
}

//签名者按升序检索授权签名者列表。
func (s *Snapshot) signers() []common.Address {
	sigs := make([]common.Address, 0, len(s.Signers))
	for sig := range s.Signers {
		sigs = append(sigs, sig)
	}
	sort.Sort(signers(sigs))
	return sigs
}

//如果给定块高度的签名者依次是或不是，则Inturn返回。
func (s *Snapshot) inturn(number uint64, signer common.Address) bool {
	signers, offset := s.signers(), 0
	for offset < len(signers) && signers[offset] != signer {
		offset++
	}
	return (number % uint64(len(signers))) == uint64(offset)
}

