
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:32</date>
//</624342608012906496>


//包编译器包装solidity编译器可执行文件（solc）。
package compiler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var versionRegexp = regexp.MustCompile(`([0-9]+)\.([0-9]+)\.([0-9]+)`)

//合同包含有关已编译合同的信息及其代码。
type Contract struct {
	Code string       `json:"code"`
	Info ContractInfo `json:"info"`
}

//收缩信息包含有关已编译合同的信息，包括访问
//到ABI定义、用户和开发人员文档以及元数据。
//
//取决于源、语言版本、编译器版本和编译器
//选项将提供有关如何编译合同的信息。
type ContractInfo struct {
	Source          string      `json:"source"`
	Language        string      `json:"language"`
	LanguageVersion string      `json:"languageVersion"`
	CompilerVersion string      `json:"compilerVersion"`
	CompilerOptions string      `json:"compilerOptions"`
	AbiDefinition   interface{} `json:"abiDefinition"`
	UserDoc         interface{} `json:"userDoc"`
	DeveloperDoc    interface{} `json:"developerDoc"`
	Metadata        string      `json:"metadata"`
}

//
type Solidity struct {
	Path, Version, FullVersion string
	Major, Minor, Patch        int
}

//——组合输出格式
type solcOutput struct {
	Contracts map[string]struct {
		Bin, Abi, Devdoc, Userdoc, Metadata string
	}
	Version string
}

func (s *Solidity) makeArgs() []string {
	p := []string{
		"--combined-json", "bin,abi,userdoc,devdoc",
"--optimize", //代码优化器已打开
	}
	if s.Major > 0 || s.Minor > 4 || s.Patch > 6 {
		p[1] += ",metadata"
	}
	return p
}

//solidityversion运行solc并解析其版本输出。
func SolidityVersion(solc string) (*Solidity, error) {
	if solc == "" {
		solc = "solc"
	}
	var out bytes.Buffer
	cmd := exec.Command(solc, "--version")
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}
	matches := versionRegexp.FindStringSubmatch(out.String())
	if len(matches) != 4 {
		return nil, fmt.Errorf("can't parse solc version %q", out.String())
	}
	s := &Solidity{Path: cmd.Path, FullVersion: out.String(), Version: matches[0]}
	if s.Major, err = strconv.Atoi(matches[1]); err != nil {
		return nil, err
	}
	if s.Minor, err = strconv.Atoi(matches[2]); err != nil {
		return nil, err
	}
	if s.Patch, err = strconv.Atoi(matches[3]); err != nil {
		return nil, err
	}
	return s, nil
}

//CompilesOlidityString生成并返回源字符串中包含的所有协定。
func CompileSolidityString(solc, source string) (map[string]*Contract, error) {
	if len(source) == 0 {
		return nil, errors.New("solc: empty source string")
	}
	s, err := SolidityVersion(solc)
	if err != nil {
		return nil, err
	}
	args := append(s.makeArgs(), "--")
	cmd := exec.Command(s.Path, append(args, "-")...)
	cmd.Stdin = strings.NewReader(source)
	return s.run(cmd, source)
}

//compilesolidity编译所有给定的solidity源文件。
func CompileSolidity(solc string, sourcefiles ...string) (map[string]*Contract, error) {
	if len(sourcefiles) == 0 {
		return nil, errors.New("solc: no source files")
	}
	source, err := slurpFiles(sourcefiles)
	if err != nil {
		return nil, err
	}
	s, err := SolidityVersion(solc)
	if err != nil {
		return nil, err
	}
	args := append(s.makeArgs(), "--")
	cmd := exec.Command(s.Path, append(args, sourcefiles...)...)
	return s.run(cmd, source)
}

func (s *Solidity) run(cmd *exec.Cmd, source string) (map[string]*Contract, error) {
	var stderr, stdout bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("solc: %v\n%s", err, stderr.Bytes())
	}

	return ParseCombinedJSON(stdout.Bytes(), source, s.Version, s.Version, strings.Join(s.makeArgs(), " "))
}

//parsecombinedjson接受solc的直接输出——组合输出运行和
//将其解析为字符串协定名称到协定结构的映射。这个
//提供的源、语言和编译器版本以及编译器选项都是
//传递到合同结构中。
//
//SOLC输出应该包含ABI、用户文档和开发文档。
//
//如果JSON格式不正确或缺少数据，或者如果JSON
//嵌入在JSON中的格式不正确。
func ParseCombinedJSON(combinedJSON []byte, source string, languageVersion string, compilerVersion string, compilerOptions string) (map[string]*Contract, error) {
	var output solcOutput
	if err := json.Unmarshal(combinedJSON, &output); err != nil {
		return nil, err
	}

//编译成功，组装并返回合同。
	contracts := make(map[string]*Contract)
	for name, info := range output.Contracts {
//分析各个编译结果。
		var abi interface{}
		if err := json.Unmarshal([]byte(info.Abi), &abi); err != nil {
			return nil, fmt.Errorf("solc: error reading abi definition (%v)", err)
		}
		var userdoc interface{}
		if err := json.Unmarshal([]byte(info.Userdoc), &userdoc); err != nil {
			return nil, fmt.Errorf("solc: error reading user doc: %v", err)
		}
		var devdoc interface{}
		if err := json.Unmarshal([]byte(info.Devdoc), &devdoc); err != nil {
			return nil, fmt.Errorf("solc: error reading dev doc: %v", err)
		}
		contracts[name] = &Contract{
			Code: "0x" + info.Bin,
			Info: ContractInfo{
				Source:          source,
				Language:        "Solidity",
				LanguageVersion: languageVersion,
				CompilerVersion: compilerVersion,
				CompilerOptions: compilerOptions,
				AbiDefinition:   abi,
				UserDoc:         userdoc,
				DeveloperDoc:    devdoc,
				Metadata:        info.Metadata,
			},
		}
	}
	return contracts, nil
}

func slurpFiles(files []string) (string, error) {
	var concat bytes.Buffer
	for _, file := range files {
		content, err := ioutil.ReadFile(file)
		if err != nil {
			return "", err
		}
		concat.Write(content)
	}
	return concat.String(), nil
}

