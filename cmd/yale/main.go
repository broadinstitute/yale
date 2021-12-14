package main

import (
	"flag"
	"github.com/broadinstitute/yale/internal/yale"
	"github.com/broadinstitute/yale/internal/yale/client"
	"github.com/broadinstitute/yale/internal/yale/logs"
	"k8s.io/client-go/util/homedir"
	"path/filepath"
)


type args struct {
	// use local kube config
	local      bool
	kubeconfig string
}
func main() {
	args := parseArgs()

/*	cfg, err := config.Read(args.configFile)*/
/*	if err != nil {
		logs.Error.Fatal(err)
	}*/

	logs.Info.Printf("Building clients...")
	clients, err := client.Build(args.local, args.kubeconfig)

	if err != nil {
		logs.Error.Fatalf("Error building clients: %v, exiting\n", err)
	}

	m, err := yale.NewYale(clients)
	if err != nil {
		logs.Error.Fatal(err)
	}
	m.GenerateKeys()

}

func parseArgs() *args {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to kubectl config")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to kubeconfig file")
	}
	local := flag.Bool("local", false, "use this flag when running locally (outside of cluster to use local kube config")
	flag.Parse()
	return &args{*local, *kubeconfig}
}