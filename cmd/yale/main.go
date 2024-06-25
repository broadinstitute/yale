package main

import (
	"flag"
	"fmt"
	"github.com/broadinstitute/yale/internal/yale"
	"github.com/broadinstitute/yale/internal/yale/cache"
	"github.com/broadinstitute/yale/internal/yale/client"
	"github.com/broadinstitute/yale/internal/yale/logs"
	"github.com/broadinstitute/yale/internal/yale/slack"
	"k8s.io/client-go/util/homedir"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type args struct {
	// use local kube config
	local                    bool
	kubeconfig               string
	cacheNamespace           string
	ignoreUsageMetrics       bool
	windowStart              string
	windowEnd                string
	disableVaultReplication  bool
	disableGitHubReplication bool
}

func main() {
	args := parseArgs()

	logs.Info.Printf("Building clients...")
	clients, err := client.Build(args.local, args.kubeconfig)

	if err != nil {
		logs.Error.Fatalf("Error building clients: %v, exiting\n", err)
	}

	window, err := parseRotateWindow(args, time.Now())
	if err != nil {
		logs.Error.Fatal(err)
	}

	m := yale.NewYale(clients, func(options *yale.Options) {
		options.CacheNamespace = args.cacheNamespace
		options.IgnoreUsageMetrics = args.ignoreUsageMetrics
		options.SlackWebhookUrl = os.Getenv(slack.WebhookEnvVar)
		options.RotateWindow = *window
		options.DisableVaultReplication = args.disableVaultReplication
		options.DisableGitHubReplication = args.disableGitHubReplication
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
	ignoreUsageMetrics := flag.Bool("ignoreusagemetrics", false, "do not check if service account key is in use before disabling")
	windowStart := flag.String("window-start", "", "use to restrict rotation to a particular time of day (HH:MM). eg. 05:00")
	windowEnd := flag.String("window-end", "", "use to restrict rotation to a particular time of day (HH:MM). eg. 06:00")
	disableVaultReplication := flag.Bool("disable-vault-replication", false, "use to globally disable Vault replication")
	disableGitHubReplication := flag.Bool("disable-github-replication", false, "use to globally disable GitHub replication")

	flag.Parse()
	return &args{
		*local,
		*kubeconfig,
		*cacheNamespace,
		*ignoreUsageMetrics,
		*windowStart,
		*windowEnd,
		*disableVaultReplication,
		*disableGitHubReplication,
	}
}

func parseRotateWindow(args *args, now time.Time) (*yale.RotateWindow, error) {
	if args.windowStart == "" {
		if args.windowEnd == "" {
			return &yale.RotateWindow{
				Enabled: false,
			}, nil
		} else {
			return nil, fmt.Errorf("-window-end requires -window-start")
		}
	} else {
		if args.windowEnd == "" {
			return nil, fmt.Errorf("-window-start requires -window-end")
		}
	}

	start, err := parseWindowBoundary(args.windowStart, now)
	if err != nil {
		return nil, fmt.Errorf("-window-start: %v", err)
	}

	end, err := parseWindowBoundary(args.windowEnd, now)
	if err != nil {
		return nil, fmt.Errorf("-window-end: %v", err)
	}

	window := &yale.RotateWindow{
		Enabled:   true,
		StartTime: *start,
		EndTime:   *end,
	}

	if window.StartTime.After(window.EndTime) {
		return nil, fmt.Errorf("-window-start must be before -window-end: %s, %s", args.windowStart, args.windowEnd)
	}

	return window, nil
}

var rotateWindowRegexp = regexp.MustCompile("^[0-9]{2}:[0-9]{2}$")

// parse HH:MM time-of-day into time.Time on today's date
func parseWindowBoundary(hhmm string, now time.Time) (*time.Time, error) {
	if !rotateWindowRegexp.MatchString(hhmm) {
		return nil, fmt.Errorf("must be in HH:MM format: %s", hhmm)
	}
	tokens := strings.SplitN(hhmm, ":", 2)
	hh, mm := tokens[0], tokens[1]
	hour, err := strconv.Atoi(hh)
	if err != nil {
		return nil, fmt.Errorf("unable to parse hour: %s", hh)
	}
	if hour < 0 || hour > 23 {
		return nil, fmt.Errorf("hour must be between 0 and 23: %s", hh)
	}

	minute, err := strconv.Atoi(mm)
	if err != nil {
		return nil, fmt.Errorf("unable to parse minute: %s", mm)
	}
	if minute < 0 || minute > 59 {
		return nil, fmt.Errorf("minute must be between 0 and 59: %s", mm)
	}

	t := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	return &t, nil
}
