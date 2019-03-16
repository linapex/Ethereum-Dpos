
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:35</date>
//</624342622743302144>


package vm

import (
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/types"
)

//存储表示合同的存储。
type Storage map[common.Hash]common.Hash

//复制复制复制当前存储。
func (s Storage) Copy() Storage {
	cpy := make(Storage)
	for key, value := range s {
		cpy[key] = value
	}

	return cpy
}

//logconfig是结构化记录器evm的配置选项
type LogConfig struct {
DisableMemory  bool //禁用内存捕获
DisableStack   bool //禁用堆栈捕获
DisableStorage bool //禁用存储捕获
Debug          bool //捕获结束时打印输出
Limit          int  //最大输出长度，但零表示无限制
}

//go:生成gencodec-type structlog-field override structlogmarshaling-out gen structlog.go

//structlog在每个周期发送给evm，并列出有关当前内部状态的信息
//在语句执行之前。
type StructLog struct {
	Pc         uint64                      `json:"pc"`
	Op         OpCode                      `json:"op"`
	Gas        uint64                      `json:"gas"`
	GasCost    uint64                      `json:"gasCost"`
	Memory     []byte                      `json:"memory"`
	MemorySize int                         `json:"memSize"`
	Stack      []*big.Int                  `json:"stack"`
	Storage    map[common.Hash]common.Hash `json:"-"`
	Depth      int                         `json:"depth"`
	Err        error                       `json:"-"`
}

//gencodec的覆盖
type structLogMarshaling struct {
	Stack       []*math.HexOrDecimal256
	Gas         math.HexOrDecimal64
	GasCost     math.HexOrDecimal64
	Memory      hexutil.Bytes
OpName      string `json:"opName"` //在marshaljson中添加对opname（）的调用
ErrorString string `json:"error"`  //在marshaljson中添加对ErrorString（）的调用
}

//opname将操作数名称格式化为可读格式。
func (s *StructLog) OpName() string {
	return s.Op.String()
}

//ErrorString将日志的错误格式化为字符串。
func (s *StructLog) ErrorString() string {
	if s.Err != nil {
		return s.Err.Error()
	}
	return ""
}

//跟踪程序用于从EVM事务收集执行跟踪
//执行。对带有
//当前VM状态。
//请注意，引用类型是实际的VM数据结构；复制
//如果您需要在当前呼叫之外保留它们。
type Tracer interface {
	CaptureStart(from common.Address, to common.Address, call bool, input []byte, gas uint64, value *big.Int) error
	CaptureState(env *EVM, pc uint64, op OpCode, gas, cost uint64, memory *Memory, stack *Stack, contract *Contract, depth int, err error) error
	CaptureFault(env *EVM, pc uint64, op OpCode, gas, cost uint64, memory *Memory, stack *Stack, contract *Contract, depth int, err error) error
	CaptureEnd(output []byte, gasUsed uint64, t time.Duration, err error) error
}

//structlogger是一个EVM状态记录器并实现跟踪程序。
//
//结构记录器可以根据给定的日志配置捕获状态，并且还可以保留
//修改后的存储的跟踪记录，用于报告
//把他们的仓库收起来。
type StructLogger struct {
	cfg LogConfig

	logs          []StructLog
	changedValues map[common.Address]Storage
	output        []byte
	err           error
}

//newstructlogger返回新的记录器
func NewStructLogger(cfg *LogConfig) *StructLogger {
	logger := &StructLogger{
		changedValues: make(map[common.Address]Storage),
	}
	if cfg != nil {
		logger.cfg = *cfg
	}
	return logger
}

//CaptureStart实现跟踪程序接口以初始化跟踪操作。
func (l *StructLogger) CaptureStart(from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) error {
	return nil
}

//CaptureState记录新的结构化日志消息并将其推送到环境中
//
//CaptureState还跟踪sstore操作以跟踪脏值。
func (l *StructLogger) CaptureState(env *EVM, pc uint64, op OpCode, gas, cost uint64, memory *Memory, stack *Stack, contract *Contract, depth int, err error) error {
//检查是否已累计指定数量的日志
	if l.cfg.Limit != 0 && l.cfg.Limit <= len(l.logs) {
		return ErrTraceLimitReached
	}

//为此合同初始化新的更改值存储容器
//如果不存在。
	if l.changedValues[contract.Address()] == nil {
		l.changedValues[contract.Address()] = make(Storage)
	}

//捕获sstore操作码并确定更改的值并存储
//它在本地存储容器中。
	if op == SSTORE && stack.len() >= 2 {
		var (
			value   = common.BigToHash(stack.data[stack.len()-2])
			address = common.BigToHash(stack.data[stack.len()-1])
		)
		l.changedValues[contract.Address()][address] = value
	}
//将当前内存状态的快照复制到新缓冲区
	var mem []byte
	if !l.cfg.DisableMemory {
		mem = make([]byte, len(memory.Data()))
		copy(mem, memory.Data())
	}
//将当前堆栈状态的快照复制到新缓冲区
	var stck []*big.Int
	if !l.cfg.DisableStack {
		stck = make([]*big.Int, len(stack.Data()))
		for i, item := range stack.Data() {
			stck[i] = new(big.Int).Set(item)
		}
	}
//将当前存储的快照复制到新容器
	var storage Storage
	if !l.cfg.DisableStorage {
		storage = l.changedValues[contract.Address()].Copy()
	}
//创建EVM的新快照。
	log := StructLog{pc, op, gas, cost, mem, memory.Len(), stck, storage, depth, err}

	l.logs = append(l.logs, log)
	return nil
}

//CaptureFault实现跟踪程序接口来跟踪执行错误
//运行操作码时。
func (l *StructLogger) CaptureFault(env *EVM, pc uint64, op OpCode, gas, cost uint64, memory *Memory, stack *Stack, contract *Contract, depth int, err error) error {
	return nil
}

//在调用完成后调用CaptureEnd以完成跟踪。
func (l *StructLogger) CaptureEnd(output []byte, gasUsed uint64, t time.Duration, err error) error {
	l.output = output
	l.err = err
	if l.cfg.Debug {
		fmt.Printf("0x%x\n", output)
		if err != nil {
			fmt.Printf(" error: %v\n", err)
		}
	}
	return nil
}

//structlogs返回捕获的日志条目。
func (l *StructLogger) StructLogs() []StructLog { return l.logs }

//错误返回跟踪捕获的VM错误。
func (l *StructLogger) Error() error { return l.err }

//输出返回跟踪捕获的VM返回值。
func (l *StructLogger) Output() []byte { return l.output }

//WriteTrace将格式化的跟踪写入给定的写入程序
func WriteTrace(writer io.Writer, logs []StructLog) {
	for _, log := range logs {
		fmt.Fprintf(writer, "%-16spc=%08d gas=%v cost=%v", log.Op, log.Pc, log.Gas, log.GasCost)
		if log.Err != nil {
			fmt.Fprintf(writer, " ERROR: %v", log.Err)
		}
		fmt.Fprintln(writer)

		if len(log.Stack) > 0 {
			fmt.Fprintln(writer, "Stack:")
			for i := len(log.Stack) - 1; i >= 0; i-- {
				fmt.Fprintf(writer, "%08d  %x\n", len(log.Stack)-i-1, math.PaddedBigBytes(log.Stack[i], 32))
			}
		}
		if len(log.Memory) > 0 {
			fmt.Fprintln(writer, "Memory:")
			fmt.Fprint(writer, hex.Dump(log.Memory))
		}
		if len(log.Storage) > 0 {
			fmt.Fprintln(writer, "Storage:")
			for h, item := range log.Storage {
				fmt.Fprintf(writer, "%x: %x\n", h, item)
			}
		}
		fmt.Fprintln(writer)
	}
}

//WriteLogs以可读的格式将VM日志写入给定的写入程序
func WriteLogs(writer io.Writer, logs []*types.Log) {
	for _, log := range logs {
		fmt.Fprintf(writer, "LOG%d: %x bn=%d txi=%x\n", len(log.Topics), log.Address, log.BlockNumber, log.TxIndex)

		for i, topic := range log.Topics {
			fmt.Fprintf(writer, "%08d  %x\n", i, topic)
		}

		fmt.Fprint(writer, hex.Dump(log.Data))
		fmt.Fprintln(writer)
	}
}

