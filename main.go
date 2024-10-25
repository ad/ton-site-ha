package main

import (
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/ad/ton-site-ha/config"
	"github.com/ad/ton-site-ha/site"

	"github.com/xssnick/tonutils-go/adnl"
	"github.com/xssnick/tonutils-go/adnl/dht"
	rldphttp "github.com/xssnick/tonutils-go/adnl/rldp/http"
	"github.com/xssnick/tonutils-go/liteclient"
)

var (
	version = "dev"
)

func main() {
	fmt.Printf("starting version %s\n", version)

	conf, errInitConfig := config.InitConfig(os.Args, version)
	if errInitConfig != nil {
		log.Println("failed to load config:", errInitConfig.Error())
		os.Exit(1)
	}

	key, err := getKey(conf.Key)
	if err != nil {
		log.Println("failed to get key:", err.Error())
		os.Exit(1)
	}

	// https://tonutils.com/ls/free-mainnet-config.json
	netCfg, err := liteclient.GetConfigFromUrl(context.Background(), "https://ton.org/global.config.json")
	if err != nil {
		log.Println("failed to download ton config:", err.Error(), "; we will take it from static cache")
		os.Exit(1)
	}

	client := liteclient.NewConnectionPool()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = client.AddConnectionsFromConfig(ctx, netCfg)
	if err != nil {
		panic(err)
	}

	_, dhtAdnlKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		panic("failed to generate ed25519 key for dht: " + err.Error())
	}

	gateway := adnl.NewGateway(dhtAdnlKey)
	err = gateway.StartClient()
	if err != nil {
		panic("failed to load network config: " + err.Error())
	}

	dhtClient, err := dht.NewClientFromConfig(gateway, netCfg)
	if err != nil {
		panic(err)
	}

	fs := http.FileServer(http.FS(site.Static))

	mx := http.NewServeMux()
	mx.HandleFunc("/", serveTemplate)
	mx.Handle("/static/", neuter(fs))

	s := rldphttp.NewServer(key, dhtClient, mx)
	s.SetExternalIP(net.ParseIP(getPublicIP()).To4())

	addr, err := rldphttp.SerializeADNLAddress(s.Address())
	if err != nil {
		panic(err)
	}
	log.Println("Server's ADNL address is", addr+".adnl ("+hex.EncodeToString(s.Address())+")")

	log.Println("Starting server on", addr+".adnl")

	err = s.ListenAndServe(net.JoinHostPort(conf.ListenHost, conf.ListenPort))
	if errors.Is(err, http.ErrServerClosed) {
		panic("server closed")
	} else if err != nil {
		panic(fmt.Sprintf("error listening for server: %s", err))
	}
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

func getKey(data string) (ed25519.PrivateKey, error) {
	dec, err := hex.DecodeString(data)
	if err != nil {
		return nil, err
	}

	return ed25519.NewKeyFromSeed(dec), nil
}

func serveTemplate(w http.ResponseWriter, r *http.Request) {
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
	w.Header().Set("Ton-Proxy-Site-Version", "Commit: custom")
	w.Header().Set("Vary", "Accept-Encoding")

	gz := gzip.NewWriter(w)
	defer gz.Close()

	err = tmpl.ExecuteTemplate(gz, "layout", nil)
	if err != nil {
		log.Print(err.Error())
		http.Error(w, http.StatusText(404), 404)

		return
	}
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
