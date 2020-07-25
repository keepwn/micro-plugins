package grpc_gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/micro/go-micro/v2"
	"github.com/micro/go-micro/v2/config/cmd"
	"github.com/micro/go-micro/v2/registry"
	"github.com/micro/go-micro/v2/web"
)

type registerHandlerFunc func(ctx context.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) (err error)

type Handler struct {
	prefix        string
	endpoint      string
	serviceName   string
	gwHandlerFunc registerHandlerFunc
	beforeHandles []handle
	afterHandles  []handle
}

type handle struct {
	Path       string
	Method     string
	HandleFunc http.HandlerFunc
}

// 添加前置Handle
func (h *Handler) BeforeHandle(path string, method string, f http.HandlerFunc) {
	h.beforeHandles = append(h.beforeHandles, handle{
		Path:       path,
		Method:     method,
		HandleFunc: f,
	})
}

// 添加后置Handle
func (h *Handler) AfterHandle(path string, method string, f http.HandlerFunc) {
	h.afterHandles = append(h.afterHandles, handle{
		Path:       path,
		Method:     method,
		HandleFunc: f,
	})
}

// 自定义错误
func CustomHTTPError(ctx context.Context, _ *runtime.ServeMux, marshaler runtime.Marshaler, w http.ResponseWriter, _ *http.Request, err error) {
	const fallback = `{"error": "failed to marshal error message"}`

	s, ok := status.FromError(err)
	if !ok {
		s = status.New(codes.Unknown, err.Error())
	}

	data := map[string]interface{}{
		"error": map[string]interface{}{
			"message": s.Message(),
			"code": int32(s.Code()),
		},
	}

	w.Header().Set("Content-type", marshaler.ContentType())
	w.WriteHeader(runtime.HTTPStatusFromCode(s.Code()))

	jErr := json.NewEncoder(w).Encode(data)
	if jErr != nil {
		w.Write([]byte(fallback))
	}
}

func (h *Handler) newGatewayHandler() (http.Handler, error) {
	prefix := h.prefix
	endpoint := h.endpoint
	gwHandlerFunc := h.gwHandlerFunc
	beforeHandles := h.beforeHandles
	afterHandles := h.afterHandles

	gwmux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{OrigName: true, EmitDefaults: true}),
		runtime.WithProtoErrorHandler(CustomHTTPError),
	)
	opts := []grpc.DialOption{
		grpc.WithInsecure(),
	}

	err := gwHandlerFunc(context.TODO(), gwmux, endpoint, opts)
	if err != nil {
		return nil, err
	}

	router := mux.NewRouter()
	s := router.PathPrefix(prefix).Subrouter()

	// load before handles
	for _, item := range beforeHandles {
		tmp := item
		s.HandleFunc(tmp.Path, func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.Replace(r.URL.Path, prefix, "", 1)
			tmp.HandleFunc.ServeHTTP(w, r)
		}).Methods(tmp.Method)
	}

	// load grpc gateway handle
	s.PathPrefix("/").Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.Replace(r.URL.Path, prefix, "", 1)
		gwmux.ServeHTTP(w, r)
	}))

	// load after handles
	for _, item := range afterHandles {
		tmp := item
		s.HandleFunc(tmp.Path, func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.Replace(r.URL.Path, prefix, "", 1)
			tmp.HandleFunc.ServeHTTP(w, r)
		}).Methods(tmp.Method)
	}

	return router, nil
}

func (h *Handler) Handler() http.Handler {
	handler, err := h.newGatewayHandler()
	if err != nil {
		log.Fatal(err)
	}

	return handler
}

func NewHandler(prefix string, endpoint string, gwFunc registerHandlerFunc) Handler {
	return Handler{
		prefix:        prefix,
		endpoint:      endpoint,
		gwHandlerFunc: gwFunc,
	}
}

func NewHandlerWithServiceName(prefix string, serviceName string, gwFunc registerHandlerFunc) Handler {
	return Handler{
		prefix:        prefix,
		serviceName:   serviceName,
		gwHandlerFunc: gwFunc,
	}
}

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

func getAddressOfServiceFromRegistry(reg registry.Registry, serviceName string) (string, error) {
	services, err := reg.GetService(serviceName)
	if err != nil {
		return "", errors.New(fmt.Sprintf("not found %s service", serviceName))
	}

	for _, srv := range services {
		for _, value := range srv.Nodes {
			address := value.Address
			return address, nil
		}
	}

	return "", errors.New(fmt.Sprintf("not found nodes in %s service", serviceName))
}

func Server(name string, address string, h Handler) micro.Option {
	ctx, cancel := context.WithCancel(context.TODO())

	if address == AddressFromEnvironment {
		address = ServerAddress
	}

	s := web.NewService(
		web.Context(ctx),
		web.Name(name),
		web.Address(address),
	)
	// s.Init()

	start := func() error {
		if h.serviceName != "" && h.endpoint == "" {
			reg := cmd.DefaultOptions().Registry

			// retry 3 times to get service from registry
			var endpoint string
			var err error

			attempts := 3
			for i := 0; ; i++ {
				endpoint, err = getAddressOfServiceFromRegistry(*reg, h.serviceName)
				if err == nil {
					break
				}

				if i >= (attempts - 1) {
					log.Fatal("Server [grpc-gateway] failed: " + err.Error())
				}
				time.Sleep(3 * time.Second)
			}

			h.endpoint = endpoint
		}
		log.Println("Server [grpc-gateway] endpoint: " + h.endpoint)

		s.Handle("/", h.Handler())

		go func() {
			log.Println("Server [grpc-gateway] is running")

			if err := s.Run(); err != nil {
				log.Fatal(fmt.Sprintf("Server [grpc-gateway]: %s", err.Error()))
			}
		}()
		return nil
	}
	stop := func() error {
		log.Println("Stopping Server [grpc-gateway]")
		cancel()
		return nil
	}

	return func(o *micro.Options) {
		o.AfterStart = append(o.AfterStart, start)
		o.BeforeStop = append(o.BeforeStop, stop)
	}
}
