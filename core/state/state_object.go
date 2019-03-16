
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:34</date>
//</624342618179899392>


package state

import (
	"bytes"
	"fmt"
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

var emptyCodeHash = crypto.Keccak256(nil)

type Code []byte

func (self Code) String() string {
return string(self) //strings.join（反汇编（self），“”）
}

type Storage map[common.Hash]common.Hash

func (self Storage) String() (str string) {
	for key, value := range self {
		str += fmt.Sprintf("%X : %X\n", key, value)
	}

	return
}

func (self Storage) Copy() Storage {
	cpy := make(Storage)
	for key, value := range self {
		cpy[key] = value
	}

	return cpy
}

//StateObject表示正在修改的以太坊帐户。
//
//使用模式如下：
//首先需要获取一个状态对象。
//可以通过对象访问和修改帐户值。
//最后，调用committrie将修改后的存储trie写入数据库。
type stateObject struct {
	address  common.Address
addrHash common.Hash //帐户的以太坊地址哈希
	data     Account
	db       *StateDB

//数据库错误。
//状态对象由共识核心和虚拟机使用，它们是
//无法处理数据库级错误。发生的任何错误
//在数据库读取过程中，将在此处记忆并最终返回
//按statedb.commit。
	dbErr error

//编写高速缓存。
trie Trie //存储trie，在第一次访问时变为非nil
code Code //契约字节码，在加载代码时设置

cachedStorage Storage //存储项缓存以避免重复读取
dirtyStorage  Storage //需要刷新到磁盘的存储项

//缓存标志。
//当一个对象被标记为自杀时，它将从trie中删除。
//在状态转换的“更新”阶段。
dirtyCode bool //如果代码已更新，则为true
	suicided  bool
	deleted   bool
}

//empty返回帐户是否被视为空。
func (s *stateObject) empty() bool {
	return s.data.Nonce == 0 && s.data.Balance.Sign() == 0 && bytes.Equal(s.data.CodeHash, emptyCodeHash)
}

//账户是账户的以太坊共识代表。
//这些对象存储在主帐户trie中。
type Account struct {
	Nonce    uint64
	Balance  *big.Int
Root     common.Hash //存储树的merkle根
	CodeHash []byte
}

//NewObject创建一个状态对象。
func newObject(db *StateDB, address common.Address, data Account) *stateObject {
	if data.Balance == nil {
		data.Balance = new(big.Int)
	}
	if data.CodeHash == nil {
		data.CodeHash = emptyCodeHash
	}
	return &stateObject{
		db:            db,
		address:       address,
		addrHash:      crypto.Keccak256Hash(address[:]),
		data:          data,
		cachedStorage: make(Storage),
		dirtyStorage:  make(Storage),
	}
}

//encoderlp实现rlp.encoder。
func (c *stateObject) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, c.data)
}

//setError记住调用它时使用的第一个非零错误。
func (self *stateObject) setError(err error) {
	if self.dbErr == nil {
		self.dbErr = err
	}
}

func (self *stateObject) markSuicided() {
	self.suicided = true
}

func (c *stateObject) touch() {
	c.db.journal.append(touchChange{
		account: &c.address,
	})
	if c.address == ripemd {
//显式地将其放入脏缓存中，否则将从
//扁平轴颈。
		c.db.journal.dirty(c.address)
	}
}

func (c *stateObject) getTrie(db Database) Trie {
	if c.trie == nil {
		var err error
		c.trie, err = db.OpenStorageTrie(c.addrHash, c.data.Root)
		if err != nil {
			c.trie, _ = db.OpenStorageTrie(c.addrHash, common.Hash{})
			c.setError(fmt.Errorf("can't create storage trie: %v", err))
		}
	}
	return c.trie
}

//GetState返回帐户存储中的值。
func (self *stateObject) GetState(db Database, key common.Hash) common.Hash {
	value, exists := self.cachedStorage[key]
	if exists {
		return value
	}
//从数据库加载以防丢失。
	enc, err := self.getTrie(db).TryGet(key[:])
	if err != nil {
		self.setError(err)
		return common.Hash{}
	}
	if len(enc) > 0 {
		_, content, _, err := rlp.Split(enc)
		if err != nil {
			self.setError(err)
		}
		value.SetBytes(content)
	}
	self.cachedStorage[key] = value
	return value
}

//setstate更新帐户存储中的值。
func (self *stateObject) SetState(db Database, key, value common.Hash) {
	self.db.journal.append(storageChange{
		account:  &self.address,
		key:      key,
		prevalue: self.GetState(db, key),
	})
	self.setState(key, value)
}

func (self *stateObject) setState(key, value common.Hash) {
	self.cachedStorage[key] = value
	self.dirtyStorage[key] = value
}

//updatetrie将缓存的存储修改写入对象的存储trie。
func (self *stateObject) updateTrie(db Database) Trie {
	tr := self.getTrie(db)
	for key, value := range self.dirtyStorage {
		delete(self.dirtyStorage, key)
		if (value == common.Hash{}) {
			self.setError(tr.TryDelete(key[:]))
			continue
		}
//编码[]字节不能失败，可以忽略错误。
		v, _ := rlp.EncodeToBytes(bytes.TrimLeft(value[:], "\x00"))
		self.setError(tr.TryUpdate(key[:], v))
	}
	return tr
}

//updateRoot将trie根设置为当前根哈希
func (self *stateObject) updateRoot(db Database) {
	self.updateTrie(db)
	self.data.Root = self.trie.Hash()
}

//将对象的存储trie提交给db。
//这将更新trie根目录。
func (self *stateObject) CommitTrie(db Database) error {
	self.updateTrie(db)
	if self.dbErr != nil {
		return self.dbErr
	}
	root, err := self.trie.Commit(nil)
	if err == nil {
		self.data.Root = root
	}
	return err
}

//AddBalance从C的余额中删除金额。
//它用于将资金添加到转账的目的地帐户。
func (c *stateObject) AddBalance(amount *big.Int) {
//EIP158：我们必须检查对象的空性，以便帐户
//清除（0,0,0个对象）可以生效。
	if amount.Sign() == 0 {
		if c.empty() {
			c.touch()
		}

		return
	}
	c.SetBalance(new(big.Int).Add(c.Balance(), amount))
}

//次平衡从C的平衡中除去了数量。
//它用于从转账的原始账户中取出资金。
func (c *stateObject) SubBalance(amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	c.SetBalance(new(big.Int).Sub(c.Balance(), amount))
}

func (self *stateObject) SetBalance(amount *big.Int) {
	self.db.journal.append(balanceChange{
		account: &self.address,
		prev:    new(big.Int).Set(self.data.Balance),
	})
	self.setBalance(amount)
}

func (self *stateObject) setBalance(amount *big.Int) {
	self.data.Balance = amount
}

//将气体返回原点。由虚拟机或闭包使用
func (c *stateObject) ReturnGas(gas *big.Int) {}

func (self *stateObject) deepCopy(db *StateDB) *stateObject {
	stateObject := newObject(db, self.address, self.data)
	if self.trie != nil {
		stateObject.trie = db.db.CopyTrie(self.trie)
	}
	stateObject.code = self.code
	stateObject.dirtyStorage = self.dirtyStorage.Copy()
	stateObject.cachedStorage = self.dirtyStorage.Copy()
	stateObject.suicided = self.suicided
	stateObject.dirtyCode = self.dirtyCode
	stateObject.deleted = self.deleted
	return stateObject
}

//
//属性访问器
//

//返回合同/帐户的地址
func (c *stateObject) Address() common.Address {
	return c.address
}

//代码返回与此对象关联的合同代码（如果有）。
func (self *stateObject) Code(db Database) []byte {
	if self.code != nil {
		return self.code
	}
	if bytes.Equal(self.CodeHash(), emptyCodeHash) {
		return nil
	}
	code, err := db.ContractCode(self.addrHash, common.BytesToHash(self.CodeHash()))
	if err != nil {
		self.setError(fmt.Errorf("can't load code hash %x: %v", self.CodeHash(), err))
	}
	self.code = code
	return code
}

func (self *stateObject) SetCode(codeHash common.Hash, code []byte) {
	prevcode := self.Code(self.db.db)
	self.db.journal.append(codeChange{
		account:  &self.address,
		prevhash: self.CodeHash(),
		prevcode: prevcode,
	})
	self.setCode(codeHash, code)
}

func (self *stateObject) setCode(codeHash common.Hash, code []byte) {
	self.code = code
	self.data.CodeHash = codeHash[:]
	self.dirtyCode = true
}

func (self *stateObject) SetNonce(nonce uint64) {
	self.db.journal.append(nonceChange{
		account: &self.address,
		prev:    self.data.Nonce,
	})
	self.setNonce(nonce)
}

func (self *stateObject) setNonce(nonce uint64) {
	self.data.Nonce = nonce
}

func (self *stateObject) CodeHash() []byte {
	return self.data.CodeHash
}

func (self *stateObject) Balance() *big.Int {
	return self.data.Balance
}

func (self *stateObject) Nonce() uint64 {
	return self.data.Nonce
}

//从未调用，但必须存在才能使用StateObject
//作为一个也满足vm.contractRef的vm.account接口
//接口。界面很棒。
func (self *stateObject) Value() *big.Int {
	panic("Value on stateObject should never be called")
}

