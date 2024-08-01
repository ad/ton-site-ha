package server

import (
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/xssnick/tonutils-go/adnl"
	"github.com/xssnick/tonutils-go/adnl/dht"
	rldphttp "github.com/xssnick/tonutils-go/adnl/rldp/http"

	conf "github.com/ad/ton-site-ha/config"
	"github.com/ad/ton-site-ha/site"
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

	fs := http.FileServer(http.FS(site.Static))
	// http.Handle("/static/", neuter(fs))

	mx := http.NewServeMux()
	mx.HandleFunc("/", listener.serveTemplate)
	mx.Handle("/static/", neuter(fs))

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

	lp := filepath.Join("templates", "layout.html")
	fp := filepath.Join("templates", filepath.Clean(r.URL.Path))

	tmpl, err := template.New("base.html").ParseFS(site.Templates, lp, fp)
	if err != nil {
		log.Print(err.Error())
		http.Error(w, http.StatusText(404), 404)

		return
	}

	if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		err = tmpl.ExecuteTemplate(w, "layout", nil)
		if err != nil {
			log.Print(err.Error())
			http.Error(w, http.StatusText(404), 404)

			return
		}

		return
	}

	w.Header().Set("Content-Encoding", "gzip")

	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("Ton-Proxy-Site-Version", "Commit: "+l.config.Version)
	w.Header().Set("Vary", "Accept-Encoding")

	gz := gzip.NewWriter(w)
	defer gz.Close()

	err = tmpl.ExecuteTemplate(gz, "layout", nil)
	if err != nil {
		log.Print(err.Error())
		http.Error(w, http.StatusText(404), 404)

		return
	}

	// Transfer-Encoding:	chunked
	// Last-Modified: Tue, 23 Jul 2024 21:13:17 GMT
	// Etag: W/"66a01ced-730"
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

func SubAndWrapFS(fSys fs.FS, dir string) http.FileSystem {
	fSys, err := fs.Sub(fSys, dir)
	if err != nil {
		log.Fatal(err)
	}

	return http.FS(fSys)
}

func neuter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}
