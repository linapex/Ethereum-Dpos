
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:34</date>
//</624342617986961408>


//包状态在以太坊状态trie上提供一个缓存层。
package state

import (
	"fmt"
	"math/big"
	"sort"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
)

type revision struct {
	id           int
	journalIndex int
}

var (
//EmptyState是空状态trie项的已知哈希。
	emptyState = crypto.Keccak256Hash(nil)

//emptycode是空EVM字节码的已知哈希。
	emptyCode = crypto.Keccak256Hash(nil)
)

//以太坊协议中的statedbs用于存储任何内容
//在梅克尔特里亚。statedbs负责缓存和存储
//嵌套状态。它是要检索的常规查询接口：
//＊合同
//＊帐户
type StateDB struct {
	db   Database
	trie Trie

//此映射包含“活动”对象，在处理状态转换时将对其进行修改。
	stateObjects      map[common.Address]*stateObject
	stateObjectsDirty map[common.Address]struct{}

//数据库错误。
//状态对象由共识核心和虚拟机使用，它们是
//无法处理数据库级错误。发生的任何错误
//在数据库读取过程中，将在此处记忆并最终返回
//按statedb.commit。
	dbErr error

//退款柜台，也用于州过渡。
	refund uint64

	thash, bhash common.Hash
	txIndex      int
	logs         map[common.Hash][]*types.Log
	logSize      uint

	preimages map[common.Hash][]byte

//国家修改杂志。这是
//快照并还原为快照。
	journal        *journal
	validRevisions []revision
	nextRevisionId int

	lock sync.Mutex
}

//从给定的trie创建新状态。
func New(root common.Hash, db Database) (*StateDB, error) {
	tr, err := db.OpenTrie(root)
	if err != nil {
		return nil, err
	}
	return &StateDB{
		db:                db,
		trie:              tr,
		stateObjects:      make(map[common.Address]*stateObject),
		stateObjectsDirty: make(map[common.Address]struct{}),
		logs:              make(map[common.Hash][]*types.Log),
		preimages:         make(map[common.Hash][]byte),
		journal:           newJournal(),
	}, nil
}

//setError记住调用它时使用的第一个非零错误。
func (self *StateDB) setError(err error) {
	if self.dbErr == nil {
		self.dbErr = err
	}
}

func (self *StateDB) Error() error {
	return self.dbErr
}

//重置从状态数据库中清除所有短暂状态对象，但保留
//基础状态将尝试避免为下一个操作重新加载数据。
func (self *StateDB) Reset(root common.Hash) error {
	tr, err := self.db.OpenTrie(root)
	if err != nil {
		return err
	}
	self.trie = tr
	self.stateObjects = make(map[common.Address]*stateObject)
	self.stateObjectsDirty = make(map[common.Address]struct{})
	self.thash = common.Hash{}
	self.bhash = common.Hash{}
	self.txIndex = 0
	self.logs = make(map[common.Hash][]*types.Log)
	self.logSize = 0
	self.preimages = make(map[common.Hash][]byte)
	self.clearJournalAndRefund()
	return nil
}

func (self *StateDB) AddLog(log *types.Log) {
	self.journal.append(addLogChange{txhash: self.thash})

	log.TxHash = self.thash
	log.BlockHash = self.bhash
	log.TxIndex = uint(self.txIndex)
	log.Index = self.logSize
	self.logs[self.thash] = append(self.logs[self.thash], log)
	self.logSize++
}

func (self *StateDB) GetLogs(hash common.Hash) []*types.Log {
	return self.logs[hash]
}

func (self *StateDB) Logs() []*types.Log {
	var logs []*types.Log
	for _, lgs := range self.logs {
		logs = append(logs, lgs...)
	}
	return logs
}

//AdvPrimI图记录由VM看到的Sa3预图像。
func (self *StateDB) AddPreimage(hash common.Hash, preimage []byte) {
	if _, ok := self.preimages[hash]; !ok {
		self.journal.append(addPreimageChange{hash: hash})
		pi := make([]byte, len(preimage))
		copy(pi, preimage)
		self.preimages[hash] = pi
	}
}

//preimages返回已提交的sha3 preimages列表。
func (self *StateDB) Preimages() map[common.Hash][]byte {
	return self.preimages
}

func (self *StateDB) AddRefund(gas uint64) {
	self.journal.append(refundChange{prev: self.refund})
	self.refund += gas
}

//exist报告给定帐户地址是否存在于状态中。
//值得注意的是，对于自杀账户，这也会返回真值。
func (self *StateDB) Exist(addr common.Address) bool {
	return self.getStateObject(addr) != nil
}

//空返回状态对象是否不存在
//或根据EIP161规范为空（余额=nonce=代码=0）
func (self *StateDB) Empty(addr common.Address) bool {
	so := self.getStateObject(addr)
	return so == nil || so.empty()
}

//从给定地址检索余额，如果找不到对象，则检索0
func (self *StateDB) GetBalance(addr common.Address) *big.Int {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.Balance()
	}
	return common.Big0
}

func (self *StateDB) GetNonce(addr common.Address) uint64 {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.Nonce()
	}

	return 0
}

func (self *StateDB) GetCode(addr common.Address) []byte {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.Code(self.db)
	}
	return nil
}

func (self *StateDB) GetCodeSize(addr common.Address) int {
	stateObject := self.getStateObject(addr)
	if stateObject == nil {
		return 0
	}
	if stateObject.code != nil {
		return len(stateObject.code)
	}
	size, err := self.db.ContractCodeSize(stateObject.addrHash, common.BytesToHash(stateObject.CodeHash()))
	if err != nil {
		self.setError(err)
	}
	return size
}

func (self *StateDB) GetCodeHash(addr common.Address) common.Hash {
	stateObject := self.getStateObject(addr)
	if stateObject == nil {
		return common.Hash{}
	}
	return common.BytesToHash(stateObject.CodeHash())
}

func (self *StateDB) GetState(addr common.Address, bhash common.Hash) common.Hash {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.GetState(self.db, bhash)
	}
	return common.Hash{}
}

//数据库检索支持低级trie操作的低级数据库。
func (self *StateDB) Database() Database {
	return self.db
}

//storage trie返回帐户的存储trie。
//返回值是副本，对于不存在的帐户为零。
func (self *StateDB) StorageTrie(addr common.Address) Trie {
	stateObject := self.getStateObject(addr)
	if stateObject == nil {
		return nil
	}
	cpy := stateObject.deepCopy(self)
	return cpy.updateTrie(self.db)
}

func (self *StateDB) HasSuicided(addr common.Address) bool {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.suicided
	}
	return false
}

/*
 *设定器
 **/


//addbalance将金额添加到与addr关联的帐户。
func (self *StateDB) AddBalance(addr common.Address, amount *big.Int) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.AddBalance(amount)
	}
}

//子余额从与addr关联的帐户中减去金额。
func (self *StateDB) SubBalance(addr common.Address, amount *big.Int) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SubBalance(amount)
	}
}

func (self *StateDB) SetBalance(addr common.Address, amount *big.Int) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetBalance(amount)
	}
}

func (self *StateDB) SetNonce(addr common.Address, nonce uint64) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetNonce(nonce)
	}
}

func (self *StateDB) SetCode(addr common.Address, code []byte) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetCode(crypto.Keccak256Hash(code), code)
	}
}

func (self *StateDB) SetState(addr common.Address, key, value common.Hash) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetState(self.db, key, value)
	}
}

//自杀将指定帐户标记为自杀。
//这将清除帐户余额。
//
//在提交状态之前，帐户的状态对象仍然可用，
//GetStateObject将在自杀后返回非零帐户。
func (self *StateDB) Suicide(addr common.Address) bool {
	stateObject := self.getStateObject(addr)
	if stateObject == nil {
		return false
	}
	self.journal.append(suicideChange{
		account:     &addr,
		prev:        stateObject.suicided,
		prevbalance: new(big.Int).Set(stateObject.Balance()),
	})
	stateObject.markSuicided()
	stateObject.data.Balance = new(big.Int)

	return true
}

//
//设置、更新和删除状态对象方法。
//

//updateStateObject将给定对象写入trie。
func (self *StateDB) updateStateObject(stateObject *stateObject) {
	addr := stateObject.Address()
	data, err := rlp.EncodeToBytes(stateObject)
	if err != nil {
		panic(fmt.Errorf("can't encode object at %x: %v", addr[:], err))
	}
	self.setError(self.trie.TryUpdate(addr[:], data))
}

//DeleteStateObject从状态trie中删除给定的对象。
func (self *StateDB) deleteStateObject(stateObject *stateObject) {
	stateObject.deleted = true
	addr := stateObject.Address()
	self.setError(self.trie.TryDelete(addr[:]))
}

//检索由地址给定的状态对象。如果未找到，则返回nil。
func (self *StateDB) getStateObject(addr common.Address) (stateObject *stateObject) {
//喜欢“活”的对象。
	if obj := self.stateObjects[addr]; obj != nil {
		if obj.deleted {
			return nil
		}
		return obj
	}

//从数据库加载对象。
	enc, err := self.trie.TryGet(addr[:])
	if len(enc) == 0 {
		self.setError(err)
		return nil
	}
	var data Account
	if err := rlp.DecodeBytes(enc, &data); err != nil {
		log.Error("Failed to decode state object", "addr", addr, "err", err)
		return nil
	}
//插入到实况集。
	obj := newObject(self, addr, data)
	self.setStateObject(obj)
	return obj
}

func (self *StateDB) setStateObject(object *stateObject) {
	self.stateObjects[object.Address()] = object
}

//检索状态对象或创建新的状态对象（如果为零）。
func (self *StateDB) GetOrNewStateObject(addr common.Address) *stateObject {
	stateObject := self.getStateObject(addr)
	if stateObject == nil || stateObject.deleted {
		stateObject, _ = self.createObject(addr)
	}
	return stateObject
}

//CreateObject创建新的状态对象。如果有现有帐户
//给定的地址，将被覆盖并作为第二个返回值返回。
func (self *StateDB) createObject(addr common.Address) (newobj, prev *stateObject) {
	prev = self.getStateObject(addr)
	newobj = newObject(self, addr, Account{})
newobj.setNonce(0) //将对象设置为脏
	if prev == nil {
		self.journal.append(createObjectChange{account: &addr})
	} else {
		self.journal.append(resetObjectChange{prev: prev})
	}
	self.setStateObject(newobj)
	return newobj, prev
}

//CreateCount显式创建状态对象。如果有地址的状态对象
//已存在余额转入新账户。
//
//在EVM创建操作期间调用CreateCount。可能出现的情况是
//合同执行以下操作：
//
//1。将资金发送到sha（account++（nonce+1））。
//2。tx_create（sha（account++nonce））（注意，这会得到1的地址）
//
//保持平衡可确保乙醚不会消失。
func (self *StateDB) CreateAccount(addr common.Address) {
	new, prev := self.createObject(addr)
	if prev != nil {
		new.setBalance(prev.data.Balance)
	}
}

func (db *StateDB) ForEachStorage(addr common.Address, cb func(key, value common.Hash) bool) {
	so := db.getStateObject(addr)
	if so == nil {
		return
	}

//在对存储进行迭代时，首先检查缓存
	for h, value := range so.cachedStorage {
		cb(h, value)
	}

	it := trie.NewIterator(so.getTrie(db.db).NodeIterator(nil))
	for it.Next() {
//忽略缓存值
		key := common.BytesToHash(db.trie.GetKey(it.Key))
		if _, ok := so.cachedStorage[key]; !ok {
			cb(key, common.BytesToHash(it.Value))
		}
	}
}

//复制创建状态的深度独立副本。
//复制状态的快照无法应用于该副本。
func (self *StateDB) Copy() *StateDB {
	self.lock.Lock()
	defer self.lock.Unlock()

//复制所有基本字段，初始化内存字段
	state := &StateDB{
		db:                self.db,
		trie:              self.db.CopyTrie(self.trie),
		stateObjects:      make(map[common.Address]*stateObject, len(self.journal.dirties)),
		stateObjectsDirty: make(map[common.Address]struct{}, len(self.journal.dirties)),
		refund:            self.refund,
		logs:              make(map[common.Hash][]*types.Log, len(self.logs)),
		logSize:           self.logSize,
		preimages:         make(map[common.Hash][]byte),
		journal:           newJournal(),
	}
//复制脏状态、日志和预映像
	for addr := range self.journal.dirties {
//如文件所述[此处]（https://github.com/ethereum/go-ethereum/pull/16485 issuecomment-380438527）
//在定稿方法中，有一种情况是对象在日志中，而不是
//在StateObjects中：oog在拜占庭之前接触到ripemd。因此，我们需要检查
//零
		if object, exist := self.stateObjects[addr]; exist {
			state.stateObjects[addr] = object.deepCopy(state)
			state.stateObjectsDirty[addr] = struct{}{}
		}
	}
//上面，我们不复制实际的日记。这意味着，如果复制副本，
//上面的循环将是无操作，因为副本的日志是空的。
//因此，这里我们迭代StateObjects，以启用副本的副本
	for addr := range self.stateObjectsDirty {
		if _, exist := state.stateObjects[addr]; !exist {
			state.stateObjects[addr] = self.stateObjects[addr].deepCopy(state)
			state.stateObjectsDirty[addr] = struct{}{}
		}
	}

	for hash, logs := range self.logs {
		state.logs[hash] = make([]*types.Log, len(logs))
		copy(state.logs[hash], logs)
	}
	for hash, preimage := range self.preimages {
		state.preimages[hash] = preimage
	}
	return state
}

//快照返回状态的当前修订版的标识符。
func (self *StateDB) Snapshot() int {
	id := self.nextRevisionId
	self.nextRevisionId++
	self.validRevisions = append(self.validRevisions, revision{id, self.journal.length()})
	return id
}

//RevertToSnapshot恢复自给定修订之后所做的所有状态更改。
func (self *StateDB) RevertToSnapshot(revid int) {
//在有效快照的堆栈中查找快照。
	idx := sort.Search(len(self.validRevisions), func(i int) bool {
		return self.validRevisions[i].id >= revid
	})
	if idx == len(self.validRevisions) || self.validRevisions[idx].id != revid {
		panic(fmt.Errorf("revision id %v cannot be reverted", revid))
	}
	snapshot := self.validRevisions[idx].journalIndex

//重播日志以撤消更改并删除失效的快照
	self.journal.revert(self, snapshot)
	self.validRevisions = self.validRevisions[:idx]
}

//GetRefund返回退款计数器的当前值。
func (self *StateDB) GetRefund() uint64 {
	return self.refund
}

//通过移除自毁对象最终确定状态
//清除日记账和退款。
func (s *StateDB) Finalise(deleteEmptyObjects bool) {
	for addr := range s.journal.dirties {
		stateObject, exist := s.stateObjects[addr]
		if !exist {
//ripemd在德克萨斯州的1714175号区块“接触”，邮编：0x1237F737031E40BCDE4A8B7E717B2D15E3ECADFE49BB1BC71EE9DEB09C6FCF2
//Tx耗尽了气体，尽管“触摸”的概念在那里并不存在，但是
//触摸事件仍将记录在日志中。因为瑞普米德是一片特殊的雪花，
//即使日志被还原，它仍将在日志中保留。在这种特殊情况下，
//它可能存在于's.journal.dirties'中，但不存在于's.stateobjects'中。
//因此，我们可以在这里安全地忽略它
			continue
		}

		if stateObject.suicided || (deleteEmptyObjects && stateObject.empty()) {
			s.deleteStateObject(stateObject)
		} else {
			stateObject.updateRoot(s.db)
			s.updateStateObject(stateObject)
		}
		s.stateObjectsDirty[addr] = struct{}{}
	}
//使日记帐无效，因为不允许跨交易记录还原。
	s.clearJournalAndRefund()
}

//IntermediateRoot计算状态trie的当前根哈希。
//它在事务之间调用，以获取
//进入交易记录收据。
func (s *StateDB) IntermediateRoot(deleteEmptyObjects bool) common.Hash {
	s.Finalise(deleteEmptyObjects)
	return s.trie.Hash()
}

//准备设置当前事务哈希、索引和块哈希，即
//在EVM发出新状态日志时使用。
func (self *StateDB) Prepare(thash, bhash common.Hash, ti int) {
	self.thash = thash
	self.bhash = bhash
	self.txIndex = ti
}

func (s *StateDB) clearJournalAndRefund() {
	s.journal = newJournal()
	s.validRevisions = s.validRevisions[:0]
	s.refund = 0
}

//commit将状态写入内存中的基础trie数据库。
func (s *StateDB) Commit(deleteEmptyObjects bool) (root common.Hash, err error) {
	defer s.clearJournalAndRefund()

	for addr := range s.journal.dirties {
		s.stateObjectsDirty[addr] = struct{}{}
	}
//将对象提交到trie。
	for addr, stateObject := range s.stateObjects {
		_, isDirty := s.stateObjectsDirty[addr]
		switch {
		case stateObject.suicided || (isDirty && deleteEmptyObjects && stateObject.empty()):
//如果对象已被删除，请不要麻烦同步它。
//只需在trie中标记为删除即可。
			s.deleteStateObject(stateObject)
		case isDirty:
//编写与状态对象关联的任何合同代码
			if stateObject.code != nil && stateObject.dirtyCode {
				s.db.TrieDB().InsertBlob(common.BytesToHash(stateObject.CodeHash()), stateObject.code)
				stateObject.dirtyCode = false
			}
//将状态对象中的任何存储更改写入其存储trie。
			if err := stateObject.CommitTrie(s.db); err != nil {
				return common.Hash{}, err
			}
//更新主帐户trie中的对象。
			s.updateStateObject(stateObject)
		}
		delete(s.stateObjectsDirty, addr)
	}
//写入trie更改。
	root, err = s.trie.Commit(func(leaf []byte, parent common.Hash) error {
		var account Account
		if err := rlp.DecodeBytes(leaf, &account); err != nil {
			return nil
		}
		if account.Root != emptyState {
			s.db.TrieDB().Reference(account.Root, parent)
		}
		code := common.BytesToHash(account.CodeHash)
		if code != emptyCode {
			s.db.TrieDB().Reference(code, parent)
		}
		return nil
	})
	log.Debug("Trie cache stats after commit", "misses", trie.CacheMisses(), "unloads", trie.CacheUnloads())
	return root, err
}

