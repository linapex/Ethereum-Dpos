
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:44</date>
//</624342659992915968>


/*
包协议是对p2p的扩展，它提供了一种用户友好的简单定义方法
通过抽象协议标准共享的代码来开发子协议。

*自动将代码索引分配给消息
*基于反射自动RLP解码/编码
*提供永久循环以读取传入消息
*标准化与通信相关的错误处理
*标准化握手谈判
*TODO:自动生成对等机的有线协议规范

**/

package protocols

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"reflect"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/swarm/spancontext"
	"github.com/ethereum/go-ethereum/swarm/tracing"
	opentracing "github.com/opentracing/opentracing-go"
)

//此协议方案使用的错误代码
const (
	ErrMsgTooLong = iota
	ErrDecode
	ErrWrite
	ErrInvalidMsgCode
	ErrInvalidMsgType
	ErrHandshake
	ErrNoHandler
	ErrHandler
)

//与代码关联的错误描述字符串
var errorToString = map[int]string{
	ErrMsgTooLong:     "Message too long",
	ErrDecode:         "Invalid message (RLP error)",
	ErrWrite:          "Error sending message",
	ErrInvalidMsgCode: "Invalid message code",
	ErrInvalidMsgType: "Invalid message type",
	ErrHandshake:      "Handshake error",
	ErrNoHandler:      "No handler registered error",
	ErrHandler:        "Message handler error",
}

/*
错误实现标准Go错误接口。
用途：

  错误F（代码、格式、参数…接口）

打印为：

 <description>：<details>

其中由ErrorToString中的代码给出描述
详细信息是fmt.sprintf（格式，参数…）

可以检查导出的字段代码
**/

type Error struct {
	Code    int
	message string
	format  string
	params  []interface{}
}

func (e Error) Error() (message string) {
	if len(e.message) == 0 {
		name, ok := errorToString[e.Code]
		if !ok {
			panic("invalid message code")
		}
		e.message = name
		if e.format != "" {
			e.message += ": " + fmt.Sprintf(e.format, e.params...)
		}
	}
	return e.message
}

func errorf(code int, format string, params ...interface{}) *Error {
	return &Error{
		Code:   code,
		format: format,
		params: params,
	}
}

//wrappedmsg用于在消息有效负载旁边传播已封送的上下文
type WrappedMsg struct {
	Context []byte
	Size    uint32
	Payload []byte
}

//规范是一种协议规范，包括其名称和版本以及
//交换的消息类型
type Spec struct {
//名称是协议的名称，通常是三个字母的单词
	Name string

//version是协议的版本号
	Version uint

//maxmsgsize是消息有效负载的最大可接受长度
	MaxMsgSize uint32

//messages是此协议使用的消息数据类型的列表，
//发送的每个消息类型及其数组索引作为代码（因此
//[&foo，&bar，&baz]将发送带有代码的foo、bar和baz
//分别为0、1和2）
//每条消息必须有一个唯一的数据类型
	Messages []interface{}

	initOnce sync.Once
	codes    map[reflect.Type]uint64
	types    map[uint64]reflect.Type
}

func (s *Spec) init() {
	s.initOnce.Do(func() {
		s.codes = make(map[reflect.Type]uint64, len(s.Messages))
		s.types = make(map[uint64]reflect.Type, len(s.Messages))
		for i, msg := range s.Messages {
			code := uint64(i)
			typ := reflect.TypeOf(msg)
			if typ.Kind() == reflect.Ptr {
				typ = typ.Elem()
			}
			s.codes[typ] = code
			s.types[code] = typ
		}
	})
}

//length返回协议中的消息类型数
func (s *Spec) Length() uint64 {
	return uint64(len(s.Messages))
}

//getcode返回一个类型的消息代码，Boolean第二个参数是
//如果未找到消息类型，则为false
func (s *Spec) GetCode(msg interface{}) (uint64, bool) {
	s.init()
	typ := reflect.TypeOf(msg)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	code, ok := s.codes[typ]
	return code, ok
}

//newmsg构造给定代码的新消息类型
func (s *Spec) NewMsg(code uint64) (interface{}, bool) {
	s.init()
	typ, ok := s.types[code]
	if !ok {
		return nil, false
	}
	return reflect.New(typ).Interface(), true
}

//对等机表示在与的对等连接上运行的远程对等机或协议实例
//远程对等体
type Peer struct {
*p2p.Peer                   //代表远程的p2p.peer对象
rw        p2p.MsgReadWriter //p2p.msgreadwriter，用于向发送消息和从中读取消息
	spec      *Spec
}

//new peer构造新的peer
//此构造函数由p2p.protocol run函数调用
//前两个参数是传递给p2p.protocol.run函数的参数
//第三个参数是描述协议的规范
func NewPeer(p *p2p.Peer, rw p2p.MsgReadWriter, spec *Spec) *Peer {
	return &Peer{
		Peer: p,
		rw:   rw,
		spec: spec,
	}
}

//运行启动处理传入消息的Forever循环
//在p2p.protocol run函数中调用
//handler参数是为接收到的每个消息调用的函数。
//从远程对等机返回的错误导致循环退出
//导致断开
func (p *Peer) Run(handler func(ctx context.Context, msg interface{}) error) error {
	for {
		if err := p.handleIncoming(handler); err != nil {
			if err != io.EOF {
				metrics.GetOrRegisterCounter("peer.handleincoming.error", nil).Inc(1)
				log.Error("peer.handleIncoming", "err", err)
			}

			return err
		}
	}
}

//DROP断开对等机的连接。
//TODO:可能只需要实现协议删除？不想把同伴踢开
//如果它们对其他协议有用
func (p *Peer) Drop(err error) {
	p.Disconnect(p2p.DiscSubprotocolError)
}

//send接收一条消息，将其编码为rlp，找到正确的消息代码并发送
//向对等端发送消息
//这个低级调用将由提供路由或广播发送的库包装。
//但通常只用于转发和将消息推送到直接连接的对等端
func (p *Peer) Send(ctx context.Context, msg interface{}) error {
	defer metrics.GetOrRegisterResettingTimer("peer.send_t", nil).UpdateSince(time.Now())
	metrics.GetOrRegisterCounter("peer.send", nil).Inc(1)

	var b bytes.Buffer
	if tracing.Enabled {
		writer := bufio.NewWriter(&b)

		tracer := opentracing.GlobalTracer()

		sctx := spancontext.FromContext(ctx)

		if sctx != nil {
			err := tracer.Inject(
				sctx,
				opentracing.Binary,
				writer)
			if err != nil {
				return err
			}
		}

		writer.Flush()
	}

	r, err := rlp.EncodeToBytes(msg)
	if err != nil {
		return err
	}

	wmsg := WrappedMsg{
		Context: b.Bytes(),
		Size:    uint32(len(r)),
		Payload: r,
	}

	code, found := p.spec.GetCode(msg)
	if !found {
		return errorf(ErrInvalidMsgType, "%v", code)
	}
	return p2p.Send(p.rw, code, wmsg)
}

//手工编码（代码）
//在发送传入消息的主永久循环的每个循环中调用
//如果返回错误，则循环将返回，并且对等端将与错误断开连接。
//此通用处理程序
//*检查邮件大小，
//*检查超出范围的消息代码，
//*处理带反射的解码，
//*作为回调的调用处理程序
func (p *Peer) handleIncoming(handle func(ctx context.Context, msg interface{}) error) error {
	msg, err := p.rw.ReadMsg()
	if err != nil {
		return err
	}
//确保有效载荷已被完全消耗。
	defer msg.Discard()

	if msg.Size > p.spec.MaxMsgSize {
		return errorf(ErrMsgTooLong, "%v > %v", msg.Size, p.spec.MaxMsgSize)
	}

//取消标记包装的邮件，其中可能包含上下文
	var wmsg WrappedMsg
	err = msg.Decode(&wmsg)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	ctx := context.Background()

//如果启用了跟踪，并且请求中的上下文是
//不是空的，试着解开它
	if tracing.Enabled && len(wmsg.Context) > 0 {
		var sctx opentracing.SpanContext

		tracer := opentracing.GlobalTracer()
		sctx, err = tracer.Extract(
			opentracing.Binary,
			bytes.NewReader(wmsg.Context))
		if err != nil {
			log.Error(err.Error())
			return err
		}

		ctx = spancontext.WithContext(ctx, sctx)
	}

	val, ok := p.spec.NewMsg(msg.Code)
	if !ok {
		return errorf(ErrInvalidMsgCode, "%v", msg.Code)
	}
	if err := rlp.DecodeBytes(wmsg.Payload, val); err != nil {
		return errorf(ErrDecode, "<= %v: %v", msg, err)
	}

//调用已注册的处理程序回调
//注册的回调将解码后的消息作为参数作为接口
//应该将处理程序强制转换为适当的类型
//不检查处理程序中的强制转换是完全安全的，因为处理程序是
//首先根据正确的类型选择
	if err := handle(ctx, val); err != nil {
		return errorf(ErrHandler, "(msg code %v): %v", msg.Code, err)
	}
	return nil
}

//握手在对等连接上协商握手
//*参数
//＊上下文
//*要发送到远程对等机的本地握手
//*远程握手时要调用的函数（可以为零）
//*需要相同类型的远程握手
//*拨号对等端需要先发送握手，然后等待远程
//*侦听对等机等待远程握手，然后发送它
//返回远程握手和错误
func (p *Peer) Handshake(ctx context.Context, hs interface{}, verify func(interface{}) error) (rhs interface{}, err error) {
	if _, ok := p.spec.GetCode(hs); !ok {
		return nil, errorf(ErrHandshake, "unknown handshake message type: %T", hs)
	}
	errc := make(chan error, 2)
	handle := func(ctx context.Context, msg interface{}) error {
		rhs = msg
		if verify != nil {
			return verify(rhs)
		}
		return nil
	}
	send := func() { errc <- p.Send(ctx, hs) }
	receive := func() { errc <- p.handleIncoming(handle) }

	go func() {
		if p.Inbound() {
			receive()
			send()
		} else {
			send()
			receive()
		}
	}()

	for i := 0; i < 2; i++ {
		select {
		case err = <-errc:
		case <-ctx.Done():
			err = ctx.Err()
		}
		if err != nil {
			return nil, errorf(ErrHandshake, err.Error())
		}
	}
	return rhs, nil
}

