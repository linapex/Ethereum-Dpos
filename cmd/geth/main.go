
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:29</date>
//</624342594570162176>


//geth是以太坊的官方命令行客户端。
package main

import (
	"fmt"
	"math"
	"os"
	"runtime"
	godebug "runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/elastic/gosigar"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/console"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/internal/debug"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/node"
	"gopkg.in/urfave/cli.v1"
)

const (
clientIdentifier = "geth" //要通过网络进行广告的客户端标识符
)

var (
//git sha1提交发布的哈希（通过链接器标志设置）
	gitCommit = ""
//保存所有命令和标志的应用程序。
	app = utils.NewApp(gitCommit, "the go-ethereum command line interface")
//配置节点的标志
	nodeFlags = []cli.Flag{
		utils.IdentityFlag,
		utils.UnlockedAccountFlag,
		utils.PasswordFileFlag,
		utils.BootnodesFlag,
		utils.BootnodesV4Flag,
		utils.BootnodesV5Flag,
		utils.DataDirFlag,
		utils.KeyStoreDirFlag,
		utils.NoUSBFlag,
		utils.DashboardEnabledFlag,
		utils.DashboardAddrFlag,
		utils.DashboardPortFlag,
		utils.DashboardRefreshFlag,
		utils.EthashCacheDirFlag,
		utils.EthashCachesInMemoryFlag,
		utils.EthashCachesOnDiskFlag,
		utils.EthashDatasetDirFlag,
		utils.EthashDatasetsInMemoryFlag,
		utils.EthashDatasetsOnDiskFlag,
		utils.TxPoolLocalsFlag,
		utils.TxPoolNoLocalsFlag,
		utils.TxPoolJournalFlag,
		utils.TxPoolRejournalFlag,
		utils.TxPoolPriceLimitFlag,
		utils.TxPoolPriceBumpFlag,
		utils.TxPoolAccountSlotsFlag,
		utils.TxPoolGlobalSlotsFlag,
		utils.TxPoolAccountQueueFlag,
		utils.TxPoolGlobalQueueFlag,
		utils.TxPoolLifetimeFlag,
		utils.SyncModeFlag,
		utils.GCModeFlag,
		utils.LightServFlag,
		utils.LightPeersFlag,
		utils.LightKDFFlag,
		utils.CacheFlag,
		utils.CacheDatabaseFlag,
		utils.CacheGCFlag,
		utils.TrieCacheGenFlag,
		utils.ListenPortFlag,
		utils.MaxPeersFlag,
		utils.MaxPendingPeersFlag,
		utils.MiningEnabledFlag,
		utils.MinerThreadsFlag,
		utils.MinerLegacyThreadsFlag,
		utils.MinerNotifyFlag,
		utils.MinerGasTargetFlag,
		utils.MinerLegacyGasTargetFlag,
		utils.MinerGasPriceFlag,
		utils.MinerLegacyGasPriceFlag,
//utils.mineretherbaseflag（实用程序.mineretherbaseflag）
		utils.MinerLegacyEtherbaseFlag,
		utils.MinerExtraDataFlag,
		utils.MinerLegacyExtraDataFlag,
		utils.MinerRecommitIntervalFlag,
		utils.NATFlag,
		utils.NoDiscoverFlag,
		utils.DiscoveryV5Flag,
		utils.NetrestrictFlag,
		utils.NodeKeyFileFlag,
		utils.NodeKeyHexFlag,
		utils.DeveloperFlag,
		utils.DeveloperPeriodFlag,
		utils.TestnetFlag,
		utils.RinkebyFlag,
		utils.VMEnableDebugFlag,
		utils.NetworkIdFlag,
		utils.RPCCORSDomainFlag,
		utils.RPCVirtualHostsFlag,
		utils.EthStatsURLFlag,
		utils.MetricsEnabledFlag,
		utils.FakePoWFlag,
		utils.NoCompactionFlag,
		utils.GpoBlocksFlag,
		utils.GpoPercentileFlag,
		configFileFlag,
	}

	rpcFlags = []cli.Flag{
		utils.RPCEnabledFlag,
		utils.RPCListenAddrFlag,
		utils.RPCPortFlag,
		utils.RPCApiFlag,
		utils.WSEnabledFlag,
		utils.WSListenAddrFlag,
		utils.WSPortFlag,
		utils.WSApiFlag,
		utils.WSAllowedOriginsFlag,
		utils.IPCDisabledFlag,
		utils.IPCPathFlag,
	}

	whisperFlags = []cli.Flag{
		utils.WhisperEnabledFlag,
		utils.WhisperMaxMessageSizeFlag,
		utils.WhisperMinPOWFlag,
	}

	metricsFlags = []cli.Flag{
		utils.MetricsEnableInfluxDBFlag,
		utils.MetricsInfluxDBEndpointFlag,
		utils.MetricsInfluxDBDatabaseFlag,
		utils.MetricsInfluxDBUsernameFlag,
		utils.MetricsInfluxDBPasswordFlag,
		utils.MetricsInfluxDBHostTagFlag,
	}
)

func init() {
//初始化cli应用程序并启动geth
	app.Action = geth
app.HideVersion = true //我们有打印版本的命令
	app.Copyright = "Copyright 2013-2018 The go-ethereum Authors"
	app.Commands = []cli.Command{
//参见chainCmd.Go：
		initCommand,
		importCommand,
		exportCommand,
		importPreimagesCommand,
		exportPreimagesCommand,
		copydbCommand,
		removedbCommand,
		dumpCommand,
//请参阅monitorCmd.go：
		monitorCommand,
//参见accountCmd.Go：
		accountCommand,
		walletCommand,
//请参阅consoleCmd.Go：
		consoleCommand,
		attachCommand,
		javascriptCommand,
//参见MISCCM.Do：
		makecacheCommand,
		makedagCommand,
		versionCommand,
		bugCommand,
		licenseCommand,
//参见CONG.GO
		dumpConfigCommand,
	}
	sort.Sort(cli.CommandsByName(app.Commands))

	app.Flags = append(app.Flags, nodeFlags...)
	app.Flags = append(app.Flags, rpcFlags...)
	app.Flags = append(app.Flags, consoleFlags...)
	app.Flags = append(app.Flags, debug.Flags...)
	app.Flags = append(app.Flags, whisperFlags...)
	app.Flags = append(app.Flags, metricsFlags...)

	app.Before = func(ctx *cli.Context) error {
		runtime.GOMAXPROCS(runtime.NumCPU())

		logdir := ""
		if ctx.GlobalBool(utils.DashboardEnabledFlag.Name) {
			logdir = (&node.Config{DataDir: utils.MakeDataDir(ctx)}).ResolvePath("logs")
		}
		if err := debug.Setup(ctx, logdir); err != nil {
			return err
		}
//限制缓存容量并优化垃圾收集器
		var mem gosigar.Mem
		if err := mem.Get(); err == nil {
			allowance := int(mem.Total / 1024 / 1024 / 3)
			if cache := ctx.GlobalInt(utils.CacheFlag.Name); cache > allowance {
				log.Warn("Sanitizing cache to Go's GC limits", "provided", cache, "updated", allowance)
				ctx.GlobalSet(utils.CacheFlag.Name, strconv.Itoa(allowance))
			}
		}
//确保go的gc忽略触发器百分比的数据库缓存
		cache := ctx.GlobalInt(utils.CacheFlag.Name)
		gogc := math.Max(20, math.Min(100, 100/(float64(cache)/1024)))

		log.Debug("Sanitizing Go's GC trigger", "percent", int(gogc))
		godebug.SetGCPercent(int(gogc))

//如果启用，则启动度量导出
		utils.SetupMetrics(ctx)

//启动系统运行时度量集合
		go metrics.CollectProcessMetrics(3 * time.Second)

		utils.SetupNetwork(ctx)
		return nil
	}

	app.After = func(ctx *cli.Context) error {
		debug.Exit()
console.Stdin.Close() //重置终端模式。
		return nil
	}
}

func main() {
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

//如果没有运行特殊的子命令，geth是进入系统的主要入口点。
//它基于命令行参数创建默认节点并在
//阻塞模式，等待关闭。
func geth(ctx *cli.Context) error {
	if args := ctx.Args(); len(args) > 0 {
		return fmt.Errorf("invalid command: %q", args[0])
	}
	node := makeFullNode(ctx)
	startNode(ctx, node)
	node.Wait()
	return nil
}

//startnode启动系统节点和所有注册的协议，之后
//它解锁所有请求的帐户，并启动RPC/IPC接口和
//
func startNode(ctx *cli.Context, stack *node.Node) {
	debug.Memsize.Add("node", stack)

//
	utils.StartNode(stack)

//
	ks := stack.AccountManager().Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)

	passwords := utils.MakePasswordList(ctx)
	unlocks := strings.Split(ctx.GlobalString(utils.UnlockedAccountFlag.Name), ",")
	for i, account := range unlocks {
		if trimmed := strings.TrimSpace(account); trimmed != "" {
			unlockAccount(ctx, ks, trimmed, i, passwords)
		}
	}
//注册钱包事件处理程序以打开和自动派生钱包
	events := make(chan accounts.WalletEvent, 16)
	stack.AccountManager().Subscribe(events)

	go func() {
//创建用于自派生的链状态读取器
		rpcClient, err := stack.Attach()
		if err != nil {
			utils.Fatalf("Failed to attach to self: %v", err)
		}
		stateReader := ethclient.NewClient(rpcClient)

//打开所有已连接的钱包
		for _, wallet := range stack.AccountManager().Wallets() {
			if err := wallet.Open(""); err != nil {
				log.Warn("Failed to open wallet", "url", wallet.URL(), "err", err)
			}
		}
//听钱包事件直到终止
		for event := range events {
			switch event.Kind {
			case accounts.WalletArrived:
				if err := event.Wallet.Open(""); err != nil {
					log.Warn("New wallet appeared, failed to open", "url", event.Wallet.URL(), "err", err)
				}
			case accounts.WalletOpened:
				status, _ := event.Wallet.Status()
				log.Info("New wallet appeared", "url", event.Wallet.URL(), "status", status)

				derivationPath := accounts.DefaultBaseDerivationPath
				if event.Wallet.URL().Scheme == "ledger" {
					derivationPath = accounts.DefaultLedgerBaseDerivationPath
				}
				event.Wallet.SelfDerive(derivationPath, stateReader)

			case accounts.WalletDropped:
				log.Info("Old wallet dropped", "url", event.Wallet.URL())
				event.Wallet.Close()
			}
		}
	}()
//启动辅助服务（如果启用）
	if ctx.GlobalBool(utils.MiningEnabledFlag.Name) || ctx.GlobalBool(utils.DeveloperFlag.Name) {
//只有当一个完整的以太坊节点正在运行时，挖掘才有意义。
		if ctx.GlobalString(utils.SyncModeFlag.Name) == "light" {
			utils.Fatalf("Light clients do not support mining")
		}
		var ethereum *eth.Ethereum
		if err := stack.Service(&ethereum); err != nil {
			utils.Fatalf("Ethereum service not running: %v", err)
		}
//如果请求，请使用减少的线程数
		threads := ctx.GlobalInt(utils.MinerLegacyThreadsFlag.Name)
		if ctx.GlobalIsSet(utils.MinerThreadsFlag.Name) {
			threads = ctx.GlobalInt(utils.MinerThreadsFlag.Name)
		}
		if threads > 0 {
			type threaded interface {
				SetThreads(threads int)
			}
			if th, ok := ethereum.Engine().(threaded); ok {
				th.SetThreads(threads)
			}
		}
//将天然气价格设置为CLI的限制，然后开始开采。
		gasprice := utils.GlobalBig(ctx, utils.MinerLegacyGasPriceFlag.Name)
		if ctx.IsSet(utils.MinerGasPriceFlag.Name) {
			gasprice = utils.GlobalBig(ctx, utils.MinerGasPriceFlag.Name)
		}
		ethereum.TxPool().SetGasPrice(gasprice)
		if err := ethereum.StartMining(true); err != nil {
			utils.Fatalf("Failed to start mining: %v", err)
		}
	}
}

