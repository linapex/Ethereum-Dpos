
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:38</date>
//</624342633354891264>


package eth

import (
	"math/big"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/eth/gasprice"
	"github.com/ethereum/go-ethereum/params"
)

//defaultconfig包含在以太坊主网上使用的默认设置。
var DefaultConfig = Config{
	SyncMode: downloader.FastSync,
	Ethash: ethash.Config{
		CacheDir:       "ethash",
		CachesInMem:    2,
		CachesOnDisk:   3,
		DatasetsInMem:  1,
		DatasetsOnDisk: 2,
	},
	NetworkId:     1,
	LightPeers:    100,
	DatabaseCache: 768,
	TrieCache:     256,
	TrieTimeout:   60 * time.Minute,
	MinerGasPrice: big.NewInt(18 * params.Shannon),
	MinerRecommit: 3 * time.Second,

	TxPool: core.DefaultTxPoolConfig,
	GPO: gasprice.Config{
		Blocks:     20,
		Percentile: 60,
	},
}

func init() {
	home := os.Getenv("HOME")
	if home == "" {
		if user, err := user.Current(); err == nil {
			home = user.HomeDir
		}
	}
	if runtime.GOOS == "windows" {
		DefaultConfig.Ethash.DatasetDir = filepath.Join(home, "AppData", "Ethash")
	} else {
		DefaultConfig.Ethash.DatasetDir = filepath.Join(home, ".ethash")
	}
}

//go:生成gencodec-type config-field override configmarshaling-formats toml-out gen_config.go

type Config struct {
//如果数据库为空，则插入Genesis块。
//如果为零，则使用以太坊主网块。
	Genesis *core.Genesis `toml:",omitempty"`

//协议选项
NetworkId uint64 //用于选择要连接的对等端的网络ID
	SyncMode  downloader.SyncMode
	NoPruning bool

//轻客户端选项
LightServ  int `toml:",omitempty"` //允许LES请求的最大时间百分比
LightPeers int `toml:",omitempty"` //最大LES客户端对等数

//数据库选项
	SkipBcVersionCheck bool `toml:"-"`
	DatabaseHandles    int  `toml:"-"`
	DatabaseCache      int
	TrieCache          int
	TrieTimeout        time.Duration

//Mining-related options
//etherbase common.address`toml:“，omitempty”`
	Validator    common.Address `toml:",omitempty"`
	Coinbase     common.Address `toml:",omitempty"`
	MinerThreads   int            `toml:",omitempty"`
	MinerNotify    []string       `toml:",omitempty"`
	MinerExtraData []byte         `toml:",omitempty"`
	MinerGasPrice  *big.Int
	MinerRecommit  time.Duration

//乙烯利选项
	Ethash ethash.Config

//事务池选项
	TxPool core.TxPoolConfig

//天然气价格Oracle选项
	GPO gasprice.Config

//允许跟踪虚拟机中的sha3 preimages
	EnablePreimageRecording bool

//其他选项
	DocRoot string `toml:"-"`
	Dpos      bool   `toml:"-"`
}

type configMarshaling struct {
	MinerExtraData hexutil.Bytes
}

