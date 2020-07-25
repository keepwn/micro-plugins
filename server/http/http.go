package http

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/micro/go-micro/v2"
	"github.com/micro/go-micro/v2/web"
)

var (
	ServerAddress = "127.0.0.1:0"
)

const (
	AddressFromEnvironment = ""
)

func init() {
	if v := os.Getenv("PLUGIN_SERVER_ADDRESS"); len(v) > 0 {
		ServerAddress = v
	}
}

func Server(name string, address string, handler http.Handler) micro.Option {
	ctx, cancel := context.WithCancel(context.TODO())

	if address == AddressFromEnvironment {
		address = ServerAddress
	}

	s := web.NewService(
		web.Context(ctx),
		web.Name(name),
		web.Handler(handler),
		web.Address(address),
	)
	// s.Init()

	start := func() error {
		go func() {
			log.Println(fmt.Sprintf("Server [%s] Is Running", name))

			if err := s.Run(); err != nil {
				log.Fatal(fmt.Sprintf("Server [%s]: %s", name, err.Error()))
			}
		}()
		return nil
	}
	stop := func() error {
		log.Println(fmt.Sprintf("Stopping Server [%s]", name))
		cancel()
		return nil
	}

	return func(o *micro.Options) {
		o.AfterStart = append(o.AfterStart, start)
		o.BeforeStop = append(o.BeforeStop, stop)
	}
}
