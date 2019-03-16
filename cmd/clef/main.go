
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:27</date>
//</624342588865908736>


//签名者是一个实用程序，可以用来对事务和
//任意数据。
package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/signer/core"
	"github.com/ethereum/go-ethereum/signer/rules"
	"github.com/ethereum/go-ethereum/signer/storage"
	"gopkg.in/urfave/cli.v1"
)

//ExternalApiVersion--请参阅extapi_changelog.md
const ExternalAPIVersion = "2.0.0"

//InternalApiVersion--请参阅intapi_changelog.md
const InternalAPIVersion = "2.0.0"

const legalWarning = `
WARNING! 

Clef is alpha software, and not yet publically released. This software has _not_ been audited, and there
are no guarantees about the workings of this software. It may contain severe flaws. You should not use this software
unless you agree to take full responsibility for doing so, and know what you are doing. 

TLDR; THIS IS NOT PRODUCTION-READY SOFTWARE! 

`

var (
	logLevelFlag = cli.IntFlag{
		Name:  "loglevel",
		Value: 4,
		Usage: "log level to emit to the screen",
	}
	keystoreFlag = cli.StringFlag{
		Name:  "keystore",
		Value: filepath.Join(node.DefaultDataDir(), "keystore"),
		Usage: "Directory for the keystore",
	}
	configdirFlag = cli.StringFlag{
		Name:  "configdir",
		Value: DefaultConfigDir(),
		Usage: "Directory for Clef configuration",
	}
	rpcPortFlag = cli.IntFlag{
		Name:  "rpcport",
		Usage: "HTTP-RPC server listening port",
		Value: node.DefaultHTTPPort + 5,
	}
	signerSecretFlag = cli.StringFlag{
		Name:  "signersecret",
		Usage: "A file containing the password used to encrypt Clef credentials, e.g. keystore credentials and ruleset hash",
	}
	dBFlag = cli.StringFlag{
		Name:  "4bytedb",
		Usage: "File containing 4byte-identifiers",
		Value: "./4byte.json",
	}
	customDBFlag = cli.StringFlag{
		Name:  "4bytedb-custom",
		Usage: "File used for writing new 4byte-identifiers submitted via API",
		Value: "./4byte-custom.json",
	}
	auditLogFlag = cli.StringFlag{
		Name:  "auditlog",
		Usage: "File used to emit audit logs. Set to \"\" to disable",
		Value: "audit.log",
	}
	ruleFlag = cli.StringFlag{
		Name:  "rules",
		Usage: "Enable rule-engine",
		Value: "rules.json",
	}
	stdiouiFlag = cli.BoolFlag{
		Name: "stdio-ui",
		Usage: "Use STDIN/STDOUT as a channel for an external UI. " +
			"This means that an STDIN/STDOUT is used for RPC-communication with a e.g. a graphical user " +
			"interface, and can be used when Clef is started by an external process.",
	}
	testFlag = cli.BoolFlag{
		Name:  "stdio-ui-test",
		Usage: "Mechanism to test interface between Clef and UI. Requires 'stdio-ui'.",
	}
	app         = cli.NewApp()
	initCommand = cli.Command{
		Action:    utils.MigrateFlags(initializeSecrets),
		Name:      "init",
		Usage:     "Initialize the signer, generate secret storage",
		ArgsUsage: "",
		Flags: []cli.Flag{
			logLevelFlag,
			configdirFlag,
		},
		Description: `
The init command generates a master seed which Clef can use to store credentials and data needed for 
the rule-engine to work.`,
	}
	attestCommand = cli.Command{
		Action:    utils.MigrateFlags(attestFile),
		Name:      "attest",
		Usage:     "Attest that a js-file is to be used",
		ArgsUsage: "<sha256sum>",
		Flags: []cli.Flag{
			logLevelFlag,
			configdirFlag,
			signerSecretFlag,
		},
		Description: `
The attest command stores the sha256 of the rule.js-file that you want to use for automatic processing of 
incoming requests. 

Whenever you make an edit to the rule file, you need to use attestation to tell 
Clef that the file is 'safe' to execute.`,
	}

	addCredentialCommand = cli.Command{
		Action:    utils.MigrateFlags(addCredential),
		Name:      "addpw",
		Usage:     "Store a credential for a keystore file",
		ArgsUsage: "<address> <password>",
		Flags: []cli.Flag{
			logLevelFlag,
			configdirFlag,
			signerSecretFlag,
		},
		Description: `
The addpw command stores a password for a given address (keyfile). If you invoke it with only one parameter, it will 
remove any stored credential for that address (keyfile)
`,
	}
)

func init() {
	app.Name = "Clef"
	app.Usage = "Manage Ethereum account operations"
	app.Flags = []cli.Flag{
		logLevelFlag,
		keystoreFlag,
		configdirFlag,
		utils.NetworkIdFlag,
		utils.LightKDFFlag,
		utils.NoUSBFlag,
		utils.RPCListenAddrFlag,
		utils.RPCVirtualHostsFlag,
		utils.IPCDisabledFlag,
		utils.IPCPathFlag,
		utils.RPCEnabledFlag,
		rpcPortFlag,
		signerSecretFlag,
		dBFlag,
		customDBFlag,
		auditLogFlag,
		ruleFlag,
		stdiouiFlag,
		testFlag,
	}
	app.Action = signer
	app.Commands = []cli.Command{initCommand, attestCommand, addCredentialCommand}

}
func main() {
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func initializeSecrets(c *cli.Context) error {
	if err := initialize(c); err != nil {
		return err
	}
	configDir := c.String(configdirFlag.Name)

	masterSeed := make([]byte, 256)
	n, err := io.ReadFull(rand.Reader, masterSeed)
	if err != nil {
		return err
	}
	if n != len(masterSeed) {
		return fmt.Errorf("failed to read enough random")
	}
	err = os.Mkdir(configDir, 0700)
	if err != nil && !os.IsExist(err) {
		return err
	}
	location := filepath.Join(configDir, "secrets.dat")
	if _, err := os.Stat(location); err == nil {
		return fmt.Errorf("file %v already exists, will not overwrite", location)
	}
	err = ioutil.WriteFile(location, masterSeed, 0700)
	if err != nil {
		return err
	}
	fmt.Printf("A master seed has been generated into %s\n", location)
	fmt.Printf(`
This is required to be able to store credentials, such as : 
* Passwords for keystores (used by rule engine)
* Storage for javascript rules
* Hash of rule-file

You should treat that file with utmost secrecy, and make a backup of it. 
NOTE: This file does not contain your accounts. Those need to be backed up separately!

`)
	return nil
}
func attestFile(ctx *cli.Context) error {
	if len(ctx.Args()) < 1 {
		utils.Fatalf("This command requires an argument.")
	}
	if err := initialize(ctx); err != nil {
		return err
	}

	stretchedKey, err := readMasterKey(ctx)
	if err != nil {
		utils.Fatalf(err.Error())
	}
	configDir := ctx.String(configdirFlag.Name)
	vaultLocation := filepath.Join(configDir, common.Bytes2Hex(crypto.Keccak256([]byte("vault"), stretchedKey)[:10]))
	confKey := crypto.Keccak256([]byte("config"), stretchedKey)

//初始化加密存储
	configStorage := storage.NewAESEncryptedStorage(filepath.Join(vaultLocation, "config.json"), confKey)
	val := ctx.Args().First()
	configStorage.Put("ruleset_sha256", val)
	log.Info("Ruleset attestation updated", "sha256", val)
	return nil
}

func addCredential(ctx *cli.Context) error {
	if len(ctx.Args()) < 1 {
		utils.Fatalf("This command requires at leaste one argument.")
	}
	if err := initialize(ctx); err != nil {
		return err
	}

	stretchedKey, err := readMasterKey(ctx)
	if err != nil {
		utils.Fatalf(err.Error())
	}
	configDir := ctx.String(configdirFlag.Name)
	vaultLocation := filepath.Join(configDir, common.Bytes2Hex(crypto.Keccak256([]byte("vault"), stretchedKey)[:10]))
	pwkey := crypto.Keccak256([]byte("credentials"), stretchedKey)

//初始化加密存储
	pwStorage := storage.NewAESEncryptedStorage(filepath.Join(vaultLocation, "credentials.json"), pwkey)
	key := ctx.Args().First()
	value := ""
	if len(ctx.Args()) > 1 {
		value = ctx.Args().Get(1)
	}
	pwStorage.Put(key, value)
	log.Info("Credential store updated", "key", key)
	return nil
}

func initialize(c *cli.Context) error {
//设置记录器以打印所有内容
	logOutput := os.Stdout
	if c.Bool(stdiouiFlag.Name) {
		logOutput = os.Stderr
//如果使用stdioui，则无法执行“确认”流
		fmt.Fprintf(logOutput, legalWarning)
	} else {
		if !confirm(legalWarning) {
			return fmt.Errorf("aborted by user")
		}
	}

	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(c.Int(logLevelFlag.Name)), log.StreamHandler(logOutput, log.TerminalFormat(true))))
	return nil
}

func signer(c *cli.Context) error {
	if err := initialize(c); err != nil {
		return err
	}
	var (
		ui core.SignerUI
	)
	if c.Bool(stdiouiFlag.Name) {
		log.Info("Using stdin/stdout as UI-channel")
		ui = core.NewStdIOUI()
	} else {
		log.Info("Using CLI as UI-channel")
		ui = core.NewCommandlineUI()
	}
	db, err := core.NewAbiDBFromFiles(c.String(dBFlag.Name), c.String(customDBFlag.Name))
	if err != nil {
		utils.Fatalf(err.Error())
	}
	log.Info("Loaded 4byte db", "signatures", db.Size(), "file", c.String("4bytedb"))

	var (
		api core.ExternalAPI
	)

	configDir := c.String(configdirFlag.Name)
	if stretchedKey, err := readMasterKey(c); err != nil {
		log.Info("No master seed provided, rules disabled")
	} else {

		if err != nil {
			utils.Fatalf(err.Error())
		}
		vaultLocation := filepath.Join(configDir, common.Bytes2Hex(crypto.Keccak256([]byte("vault"), stretchedKey)[:10]))

//生成特定于域的密钥
		pwkey := crypto.Keccak256([]byte("credentials"), stretchedKey)
		jskey := crypto.Keccak256([]byte("jsstorage"), stretchedKey)
		confkey := crypto.Keccak256([]byte("config"), stretchedKey)

//初始化加密存储
		pwStorage := storage.NewAESEncryptedStorage(filepath.Join(vaultLocation, "credentials.json"), pwkey)
		jsStorage := storage.NewAESEncryptedStorage(filepath.Join(vaultLocation, "jsstorage.json"), jskey)
		configStorage := storage.NewAESEncryptedStorage(filepath.Join(vaultLocation, "config.json"), confkey)

//我们有规则文件吗？
		ruleJS, err := ioutil.ReadFile(c.String(ruleFlag.Name))
		if err != nil {
			log.Info("Could not load rulefile, rules not enabled", "file", "rulefile")
		} else {
			hasher := sha256.New()
			hasher.Write(ruleJS)
			shasum := hasher.Sum(nil)
			storedShasum := configStorage.Get("ruleset_sha256")
			if storedShasum != hex.EncodeToString(shasum) {
				log.Info("Could not validate ruleset hash, rules not enabled", "got", hex.EncodeToString(shasum), "expected", storedShasum)
			} else {
//初始化规则
				ruleEngine, err := rules.NewRuleEvaluator(ui, jsStorage, pwStorage)
				if err != nil {
					utils.Fatalf(err.Error())
				}
				ruleEngine.Init(string(ruleJS))
				ui = ruleEngine
				log.Info("Rule engine configured", "file", c.String(ruleFlag.Name))
			}
		}
	}

	apiImpl := core.NewSignerAPI(
		c.Int64(utils.NetworkIdFlag.Name),
		c.String(keystoreFlag.Name),
		c.Bool(utils.NoUSBFlag.Name),
		ui, db,
		c.Bool(utils.LightKDFFlag.Name))

	api = apiImpl

//审计日志
	if logfile := c.String(auditLogFlag.Name); logfile != "" {
		api, err = core.NewAuditLogger(logfile, api)
		if err != nil {
			utils.Fatalf(err.Error())
		}
		log.Info("Audit logs configured", "file", logfile)
	}
//向服务器注册签名者API
	var (
		extapiURL = "n/a"
		ipcapiURL = "n/a"
	)
	rpcAPI := []rpc.API{
		{
			Namespace: "account",
			Public:    true,
			Service:   api,
			Version:   "1.0"},
	}
	if c.Bool(utils.RPCEnabledFlag.Name) {

		vhosts := splitAndTrim(c.GlobalString(utils.RPCVirtualHostsFlag.Name))
		cors := splitAndTrim(c.GlobalString(utils.RPCCORSDomainFlag.Name))

//
		httpEndpoint := fmt.Sprintf("%s:%d", c.String(utils.RPCListenAddrFlag.Name), c.Int(rpcPortFlag.Name))
		listener, _, err := rpc.StartHTTPEndpoint(httpEndpoint, rpcAPI, []string{"account"}, cors, vhosts, rpc.DefaultHTTPTimeouts)
		if err != nil {
			utils.Fatalf("Could not start RPC api: %v", err)
		}
extapiURL = fmt.Sprintf("http://%s“，httpendpoint）
		log.Info("HTTP endpoint opened", "url", extapiURL)

		defer func() {
			listener.Close()
			log.Info("HTTP endpoint closed", "url", httpEndpoint)
		}()

	}
	if !c.Bool(utils.IPCDisabledFlag.Name) {
		if c.IsSet(utils.IPCPathFlag.Name) {
			ipcapiURL = c.String(utils.IPCPathFlag.Name)
		} else {
			ipcapiURL = filepath.Join(configDir, "clef.ipc")
		}

		listener, _, err := rpc.StartIPCEndpoint(ipcapiURL, rpcAPI)
		if err != nil {
			utils.Fatalf("Could not start IPC api: %v", err)
		}
		log.Info("IPC endpoint opened", "url", ipcapiURL)
		defer func() {
			listener.Close()
			log.Info("IPC endpoint closed", "url", ipcapiURL)
		}()

	}

	if c.Bool(testFlag.Name) {
		log.Info("Performing UI test")
		go testExternalUI(apiImpl)
	}
	ui.OnSignerStartup(core.StartupInfo{
		Info: map[string]interface{}{
			"extapi_version": ExternalAPIVersion,
			"intapi_version": InternalAPIVersion,
			"extapi_http":    extapiURL,
			"extapi_ipc":     ipcapiURL,
		},
	})

	abortChan := make(chan os.Signal)
	signal.Notify(abortChan, os.Interrupt)

	sig := <-abortChan
	log.Info("Exiting...", "signal", sig)

	return nil
}

//splitandtrim拆分由逗号分隔的输入
//并修剪子字符串中多余的空白。
func splitAndTrim(input string) []string {
	result := strings.Split(input, ",")
	for i, r := range result {
		result[i] = strings.TrimSpace(r)
	}
	return result
}

//
//持久性要求。
func DefaultConfigDir() string {
//尝试将数据文件夹放在用户的home目录中
	home := homeDir()
	if home != "" {
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, "Library", "Signer")
		} else if runtime.GOOS == "windows" {
			return filepath.Join(home, "AppData", "Roaming", "Signer")
		} else {
			return filepath.Join(home, ".clef")
		}
	}
//因为我们无法猜测一个稳定的位置，所以返回空的，稍后再处理
	return ""
}

func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if usr, err := user.Current(); err == nil {
		return usr.HomeDir
	}
	return ""
}
func readMasterKey(ctx *cli.Context) ([]byte, error) {
	var (
		file      string
		configDir = ctx.String(configdirFlag.Name)
	)
	if ctx.IsSet(signerSecretFlag.Name) {
		file = ctx.String(signerSecretFlag.Name)
	} else {
		file = filepath.Join(configDir, "secrets.dat")
	}
	if err := checkFile(file); err != nil {
		return nil, err
	}
	masterKey, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	if len(masterKey) < 256 {
		return nil, fmt.Errorf("master key of insufficient length, expected >255 bytes, got %d", len(masterKey))
	}
//创建保管库位置
	vaultLocation := filepath.Join(configDir, common.Bytes2Hex(crypto.Keccak256([]byte("vault"), masterKey)[:10]))
	err = os.Mkdir(vaultLocation, 0700)
	if err != nil && !os.IsExist(err) {
		return nil, err
	}
//！todo，使用kdf拉伸主密钥
//拉伸键：=拉伸键（主\键）

	return masterKey, nil
}

//check file是检查文件
//*存在
//＊模式0600
func checkFile(filename string) error {
	info, err := os.Stat(filename)
	if err != nil {
		return fmt.Errorf("failed stat on %s: %v", filename, err)
	}
//检查Unix权限位
	if info.Mode().Perm()&077 != 0 {
		return fmt.Errorf("file (%v) has insecure file permissions (%v)", filename, info.Mode().String())
	}
	return nil
}

//确认显示文本并请求用户确认
func confirm(text string) bool {
	fmt.Printf(text)
	fmt.Printf("\nEnter 'ok' to proceed:\n>")

	text, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		log.Crit("Failed to read user input", "err", err)
	}

	if text := strings.TrimSpace(text); text == "ok" {
		return true
	}
	return false
}

func testExternalUI(api *core.SignerAPI) {

	ctx := context.WithValue(context.Background(), "remote", "clef binary")
	ctx = context.WithValue(ctx, "scheme", "in-proc")
	ctx = context.WithValue(ctx, "local", "main")

	errs := make([]string, 0)

	api.UI.ShowInfo("Testing 'ShowInfo'")
	api.UI.ShowError("Testing 'ShowError'")

	checkErr := func(method string, err error) {
		if err != nil && err != core.ErrRequestDenied {
			errs = append(errs, fmt.Sprintf("%v: %v", method, err.Error()))
		}
	}
	var err error

	_, err = api.SignTransaction(ctx, core.SendTxArgs{From: common.MixedcaseAddress{}}, nil)
	checkErr("SignTransaction", err)
	_, err = api.Sign(ctx, common.MixedcaseAddress{}, common.Hex2Bytes("01020304"))
	checkErr("Sign", err)
	_, err = api.List(ctx)
	checkErr("List", err)
	_, err = api.New(ctx)
	checkErr("New", err)
	_, err = api.Export(ctx, common.Address{})
	checkErr("Export", err)
	_, err = api.Import(ctx, json.RawMessage{})
	checkErr("Import", err)

	api.UI.ShowInfo("Tests completed")

	if len(errs) > 0 {
		log.Error("Got errors")
		for _, e := range errs {
			log.Error(e)
		}
	} else {
		log.Info("No errors")
	}

}

/*


curl-h“content-type:application/json”-x post--data'“jsonrpc”：“2.0”，“method”：“account_new”，“params”：[“test”]，“id”：67“localhost:8550”

//列出帐户

curl-i-h“内容类型：application/json”-x post--data'“jsonrpc”：“2.0”，“method”：“account_list”，“params”：[“”]，“id”：67”http://localhost:8550/


//安全端（0x12）
//4401A6E4000000000000000000000000000000000000000000000000000000000012

/供给ABI



curl-i-h“content-type:application/json”-x post--data'“jsonrpc”：“2.0”，“method”：“account-signtransaction”，“params”：[“from”：“0x82A2A876D39022B3019932D30CD9C97AD5616813”，“gas”：“0x333”，“gasprice”：“0x123”，“nonce”：“0x 0”，“to”：“0x07A565B7ED7D7A68680A4C162885BEDB695FE0”，“value”：“0x10”，“data”：“0x4401A6E400000000000000000000000000000000000000000”000000000000000000 12“”，“id”：67”http://localhost:8550/



curl-i-h“内容类型：application/json”-x post--数据'“jsonrpc”：“2.0”，“方法”：“帐户符号”，“参数”：[“0x694267f14675d7e1b9494fd8d72fe1755710fa”，“bazonk gaz baz”]，“id”：67“http://localhost:8550/


*/


