// The petstore example CLI: the generated command surface wired to a real
// gRPC client, the profile store, and the config command group.
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

	profilePath, err := pkgcmd.HomePath(".petstore.d", "config.yaml")
	if err != nil {
		return 1
	}

	deps := &pkgcmd.Deps{Profiles: pkgcmd.ProfileStore{Path: profilePath}}
	deps.Clients = map[string]func(ctx context.Context) (any, error){
		"petstore": func(context.Context) (any, error) {
			// The env variable wins over the profile, so a one-off call
			// needs no profile at all.
			endpoint := os.Getenv("PETSTORE_ENDPOINT")
			if endpoint == "" {
				if endpoint, err = deps.Setting("endpoint"); err != nil {
					return nil, err
				}
			}
			conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				return nil, err
			}
			return petstorev1.NewPetStoreServiceClient(conn), nil
		},
	}

	root := generated.NewRootCommand()
	deps.BindProfileFlag(root, "PETSTORE_PROFILE")
	for _, c := range generated.Commands(deps) {
		root.AddCommand(c)
	}
	root.AddCommand(pkgcmd.NewProfilesCommand(deps, []pkgcmd.ProfileField{
		{Key: "endpoint", Usage: "the petstore API, host:port"},
	}))

	return pkgcmd.Execute(context.Background(), root)
}
