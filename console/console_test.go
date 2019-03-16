
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:33</date>
//</624342612681166848>


package console

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/internal/jsre"
	"github.com/ethereum/go-ethereum/node"
)

const (
	testInstance = "console-tester"
	testAddress  = "0x8605cdbbdb6d264aa742e77020dcbc58fcdce182"
)

//HookedPrompter实现了用户Prompter来模拟通过通道的使用输入。
type hookedPrompter struct {
	scheduler chan string
}

func (p *hookedPrompter) PromptInput(prompt string) (string, error) {
//
	select {
	case p.scheduler <- prompt:
	case <-time.After(time.Second):
		return "", errors.New("prompt timeout")
	}
//检索响应并反馈到控制台
	select {
	case input := <-p.scheduler:
		return input, nil
	case <-time.After(time.Second):
		return "", errors.New("input timeout")
	}
}

func (p *hookedPrompter) PromptPassword(prompt string) (string, error) {
	return "", errors.New("not implemented")
}
func (p *hookedPrompter) PromptConfirm(prompt string) (bool, error) {
	return false, errors.New("not implemented")
}
func (p *hookedPrompter) SetHistory(history []string)              {}
func (p *hookedPrompter) AppendHistory(command string)             {}
func (p *hookedPrompter) ClearHistory()                            {}
func (p *hookedPrompter) SetWordCompleter(completer WordCompleter) {}

//测试仪是控制台测试的控制台测试环境。
type tester struct {
	workspace string
	stack     *node.Node
	ethereum  *eth.Ethereum
	console   *Console
	input     *hookedPrompter
	output    *bytes.Buffer
}

//NewTester创建了一个测试环境，控制台可以根据这个环境进行操作。
//请确保在返回的测试仪上调用close（）以避免泄漏。
func newTester(t *testing.T, confOverride func(*eth.Config)) *tester {
//为节点键创建临时存储并初始化它
	workspace, err := ioutil.TempDir("", "console-tester-")
	if err != nil {
		t.Fatalf("failed to create temporary keystore: %v", err)
	}

//创建无网络协议栈并在
	stack, err := node.New(&node.Config{DataDir: workspace, UseLightweightKDF: true, Name: testInstance})
	if err != nil {
		t.Fatalf("failed to create node: %v", err)
	}
	ethConf := &eth.Config{
		Genesis:   core.DeveloperGenesisBlock(15, common.Address{}),
		Etherbase: common.HexToAddress(testAddress),
		Ethash: ethash.Config{
			PowMode: ethash.ModeTest,
		},
	}
	if confOverride != nil {
		confOverride(ethConf)
	}
	if err = stack.Register(func(ctx *node.ServiceContext) (node.Service, error) { return eth.New(ctx, ethConf) }); err != nil {
		t.Fatalf("failed to register Ethereum protocol: %v", err)
	}
//启动节点并围绕它组装JavaScript控制台
	if err = stack.Start(); err != nil {
		t.Fatalf("failed to start test stack: %v", err)
	}
	client, err := stack.Attach()
	if err != nil {
		t.Fatalf("failed to attach to node: %v", err)
	}
	prompter := &hookedPrompter{scheduler: make(chan string)}
	printer := new(bytes.Buffer)

	console, err := New(Config{
		DataDir:  stack.DataDir(),
		DocRoot:  "testdata",
		Client:   client,
		Prompter: prompter,
		Printer:  printer,
		Preload:  []string{"preload.js"},
	})
	if err != nil {
		t.Fatalf("failed to create JavaScript console: %v", err)
	}
//创建最终测试仪并返回
	var ethereum *eth.Ethereum
	stack.Service(&ethereum)

	return &tester{
		workspace: workspace,
		stack:     stack,
		ethereum:  ethereum,
		console:   console,
		input:     prompter,
		output:    printer,
	}
}

//关闭清除所有临时数据文件夹和保留的资源。
func (env *tester) Close(t *testing.T) {
	if err := env.console.Stop(false); err != nil {
		t.Errorf("failed to stop embedded console: %v", err)
	}
	if err := env.stack.Stop(); err != nil {
		t.Errorf("failed to stop embedded node: %v", err)
	}
	os.RemoveAll(env.workspace)
}

//测试节点是否列出了正确的欢迎消息，特别是它包含
//实例名、coinbase账号、块号、数据目录，支持
//控制台模块。
func TestWelcome(t *testing.T) {
	tester := newTester(t, nil)
	defer tester.Close(t)

	tester.console.Welcome()

	output := tester.output.String()
	if want := "Welcome"; !strings.Contains(output, want) {
		t.Fatalf("console output missing welcome message: have\n%s\nwant also %s", output, want)
	}
	if want := fmt.Sprintf("instance: %s", testInstance); !strings.Contains(output, want) {
		t.Fatalf("console output missing instance: have\n%s\nwant also %s", output, want)
	}
	if want := fmt.Sprintf("coinbase: %s", testAddress); !strings.Contains(output, want) {
		t.Fatalf("console output missing coinbase: have\n%s\nwant also %s", output, want)
	}
	if want := "at block: 0"; !strings.Contains(output, want) {
		t.Fatalf("console output missing sync status: have\n%s\nwant also %s", output, want)
	}
	if want := fmt.Sprintf("datadir: %s", tester.workspace); !strings.Contains(output, want) {
		t.Fatalf("console output missing coinbase: have\n%s\nwant also %s", output, want)
	}
}

//测试javascript语句评估是否按预期工作。
func TestEvaluate(t *testing.T) {
	tester := newTester(t, nil)
	defer tester.Close(t)

	tester.console.Evaluate("2 + 2")
	if output := tester.output.String(); !strings.Contains(output, "4") {
		t.Fatalf("statement evaluation failed: have %s, want %s", output, "4")
	}
}

//测试控制台是否可以在交互模式下使用。
func TestInteractive(t *testing.T) {
//创建一个测试人员并在后台运行一个交互式控制台
	tester := newTester(t, nil)
	defer tester.Close(t)

	go tester.console.Interactive()

//等待提示并返回语句
	select {
	case <-tester.input.scheduler:
	case <-time.After(time.Second):
		t.Fatalf("initial prompt timeout")
	}
	select {
	case tester.input.scheduler <- "2+2":
	case <-time.After(time.Second):
		t.Fatalf("input feedback timeout")
	}
//等待第二个提示并确保对第一个语句进行了计算
	select {
	case <-tester.input.scheduler:
	case <-time.After(time.Second):
		t.Fatalf("secondary prompt timeout")
	}
	if output := tester.output.String(); !strings.Contains(output, "4") {
		t.Fatalf("statement evaluation failed: have %s, want %s", output, "4")
	}
}

//在给定用户之前已执行预加载的javascript文件的测试
//输入。
func TestPreload(t *testing.T) {
	tester := newTester(t, nil)
	defer tester.Close(t)

	tester.console.Evaluate("preloaded")
	if output := tester.output.String(); !strings.Contains(output, "some-preloaded-string") {
		t.Fatalf("preloaded variable missing: have %s, want %s", output, "some-preloaded-string")
	}
}

//测试可以从配置的资产路径执行javascript脚本。
func TestExecute(t *testing.T) {
	tester := newTester(t, nil)
	defer tester.Close(t)

	tester.console.Execute("exec.js")

	tester.console.Evaluate("execed")
	if output := tester.output.String(); !strings.Contains(output, "some-executed-string") {
		t.Fatalf("execed variable missing: have %s, want %s", output, "some-executed-string")
	}
}

//测试语句执行返回的javascript对象是否正确
//漂亮的打印，而不是仅仅显示“[对象]”。
func TestPrettyPrint(t *testing.T) {
	tester := newTester(t, nil)
	defer tester.Close(t)

	tester.console.Evaluate("obj = {int: 1, string: 'two', list: [3, 3, 3], obj: {null: null, func: function(){}}}")

//定义一些特殊格式的字段
	var (
		one   = jsre.NumberColor("1")
		two   = jsre.StringColor("\"two\"")
		three = jsre.NumberColor("3")
		null  = jsre.SpecialColor("null")
		fun   = jsre.FunctionColor("function()")
	)
//把我们需要的实际输出集合起来并验证
	want := `{
  int: ` + one + `,
  list: [` + three + `, ` + three + `, ` + three + `],
  obj: {
    null: ` + null + `,
    func: ` + fun + `
  },
  string: ` + two + `
}
`
	if output := tester.output.String(); output != want {
		t.Fatalf("pretty print mismatch: have %s, want %s", output, want)
	}
}

//测试javascript异常的格式和颜色是否正确。
func TestPrettyError(t *testing.T) {
	tester := newTester(t, nil)
	defer tester.Close(t)
	tester.console.Evaluate("throw 'hello'")

	want := jsre.ErrorColor("hello") + "\n"
	if output := tester.output.String(); output != want {
		t.Fatalf("pretty error mismatch: have %s, want %s", output, want)
	}
}

//测试JS输入的缩进数是否计算正确。
func TestIndenting(t *testing.T) {
	testCases := []struct {
		input               string
		expectedIndentCount int
	}{
		{`var a = 1;`, 0},
		{`"some string"`, 0},
		{`"some string with (parenthesis`, 0},
		{`"some string with newline
		("`, 0},
		{`function v(a,b) {}`, 0},
		{`function f(a,b) { var str = "asd("; };`, 0},
		{`function f(a) {`, 1},
		{`function f(a, function(b) {`, 2},
		{`function f(a, function(b) {
		     var str = "a)}";
		  });`, 0},
		{`function f(a,b) {
		   var str = "a{b(" + a, ", " + b;
		   }`, 0},
		{`var str = "\"{"`, 0},
		{`var str = "'("`, 0},
		{`var str = "\\{"`, 0},
		{`var str = "\\\\{"`, 0},
		{`var str = 'a"{`, 0},
		{`var obj = {`, 1},
		{`var obj = { {a:1`, 2},
		{`var obj = { {a:1}`, 1},
		{`var obj = { {a:1}, b:2}`, 0},
		{`var obj = {}`, 0},
		{`var obj = {
			a: 1, b: 2
		}`, 0},
		{`var test = }`, -1},
		{`var str = "a\""; var obj = {`, 1},
	}

	for i, tt := range testCases {
		counted := countIndents(tt.input)
		if counted != tt.expectedIndentCount {
			t.Errorf("test %d: invalid indenting: have %d, want %d", i, counted, tt.expectedIndentCount)
		}
	}
}

