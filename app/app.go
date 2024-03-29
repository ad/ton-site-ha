package app

import (
	"context"
	"io"
	"os"

	conf "github.com/ad/ton-site-ha/config"
	"github.com/ad/ton-site-ha/logger"
	server "github.com/ad/ton-site-ha/server"
)

var (
	config *conf.Config
)

func Run(ctx context.Context, w io.Writer, args []string) error {
	confLoad, errInitConfig := conf.InitConfig(os.Args)
	if errInitConfig != nil {
		return errInitConfig
	}

	config = confLoad

	lgr := logger.InitLogger(config.Debug)

	_, errInitListener := server.InitListener(lgr, config)
	if errInitListener != nil {
		return errInitListener
	}

	return nil
}
