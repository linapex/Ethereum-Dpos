
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:34</date>
//</624342616183410688>


package core

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
)

//go：生成gencodec-类型genesis-场覆盖genesspecmarshaling-out gen_genesis.go
//go:generate gencodec-type genesiaccount-field override genesiaccountmarshaling-out gen_genesis_account.go


//go：生成gencodec-类型genesis-场覆盖genesspecmarshaling-out gen_genesis.go
//go:generate gencodec-type genesiaccount-field override genesiaccountmarshaling-out gen_genesis_account.go

var errGenesisNoConfig = errors.New("genesis has no chain configuration")

//Genesis指定头字段，Genesis块的状态。它也很难定义
//拨叉转换块通过链条配置。
type Genesis struct {
	Config     *params.ChainConfig `json:"config"`
	Nonce      uint64              `json:"nonce"`
	Timestamp  uint64              `json:"timestamp"`
	ExtraData  []byte              `json:"extraData"`
	GasLimit   uint64              `json:"gasLimit"   gencodec:"required"`
	Difficulty *big.Int            `json:"difficulty" gencodec:"required"`
	Mixhash    common.Hash         `json:"mixHash"`
	Coinbase   common.Address      `json:"coinbase"`
	Alloc      GenesisAlloc        `json:"alloc"      gencodec:"required"`

//这些字段用于一致性测试。请不要用它们
//在真正的创世纪块体中。
	Number     uint64      `json:"number"`
	GasUsed    uint64      `json:"gasUsed"`
	ParentHash common.Hash `json:"parentHash"`
}

//genesisalloc指定作为Genesis块一部分的初始状态。
type GenesisAlloc map[common.Address]GenesisAccount

func (ga *GenesisAlloc) UnmarshalJSON(data []byte) error {
	m := make(map[common.UnprefixedAddress]GenesisAccount)
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	*ga = make(GenesisAlloc)
	for addr, a := range m {
		(*ga)[common.Address(addr)] = a
	}
	return nil
}

//Genesiaccount是一个处于Genesis区块状态的账户。
type GenesisAccount struct {
	Code       []byte                      `json:"code,omitempty"`
	Storage    map[common.Hash]common.Hash `json:"storage,omitempty"`
	Balance    *big.Int                    `json:"balance" gencodec:"required"`
	Nonce      uint64                      `json:"nonce,omitempty"`
PrivateKey []byte                      `json:"secretKey,omitempty"` //为了测试
}

//gencodec的字段类型重写
type genesisSpecMarshaling struct {
	Nonce      math.HexOrDecimal64
	Timestamp  math.HexOrDecimal64
	ExtraData  hexutil.Bytes
	GasLimit   math.HexOrDecimal64
	GasUsed    math.HexOrDecimal64
	Number     math.HexOrDecimal64
	Difficulty *math.HexOrDecimal256
	Alloc      map[common.UnprefixedAddress]GenesisAccount
}

type genesisAccountMarshaling struct {
	Code       hexutil.Bytes
	Balance    *math.HexOrDecimal256
	Nonce      math.HexOrDecimal64
	Storage    map[storageJSON]storageJSON
	PrivateKey hexutil.Bytes
}

//storagejson表示一个256位字节数组，但当
//从十六进制解组。
type storageJSON common.Hash

func (h *storageJSON) UnmarshalText(text []byte) error {
	text = bytes.TrimPrefix(text, []byte("0x"))
	if len(text) > 64 {
		return fmt.Errorf("too many hex characters in storage key/value %q", text)
	}
offset := len(h) - len(text)/2 //左边的垫子
	if _, err := hex.Decode(h[offset:], text); err != nil {
		fmt.Println(err)
		return fmt.Errorf("invalid hex storage key/value %q", text)
	}
	return nil
}

func (h storageJSON) MarshalText() ([]byte, error) {
	return hexutil.Bytes(h[:]).MarshalText()
}

//尝试覆盖现有的
//与不相容的起源块。
type GenesisMismatchError struct {
	Stored, New common.Hash
}

func (e *GenesisMismatchError) Error() string {
	return fmt.Sprintf("database already contains an incompatible genesis block (have %x, new %x)", e.Stored[:8], e.New[:8])
}

//SetupGenesBlock在数据库中写入或更新Genesis块。
//将要使用的块是：
//
//创世纪=零创世纪！=零
//+—————————————————————————————————————
//DB没有Genesis主网络默认Genesis
//DB有来自DB的Genesis（如果兼容）
//
//如果存储链配置兼容（即不兼容），则将更新该配置
//在本地头块下面指定一个叉块）。如果发生冲突，
//错误是一个*params.configcompaterror，并返回新的未写入配置。
//
//返回的链配置从不为零。
func SetupGenesisBlock(db ethdb.Database, genesis *Genesis) (*params.ChainConfig, common.Hash, error) {
	if genesis != nil && genesis.Config == nil {
		return params.DposChainConfig, common.Hash{}, errGenesisNoConfig
	}

//如果没有存储的Genesis块，只需提交新块。
	stored := rawdb.ReadCanonicalHash(db, 0)
	if (stored == common.Hash{}) {
		if genesis == nil {
			log.Info("Writing default main-net genesis block")
			genesis = DefaultGenesisBlock()
		} else {
			log.Info("Writing custom genesis block")
		}
		block, err := genesis.Commit(db)
		return genesis.Config, block.Hash(), err
	}

//检查Genesis块是否已经写入。
	if genesis != nil {
		hash := genesis.ToBlock(nil).Hash()
		if hash != stored {
			return genesis.Config, hash, &GenesisMismatchError{stored, hash}
		}
	}

//获取现有的链配置。
	newcfg := genesis.configOrDefault(stored)
	storedcfg := rawdb.ReadChainConfig(db, stored)
	if storedcfg == nil {
		log.Warn("Found genesis block without chain config")
		rawdb.WriteChainConfig(db, stored, newcfg)
		return newcfg, stored, nil
	}
//特殊情况：如果没有新的，不要更改非主网链的现有配置
//提供了配置。这些链将得到所有的协议更改（以及compat错误）
//如果我们继续的话。
	if genesis == nil && stored != params.MainnetGenesisHash {
		return storedcfg, stored, nil
	}

//检查配置兼容性并写入配置。兼容性错误
//除非我们已经在零区，否则将返回给调用方。
	height := rawdb.ReadHeaderNumber(db, rawdb.ReadHeadHeaderHash(db))
	if height == nil {
		return newcfg, stored, fmt.Errorf("missing block number for head header hash")
	}
	compatErr := storedcfg.CheckCompatible(newcfg, *height)
	if compatErr != nil && *height != 0 && compatErr.RewindTo != 0 {
		return newcfg, stored, compatErr
	}
	rawdb.WriteChainConfig(db, stored, newcfg)
	return newcfg, stored, nil
}

func (g *Genesis) configOrDefault(ghash common.Hash) *params.ChainConfig {
	switch {
	case g != nil:
		return g.Config
	case ghash == params.MainnetGenesisHash:
		return params.MainnetChainConfig
	case ghash == params.TestnetGenesisHash:
		return params.TestnetChainConfig
	default:
		return params.DposChainConfig
	}
}

//toblock创建genesis块并写入genesis规范的状态
//到给定的数据库（如果没有则丢弃它）。
func (g *Genesis) ToBlock(db ethdb.Database) *types.Block {
	if db == nil {
		db = ethdb.NewMemDatabase()
	}
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(db))
	for addr, account := range g.Alloc {
		statedb.AddBalance(addr, account.Balance)
		statedb.SetCode(addr, account.Code)
		statedb.SetNonce(addr, account.Nonce)
		for key, value := range account.Storage {
			statedb.SetState(addr, key, value)
		}
	}
	root := statedb.IntermediateRoot(false)

//添加上下文
	dposContext := initGenesisDposContext(g, statedb.Database().TrieDB())
	dposContextProto := dposContext.ToProto()

	head := &types.Header{
		Number:     new(big.Int).SetUint64(g.Number),
		Nonce:      types.EncodeNonce(g.Nonce),
		Time:       new(big.Int).SetUint64(g.Timestamp),
		ParentHash: g.ParentHash,
		Extra:      g.ExtraData,
		GasLimit:   g.GasLimit,
		GasUsed:    g.GasUsed,
		Difficulty: g.Difficulty,
		MixDigest:  g.Mixhash,
		Coinbase:   g.Coinbase,
		Root:       root,
		DposContext: dposContextProto,
		MaxValidatorSize: g.Config.Dpos.MaxValidatorSize,
		BlockInterval: g.Config.Dpos.BlockInterval,
	}
	if g.GasLimit == 0 {
		head.GasLimit = params.GenesisGasLimit
	}
	if g.Difficulty == nil {
		head.Difficulty = params.GenesisDifficulty
	}
	statedb.Commit(false)
	statedb.Database().TrieDB().Commit(root, true)

	block := types.NewBlock(head, nil, nil, nil)
	block.DposContext = dposContext

	return block
}

//commit将Genesis规范的块和状态写入数据库。
//该块作为规范头块提交。
func (g *Genesis) Commit(db ethdb.Database) (*types.Block, error) {
	block := g.ToBlock(db)
	if block.Number().Sign() != 0 {
		return nil, fmt.Errorf("can't commit genesis block with number > 0")
	}
//添加上下文
	if _, err := block.DposContext.Commit(); err != nil {
		return nil, err
	}
	rawdb.WriteTd(db, block.Hash(), block.NumberU64(), g.Difficulty)
	rawdb.WriteBlock(db, block)
	rawdb.WriteReceipts(db, block.Hash(), block.NumberU64(), nil)
	rawdb.WriteCanonicalHash(db, block.Hash(), block.NumberU64())
	rawdb.WriteHeadBlockHash(db, block.Hash())
	rawdb.WriteHeadHeaderHash(db, block.Hash())

	config := g.Config
	if config == nil {
		config = params.DposChainConfig
	}
	rawdb.WriteChainConfig(db, block.Hash(), config)
	return block, nil
}

//mustcommit将genesis块和状态写入db，并在出错时惊慌失措。
//该块作为规范头块提交。
func (g *Genesis) MustCommit(db ethdb.Database) *types.Block {
	block, err := g.Commit(db)
	if err != nil {
		panic(err)
	}
	return block
}

//genesisblockfortesting创建并写入一个块，其中addr具有给定的wei平衡。
func GenesisBlockForTesting(db ethdb.Database, addr common.Address, balance *big.Int) *types.Block {
	g := Genesis{Alloc: GenesisAlloc{addr: {Balance: balance}}}
	return g.MustCommit(db)
}

//defaultgenesisblock返回以太坊主网Genesis块。
func DefaultGenesisBlock() *Genesis {
	return &Genesis{
		Config:     params.DposChainConfig,
		Nonce:      66,
		Timestamp:  1522052340,
		ExtraData:  hexutil.MustDecode("0x11bbe8db4e347b4e8c937c1c8370e4b5ed33adb3db69cbdb7a38e1e50b1b82fa"),
		GasLimit:   4712388,
		Difficulty: big.NewInt(17179869184),
		Alloc:      decodePrealloc(mainnetAllocData),
	}
}

//DefaultTestNetGenesBlock返回Ropsten Network Genesis块。
func DefaultTestnetGenesisBlock() *Genesis {
	return &Genesis{
		Config:     params.TestnetChainConfig,
		Nonce:      66,
		ExtraData:  hexutil.MustDecode("0x3535353535353535353535353535353535353535353535353535353535353535"),
		GasLimit:   16777216,
		Difficulty: big.NewInt(1048576),
		Alloc:      decodePrealloc(testnetAllocData),
	}
}

//defaultrinkebygenesblock返回rinkeby网络genesis块。
func DefaultRinkebyGenesisBlock() *Genesis {
	return &Genesis{
		Config:     params.RinkebyChainConfig,
		Timestamp:  1492009146,
		ExtraData:  hexutil.MustDecode("0x52657370656374206d7920617574686f7269746168207e452e436172746d616e42eb768f2244c8811c63729a21a3569731535f067ffc57839b00206d1ad20c69a1981b489f772031b279182d99e65703f0076e4812653aab85fca0f00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		GasLimit:   4700000,
		Difficulty: big.NewInt(1),
		Alloc:      decodePrealloc(rinkebyAllocData),
	}
}

//developergenessblock返回“geth--dev”genesis块。注意，这必须
//播种
func DeveloperGenesisBlock(period uint64, faucet common.Address) *Genesis {
//将默认期间覆盖到用户请求的期间
	config := *params.AllCliqueProtocolChanges
	config.Clique.Period = period

//组装并返回带有预编译和水龙头的Genesis
	return &Genesis{
		Config:     &config,
		ExtraData:  append(append(make([]byte, 32), faucet[:]...), make([]byte, 65)...),
		GasLimit:   6283185,
		Difficulty: big.NewInt(1),
		Alloc: map[common.Address]GenesisAccount{
common.BytesToAddress([]byte{1}): {Balance: big.NewInt(1)}, //恢复正常
common.BytesToAddress([]byte{2}): {Balance: big.NewInt(1)}, //沙256
common.BytesToAddress([]byte{3}): {Balance: big.NewInt(1)}, //里米德
common.BytesToAddress([]byte{4}): {Balance: big.NewInt(1)}, //身份
common.BytesToAddress([]byte{5}): {Balance: big.NewInt(1)}, //莫德斯普
common.BytesToAddress([]byte{6}): {Balance: big.NewInt(1)}, //埃卡德
common.BytesToAddress([]byte{7}): {Balance: big.NewInt(1)}, //蜕皮素
common.BytesToAddress([]byte{8}): {Balance: big.NewInt(1)}, //蜕变
			faucet: {Balance: new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(9))},
		},
	}
}

func decodePrealloc(data string) GenesisAlloc {
	var p []struct{ Addr, Balance *big.Int }
	if err := rlp.NewStream(strings.NewReader(data), 0).Decode(&p); err != nil {
		panic(err)
	}
	ga := make(GenesisAlloc, len(p))
	for _, account := range p {
		ga[common.BigToAddress(account.Addr)] = GenesisAccount{Balance: account.Balance}
	}
	return ga
}

func initGenesisDposContext(g *Genesis, db *trie.Database) *types.DposContext {
	dc, err := types.NewDposContextFromProto(db, &types.DposContextProto{})
	if err != nil {
		log.Error("initGenesisDposContext-NewDposContextFromProto-new", "DposContext", dc, "error", err)
		return nil
	}
	if g.Config != nil && g.Config.Dpos != nil && g.Config.Dpos.Validators != nil {
		dc.SetValidators(g.Config.Dpos.Validators)
		for _, validator := range g.Config.Dpos.Validators {
			dc.DelegateTrie().TryUpdate(append(validator.Bytes(), validator.Bytes()...), validator.Bytes())
			dc.CandidateTrie().TryUpdate(validator.Bytes(), validator.Bytes())
		}
	}
	return dc
}

