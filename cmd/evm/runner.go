
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:27</date>
//</624342590094839808>


package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	goruntime "runtime"
	"runtime/pprof"
	"time"

	"github.com/ethereum/go-ethereum/cmd/evm/internal/compiler"
	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/core/vm/runtime"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	cli "gopkg.in/urfave/cli.v1"
)

var runCommand = cli.Command{
	Action:      runCmd,
	Name:        "run",
	Usage:       "run arbitrary evm binary",
	ArgsUsage:   "<code>",
	Description: `The run command runs arbitrary EVM code.`,
}

//read genesis将读取给定的JSON格式genesis文件并返回
//初始化的Genesis结构
func readGenesis(genesisPath string) *core.Genesis {
//确保我们有一个有效的Genesis JSON
//genesPath:=ctx.args（）.first（））
	if len(genesisPath) == 0 {
		utils.Fatalf("Must supply path to genesis JSON file")
	}
	file, err := os.Open(genesisPath)
	if err != nil {
		utils.Fatalf("Failed to read genesis file: %v", err)
	}
	defer file.Close()

	genesis := new(core.Genesis)
	if err := json.NewDecoder(file).Decode(genesis); err != nil {
		utils.Fatalf("invalid genesis file: %v", err)
	}
	return genesis
}

func runCmd(ctx *cli.Context) error {
	glogger := log.NewGlogHandler(log.StreamHandler(os.Stderr, log.TerminalFormat(false)))
	glogger.Verbosity(log.Lvl(ctx.GlobalInt(VerbosityFlag.Name)))
	log.Root().SetHandler(glogger)
	logconfig := &vm.LogConfig{
		DisableMemory: ctx.GlobalBool(DisableMemoryFlag.Name),
		DisableStack:  ctx.GlobalBool(DisableStackFlag.Name),
		Debug:         ctx.GlobalBool(DebugFlag.Name),
	}

	var (
		tracer      vm.Tracer
		debugLogger *vm.StructLogger
		statedb     *state.StateDB
		chainConfig *params.ChainConfig
		sender      = common.BytesToAddress([]byte("sender"))
		receiver    = common.BytesToAddress([]byte("receiver"))
		blockNumber uint64
	)
	if ctx.GlobalBool(MachineFlag.Name) {
		tracer = NewJSONLogger(logconfig, os.Stdout)
	} else if ctx.GlobalBool(DebugFlag.Name) {
		debugLogger = vm.NewStructLogger(logconfig)
		tracer = debugLogger
	} else {
		debugLogger = vm.NewStructLogger(logconfig)
	}
	if ctx.GlobalString(GenesisFlag.Name) != "" {
		gen := readGenesis(ctx.GlobalString(GenesisFlag.Name))
		db := ethdb.NewMemDatabase()
		genesis := gen.ToBlock(db)
		statedb, _ = state.New(genesis.Root(), state.NewDatabase(db))
		chainConfig = gen.Config
		blockNumber = gen.Number
	} else {
		statedb, _ = state.New(common.Hash{}, state.NewDatabase(ethdb.NewMemDatabase()))
	}
	if ctx.GlobalString(SenderFlag.Name) != "" {
		sender = common.HexToAddress(ctx.GlobalString(SenderFlag.Name))
	}
	statedb.CreateAccount(sender)

	if ctx.GlobalString(ReceiverFlag.Name) != "" {
		receiver = common.HexToAddress(ctx.GlobalString(ReceiverFlag.Name))
	}

	var (
		code []byte
		ret  []byte
		err  error
	)
//“--code”或“--codefile”标志重写处于状态的代码
	if ctx.GlobalString(CodeFileFlag.Name) != "" {
		var hexcode []byte
		var err error
//如果指定了-则表示代码来自stdin
		if ctx.GlobalString(CodeFileFlag.Name) == "-" {
//尝试从stdin读取
			if hexcode, err = ioutil.ReadAll(os.Stdin); err != nil {
				fmt.Printf("Could not load code from stdin: %v\n", err)
				os.Exit(1)
			}
		} else {
//带十六进制程序集的代码文件
			if hexcode, err = ioutil.ReadFile(ctx.GlobalString(CodeFileFlag.Name)); err != nil {
				fmt.Printf("Could not load code from file: %v\n", err)
				os.Exit(1)
			}
		}
		code = common.Hex2Bytes(string(bytes.TrimRight(hexcode, "\n")))

	} else if ctx.GlobalString(CodeFlag.Name) != "" {
		code = common.Hex2Bytes(ctx.GlobalString(CodeFlag.Name))
	} else if fn := ctx.Args().First(); len(fn) > 0 {
//要编译的EASM文件
		src, err := ioutil.ReadFile(fn)
		if err != nil {
			return err
		}
		bin, err := compiler.Compile(fn, src, false)
		if err != nil {
			return err
		}
		code = common.Hex2Bytes(bin)
	}

	initialGas := ctx.GlobalUint64(GasFlag.Name)
	runtimeConfig := runtime.Config{
		Origin:      sender,
		State:       statedb,
		GasLimit:    initialGas,
		GasPrice:    utils.GlobalBig(ctx, PriceFlag.Name),
		Value:       utils.GlobalBig(ctx, ValueFlag.Name),
		BlockNumber: new(big.Int).SetUint64(blockNumber),
		EVMConfig: vm.Config{
			Tracer: tracer,
			Debug:  ctx.GlobalBool(DebugFlag.Name) || ctx.GlobalBool(MachineFlag.Name),
		},
	}

	if cpuProfilePath := ctx.GlobalString(CPUProfileFlag.Name); cpuProfilePath != "" {
		f, err := os.Create(cpuProfilePath)
		if err != nil {
			fmt.Println("could not create CPU profile: ", err)
			os.Exit(1)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			fmt.Println("could not start CPU profile: ", err)
			os.Exit(1)
		}
		defer pprof.StopCPUProfile()
	}

	if chainConfig != nil {
		runtimeConfig.ChainConfig = chainConfig
	}
	tstart := time.Now()
	var leftOverGas uint64
	if ctx.GlobalBool(CreateFlag.Name) {
		input := append(code, common.Hex2Bytes(ctx.GlobalString(InputFlag.Name))...)
		ret, _, leftOverGas, err = runtime.Create(input, &runtimeConfig)
	} else {
		if len(code) > 0 {
			statedb.SetCode(receiver, code)
		}
		ret, leftOverGas, err = runtime.Call(receiver, common.Hex2Bytes(ctx.GlobalString(InputFlag.Name)), &runtimeConfig)
	}
	execTime := time.Since(tstart)

	if ctx.GlobalBool(DumpFlag.Name) {
		statedb.IntermediateRoot(true)
		fmt.Println(string(statedb.Dump()))
	}

	if memProfilePath := ctx.GlobalString(MemProfileFlag.Name); memProfilePath != "" {
		f, err := os.Create(memProfilePath)
		if err != nil {
			fmt.Println("could not create memory profile: ", err)
			os.Exit(1)
		}
		if err := pprof.WriteHeapProfile(f); err != nil {
			fmt.Println("could not write memory profile: ", err)
			os.Exit(1)
		}
		f.Close()
	}

	if ctx.GlobalBool(DebugFlag.Name) {
		if debugLogger != nil {
			fmt.Fprintln(os.Stderr, "#### TRACE ####")
			vm.WriteTrace(os.Stderr, debugLogger.StructLogs())
		}
		fmt.Fprintln(os.Stderr, "#### LOGS ####")
		vm.WriteLogs(os.Stderr, statedb.Logs())
	}

	if ctx.GlobalBool(StatDumpFlag.Name) {
		var mem goruntime.MemStats
		goruntime.ReadMemStats(&mem)
		fmt.Fprintf(os.Stderr, `evm execution time: %v
heap objects:       %d
allocations:        %d
total allocations:  %d
GC calls:           %d
Gas used:           %d

`, execTime, mem.HeapObjects, mem.Alloc, mem.TotalAlloc, mem.NumGC, initialGas-leftOverGas)
	}
	if tracer == nil {
		fmt.Printf("0x%x\n", ret)
		if err != nil {
			fmt.Printf(" error: %v\n", err)
		}
	}

	return nil
}

