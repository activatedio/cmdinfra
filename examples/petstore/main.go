// The petstore example CLI: the generated command surface wired to a real
// gRPC client, the named-context store, and the config command group.
package main

import (
	"context"
	"os"

	petstorev1 "github.com/activatedio/tfinfra/examples/petstore/gen/petstore/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/activatedio/cmdinfra/examples/petstore/generated"
	pkgcmd "github.com/activatedio/cmdinfra/pkg/cmd"
)

func main() {
	os.Exit(run())
}

func run() int {

	contextPath, err := pkgcmd.HomeContextPath(".petstore.yaml")
	if err != nil {
		return 1
	}

	deps := &pkgcmd.Deps{
		Clients: map[string]func(ctx context.Context) (any, error){
			"petstore": func(context.Context) (any, error) {
				conn, err := grpc.NewClient(
					os.Getenv("PETSTORE_ENDPOINT"),
					grpc.WithTransportCredentials(insecure.NewCredentials()),
				)
				if err != nil {
					return nil, err
				}
				return petstorev1.NewPetStoreServiceClient(conn), nil
			},
		},
		Contexts: pkgcmd.ContextStore{Path: contextPath},
	}

	root := generated.NewRootCommand()
	for _, c := range generated.Commands(deps) {
		root.AddCommand(c)
	}
	root.AddCommand(pkgcmd.NewConfigCommand(deps.Contexts))

	return pkgcmd.Execute(context.Background(), root)
}
