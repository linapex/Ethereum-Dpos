
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:45</date>
//</624342662199119872>


package params

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

//Genesis散列以强制执行下面的配置。
var (
	MainnetGenesisHash = common.HexToHash("0xd4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3")
	TestnetGenesisHash = common.HexToHash("0x41941023680923e0fe4d74a34bdac8141f2540e3ae90623718e47d66d1ca4a2d")
	RinkebyGenesisHash = common.HexToHash("0x6341fd3daf94b748c72ced5a5b26028f2474f5f00d824504e4fa37a75767e177")
)

var (
//mainnetchainconfig是在主网络上运行节点的链参数。
	MainnetChainConfig = &ChainConfig{
		ChainID:             big.NewInt(1),
		HomesteadBlock:      big.NewInt(1150000),
		DAOForkBlock:        big.NewInt(1920000),
		DAOForkSupport:      true,
		EIP150Block:         big.NewInt(2463000),
		EIP150Hash:          common.HexToHash("0x2086799aeebeae135c246c65021c82b4e15a2c451340993aacfd2751886514f0"),
		EIP155Block:         big.NewInt(2675000),
		EIP158Block:         big.NewInt(2675000),
		ByzantiumBlock:      big.NewInt(4370000),
		ConstantinopleBlock: nil,
		Ethash:              new(EthashConfig),
	}
	DposChainConfig = &ChainConfig{
		ChainID:        big.NewInt(5),
		HomesteadBlock: big.NewInt(0),
		DAOForkBlock:   nil,
		DAOForkSupport: false,
		EIP150Block:    big.NewInt(0),
		EIP150Hash:     common.Hash{},
		EIP155Block:    big.NewInt(0),
		EIP158Block:    big.NewInt(0),
		ByzantiumBlock: big.NewInt(0),
		Dpos: &DposConfig{},
	}
//testNetChainConfig包含在Ropsten测试网络上运行节点的链参数。
	TestnetChainConfig = &ChainConfig{
		ChainID:             big.NewInt(3),
		HomesteadBlock:      big.NewInt(0),
		DAOForkBlock:        nil,
		DAOForkSupport:      true,
		EIP150Block:         big.NewInt(0),
		EIP150Hash:          common.HexToHash("0x41941023680923e0fe4d74a34bdac8141f2540e3ae90623718e47d66d1ca4a2d"),
		EIP155Block:         big.NewInt(10),
		EIP158Block:         big.NewInt(10),
		ByzantiumBlock:      big.NewInt(1700000),
		ConstantinopleBlock: nil,
		Ethash:              new(EthashConfig),
	}

//rinkebychainconfig包含在rinkeby测试网络上运行节点的链参数。
	RinkebyChainConfig = &ChainConfig{
		ChainID:             big.NewInt(4),
		HomesteadBlock:      big.NewInt(1),
		DAOForkBlock:        nil,
		DAOForkSupport:      true,
		EIP150Block:         big.NewInt(2),
		EIP150Hash:          common.HexToHash("0x9b095b36c15eaf13044373aef8ee0bd3a382a5abb92e402afa44b8249c3a90e9"),
		EIP155Block:         big.NewInt(3),
		EIP158Block:         big.NewInt(3),
		ByzantiumBlock:      big.NewInt(1035301),
		ConstantinopleBlock: nil,
		Clique: &CliqueConfig{
			Period: 15,
			Epoch:  30000,
		},
	}

//AllethashProtocolChanges包含所有引入的协议更改（EIP）
//并被以太坊核心开发者接受进入了ethash共识。
//
//此配置有意不使用键字段强制任何人
//向配置中添加标志也必须设置这些字段。
	AllEthashProtocolChanges = &ChainConfig{big.NewInt(1337), big.NewInt(0), nil, false, big.NewInt(0), common.Hash{}, big.NewInt(0), big.NewInt(0), big.NewInt(0), nil, new(EthashConfig), nil,nil}

//AllCliqueProtocolChanges包含引入的每个协议更改（EIP）
//并被以太坊核心开发者接纳为集团共识。
//
//此配置有意不使用键字段强制任何人
//向配置中添加标志也必须设置这些字段。
	AllCliqueProtocolChanges = &ChainConfig{big.NewInt(1337), big.NewInt(0), nil, false, big.NewInt(0), common.Hash{}, big.NewInt(0), big.NewInt(0), big.NewInt(0), nil, nil, &CliqueConfig{Period: 0, Epoch: 30000},nil}

	TestChainConfig = &ChainConfig{big.NewInt(1), big.NewInt(0), nil, false, big.NewInt(0), common.Hash{}, big.NewInt(0), big.NewInt(0), big.NewInt(0), nil, new(EthashConfig), nil,nil}
	TestRules       = TestChainConfig.Rules(new(big.Int))
)

//chainconfig是决定区块链设置的核心配置。
//
//chainconfig以块为单位存储在数据库中。这意味着
//任何一个网络，通过它的起源块来识别，都可以有它自己的
//一组配置选项。
type ChainConfig struct {
ChainID *big.Int `json:"chainId"` //chainID标识当前链并用于重播保护

HomesteadBlock *big.Int `json:"homesteadBlock,omitempty"` //宅基地开关块（零=无叉，0=已宅基地）

DAOForkBlock   *big.Int `json:"daoForkBlock,omitempty"`   //道硬叉开关块（零=无叉）
DAOForkSupport bool     `json:"daoForkSupport,omitempty"` //节点是支持还是反对DAO硬叉

//EIP150实施天然气价格变化（https://github.com/ethereum/eips/issues/150）
EIP150Block *big.Int    `json:"eip150Block,omitempty"` //EIP150 HF模块（零=无拨叉）
EIP150Hash  common.Hash `json:"eip150Hash,omitempty"`  //EIP150 HF哈希（仅限于标题客户，因为只有天然气价格发生变化）

EIP155Block *big.Int `json:"eip155Block,omitempty"` //EIP155高频阻滞
EIP158Block *big.Int `json:"eip158Block,omitempty"` //EIP158高频阻滞

ByzantiumBlock      *big.Int `json:"byzantiumBlock,omitempty"`      //拜占庭开关块（nil=无分叉，0=已在拜占庭）
ConstantinopleBlock *big.Int `json:"constantinopleBlock,omitempty"` //君士坦丁堡开关块（nil=无叉，0=已激活）

//各种共识引擎
	Ethash *EthashConfig `json:"ethash,omitempty"`
	Clique *CliqueConfig `json:"clique,omitempty"`
	Dpos   *DposConfig 	 `json:"dpos,omitempty"`

}
//DPOSconfig是授权的基于桩的密封证明的共识引擎配置。
type DposConfig struct {
Validators []common.Address `json:"validators"` //Genesis验证程序列表
MaxValidatorSize uint64		`json:"maxValidatorSize"` //Genesis最大验证大小
	BlockInterval 	 uint64		`json:"blockInterval"`
}

//字符串实现Stringer接口，返回共识引擎详细信息。
func (d *DposConfig) String() string {
	return "dpos"
}
//ethashconfig是基于工作证明的密封的共识引擎配置。
type EthashConfig struct{}

//字符串实现Stringer接口，返回共识引擎详细信息。
func (c *EthashConfig) String() string {
	return "ethash"
}

//cliqueconfig是基于权威的密封证明的共识引擎配置。
type CliqueConfig struct {
Period uint64 `json:"period"` //要强制执行的块之间的秒数
Epoch  uint64 `json:"epoch"`  //重置投票和检查点的epoch长度
}

//字符串实现Stringer接口，返回共识引擎详细信息。
func (c *CliqueConfig) String() string {
	return "clique"
}

//字符串实现fmt.Stringer接口。
func (c *ChainConfig) String() string {
	var engine interface{}
	switch {
	case c.Ethash != nil:
		engine = c.Ethash
	case c.Clique != nil:
		engine = c.Clique
	default:
		engine = "unknown"
	}
	return fmt.Sprintf("{ChainID: %v Homestead: %v DAO: %v DAOSupport: %v EIP150: %v EIP155: %v EIP158: %v Byzantium: %v Constantinople: %v Engine: %v}",
		c.ChainID,
		c.HomesteadBlock,
		c.DAOForkBlock,
		c.DAOForkSupport,
		c.EIP150Block,
		c.EIP155Block,
		c.EIP158Block,
		c.ByzantiumBlock,
		c.ConstantinopleBlock,
		engine,
	)
}

//is homestead返回num是否等于homestead块或更大。
func (c *ChainConfig) IsHomestead(num *big.Int) bool {
	return isForked(c.HomesteadBlock, num)
}

//IsDaoFork返回num是否等于或大于dao fork块。
func (c *ChainConfig) IsDAOFork(num *big.Int) bool {
	return isForked(c.DAOForkBlock, num)
}

//ISEIP150返回num是否等于eip150 fork块或更大。
func (c *ChainConfig) IsEIP150(num *big.Int) bool {
	return isForked(c.EIP150Block, num)
}

//ISEIP155返回num是否等于eip155 fork块或更大。
func (c *ChainConfig) IsEIP155(num *big.Int) bool {
	return isForked(c.EIP155Block, num)
}

//ISEIP158返回num是否等于eip158 fork块或更大。
func (c *ChainConfig) IsEIP158(num *big.Int) bool {
	return isForked(c.EIP158Block, num)
}

//IsByzantium返回num是否等于或大于Byzantium fork块。
func (c *ChainConfig) IsByzantium(num *big.Int) bool {
	return isForked(c.ByzantiumBlock, num)
}

//is constantinople返回num是否等于或大于constantinople fork块。
func (c *ChainConfig) IsConstantinople(num *big.Int) bool {
	return isForked(c.ConstantinopleBlock, num)
}

//Gastable返回与当前阶段（宅基地或宅基地重印）对应的气体表。
//
//在任何情况下，返回的加斯塔布尔的字段都不应该更改。
func (c *ChainConfig) GasTable(num *big.Int) GasTable {
	if num == nil {
		return GasTableHomestead
	}
	switch {
	case c.IsConstantinople(num):
		return GasTableConstantinople
	case c.IsEIP158(num):
		return GasTableEIP158
	case c.IsEIP150(num):
		return GasTableEIP150
	default:
		return GasTableHomestead
	}
}

//检查兼容检查是否已导入计划的分叉转换
//链配置不匹配。
func (c *ChainConfig) CheckCompatible(newcfg *ChainConfig, height uint64) *ConfigCompatError {
	bhead := new(big.Int).SetUint64(height)

//迭代checkCompatible以查找最低的冲突。
	var lasterr *ConfigCompatError
	for {
		err := c.checkCompatible(newcfg, bhead)
		if err == nil || (lasterr != nil && err.RewindTo == lasterr.RewindTo) {
			break
		}
		lasterr = err
		bhead.SetUint64(err.RewindTo)
	}
	return lasterr
}

func (c *ChainConfig) checkCompatible(newcfg *ChainConfig, head *big.Int) *ConfigCompatError {
	if isForkIncompatible(c.HomesteadBlock, newcfg.HomesteadBlock, head) {
		return newCompatError("Homestead fork block", c.HomesteadBlock, newcfg.HomesteadBlock)
	}
	if isForkIncompatible(c.DAOForkBlock, newcfg.DAOForkBlock, head) {
		return newCompatError("DAO fork block", c.DAOForkBlock, newcfg.DAOForkBlock)
	}
	if c.IsDAOFork(head) && c.DAOForkSupport != newcfg.DAOForkSupport {
		return newCompatError("DAO fork support flag", c.DAOForkBlock, newcfg.DAOForkBlock)
	}
	if isForkIncompatible(c.EIP150Block, newcfg.EIP150Block, head) {
		return newCompatError("EIP150 fork block", c.EIP150Block, newcfg.EIP150Block)
	}
	if isForkIncompatible(c.EIP155Block, newcfg.EIP155Block, head) {
		return newCompatError("EIP155 fork block", c.EIP155Block, newcfg.EIP155Block)
	}
	if isForkIncompatible(c.EIP158Block, newcfg.EIP158Block, head) {
		return newCompatError("EIP158 fork block", c.EIP158Block, newcfg.EIP158Block)
	}
	if c.IsEIP158(head) && !configNumEqual(c.ChainID, newcfg.ChainID) {
		return newCompatError("EIP158 chain ID", c.EIP158Block, newcfg.EIP158Block)
	}
	if isForkIncompatible(c.ByzantiumBlock, newcfg.ByzantiumBlock, head) {
		return newCompatError("Byzantium fork block", c.ByzantiumBlock, newcfg.ByzantiumBlock)
	}
	if isForkIncompatible(c.ConstantinopleBlock, newcfg.ConstantinopleBlock, head) {
		return newCompatError("Constantinople fork block", c.ConstantinopleBlock, newcfg.ConstantinopleBlock)
	}
	return nil
}

//如果无法将在s1上计划的分叉重新计划为，则IsForkCompatible返回true
//阻塞s2，因为头已经过了分叉。
func isForkIncompatible(s1, s2, head *big.Int) bool {
	return (isForked(s1, head) || isForked(s2, head)) && !configNumEqual(s1, s2)
}

//isforked返回在块S调度的分叉在给定的头块是否处于活动状态。
func isForked(s, head *big.Int) bool {
	if s == nil || head == nil {
		return false
	}
	return s.Cmp(head) <= 0
}

func configNumEqual(x, y *big.Int) bool {
	if x == nil {
		return y == nil
	}
	if y == nil {
		return x == nil
	}
	return x.Cmp(y) == 0
}

//如果本地存储的区块链初始化为
//可以改变过去的chainconfig。
type ConfigCompatError struct {
	What string
//存储和新配置的块编号
	StoredConfig, NewConfig *big.Int
//要更正错误，必须将本地链重绕到的块编号
	RewindTo uint64
}

func newCompatError(what string, storedblock, newblock *big.Int) *ConfigCompatError {
	var rew *big.Int
	switch {
	case storedblock == nil:
		rew = newblock
	case newblock == nil || storedblock.Cmp(newblock) < 0:
		rew = storedblock
	default:
		rew = newblock
	}
	err := &ConfigCompatError{what, storedblock, newblock, 0}
	if rew != nil && rew.Sign() > 0 {
		err.RewindTo = rew.Uint64() - 1
	}
	return err
}

func (err *ConfigCompatError) Error() string {
	return fmt.Sprintf("mismatching %s in database (have %d, want %d, rewindto %d)", err.What, err.StoredConfig, err.NewConfig, err.RewindTo)
}

//规则包装了chainconfig，只是语法上的糖分，或者可以用于函数
//不包含或不需要有关块的信息的。
//
//规则是一次性接口，这意味着不应在转换之间使用它
//阶段。
type Rules struct {
	ChainID                                   *big.Int
	IsHomestead, IsEIP150, IsEIP155, IsEIP158 bool
	IsByzantium                               bool
}

//规则确保C的chainID不为零。
func (c *ChainConfig) Rules(num *big.Int) Rules {
	chainID := c.ChainID
	if chainID == nil {
		chainID = new(big.Int)
	}
	return Rules{ChainID: new(big.Int).Set(chainID), IsHomestead: c.IsHomestead(num), IsEIP150: c.IsEIP150(num), IsEIP155: c.IsEIP155(num), IsEIP158: c.IsEIP158(num), IsByzantium: c.IsByzantium(num)}
}

