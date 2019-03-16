
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:28</date>
//</624342590287777792>


//
package main

//go:generate go bindata-nometadata-o website.go水龙头.html
//go：生成gofmt-w-s网站。go

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/ethstats"
	"github.com/ethereum/go-ethereum/les"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/ethereum/go-ethereum/p2p/nat"
	"github.com/ethereum/go-ethereum/params"
	"golang.org/x/net/websocket"
)




var (
	genesisFlag = flag.String("genesis", "", "Genesis json file to seed the chain with")
	apiPortFlag = flag.Int("apiport", 8080, "Listener port for the HTTP API connection")
	ethPortFlag = flag.Int("ethport", 30303, "Listener port for the devp2p connection")
	bootFlag    = flag.String("bootnodes", "", "Comma separated bootnode enode URLs to seed with")
	netFlag     = flag.Uint64("network", 0, "Network ID to use for the Ethereum protocol")
	statsFlag   = flag.String("ethstats", "", "Ethstats network monitoring auth string")

	netnameFlag = flag.String("faucet.name", "", "Network name to assign to the faucet")
	payoutFlag  = flag.Int("faucet.amount", 1, "Number of Ethers to pay out per user request")
	minutesFlag = flag.Int("faucet.minutes", 1440, "Number of minutes to wait between funding rounds")
	tiersFlag   = flag.Int("faucet.tiers", 3, "Number of funding tiers to enable (x3 time, x2.5 funds)")

	accJSONFlag = flag.String("account.json", "", "Key json file to fund user requests with")
	accPassFlag = flag.String("account.pass", "", "Decryption password to access faucet funds")

	captchaToken  = flag.String("captcha.token", "", "Recaptcha site key to authenticate client side")
	captchaSecret = flag.String("captcha.secret", "", "Recaptcha secret key to authenticate server side")

	noauthFlag = flag.Bool("noauth", false, "Enables funding requests without authentication")
	logFlag    = flag.Int("loglevel", 3, "Log level to use for Ethereum and the faucet")
)

var (
	ether = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
)

func main() {
//分析标志并设置记录器以打印所请求的所有内容
	flag.Parse()
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(*logFlag), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

//构建支出层
	amounts := make([]string, *tiersFlag)
	periods := make([]string, *tiersFlag)
	for i := 0; i < *tiersFlag; i++ {
//计算下一层的金额并格式化
		amount := float64(*payoutFlag) * math.Pow(2.5, float64(i))
		amounts[i] = fmt.Sprintf("%s Ethers", strconv.FormatFloat(amount, 'f', -1, 64))
		if amount == 1 {
			amounts[i] = strings.TrimSuffix(amounts[i], "s")
		}
//计算下一层的期间并设置其格式
		period := *minutesFlag * int(math.Pow(3, float64(i)))
		periods[i] = fmt.Sprintf("%d mins", period)
		if period%60 == 0 {
			period /= 60
			periods[i] = fmt.Sprintf("%d hours", period)

			if period%24 == 0 {
				period /= 24
				periods[i] = fmt.Sprintf("%d days", period)
			}
		}
		if period == 1 {
			periods[i] = strings.TrimSuffix(periods[i], "s")
		}
	}
//加载并呈现水龙头网站
	tmpl, err := Asset("faucet.html")
	if err != nil {
		log.Crit("Failed to load the faucet template", "err", err)
	}
	website := new(bytes.Buffer)
	err = template.Must(template.New("").Parse(string(tmpl))).Execute(website, map[string]interface{}{
		"Network":   *netnameFlag,
		"Amounts":   amounts,
		"Periods":   periods,
		"Recaptcha": *captchaToken,
		"NoAuth":    *noauthFlag,
	})
	if err != nil {
		log.Crit("Failed to render the faucet template", "err", err)
	}
//加载并分析用户请求的Genesis块
	blob, err := ioutil.ReadFile(*genesisFlag)
	if err != nil {
		log.Crit("Failed to read genesis block contents", "genesis", *genesisFlag, "err", err)
	}
	genesis := new(core.Genesis)
	if err = json.Unmarshal(blob, genesis); err != nil {
		log.Crit("Failed to parse genesis block json", "err", err)
	}
//将bootnode转换为内部enode表示形式
	var enodes []*discv5.Node
	for _, boot := range strings.Split(*bootFlag, ",") {
		if url, err := discv5.ParseNode(boot); err == nil {
			enodes = append(enodes, url)
		} else {
			log.Error("Failed to parse bootnode URL", "url", boot, "err", err)
		}
	}
//加载帐户密钥并解密其密码
	if blob, err = ioutil.ReadFile(*accPassFlag); err != nil {
		log.Crit("Failed to read account password contents", "file", *accPassFlag, "err", err)
	}
	pass := string(blob)

	ks := keystore.NewKeyStore(filepath.Join(os.Getenv("HOME"), ".faucet", "keys"), keystore.StandardScryptN, keystore.StandardScryptP)
	if blob, err = ioutil.ReadFile(*accJSONFlag); err != nil {
		log.Crit("Failed to read account key contents", "file", *accJSONFlag, "err", err)
	}
	acc, err := ks.Import(blob, pass, pass)
	if err != nil {
		log.Crit("Failed to import faucet signer account", "err", err)
	}
	ks.Unlock(acc, pass)

//组装并启动水龙头照明服务
	faucet, err := newFaucet(genesis, *ethPortFlag, enodes, *netFlag, *statsFlag, ks, website.Bytes())
	if err != nil {
		log.Crit("Failed to start faucet", "err", err)
	}
	defer faucet.close()

	if err := faucet.listenAndServe(*apiPortFlag); err != nil {
		log.Crit("Failed to launch faucet API", "err", err)
	}
}

//请求表示已接受的资金请求。
type request struct {
Avatar  string             `json:"avatar"`  //使用户界面更美好的虚拟人物URL
Account common.Address     `json:"account"` //正在资助以太坊地址
Time    time.Time          `json:"time"`    //接受请求时的时间戳
Tx      *types.Transaction `json:"tx"`      //为账户提供资金的交易
}

//
type faucet struct {
config *params.ChainConfig //签名的链配置
stack  *node.Node          //以太坊协议栈
client *ethclient.Client   //
index  []byte              //在网上提供的索引页

keystore *keystore.KeyStore //包含单个签名者的密钥库
account  accounts.Account   //帐户资金用户水龙头请求
nonce    uint64             //水龙头的当前挂起时间
price    *big.Int           //发行资金的当前天然气价格

conns    []*websocket.Conn    //当前活动的WebSocket连接
timeouts map[string]time.Time //用户历史及其资金超时
reqs     []*request           //当前待定的资金请求
update   chan struct{}        //通道到信号请求更新

lock sync.RWMutex //锁保护水龙头内部
}

func newFaucet(genesis *core.Genesis, port int, enodes []*discv5.Node, network uint64, stats string, ks *keystore.KeyStore, index []byte) (*faucet, error) {
//组装原始devp2p协议栈
	stack, err := node.New(&node.Config{
		Name:    "geth",
		Version: params.VersionWithMeta,
		DataDir: filepath.Join(os.Getenv("HOME"), ".faucet"),
		P2P: p2p.Config{
			NAT:              nat.Any(),
			NoDiscovery:      true,
			DiscoveryV5:      true,
			ListenAddr:       fmt.Sprintf(":%d", port),
			MaxPeers:         25,
			BootstrapNodesV5: enodes,
		},
	})
	if err != nil {
		return nil, err
	}
//组装以太坊Light客户端协议
	if err := stack.Register(func(ctx *node.ServiceContext) (node.Service, error) {
		cfg := eth.DefaultConfig
		cfg.SyncMode = downloader.LightSync
		cfg.NetworkId = network
		cfg.Genesis = genesis
		return les.New(ctx, &cfg)
	}); err != nil {
		return nil, err
	}
//组装ethstats监视和报告服务'
	if stats != "" {
		if err := stack.Register(func(ctx *node.ServiceContext) (node.Service, error) {
			var serv *les.LightEthereum
			ctx.Service(&serv)
			return ethstats.New(stats, nil, serv)
		}); err != nil {
			return nil, err
		}
	}
//启动客户机并确保它连接到引导节点
	if err := stack.Start(); err != nil {
		return nil, err
	}
	for _, boot := range enodes {
		old, _ := discover.ParseNode(boot.String())
		stack.Server().AddPeer(old)
	}
//附加到客户端并检索有趣的元数据
	api, err := stack.Attach()
	if err != nil {
		stack.Stop()
		return nil, err
	}
	client := ethclient.NewClient(api)

	return &faucet{
		config:   genesis.Config,
		stack:    stack,
		client:   client,
		index:    index,
		keystore: ks,
		account:  ks.Accounts()[0],
		timeouts: make(map[string]time.Time),
		update:   make(chan struct{}, 1),
	}, nil
}

//关闭会终止以太坊连接并将水龙头拆下。
func (f *faucet) close() error {
	return f.stack.Stop()
}

//listenandserve注册水龙头的HTTP处理程序并启动它。
//服务用户资金请求。
func (f *faucet) listenAndServe(port int) error {
	go f.loop()

	http.HandleFunc("/", f.webHandler)
	http.Handle("/api", websocket.Handler(f.apiHandler))

	return http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}

//WebHandler处理所有非API请求，只需扁平化并返回
//水龙头网站。
func (f *faucet) webHandler(w http.ResponseWriter, r *http.Request) {
	w.Write(f.index)
}

//apiHandler处理乙醚授权和事务状态的请求。
func (f *faucet) apiHandler(conn *websocket.Conn) {
//开始跟踪连接并在末尾放置
	defer conn.Close()

	f.lock.Lock()
	f.conns = append(f.conns, conn)
	f.lock.Unlock()

	defer func() {
		f.lock.Lock()
		for i, c := range f.conns {
			if c == conn {
				f.conns = append(f.conns[:i], f.conns[i+1:]...)
				break
			}
		}
		f.lock.Unlock()
	}()
//从网络收集初始统计数据以进行报告
	var (
		head    *types.Header
		balance *big.Int
		nonce   uint64
		err     error
	)
	for {
//尝试检索统计信息，可能在没有水龙头连接时出错。
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		head, err = f.client.HeaderByNumber(ctx, nil)
		if err == nil {
			balance, err = f.client.BalanceAt(ctx, f.account.Address, head.Number)
			if err == nil {
				nonce, err = f.client.NonceAt(ctx, f.account.Address, nil)
			}
		}
		cancel()

//如果状态检索失败，请稍等片刻，然后重试。
		if err != nil {
			if err = sendError(conn, errors.New("Faucet offline: "+err.Error())); err != nil {
				log.Warn("Failed to send faucet error to client", "err", err)
				return
			}
			time.Sleep(3 * time.Second)
			continue
		}
//已成功报告初始状态，继续用户交互
		break
	}
//发送初始数据和最新标题
	if err = send(conn, map[string]interface{}{
		"funds":    balance.Div(balance, ether),
		"funded":   nonce,
		"peers":    f.stack.Server().PeerCount(),
		"requests": f.reqs,
	}, 3*time.Second); err != nil {
		log.Warn("Failed to send initial stats to client", "err", err)
		return
	}
	if err = send(conn, head, 3*time.Second); err != nil {
		log.Warn("Failed to send initial header to client", "err", err)
		return
	}
//继续从WebSocket读取请求，直到连接断开
	for {
//获取下一个融资请求并根据Github进行验证
		var msg struct {
			URL     string `json:"url"`
			Tier    uint   `json:"tier"`
			Captcha string `json:"captcha"`
		}
		if err = websocket.JSON.Receive(conn, &msg); err != nil {
			return
		}
if !*noauthFlag && !strings.HasPrefix(msg.URL, "https://
!strings.HasPrefix(msg.URL, "https://另外，google.com/“）&&！strings.hasPrefix（msg.url，“https://www.facebook.com/”）
			if err = sendError(conn, errors.New("URL doesn't link to supported services")); err != nil {
				log.Warn("Failed to send URL error to client", "err", err)
				return
			}
			continue
		}
		if msg.Tier >= uint(*tiersFlag) {
			if err = sendError(conn, errors.New("Invalid funding tier requested")); err != nil {
				log.Warn("Failed to send tier error to client", "err", err)
				return
			}
			continue
		}
		log.Info("Faucet funds requested", "url", msg.URL, "tier", msg.Tier)

//如果验证码验证被启用，确保我们没有处理机器人
		if *captchaToken != "" {
			form := url.Values{}
			form.Add("secret", *captchaSecret)
			form.Add("response", msg.Captcha)

res, err := http.PostForm("https://www.google.com/recaptcha/api/siteverify“，表单）
			if err != nil {
				if err = sendError(conn, err); err != nil {
					log.Warn("Failed to send captcha post error to client", "err", err)
					return
				}
				continue
			}
			var result struct {
				Success bool            `json:"success"`
				Errors  json.RawMessage `json:"error-codes"`
			}
			err = json.NewDecoder(res.Body).Decode(&result)
			res.Body.Close()
			if err != nil {
				if err = sendError(conn, err); err != nil {
					log.Warn("Failed to send captcha decode error to client", "err", err)
					return
				}
				continue
			}
			if !result.Success {
				log.Warn("Captcha verification failed", "err", string(result.Errors))
				if err = sendError(conn, errors.New("Beep-bop, you're a robot!")); err != nil {
					log.Warn("Failed to send captcha failure to client", "err", err)
					return
				}
				continue
			}
		}
//检索以太坊资金地址、请求用户和个人资料图片
		var (
			username string
			avatar   string
			address  common.Address
		)
		switch {
case strings.HasPrefix(msg.URL, "https://gist.github.com/“）：
			if err = sendError(conn, errors.New("GitHub authentication discontinued at the official request of GitHub")); err != nil {
				log.Warn("Failed to send GitHub deprecation to client", "err", err)
				return
			}
			continue
case strings.HasPrefix(msg.URL, "https://Twitter .com /（）：
			username, avatar, address, err = authTwitter(msg.URL)
case strings.HasPrefix(msg.URL, "https://加上google.com/“）：
			username, avatar, address, err = authGooglePlus(msg.URL)
case strings.HasPrefix(msg.URL, "https://www.facebook.com/“）：
			username, avatar, address, err = authFacebook(msg.URL)
		case *noauthFlag:
			username, avatar, address, err = authNoAuth(msg.URL)
		default:
err = errors.New("Something funky happened, please open an issue at https://github.com/haxicode/go-ethereum/issues“）
		}
		if err != nil {
			if err = sendError(conn, err); err != nil {
				log.Warn("Failed to send prefix error to client", "err", err)
				return
			}
			continue
		}
		log.Info("Faucet request valid", "url", msg.URL, "tier", msg.Tier, "user", username, "address", address)

//确保用户最近没有申请资金
		f.lock.Lock()
		var (
			fund    bool
			timeout time.Time
		)
		if timeout = f.timeouts[username]; time.Now().After(timeout) {
//用户最近没有资金，创建资金交易
			amount := new(big.Int).Mul(big.NewInt(int64(*payoutFlag)), ether)
			amount = new(big.Int).Mul(amount, new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(msg.Tier)), nil))
			amount = new(big.Int).Div(amount, new(big.Int).Exp(big.NewInt(2), big.NewInt(int64(msg.Tier)), nil))

			tx := types.NewTransaction(types.Binary, f.nonce+uint64(len(f.reqs)), address, amount, 21000, f.price, nil)
			signed, err := f.keystore.SignTx(f.account, tx, f.config.ChainID)
			if err != nil {
				f.lock.Unlock()
				if err = sendError(conn, err); err != nil {
					log.Warn("Failed to send transaction creation error to client", "err", err)
					return
				}
				continue
			}
//
			if err := f.client.SendTransaction(context.Background(), signed); err != nil {
				f.lock.Unlock()
				if err = sendError(conn, err); err != nil {
					log.Warn("Failed to send transaction transmission error to client", "err", err)
					return
				}
				continue
			}
			f.reqs = append(f.reqs, &request{
				Avatar:  avatar,
				Account: address,
				Time:    time.Now(),
				Tx:      signed,
			})
			f.timeouts[username] = time.Now().Add(time.Duration(*minutesFlag*int(math.Pow(3, float64(msg.Tier)))) * time.Minute)
			fund = true
		}
		f.lock.Unlock()

//如果融资过于频繁，则发送错误，否则将成功
		if !fund {
if err = sendError(conn, fmt.Errorf("%s left until next allowance", common.PrettyDuration(timeout.Sub(time.Now())))); err != nil { //诺林：天哪
				log.Warn("Failed to send funding error to client", "err", err)
				return
			}
			continue
		}
		if err = sendSuccess(conn, fmt.Sprintf("Funding request accepted for %s into %s", username, address.Hex())); err != nil {
			log.Warn("Failed to send funding success to client", "err", err)
			return
		}
		select {
		case f.update <- struct{}{}:
		default:
		}
	}
}

//循环一直在等待有趣的事件，并将它们推出到Connected
//WebSoCukes。
func (f *faucet) loop() {
//等待链事件并将其推送到客户端
	heads := make(chan *types.Header, 16)
	sub, err := f.client.SubscribeNewHead(context.Background(), heads)
	if err != nil {
		log.Crit("Failed to subscribe to head events", "err", err)
	}
	defer sub.Unsubscribe()

//启动goroutine以从后台的头通知更新状态
	update := make(chan *types.Header)

	go func() {
		for head := range update {
//新的链头到达，查询当前状态并流到客户端
			var (
				balance *big.Int
				nonce   uint64
				price   *big.Int
				err     error
			)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			balance, err = f.client.BalanceAt(ctx, f.account.Address, head.Number)
			if err == nil {
				nonce, err = f.client.NonceAt(ctx, f.account.Address, nil)
				if err == nil {
					price, err = f.client.SuggestGasPrice(ctx)
				}
			}
			cancel()

//如果查询数据失败，请尝试下一个块
			if err != nil {
				log.Warn("Failed to update faucet state", "block", head.Number, "hash", head.Hash(), "err", err)
				continue
			} else {
				log.Info("Updated faucet state", "block", head.Number, "hash", head.Hash(), "balance", balance, "nonce", nonce, "price", price)
			}
//检索水龙头状态，本地更新并发送给客户
			balance = new(big.Int).Div(balance, ether)

			f.lock.Lock()
			f.price, f.nonce = price, nonce
			for len(f.reqs) > 0 && f.reqs[0].Tx.Nonce() < f.nonce {
				f.reqs = f.reqs[1:]
			}
			f.lock.Unlock()

			f.lock.RLock()
			for _, conn := range f.conns {
				if err := send(conn, map[string]interface{}{
					"funds":    balance,
					"funded":   f.nonce,
					"peers":    f.stack.Server().PeerCount(),
					"requests": f.reqs,
				}, time.Second); err != nil {
					log.Warn("Failed to send stats to client", "err", err)
					conn.Close()
					continue
				}
				if err := send(conn, head, time.Second); err != nil {
					log.Warn("Failed to send header to client", "err", err)
					conn.Close()
				}
			}
			f.lock.RUnlock()
		}
	}()
//等待各种事件并分配到适当的后台线程
	for {
		select {
		case head := <-heads:
//新的头已到达，如果没有运行则发送if以进行状态更新
			select {
			case update <- head:
			default:
			}

		case <-f.update:
//更新了挂起的请求，流式传输到客户端
			f.lock.RLock()
			for _, conn := range f.conns {
				if err := send(conn, map[string]interface{}{"requests": f.reqs}, time.Second); err != nil {
					log.Warn("Failed to send requests to client", "err", err)
					conn.Close()
				}
			}
			f.lock.RUnlock()
		}
	}
}

//发送数据包到WebSocket的远程端，但也
//设置写入截止时间以防止永远等待在节点上。
func send(conn *websocket.Conn, value interface{}, timeout time.Duration) error {
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	conn.SetWriteDeadline(time.Now().Add(timeout))
	return websocket.JSON.Send(conn, value)
}

//sendError将错误传输到WebSocket的远程端，同时设置
//写截止时间为1秒，以防永远等待。
func sendError(conn *websocket.Conn, err error) error {
	return send(conn, map[string]string{"error": err.Error()}, time.Second)
}

//sendssuccess还将成功消息发送到websocket的远程端
//将写入截止时间设置为1秒，以防止永远等待。
func sendSuccess(conn *websocket.Conn, msg string) error {
	return send(conn, map[string]string{"success": msg}, time.Second)
}

//AuthTwitter尝试使用Twitter帖子验证水龙头请求，返回
//用户名、虚拟人物URL和以太坊地址将在成功时提供资金。
func authTwitter(url string) (string, string, common.Address, error) {
//确保用户指定了一个有意义的URL，没有花哨的胡说八道。
	parts := strings.Split(url, "/")
	if len(parts) < 4 || parts[len(parts)-2] != "status" {
		return "", "", common.Address{}, errors.New("Invalid Twitter status URL")
	}
//
//想做的是询问用户的读权限，所以只需加载公共文章和
//从以太坊地址和配置文件URL中清除它。
	res, err := http.Get(url)
	if err != nil {
		return "", "", common.Address{}, err
	}
	defer res.Body.Close()

//从最终重定向中解析用户名，无中间垃圾邮件
	parts = strings.Split(res.Request.URL.String(), "/")
	if len(parts) < 4 || parts[len(parts)-2] != "status" {
		return "", "", common.Address{}, errors.New("Invalid Twitter status URL")
	}
	username := parts[len(parts)-3]

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", "", common.Address{}, err
	}
	address := common.HexToAddress(string(regexp.MustCompile("0x[0-9a-fA-F]{40}").Find(body)))
	if address == (common.Address{}) {
		return "", "", common.Address{}, errors.New("No Ethereum address found to fund")
	}
	var avatar string
	if parts = regexp.MustCompile("src=\"([^\"]+twimg.com/profile_images[^\"]+)\"").FindStringSubmatch(string(body)); len(parts) == 2 {
		avatar = parts[1]
	}
	return username + "@twitter", avatar, address, nil
}

//authgoogleplus尝试使用googleplus帖子验证水龙头请求，
//成功后返回用户名、虚拟人物URL和以太坊地址进行投资。
func authGooglePlus(url string) (string, string, common.Address, error) {
//确保用户指定了一个有意义的URL，没有花哨的胡说八道。
	parts := strings.Split(url, "/")
	if len(parts) < 4 || parts[len(parts)-2] != "posts" {
		return "", "", common.Address{}, errors.New("Invalid Google+ post URL")
	}
	username := parts[len(parts)-3]

//谷歌的API对直接链接不是很友好。但是，我们没有
//想做的是询问用户的读权限，所以只需加载公共文章和
//从以太坊地址和配置文件URL中清除它。
	res, err := http.Get(url)
	if err != nil {
		return "", "", common.Address{}, err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", "", common.Address{}, err
	}
	address := common.HexToAddress(string(regexp.MustCompile("0x[0-9a-fA-F]{40}").Find(body)))
	if address == (common.Address{}) {
		return "", "", common.Address{}, errors.New("No Ethereum address found to fund")
	}
	var avatar string
	if parts = regexp.MustCompile("src=\"([^\"]+googleusercontent.com[^\"]+photo.jpg)\"").FindStringSubmatch(string(body)); len(parts) == 2 {
		avatar = parts[1]
	}
	return username + "@google+", avatar, address, nil
}

//AuthFacebook尝试使用Facebook帖子验证水龙头请求，
//成功后返回用户名、虚拟人物URL和以太坊地址进行投资。
func authFacebook(url string) (string, string, common.Address, error) {
//确保用户指定了一个有意义的URL，没有花哨的胡说八道。
	parts := strings.Split(url, "/")
	if len(parts) < 4 || parts[len(parts)-2] != "posts" {
		return "", "", common.Address{}, errors.New("Invalid Facebook post URL")
	}
	username := parts[len(parts)-3]

//Facebook的图形API对直接链接不太友好。但是，我们没有
//想做的是询问用户的读权限，所以只需加载公共文章和
//从以太坊地址和配置文件URL中清除它。
	res, err := http.Get(url)
	if err != nil {
		return "", "", common.Address{}, err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", "", common.Address{}, err
	}
	address := common.HexToAddress(string(regexp.MustCompile("0x[0-9a-fA-F]{40}").Find(body)))
	if address == (common.Address{}) {
		return "", "", common.Address{}, errors.New("No Ethereum address found to fund")
	}
	var avatar string
	if parts = regexp.MustCompile("src=\"([^\"]+fbcdn.net[^\"]+)\"").FindStringSubmatch(string(body)); len(parts) == 2 {
		avatar = parts[1]
	}
	return username + "@facebook", avatar, address, nil
}

//AuthNoAuth试图将水龙头请求解释为一个普通的以太坊地址，
//没有实际执行任何远程身份验证。这种模式很容易
//拜占庭式攻击，所以只能用于真正的私人网络。
func authNoAuth(url string) (string, string, common.Address, error) {
	address := common.HexToAddress(regexp.MustCompile("0x[0-9a-fA-F]{40}").FindString(url))
	if address == (common.Address{}) {
		return "", "", common.Address{}, errors.New("No Ethereum address found to fund")
	}
	return address.Hex() + "@noauth", "", address, nil
}

