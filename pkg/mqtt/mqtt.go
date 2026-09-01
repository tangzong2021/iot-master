package mqtt

import (
	"bytes"
	"encoding/json"
	"net/url"
	"os"
	"runtime"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/god-jason/iot-master/pkg/config"
	"github.com/god-jason/iot-master/pkg/log"
	"github.com/god-jason/iot-master/pkg/pool"
)

var CustomConnectionFunc paho.OpenConnectionFunc

var client paho.Client

func Client() paho.Client {
	return client
}

func Startup() error {

	//正常流程
	opts := paho.NewClientOptions()

	//优先使用UnixSocket，速度更快
	if runtime.GOOS == "windows" {
		unixsock := os.TempDir() + "/boat.sock"
		unixsock = "unix:///" + url.PathEscape(unixsock)
		u, err := url.Parse(unixsock)
		if err != nil {
			return err
		}
		u.Path = u.Path[1:]          //删除第一个/
		opts.Servers = []*url.URL{u} //直接添加
	} else {
		unixsock := "unix:///var/run/boat.sock"
		opts.AddBroker(unixsock)
	}

	opts.AddBroker(config.GetString(MODULE, "url"))
	opts.SetClientID(config.GetString(MODULE, "clientId"))
	opts.SetUsername(config.GetString(MODULE, "username"))
	opts.SetPassword(config.GetString(MODULE, "password"))
	opts.SetConnectRetry(true) //重试

	opts.SetKeepAlive(50 * time.Second)

	//重连时，恢复订阅
	opts.SetCleanSession(false)
	opts.SetResumeSubs(true)

	//使用虚拟连接，直通broker，免进程间通讯
	opts.SetCustomOpenConnectionFn(CustomConnectionFunc)

	//连接(含自动重连)建立后补订全部过滤器：
	//外接broker重启/会话丢失后，paho的ResumeSubs不总能生效，此处兜底重订
	opts.SetOnConnectHandler(func(c paho.Client) {
		subsMu.RLock()
		filters := make([]string, 0, len(subs))
		for filter := range subs {
			filters = append(filters, filter)
		}
		subsMu.RUnlock()
		for _, filter := range filters {
			if t := c.Subscribe(filter, 0, makeHandler(filter)); t.Wait() && t.Error() != nil {
				log.Error("resubscribe ", filter, " fail: ", t.Error())
			} else {
				log.Info("resubscribe ", filter)
			}
		}
	})

	client = paho.NewClient(opts)
	token := client.Connect()
	//token.Wait()
	return token.Error()
}

func Shutdown() error {
	client.Disconnect(0)
	return nil
}

func Publish(topic string, payload any) paho.Token {
	switch payload.(type) {
	case string:
	case []byte:
	case bytes.Buffer:
	default:
		payload, _ = json.Marshal(payload)
	}
	return client.Publish(topic, 0, false, payload)
}

func PublishEx(topics []string, payload any) {
	switch payload.(type) {
	case string:
	case []byte:
	case bytes.Buffer:
	default:
		payload, _ = json.Marshal(payload)
	}

	for _, topic := range topics {
		client.Publish(topic, 0, false, payload)
	}
}

type callback func(topic string, payload []byte)

var subsMu sync.RWMutex
var subs = map[string][]callback{}

// makeHandler 生成某过滤器的消息分发器(重连补订后复用)
func makeHandler(filter string) func(client paho.Client, message paho.Message) {
	return func(client paho.Client, message paho.Message) {
		subsMu.RLock()
		cbs := subs[filter]
		subsMu.RUnlock()
		//回调
		for _, c := range cbs {
			if pool.Pool == nil {
				go c(message.Topic(), message.Payload())
				continue
			}
			//放入线程池处理
			err := pool.Insert(func() {
				c(message.Topic(), message.Payload())
			})
			if err != nil {
				log.Error(err)
				go c(message.Topic(), message.Payload())
			}
		}
	}
}

func Subscribe(filter string, cb func(topic string, payload []byte)) {

	subsMu.Lock()
	//重复订阅，直接入列
	if callbacks, ok := subs[filter]; ok {
		subs[filter] = append(callbacks, cb)
		subsMu.Unlock()
		return
	}
	subs[filter] = []callback{cb}
	subsMu.Unlock()

	//初次订阅
	client.Subscribe(filter, 0, makeHandler(filter))
}

func SubscribeStruct[T any](filter string, cb func(topic string, data *T)) {
	Subscribe(filter, func(topic string, payload []byte) {
		var value T
		if len(payload) > 0 {
			err := json.Unmarshal(payload, &value)
			if err != nil {
				log.Error(err)
				return
			}
		}
		cb(topic, &value)
	})
}
