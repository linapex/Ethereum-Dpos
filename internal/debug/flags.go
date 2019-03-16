
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:40</date>
//</624342640711700480>


package debug

import (
	"fmt"
	"io"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/log/term"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/metrics/exp"
	"github.com/fjl/memsize/memsizeui"
	colorable "github.com/mattn/go-colorable"
	"gopkg.in/urfave/cli.v1"
)

var Memsize memsizeui.Handler

var (
	verbosityFlag = cli.IntFlag{
		Name:  "verbosity",
		Usage: "Logging verbosity: 0=silent, 1=error, 2=warn, 3=info, 4=debug, 5=detail",
		Value: 3,
	}
	vmoduleFlag = cli.StringFlag{
		Name:  "vmodule",
  /*GE：“每个模块的详细程度：逗号分隔的<pattern>=<level>（例如eth/*=5，p2p=4）”，
  值：“
 }
 backtraceAtFlag=cli.stringFlag_
  名称：“回溯”，
  用法：“在特定的日志记录语句（例如\”block.go:271\“）请求堆栈跟踪”，
  值：“
 }
 debugflag=cli.boolflag_
  名称：“调试”，
  用法：“用调用站点位置（文件和行号）预先准备日志消息”，
 }
 pprofflag=cli.boolflag_
  姓名：“PPROF”，
  用法：“启用pprof http服务器”，
 }
 pprofPortFlag=cli.intFlag_
  姓名：“PrPopPt”，
  用法：“pprof http服务器侦听端口”，
  值：6060，
 }
 pprofaddflag=cli.stringflag_
  名称：“pprofaddr”，
  用法：“pprof http服务器侦听接口”，
  值：“127.0.0.1”，
 }
 memprofilerateflag=cli.intflag_
  名称：“MemProfileRate”，
  用法：“以给定速率打开内存分析”，
  值：runtime.memprofilerate，
 }
 blockProfileRateFlag=cli.intFlag_
  名称：“blockprofilerate”，
  用法：“以给定速率打开块分析”，
 }
 CpPrPuliFrase= CLI.STRIGLAG {
  名称：“cpuprofile”，
  用法：“将CPU配置文件写入给定文件”，
 }
 traceFlag=cli.stringFlag_
  姓名：“追踪”，
  用法：“将执行跟踪写入给定文件”，
 }
）

//标志保存调试所需的所有命令行标志。
var flags=[]cli.flag_
 详细标志，vmoduleflag，backtraceatflag，debugflag，
 pprofFlag、pprofAddFlag、pprofPortFlag
 memprofilerateflag、blockprofilerateflag、cpupprofileflag、traceflag、
}

var
 Ostream日志处理程序
 glogger*日志.gloghandler
）

函数（）
 usecolor：=term.istty（os.stderr.fd（））和os.getenv（“term”）！=“哑巴”
 输出：=io.writer（os.stderr）
 如果使用颜色{
  输出=可着色。新的可着色stderr（）
 }
 ostream=log.streamHandler（输出，log.terminalFormat（useColor））
 glogger=log.newgloghandler（奥斯特里姆）
}

//St设置基于CLI标志初始化配置文件和日志记录。
//应该在程序中尽早调用它。
func设置（ctx*cli.context，logdir string）错误
 /测井
 log printorigins（ctx.globalbool（debugflag.name））。
 如果Logdir！=“{”
  rfh，err：=log.rotatingfilehandler（
   洛迪尔
   262144，
   log.JSONFormatOrderedEx(false, true),
  ）
  如果犯错！= nIL{
   返回错误
  }
  glogger.sethandler（log.multihandler（ostream，rfh））。
 }
 glogger.verbosity（log.lvl（ctx.globalint（verbosityflag.name）））
 glogger.vmodule（ctx.globalString（vmoduleFlag.name））。
 glogger.backtraceat（ctx.globalstring（backtraceatflag.name））。
 log.root（）.sethandler（glogger）

 //分析，跟踪
 runtime.memprofilerate=ctx.globalint（memprofilerateflag.name）
 handler.setBlockProfileRate（ctx.globalint（blockProfileRateFlag.name））。
 如果tracefile：=ctx.globalstring（traceflag.name）；tracefile！=“{”
  如果错误：=handler.startgotrace（tracefile）；错误！= nIL{
   返回错误
  }
 }
 如果cpufile：=ctx.globalString（cpuProfileFlag.name）；cpufile！=“{”
  if err := Handler.StartCPUProfile(cpuFile); err != nIL{
   返回错误
  }
 }

 //PPROF服务器
 如果ctx.globalbool（pprofflag.name）
  地址：=fmt.sprintf（“%s:%d”，ctx.globalString（pprofaddflag.name），ctx.globalint（pprofportflag.name））
  startpprof（地址）
 }
 返回零
}

func StartPProf（地址字符串）{
 //在任何/debug/metrics请求中将go metrics挂接到expvar中，加载所有var
 //从注册表到ExpVar，并执行常规ExpVaR处理程序。
 exp.exp（metrics.defaultregistry）
 HTTP.句柄（“/MeistSe/”，http.StripPrefix（“/MESIZE”和MESIZE））
 log.info（“启动pprof服务器”，“addr”，fmt.sprintf（“http://%s/debug/pprof”，address））。
 转到函数（）
  如果错误：=http.listenandserve（address，nil）；错误！= nIL{
   log.error（“运行pprof server失败”，“err”，err）
  }
 }（）
}

//exit停止所有正在运行的配置文件，将其输出刷新到
//各自的文件。
FUNC退出（）{
 handler.stopcupprofile（）处理程序
 handler.stopgotrace（）。
}

