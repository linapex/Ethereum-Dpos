
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:33</date>
//</624342612765052928>


package console

import (
	"fmt"
	"strings"

	"github.com/peterh/liner"
)

//stdin保存stdin行阅读器（也使用stdout进行打印提示）。
//
var Stdin = newTerminalPrompter()

//userprompter定义控制台提示用户
//各种类型的输入。
type UserPrompter interface {
//promptinput向用户显示给定的提示并请求一些文本
//
	PromptInput(prompt string) (string, error)

//
//要输入的数据，但不能回送到终端。
//方法返回用户提供的输入。
	PromptPassword(prompt string) (string, error)

//promptconfirm向用户显示给定的提示并请求布尔值
//做出选择，返回该选择。
	PromptConfirm(prompt string) (bool, error)

//setHistory设置prompter允许的输入回滚历史记录
//the user to scroll back to.
	SetHistory(history []string)

//AppendHistory将一个条目追加到回滚历史记录。应该叫它
//如果且仅当追加提示是有效命令时。
	AppendHistory(command string)

//清除历史记录清除整个历史记录
	ClearHistory()

//setwordCompleter设置提示器将调用的完成函数
//当用户按Tab键时获取完成候选项。
	SetWordCompleter(completer WordCompleter)
}

//WordCompleter使用光标位置获取当前编辑的行，并
//返回要完成的部分单词的完成候选词。如果
//电话是“你好，我！！光标在第一个“！”之前，（“你好，
//哇！！，9）传递给可能返回的完成者（“hello，”，“world”，
//“Word”}，“！！！！“你好，世界！！.
type WordCompleter func(line string, pos int) (string, []string, string)

//TerminalPrompter是一个由liner包支持的用户Prompter。它支持
//提示用户输入各种输入，其中包括不回显密码
//输入。
type terminalPrompter struct {
	*liner.State
	warned     bool
	supported  bool
	normalMode liner.ModeApplier
	rawMode    liner.ModeApplier
}

//
//standard input and output streams.
func newTerminalPrompter() *terminalPrompter {
	p := new(terminalPrompter)
//在调用newliner之前获取原始模式。
//这通常是常规的“煮熟”模式，其中字符回音。
	normalMode, _ := liner.TerminalMode()
//打开班轮。它切换到原始模式。
	p.State = liner.NewLiner()
	rawMode, err := liner.TerminalMode()
	if err != nil || !liner.TerminalSupported() {
		p.supported = false
	} else {
		p.supported = true
		p.normalMode = normalMode
		p.rawMode = rawMode
//在不提示的情况下切换回正常模式。
		normalMode.ApplyMode()
	}
	p.SetCtrlCAborts(true)
	p.SetTabCompletionStyle(liner.TabPrints)
	p.SetMultiLineMode(true)
	return p
}

//promptinput向用户显示给定的提示并请求一些文本
//要输入的数据，返回用户的输入。
func (p *terminalPrompter) PromptInput(prompt string) (string, error) {
	if p.supported {
		p.rawMode.ApplyMode()
		defer p.normalMode.ApplyMode()
	} else {
//liner试图巧妙地打印提示
//如果输入被重定向，则不打印任何内容。
//总是通过打印提示来取消智能。
		fmt.Print(prompt)
		prompt = ""
		defer fmt.Println()
	}
	return p.State.Prompt(prompt)
}

//提示密码向用户显示给定的提示并请求一些文本
//要输入的数据，但不能回送到终端。
//方法返回用户提供的输入。
func (p *terminalPrompter) PromptPassword(prompt string) (passwd string, err error) {
	if p.supported {
		p.rawMode.ApplyMode()
		defer p.normalMode.ApplyMode()
		return p.State.PasswordPrompt(prompt)
	}
	if !p.warned {
		fmt.Println("!! Unsupported terminal, password will be echoed.")
		p.warned = true
	}
//正如在提示中一样，在这里处理打印提示，而不是依赖于衬线。
	fmt.Print(prompt)
	passwd, err = p.State.Prompt("")
	fmt.Println()
	return passwd, err
}

//promptconfirm向用户显示给定的提示并请求布尔值
//做出选择，返回该选择。
func (p *terminalPrompter) PromptConfirm(prompt string) (bool, error) {
	input, err := p.Prompt(prompt + " [y/N] ")
	if len(input) > 0 && strings.ToUpper(input[:1]) == "Y" {
		return true, nil
	}
	return false, err
}

//setHistory设置prompter允许的输入回滚历史记录
//要回滚到的用户。
func (p *terminalPrompter) SetHistory(history []string) {
	p.State.ReadHistory(strings.NewReader(strings.Join(history, "\n")))
}

//AppendHistory将一个条目追加到回滚历史记录。
func (p *terminalPrompter) AppendHistory(command string) {
	p.State.AppendHistory(command)
}

//清除历史记录清除整个历史记录
func (p *terminalPrompter) ClearHistory() {
	p.State.ClearHistory()
}

//setwordCompleter设置提示器将调用的完成函数
//当用户按Tab键时获取完成候选项。
func (p *terminalPrompter) SetWordCompleter(completer WordCompleter) {
	p.State.SetWordCompleter(liner.WordCompleter(completer))
}

