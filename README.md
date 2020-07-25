# micro-plugins

## plugins
- http 服务
  - 提供自定义 http 服务
  - server/http
- grpc-gateway 服务
  - server/grpc-gateway
  - 支持在启动 grpc 服务情况下，提供 grpc-gateway 的自动映射能力
  

## example
启动 grpc 服务的同时，启动一个名为`com.keepwn.api.example`的 grpc-gateway 服务，并提供`restful` 接口。 

```go
package main

import (
    "github.com/micro/go-micro/v2"

    gwPlugin "github.com/keepwn/micro-plugins/server/grpc-gateway"
)

func main() {
     service := micro.NewService(
         micro.Name("com.keepwn.srv.example"),
     )
     
     service.Init(
		gwPlugin.Server(
			"com.keepwn.api.example",
			gwPlugin.AddressFromEnvironment,
			gwPlugin.NewHandlerWithServiceName(
				"/example",
				"com.keepwn.api.example",
				proto.RegisterLoggingServiceGwFromEndpoint,
			),
		),
     )
     
     // Register Handler
     
     // Run service
     service.Run()
}
```
