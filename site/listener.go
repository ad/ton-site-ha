package site

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"

	"github.com/xssnick/tonutils-go/adnl"
	"github.com/xssnick/tonutils-go/adnl/dht"
	rldphttp "github.com/xssnick/tonutils-go/adnl/rldp/http"

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
	key, err := getKey(config.Key)
	if err != nil {
		return nil, err
	}

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
	mx.HandleFunc("/", listener.handler)

	s := rldphttp.NewServer(key, dhtClient, mx)

	addr, err := rldphttp.SerializeADNLAddress(s.Address())
	if err != nil {
		return nil, err
	}

	addrHex, err := rldphttp.ParseADNLAddress("vdov52s2qyfwe3n5l24mgi7dbeobu2aevlo5xwcusmpbaogd5ljfcpc")
	if err != nil {
		return nil, err
	}

	fmt.Println("addr for TON DNS", hex.EncodeToString(addrHex))

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

func (l *Listener) handler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("%+v\n", r)

	_, _ = w.Write([]byte("Hi, " + r.URL.Query().Get("name") + "\nThis TON site"))
}

func getKey(data string) (ed25519.PrivateKey, error) {
	dec, err := hex.DecodeString(data)
	if err != nil {
		return nil, err
	}

	return ed25519.NewKeyFromSeed(dec), nil
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
