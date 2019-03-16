
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:46</date>
//</624342666275983360>


package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"reflect"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/accounts/usbwallet"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/internal/ethapi"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
)

//ExternalAPI定义用于发出签名请求的外部API。
type ExternalAPI interface {
//列出可用帐户
	List(ctx context.Context) (Accounts, error)
//创建新帐户的新请求
	New(ctx context.Context) (accounts.Account, error)
//SignTransaction请求签署指定的事务
	SignTransaction(ctx context.Context, args SendTxArgs, methodSelector *string) (*ethapi.SignTransactionResult, error)
//签名-请求对给定数据进行签名（加前缀）
	Sign(ctx context.Context, addr common.MixedcaseAddress, data hexutil.Bytes) (hexutil.Bytes, error)
//ecrecover-请求执行ecrecover
	EcRecover(ctx context.Context, data, sig hexutil.Bytes) (common.Address, error)
//导出-请求导出帐户
	Export(ctx context.Context, addr common.Address) (json.RawMessage, error)
//导入-请求导入帐户
	Import(ctx context.Context, keyJSON json.RawMessage) (Account, error)
}

//SignerRui指定UI需要实现什么方法才能用作签名者的UI
type SignerUI interface {
//approvetx提示用户确认请求签署交易
	ApproveTx(request *SignTxRequest) (SignTxResponse, error)
//ApproveSignData提示用户确认请求签署数据
	ApproveSignData(request *SignDataRequest) (SignDataResponse, error)
//approveexport提示用户确认导出加密帐户json
	ApproveExport(request *ExportRequest) (ExportResponse, error)
//approveImport提示用户确认导入账号json
	ApproveImport(request *ImportRequest) (ImportResponse, error)
//批准提示用户确认列出帐户
//用户界面可以修改要列出的科目列表
	ApproveListing(request *ListRequest) (ListResponse, error)
//ApproveWaccount提示用户确认创建新帐户，并显示给调用方
	ApproveNewAccount(request *NewAccountRequest) (NewAccountResponse, error)
//ShowError向用户显示错误消息
	ShowError(message string)
//ShowInfo向用户显示信息消息
	ShowInfo(message string)
//OnApprovedTX通知用户界面一个事务已成功签名。
//用户界面可以使用此方法跟踪发送给特定收件人的邮件数量。
	OnApprovedTx(tx ethapi.SignTransactionResult)
//当签名者启动时调用OnSignerStartup，并告诉用户界面有关外部API位置和版本的信息。
//信息
	OnSignerStartup(info StartupInfo)
}

//signerapi定义了externalAPI的实际实现
type SignerAPI struct {
	chainID   *big.Int
	am        *accounts.Manager
	UI        SignerUI
	validator *Validator
}

//有关请求的元数据
type Metadata struct {
	Remote string `json:"remote"`
	Local  string `json:"local"`
	Scheme string `json:"scheme"`
}

//MetadataFromContext从给定的Context.Context中提取元数据
func MetadataFromContext(ctx context.Context) Metadata {
m := Metadata{"NA", "NA", "NA"} //蝙蝠侠

	if v := ctx.Value("remote"); v != nil {
		m.Remote = v.(string)
	}
	if v := ctx.Value("scheme"); v != nil {
		m.Scheme = v.(string)
	}
	if v := ctx.Value("local"); v != nil {
		m.Local = v.(string)
	}
	return m
}

//字符串实现字符串接口
func (m Metadata) String() string {
	s, err := json.Marshal(m)
	if err == nil {
		return string(s)
	}
	return err.Error()
}

//签名者和用户界面之间的请求/响应类型的类型
type (
//signtxrequest包含要签名的事务的信息
	SignTxRequest struct {
		Transaction SendTxArgs       `json:"transaction"`
		Callinfo    []ValidationInfo `json:"call_info"`
		Meta        Metadata         `json:"meta"`
	}
//SigntxRequest的SigntxResponse结果
	SignTxResponse struct {
//用户界面可以更改Tx
		Transaction SendTxArgs `json:"transaction"`
		Approved    bool       `json:"approved"`
		Password    string     `json:"password"`
	}
//将有关查询的信息导出到导出帐户
	ExportRequest struct {
		Address common.Address `json:"address"`
		Meta    Metadata       `json:"meta"`
	}
//导出响应对导出请求的响应
	ExportResponse struct {
		Approved bool `json:"approved"`
	}
//导入请求有关导入帐户请求的信息
	ImportRequest struct {
		Meta Metadata `json:"meta"`
	}
	ImportResponse struct {
		Approved    bool   `json:"approved"`
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	SignDataRequest struct {
		Address common.MixedcaseAddress `json:"address"`
		Rawdata hexutil.Bytes           `json:"raw_data"`
		Message string                  `json:"message"`
		Hash    hexutil.Bytes           `json:"hash"`
		Meta    Metadata                `json:"meta"`
	}
	SignDataResponse struct {
		Approved bool `json:"approved"`
		Password string
	}
	NewAccountRequest struct {
		Meta Metadata `json:"meta"`
	}
	NewAccountResponse struct {
		Approved bool   `json:"approved"`
		Password string `json:"password"`
	}
	ListRequest struct {
		Accounts []Account `json:"accounts"`
		Meta     Metadata  `json:"meta"`
	}
	ListResponse struct {
		Accounts []Account `json:"accounts"`
	}
	Message struct {
		Text string `json:"text"`
	}
	StartupInfo struct {
		Info map[string]interface{} `json:"info"`
	}
)

var ErrRequestDenied = errors.New("Request denied")

//NewSignerAPI创建了一个新的可用于帐户管理的API。
//kslocation指定存储受密码保护的private的目录
//创建新帐户时生成的键。
//nousb禁用支持硬件设备所需的USB支持，如
//Ledger和Trezor。
func NewSignerAPI(chainID int64, ksLocation string, noUSB bool, ui SignerUI, abidb *AbiDb, lightKDF bool) *SignerAPI {
	var (
		backends []accounts.Backend
		n, p     = keystore.StandardScryptN, keystore.StandardScryptP
	)
	if lightKDF {
		n, p = keystore.LightScryptN, keystore.LightScryptP
	}
//支持基于密码的帐户
	if len(ksLocation) > 0 {
		backends = append(backends, keystore.NewKeyStore(ksLocation, n, p))
	}
	if !noUSB {
//启动用于分类帐硬件钱包的USB集线器
		if ledgerhub, err := usbwallet.NewLedgerHub(); err != nil {
			log.Warn(fmt.Sprintf("Failed to start Ledger hub, disabling: %v", err))
		} else {
			backends = append(backends, ledgerhub)
			log.Debug("Ledger support enabled")
		}
//启动Trezor硬件钱包的USB集线器
		if trezorhub, err := usbwallet.NewTrezorHub(); err != nil {
			log.Warn(fmt.Sprintf("Failed to start Trezor hub, disabling: %v", err))
		} else {
			backends = append(backends, trezorhub)
			log.Debug("Trezor support enabled")
		}
	}
	return &SignerAPI{big.NewInt(chainID), accounts.NewManager(backends...), ui, NewValidator(abidb)}
}

//list返回签名者管理的钱包集。每个钱包都可以包含
//多个帐户。
func (api *SignerAPI) List(ctx context.Context) (Accounts, error) {
	var accs []Account
	for _, wallet := range api.am.Wallets() {
		for _, acc := range wallet.Accounts() {
			acc := Account{Typ: "Account", URL: wallet.URL(), Address: acc.Address}
			accs = append(accs, acc)
		}
	}
	result, err := api.UI.ApproveListing(&ListRequest{Accounts: accs, Meta: MetadataFromContext(ctx)})
	if err != nil {
		return nil, err
	}
	if result.Accounts == nil {
		return nil, ErrRequestDenied

	}
	return result.Accounts, nil
}

//新建创建新的密码保护帐户。私钥受保护
//给定的密码。用户负责备份存储的私钥
//在密钥库位置中，创建此API时指定了THA。
func (api *SignerAPI) New(ctx context.Context) (accounts.Account, error) {
	be := api.am.Backends(keystore.KeyStoreType)
	if len(be) == 0 {
		return accounts.Account{}, errors.New("password based accounts not supported")
	}
	resp, err := api.UI.ApproveNewAccount(&NewAccountRequest{MetadataFromContext(ctx)})

	if err != nil {
		return accounts.Account{}, err
	}
	if !resp.Approved {
		return accounts.Account{}, ErrRequestDenied
	}
	return be[0].(*keystore.KeyStore).NewAccount(resp.Password)
}

//logdiff记录传入（原始）事务和从签名者返回的事务之间的差异。
//如果修改了事务，它还返回“true”，以便可以将签名者配置为不允许
//请求的用户界面修改
func logDiff(original *SignTxRequest, new *SignTxResponse) bool {
	modified := false
	if f0, f1 := original.Transaction.From, new.Transaction.From; !reflect.DeepEqual(f0, f1) {
		log.Info("Sender-account changed by UI", "was", f0, "is", f1)
		modified = true
	}
	if t0, t1 := original.Transaction.To, new.Transaction.To; !reflect.DeepEqual(t0, t1) {
		log.Info("Recipient-account changed by UI", "was", t0, "is", t1)
		modified = true
	}
	if g0, g1 := original.Transaction.Gas, new.Transaction.Gas; g0 != g1 {
		modified = true
		log.Info("Gas changed by UI", "was", g0, "is", g1)
	}
	if g0, g1 := big.Int(original.Transaction.GasPrice), big.Int(new.Transaction.GasPrice); g0.Cmp(&g1) != 0 {
		modified = true
		log.Info("GasPrice changed by UI", "was", g0, "is", g1)
	}
	if v0, v1 := big.Int(original.Transaction.Value), big.Int(new.Transaction.Value); v0.Cmp(&v1) != 0 {
		modified = true
		log.Info("Value changed by UI", "was", v0, "is", v1)
	}
	if d0, d1 := original.Transaction.Data, new.Transaction.Data; d0 != d1 {
		d0s := ""
		d1s := ""
		if d0 != nil {
			d0s = common.ToHex(*d0)
		}
		if d1 != nil {
			d1s = common.ToHex(*d1)
		}
		if d1s != d0s {
			modified = true
			log.Info("Data changed by UI", "was", d0s, "is", d1s)
		}
	}
	if n0, n1 := original.Transaction.Nonce, new.Transaction.Nonce; n0 != n1 {
		modified = true
		log.Info("Nonce changed by UI", "was", n0, "is", n1)
	}
	return modified
}

//signTransaction对给定的事务进行签名，并将其作为json和rlp编码的形式返回
func (api *SignerAPI) SignTransaction(ctx context.Context, args SendTxArgs, methodSelector *string) (*ethapi.SignTransactionResult, error) {
	var (
		err    error
		result SignTxResponse
	)
	msgs, err := api.validator.ValidateTransaction(&args, methodSelector)
	if err != nil {
		return nil, err
	}

	req := SignTxRequest{
		Transaction: args,
		Meta:        MetadataFromContext(ctx),
		Callinfo:    msgs.Messages,
	}
//工艺批准
	result, err = api.UI.ApproveTx(&req)
	if err != nil {
		return nil, err
	}
	if !result.Approved {
		return nil, ErrRequestDenied
	}
//记录用户界面对签名请求所做的更改
	logDiff(&req, &result)
	var (
		acc    accounts.Account
		wallet accounts.Wallet
	)
	acc = accounts.Account{Address: result.Transaction.From.Address()}
	wallet, err = api.am.Find(acc)
	if err != nil {
		return nil, err
	}
//将字段转换为实际事务
	var unsignedTx = result.Transaction.toTransaction()

//要签名的是从UI返回的那个
	signedTx, err := wallet.SignTxWithPassphrase(acc, result.Password, unsignedTx, api.chainID)
	if err != nil {
		api.UI.ShowError(err.Error())
		return nil, err
	}

	rlpdata, err := rlp.EncodeToBytes(signedTx)
	response := ethapi.SignTransactionResult{Raw: rlpdata, Tx: signedTx}

//最后，将签名的Tx发送到UI
	api.UI.OnApprovedTx(response)
//…和外部呼叫者
	return &response, nil

}

//sign计算以太坊ECDSA签名：
//keccack256（“\x19ethereum签名消息：\n”+len（消息）+消息）
//
//注：生成的签名符合secp256k1曲线r、s和v值，
//由于遗产原因，V值将为27或28。
//
//用于计算签名的密钥用给定的密码解密。
//
//https://github.com/ethereum/go-ethereum/wiki/management-apis个人签名
func (api *SignerAPI) Sign(ctx context.Context, addr common.MixedcaseAddress, data hexutil.Bytes) (hexutil.Bytes, error) {
	sighash, msg := SignHash(data)
//我们在查询是否有账户之前提出请求，以防止
//通过API进行帐户枚举
	req := &SignDataRequest{Address: addr, Rawdata: data, Message: msg, Hash: sighash, Meta: MetadataFromContext(ctx)}
	res, err := api.UI.ApproveSignData(req)

	if err != nil {
		return nil, err
	}
	if !res.Approved {
		return nil, ErrRequestDenied
	}
//查找包含请求签名者的钱包
	account := accounts.Account{Address: addr.Address()}
	wallet, err := api.am.Find(account)
	if err != nil {
		return nil, err
	}
//集合用钱包签名数据
	signature, err := wallet.SignHashWithPassphrase(account, res.Password, sighash)
	if err != nil {
		api.UI.ShowError(err.Error())
		return nil, err
	}
signature[64] += 27 //根据黄纸将V从0/1转换为27/28
	return signature, nil
}

//ecrecover返回用于创建签名的帐户的地址。
//注意，此功能与ETH标志和个人标志兼容。因此，它恢复了
//地址：
//hash=keccak256（“\x19ethereum签名消息：\n”$消息长度$消息）
//addr=ecrecover（哈希，签名）
//
//注意，签名必须符合secp256k1曲线r、s和v值，其中
//由于遗留原因，V值必须是27或28。
//
//https://github.com/ethereum/go-ethereum/wiki/management-apis个人\u-ecrecover
func (api *SignerAPI) EcRecover(ctx context.Context, data, sig hexutil.Bytes) (common.Address, error) {
	if len(sig) != 65 {
		return common.Address{}, fmt.Errorf("signature must be 65 bytes long")
	}
	if sig[64] != 27 && sig[64] != 28 {
		return common.Address{}, fmt.Errorf("invalid Ethereum signature (V is not 27 or 28)")
	}
sig[64] -= 27 //将黄纸V从27/28转换为0/1
	hash, _ := SignHash(data)
	rpk, err := crypto.SigToPub(hash, sig)
	if err != nil {
		return common.Address{}, err
	}
	return crypto.PubkeyToAddress(*rpk), nil
}

//signhash是一个帮助函数，用于计算给定消息的哈希
//安全地用于计算签名。
//
//哈希计算为
//keccak256（“\x19ethereum签名消息：\n”$消息长度$消息）。
//
//这将为已签名的消息提供上下文，并防止对事务进行签名。
func SignHash(data []byte) ([]byte, string) {
	msg := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(data), data)
	return crypto.Keccak256([]byte(msg)), msg
}

//export以Web3密钥库格式返回与给定地址关联的加密私钥。
func (api *SignerAPI) Export(ctx context.Context, addr common.Address) (json.RawMessage, error) {
	res, err := api.UI.ApproveExport(&ExportRequest{Address: addr, Meta: MetadataFromContext(ctx)})

	if err != nil {
		return nil, err
	}
	if !res.Approved {
		return nil, ErrRequestDenied
	}
//查找包含请求签名者的钱包
	wallet, err := api.am.Find(accounts.Account{Address: addr})
	if err != nil {
		return nil, err
	}
	if wallet.URL().Scheme != keystore.KeyStoreScheme {
		return nil, fmt.Errorf("Account is not a keystore-account")
	}
	return ioutil.ReadFile(wallet.URL().Path)
}

//import尝试在本地密钥库中导入给定的keyjson。keyjson数据应为
//以Web3密钥库格式。它将使用给定的密码短语解密keyjson，并在成功时
//解密它将使用给定的新密码短语加密密钥，并将其存储在密钥库中。
func (api *SignerAPI) Import(ctx context.Context, keyJSON json.RawMessage) (Account, error) {
	be := api.am.Backends(keystore.KeyStoreType)

	if len(be) == 0 {
		return Account{}, errors.New("password based accounts not supported")
	}
	res, err := api.UI.ApproveImport(&ImportRequest{Meta: MetadataFromContext(ctx)})

	if err != nil {
		return Account{}, err
	}
	if !res.Approved {
		return Account{}, ErrRequestDenied
	}
	acc, err := be[0].(*keystore.KeyStore).Import(keyJSON, res.OldPassword, res.NewPassword)
	if err != nil {
		api.UI.ShowError(err.Error())
		return Account{}, err
	}
	return Account{Typ: "Account", URL: acc.URL, Address: acc.Address}, nil
}

