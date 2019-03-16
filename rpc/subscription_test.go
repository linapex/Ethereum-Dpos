
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:45</date>
//</624342665508425728>


package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
)

type NotificationTestService struct {
	mu           sync.Mutex
	unsubscribed bool

	gotHangSubscriptionReq  chan struct{}
	unblockHangSubscription chan struct{}
}

func (s *NotificationTestService) Echo(i int) int {
	return i
}

func (s *NotificationTestService) wasUnsubCallbackCalled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.unsubscribed
}

func (s *NotificationTestService) Unsubscribe(subid string) {
	s.mu.Lock()
	s.unsubscribed = true
	s.mu.Unlock()
}

func (s *NotificationTestService) SomeSubscription(ctx context.Context, n, val int) (*Subscription, error) {
	notifier, supported := NotifierFromContext(ctx)
	if !supported {
		return nil, ErrNotificationsUnsupported
	}

//通过显式创建订阅，我们确保将订阅ID发送回客户端
//在第一次订阅之前。调用notify。否则，事件可能会在响应之前发送
//对于eth-subscribe方法。
	subscription := notifier.CreateSubscription()

	go func() {
//测试需要n个事件，如果我们立即开始发送事件，则某些事件
//可能会删除，因为订阅ID可能不会发送到
//客户。
		time.Sleep(5 * time.Second)
		for i := 0; i < n; i++ {
			if err := notifier.Notify(subscription.ID, val+i); err != nil {
				return
			}
		}

		select {
		case <-notifier.Closed():
			s.mu.Lock()
			s.unsubscribed = true
			s.mu.Unlock()
		case <-subscription.Err():
			s.mu.Lock()
			s.unsubscribed = true
			s.mu.Unlock()
		}
	}()

	return subscription, nil
}

//在s.unblockhangsubscription上挂起订阅块
//发送任何东西。
func (s *NotificationTestService) HangSubscription(ctx context.Context, val int) (*Subscription, error) {
	notifier, supported := NotifierFromContext(ctx)
	if !supported {
		return nil, ErrNotificationsUnsupported
	}

	s.gotHangSubscriptionReq <- struct{}{}
	<-s.unblockHangSubscription
	subscription := notifier.CreateSubscription()

	go func() {
		notifier.Notify(subscription.ID, val)
	}()
	return subscription, nil
}

func TestNotifications(t *testing.T) {
	server := NewServer()
	service := &NotificationTestService{}

	if err := server.RegisterName("eth", service); err != nil {
		t.Fatalf("unable to register test service %v", err)
	}

	clientConn, serverConn := net.Pipe()

	go server.ServeCodec(NewJSONCodec(serverConn), OptionMethodInvocation|OptionSubscriptions)

	out := json.NewEncoder(clientConn)
	in := json.NewDecoder(clientConn)

	n := 5
	val := 12345
	request := map[string]interface{}{
		"id":      1,
		"method":  "eth_subscribe",
		"version": "2.0",
		"params":  []interface{}{"someSubscription", n, val},
	}

//创建订阅
	if err := out.Encode(request); err != nil {
		t.Fatal(err)
	}

	var subid string
	response := jsonSuccessResponse{Result: subid}
	if err := in.Decode(&response); err != nil {
		t.Fatal(err)
	}

	var ok bool
	if _, ok = response.Result.(string); !ok {
		t.Fatalf("expected subscription id, got %T", response.Result)
	}

	for i := 0; i < n; i++ {
		var notification jsonNotification
		if err := in.Decode(&notification); err != nil {
			t.Fatalf("%v", err)
		}

		if int(notification.Params.Result.(float64)) != val+i {
			t.Fatalf("expected %d, got %d", val+i, notification.Params.Result)
		}
	}

clientConn.Close() //导致调用通知取消订阅回调
	time.Sleep(1 * time.Second)

	if !service.wasUnsubCallbackCalled() {
		t.Error("unsubscribe callback not called after closing connection")
	}
}

func waitForMessages(t *testing.T, in *json.Decoder, successes chan<- jsonSuccessResponse,
	failures chan<- jsonErrResponse, notifications chan<- jsonNotification, errors chan<- error) {

//读取和分析服务器消息
	for {
		var rmsg json.RawMessage
		if err := in.Decode(&rmsg); err != nil {
			return
		}

		var responses []map[string]interface{}
		if rmsg[0] == '[' {
			if err := json.Unmarshal(rmsg, &responses); err != nil {
				errors <- fmt.Errorf("Received invalid message: %s", rmsg)
				return
			}
		} else {
			var msg map[string]interface{}
			if err := json.Unmarshal(rmsg, &msg); err != nil {
				errors <- fmt.Errorf("Received invalid message: %s", rmsg)
				return
			}
			responses = append(responses, msg)
		}

		for _, msg := range responses {
//确定接收和广播的消息类型
//通过相应的通道
			if _, found := msg["result"]; found {
				successes <- jsonSuccessResponse{
					Version: msg["jsonrpc"].(string),
					Id:      msg["id"],
					Result:  msg["result"],
				}
				continue
			}
			if _, found := msg["error"]; found {
				params := msg["params"].(map[string]interface{})
				failures <- jsonErrResponse{
					Version: msg["jsonrpc"].(string),
					Id:      msg["id"],
					Error:   jsonError{int(params["subscription"].(float64)), params["message"].(string), params["data"]},
				}
				continue
			}
			if _, found := msg["params"]; found {
				params := msg["params"].(map[string]interface{})
				notifications <- jsonNotification{
					Version: msg["jsonrpc"].(string),
					Method:  msg["method"].(string),
					Params:  jsonSubscription{params["subscription"].(string), params["result"]},
				}
				continue
			}
			errors <- fmt.Errorf("Received invalid message: %s", msg)
		}
	}
}

//testsubscriptionmultiplename空间确保订阅可以存在
//对于多个不同的命名空间。
func TestSubscriptionMultipleNamespaces(t *testing.T) {
	var (
		namespaces             = []string{"eth", "shh", "bzz"}
		server                 = NewServer()
		service                = NotificationTestService{}
		clientConn, serverConn = net.Pipe()

		out           = json.NewEncoder(clientConn)
		in            = json.NewDecoder(clientConn)
		successes     = make(chan jsonSuccessResponse)
		failures      = make(chan jsonErrResponse)
		notifications = make(chan jsonNotification)

		errors = make(chan error, 10)
	)

//安装并启动服务器
	for _, namespace := range namespaces {
		if err := server.RegisterName(namespace, &service); err != nil {
			t.Fatalf("unable to register test service %v", err)
		}
	}

	go server.ServeCodec(NewJSONCodec(serverConn), OptionMethodInvocation|OptionSubscriptions)
	defer server.Stop()

//等待消息并将其写入给定的通道
	go waitForMessages(t, in, successes, failures, notifications, errors)

//逐个创建订阅
	n := 3
	for i, namespace := range namespaces {
		request := map[string]interface{}{
			"id":      i,
			"method":  fmt.Sprintf("%s_subscribe", namespace),
			"version": "2.0",
			"params":  []interface{}{"someSubscription", n, i},
		}

		if err := out.Encode(&request); err != nil {
			t.Fatalf("Could not create subscription: %v", err)
		}
	}

//在一批中创建所有订阅
	var requests []interface{}
	for i, namespace := range namespaces {
		requests = append(requests, map[string]interface{}{
			"id":      i,
			"method":  fmt.Sprintf("%s_subscribe", namespace),
			"version": "2.0",
			"params":  []interface{}{"someSubscription", n, i},
		})
	}

	if err := out.Encode(&requests); err != nil {
		t.Fatalf("Could not create subscription in batch form: %v", err)
	}

	timeout := time.After(30 * time.Second)
	subids := make(map[string]string, 2*len(namespaces))
	count := make(map[string]int, 2*len(namespaces))

	for {
		done := true
		for id := range count {
			if count, found := count[id]; !found || count < (2*n) {
				done = false
			}
		}

		if done && len(count) == len(namespaces) {
			break
		}

		select {
		case err := <-errors:
			t.Fatal(err)
case suc := <-successes: //已创建订阅
			subids[namespaces[int(suc.Id.(float64))]] = suc.Result.(string)
		case failure := <-failures:
			t.Errorf("received error: %v", failure.Error)
		case notification := <-notifications:
			if cnt, found := count[notification.Params.Subscription]; found {
				count[notification.Params.Subscription] = cnt + 1
			} else {
				count[notification.Params.Subscription] = 1
			}
		case <-timeout:
			for _, namespace := range namespaces {
				subid, found := subids[namespace]
				if !found {
					t.Errorf("Subscription for '%s' not created", namespace)
					continue
				}
				if count, found := count[subid]; !found || count < n {
					t.Errorf("Didn't receive all notifications (%d<%d) in time for namespace '%s'", count, n, namespace)
				}
			}
			return
		}
	}
}

