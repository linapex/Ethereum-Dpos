
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:33</date>
//</624342614291779584>


package asm

import (
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/vm"
)

//编译器包含有关已分析源的信息
//并持有程序的令牌。
type Compiler struct {
	tokens []token
	binary []interface{}

	labels map[string]int

	pc, pos int

	debug bool
}

//NewCompiler返回新分配的编译器。
func NewCompiler(debug bool) *Compiler {
	return &Compiler{
		labels: make(map[string]int),
		debug:  debug,
	}
}

//feed将令牌馈送到ch，并由
//编译器。
//
//feed是编译阶段的第一个步骤，因为它
//收集程序中使用的标签并保留
//用于确定位置的程序计数器
//跳跃的目的地。标签不能用于
//第二阶段推标签并确定正确的
//位置。
func (c *Compiler) Feed(ch <-chan token) {
	for i := range ch {
		switch i.typ {
		case number:
			num := math.MustParseBig256(i.text).Bytes()
			if len(num) == 0 {
				num = []byte{0}
			}
			c.pc += len(num)
		case stringValue:
			c.pc += len(i.text) - 2
		case element:
			c.pc++
		case labelDef:
			c.labels[i.text] = c.pc
			c.pc++
		case label:
			c.pc += 5
		}

		c.tokens = append(c.tokens, i)
	}
	if c.debug {
		fmt.Fprintln(os.Stderr, "found", len(c.labels), "labels")
	}
}

//编译编译当前令牌并返回一个
//可由EVM解释的二进制字符串
//如果失败了就会出错。
//
//编译是编译阶段的第二个阶段
//
func (c *Compiler) Compile() (string, []error) {
	var errors []error
//继续循环令牌，直到
//
	for c.pos < len(c.tokens) {
		if err := c.compileLine(); err != nil {
			errors = append(errors, err)
		}
	}

//将二进制转换为十六进制
	var bin string
	for _, v := range c.binary {
		switch v := v.(type) {
		case vm.OpCode:
			bin += fmt.Sprintf("%x", []byte{byte(v)})
		case []byte:
			bin += fmt.Sprintf("%x", v)
		}
	}
	return bin, errors
}

//next返回下一个标记并递增
//位置。
func (c *Compiler) next() token {
	token := c.tokens[c.pos]
	c.pos++
	return token
}

//compileline编译单行指令，例如
//“push 1”，“jump@label”。
func (c *Compiler) compileLine() error {
	n := c.next()
	if n.typ != lineStart {
		return compileErr(n, n.typ.String(), lineStart.String())
	}

	lvalue := c.next()
	switch lvalue.typ {
	case eof:
		return nil
	case element:
		if err := c.compileElement(lvalue); err != nil {
			return err
		}
	case labelDef:
		c.compileLabel()
	case lineEnd:
		return nil
	default:
		return compileErr(lvalue, lvalue.text, fmt.Sprintf("%v or %v", labelDef, element))
	}

	if n := c.next(); n.typ != lineEnd {
		return compileErr(n, n.text, lineEnd.String())
	}

	return nil
}

//compileNumber将数字编译为字节
func (c *Compiler) compileNumber(element token) (int, error) {
	num := math.MustParseBig256(element.text).Bytes()
	if len(num) == 0 {
		num = []byte{0}
	}
	c.pushBin(num)
	return len(num), nil
}

//compileElement编译元素（push&label或两者兼有）
//以二进制表示，如果语句不正确，则可能出错。
//喂的地方。
func (c *Compiler) compileElement(element token) error {
//检查是否有跳跃。必须读取和编译跳转
//从右到左。
	if isJump(element.text) {
		rvalue := c.next()
		switch rvalue.typ {
		case number:
//TODO了解如何正确返回错误
			c.compileNumber(rvalue)
		case stringValue:
//字符串被引用，请删除它们。
			c.pushBin(rvalue.text[1 : len(rvalue.text)-2])
		case label:
			c.pushBin(vm.PUSH4)
			pos := big.NewInt(int64(c.labels[rvalue.text])).Bytes()
			pos = append(make([]byte, 4-len(pos)), pos...)
			c.pushBin(pos)
		default:
			return compileErr(rvalue, rvalue.text, "number, string or label")
		}
//推动操作
		c.pushBin(toBinary(element.text))
		return nil
	} else if isPush(element.text) {
//把手推。按从左到右读取。
		var value []byte

		rvalue := c.next()
		switch rvalue.typ {
		case number:
			value = math.MustParseBig256(rvalue.text).Bytes()
			if len(value) == 0 {
				value = []byte{0}
			}
		case stringValue:
			value = []byte(rvalue.text[1 : len(rvalue.text)-1])
		case label:
			value = make([]byte, 4)
			copy(value, big.NewInt(int64(c.labels[rvalue.text])).Bytes())
		default:
			return compileErr(rvalue, rvalue.text, "number, string or label")
		}

		if len(value) > 32 {
			return fmt.Errorf("%d type error: unsupported string or number with size > 32", rvalue.lineno)
		}

		c.pushBin(vm.OpCode(int(vm.PUSH1) - 1 + len(value)))
		c.pushBin(value)
	} else {
		c.pushBin(toBinary(element.text))
	}

	return nil
}

//compileLabel将JumpDest推送到二进制切片。
func (c *Compiler) compileLabel() {
	c.pushBin(vm.JUMPDEST)
}

//推杆将值V推送到二进制堆栈。
func (c *Compiler) pushBin(v interface{}) {
	if c.debug {
		fmt.Printf("%d: %v\n", len(c.binary), v)
	}
	c.binary = append(c.binary, v)
}

//
//推（n）。
func isPush(op string) bool {
	return strings.ToUpper(op) == "PUSH"
}

//is jump返回字符串op是否为jump（i）
func isJump(op string) bool {
	return strings.ToUpper(op) == "JUMPI" || strings.ToUpper(op) == "JUMP"
}

//ToBinary将文本转换为vm.opcode
func toBinary(text string) vm.OpCode {
	return vm.StringToOp(strings.ToUpper(text))
}

type compileError struct {
	got  string
	want string

	lineno int
}

func (err compileError) Error() string {
	return fmt.Sprintf("%d syntax error: unexpected %v, expected %v", err.lineno, err.got, err.want)
}

func compileErr(c token, got, want string) error {
	return compileError{
		got:    got,
		want:   want,
		lineno: c.lineno,
	}
}

