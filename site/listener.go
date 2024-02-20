package site

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/xssnick/tonutils-go/adnl"
	"github.com/xssnick/tonutils-go/adnl/dht"
	rldphttp "github.com/xssnick/tonutils-go/adnl/rldp/http"
	"io"
	"log/slog"
	"net"
	"net/http"

	conf "github.com/ad/ton-site-ha/config"
)

type Listener struct {
	lgr    *slog.Logger
	config *conf.Config
	Server *rldphttp.Server
}

type serverContextKey string

const keyServerAddr = "serverAddr"

func InitListener(lgr *slog.Logger, config *conf.Config) (*Listener, error) {
	listener := &Listener{
		lgr:    lgr,
		config: config,
	}

	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, err
	}

	gateway := adnl.NewGateway(priv)
	err = gateway.StartClient()
	if err != nil {
		return nil, err
	}

	dhtClient, err := dht.NewClientFromConfigUrl(context.Background(), gateway, "https://ton.org/global.config.json")
	if err != nil {
		return nil, err
	}

	mx := http.NewServeMux()
	mx.HandleFunc("/", handler) //listener.handler

	s := rldphttp.NewServer(ed25519.PrivateKey(config.Key), dhtClient, mx)

	addr, err := rldphttp.SerializeADNLAddress(s.Address())
	if err != nil {
		return nil, err
	}

	fmt.Println("Listening on", addr+".adnl")
	publicIP := getPublicIP()
	fmt.Println("Public IP:", publicIP)

	s.SetExternalIP(net.ParseIP(publicIP))

	_, cancelCtx := context.WithCancel(context.Background())
	go func(*rldphttp.Server) {
		err := s.ListenAndServe(net.JoinHostPort(config.ListenHost, config.ListenPort))
		if errors.Is(err, http.ErrServerClosed) {
			lgr.Info("server closed")
		} else if err != nil {
			lgr.Error(fmt.Sprintf("error listening for server: %s", err))
		}

		cancelCtx()
	}(s)

	listener.Server = s

	return listener, nil
}

// func (l *Listener) handler(w http.ResponseWriter, r *http.Request) {
// 	bodyValue, _ := io.ReadAll(r.Body)
// 	r.Body.Close()

// 	result := &models.VkCallbackRequest{}
// 	errUnmarshal := json.Unmarshal(bodyValue, result)

// 	if errUnmarshal != nil {
// 		if _, err := io.WriteString(w, "ok"); err != nil {
// 			l.lgr.Error(fmt.Sprintf("error writing response: %s", err))
// 		}

// 		return
// 	}

// 	l.lgr.Debug(fmt.Sprintf("%s: %s", result.Type, string(bodyValue)))

// 	if l.config.VkSecret != "" && result.Secret != l.config.VkSecret {
// 		l.lgr.Debug(fmt.Sprintf("secret mistmatch %s != %s", l.config.VkSecret, result.Secret))
// 		l.lgr.Debug(string(bodyValue))

// 		if _, err := io.WriteString(w, "ok"); err != nil {
// 			l.lgr.Error(fmt.Sprintf("error writing response: %s", err))
// 		}

// 		return
// 	}

// 	if result.Type == "confirmation" {
// 		if _, err := io.WriteString(w, l.config.VkConfirmation); err != nil {
// 			l.lgr.Error(fmt.Sprintf("error writing response: %s", err))
// 		}

// 		return
// 	}

// 	if _, err := io.WriteString(w, "ok"); err != nil {
// 		l.lgr.Error(fmt.Sprintf("error writing response: %s", err))
// 	}

// 	_ = l.Sender.ProcessVKMessage(result)
// }

func handler(writer http.ResponseWriter, request *http.Request) {
	_, _ = writer.Write([]byte("Hi, " + request.URL.Query().Get("name") +
		"\nThis TON site works natively using tonutils-go!"))
}

func getPublicIP() string {
	req, err := http.Get("http://ip-api.com/json/")
	if err != nil {
		return err.Error()
	}
	defer req.Body.Close()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return err.Error()
	}

	var ip struct {
		Query string
	}
	_ = json.Unmarshal(body, &ip)

	return ip.Query
}
