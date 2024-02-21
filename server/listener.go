package server

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"

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

	fs := http.FileServer(http.Dir("site/static"))
	// http.Handle("/static/", http.StripPrefix("/static/", fs))

	mx := http.NewServeMux()
	mx.HandleFunc("/", listener.serveTemplate)
	mx.Handle("/static/", http.StripPrefix("/static/", fs))

	s := rldphttp.NewServer(key, dhtClient, mx)

	addr, err := rldphttp.SerializeADNLAddress(s.Address())
	if err != nil {
		return nil, err
	}

	addrHex, err := rldphttp.ParseADNLAddress(addr)
	if err != nil {
		return nil, err
	}

	publicIP := getPublicIP()

	fmt.Println("Public IP:", publicIP)
	fmt.Println("addr for TON DNS", hex.EncodeToString(addrHex))
	fmt.Println("Listening on", addr+".adnl")

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

	// http.HandleFunc("/", listener.serveTemplate)
	// go func() {
	// 	err := http.ListenAndServe(":3000", nil)
	// 	if errors.Is(err, http.ErrServerClosed) {
	// 		lgr.Info("server closed")
	// 	} else if err != nil {
	// 		lgr.Error(fmt.Sprintf("error listening for server: %s", err))
	// 	}
	// }()

	listener.Server = s

	return listener, nil
}

func (l *Listener) serveTemplate(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("%+v\n", r)

	if r.URL.Path == "" || r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/?") {
		r.URL.Path = "/index.html"
	}

	lp := filepath.Join("site/templates", "layout.html")
	fp := filepath.Join("site/templates", filepath.Clean(r.URL.Path))

	// Return a 404 if the template doesn't exist
	info, err := os.Stat(fp)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
	}

	// Return a 404 if the request is for a directory
	if info.IsDir() {
		http.NotFound(w, r)
		return
	}

	tmpl, err := template.ParseFiles(lp, fp)
	if err != nil {
		// Log the detailed error
		log.Print(err.Error())
		// Return a generic "Internal Server Error" message
		http.Error(w, http.StatusText(500), 500)
		return
	}

	err = tmpl.ExecuteTemplate(w, "layout", nil)
	if err != nil {
		log.Print(err.Error())
		http.Error(w, http.StatusText(500), 500)
	}
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
