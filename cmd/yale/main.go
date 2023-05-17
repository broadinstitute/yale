package main

import (
	"flag"
	"github.com/broadinstitute/yale/internal/yale"
	"github.com/broadinstitute/yale/internal/yale/cache"
	"github.com/broadinstitute/yale/internal/yale/client"
	"github.com/broadinstitute/yale/internal/yale/logs"
	"github.com/broadinstitute/yale/internal/yale/slack"
	"k8s.io/client-go/util/homedir"
	"os"
	"path/filepath"
)

type args struct {
	// use local kube config
	local          bool
	kubeconfig     string
	cacheNamespace string
	checkInUse     bool
}

func main() {
	args := parseArgs()

	logs.Info.Printf("Building clients...")
	clients, err := client.Build(args.local, args.kubeconfig)

	if err != nil {
		logs.Error.Fatalf("Error building clients: %v, exiting\n", err)
	}
	m := yale.NewYale(clients, func(options *yale.Options) {
		options.CacheNamespace = args.cacheNamespace
		options.CheckInUseBeforeDisabling = args.checkInUse
		options.SlackWebhookUrl = os.Getenv(slack.WebhookEnvVar)
	})
	if err = m.Run(); err != nil {
		logs.Error.Fatal(err)
	}
}

func parseArgs() *args {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to kubectl config")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to kubeconfig file")
	}
	local := flag.Bool("local", false, "use this flag when running locally (outside of cluster to use local kube config")
	cacheNamespace := flag.String("cachenamespace", cache.DefaultCacheNamespace, "namespace where yale should cache service account keys")
	checkInUse := flag.Bool("checkinuse", true, "check if service account is in use before disabling")
	flag.Parse()
	return &args{*local, *kubeconfig, *cacheNamespace, *checkInUse}
}
