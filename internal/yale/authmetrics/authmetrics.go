package main

import (
	"context"
	"fmt"
	"github.com/broadinstitute/yale/internal/yale/logs"
	"google.golang.org/api/iam/v1"
	"sync"
	"time"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/api/iterator"
)

// metricType name of metric that Google creates to track key auth events
const metricType = "iam.googleapis.com/service_account/key/authn_events_count"

// keyIdLabel label on the metric that contains service account key ID
const keyIdLabel = "key_id"

// uniqueIdLabel label on the metric that contains service account's ID
const uniqueIdLabel = "unique_id"

// lookbackWindow - how far back we should check for authentications
const lookbackWindow = time.Hour * 24 * 7

// AuthMetrics returns the last time a service account key was used to authenticate
type AuthMetrics interface {
	// LastAuthTime returns the approximate last time a service account key was used to authenticate, based
	// on data from the Cloud Metrics API.
	// If the key has not been used to authenticate within the last 7 days, nil is returned
	LastAuthTime(project string, serviceAccountEmail string, keyID string) (*time.Time, error)
}

func New(metricClient *monitoring.MetricClient, iam *iam.Service) AuthMetrics {
	return newWithClients(metricClient, iam, time.Now())
}

// package-private constructor for testing
func newWithClients(metricClient *monitoring.MetricClient, iam *iam.Service, now time.Time) *authMetrics {
	return &authMetrics{
		mutex:        sync.Mutex{},
		lastAuthMap:  make(map[string]map[string]time.Time),
		metricClient: metricClient,
		iam:          iam,
		now:          now,
	}
}

type authMetrics struct {
	mutex        sync.Mutex
	lastAuthMap  map[string]map[string]time.Time
	metricClient *monitoring.MetricClient
	iam          *iam.Service
	now          time.Time
}

func (a *authMetrics) LastAuthTime(project string, serviceAccountEmail string, keyID string) (*time.Time, error) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	var err error
	m, exists := a.lastAuthMap[project]
	if !exists {
		m, err = a.buildLastAuthMap(project)
		if err != nil {
			return nil, fmt.Errorf("error building last auth map for service account keys in %s: %v", project, err)
		}
		a.lastAuthMap[project] = m
	}

	lastAuthTime, exists := m[key(serviceAccountEmail, keyID)]
	if !exists {
		return nil, nil
	}
	return &lastAuthTime, nil
}

// for the given project, build a map of last authentication times for its service account keys
// eg. { "service-account@project/abcdef0123456789abcdef0123456789": 2020-03-12T07:00:00Z }
// if a key has not been authenticated within the history window, it will not be in the map
//
// ref https://cloud.google.com/monitoring/custom-metrics/reading-metrics#monitoring_read_timeseries_fields-go
func (a *authMetrics) buildLastAuthMap(project string) (map[string]time.Time, error) {
	serviceAccountIds, err := a.buildServiceAccountUniqueIdMap(project)
	if err != nil {
		return nil, fmt.Errorf("error building service account ID map for %s: %v", project, err)
	}

	lastAuthTimes := make(map[string]time.Time)

	startWindow := a.now.UTC().Add(lookbackWindow * -1).Unix()
	endWindow := a.now.UTC().Unix()
	req := &monitoringpb.ListTimeSeriesRequest{
		Name:   "projects/" + project,
		Filter: fmt.Sprintf("metric.type=\"%s\"", metricType),
		Interval: &monitoringpb.TimeInterval{
			StartTime: &timestamp.Timestamp{Seconds: startWindow},
			EndTime:   &timestamp.Timestamp{Seconds: endWindow},
		},
	}

	iter := a.metricClient.ListTimeSeries(context.Background(), req)
	for {
		resp, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error pulling time series data: %v ", err)
		}
		keyId, exists := resp.GetMetric().GetLabels()[keyIdLabel]
		if !exists {
			return nil, fmt.Errorf("time series %s missing label: %s", metricType, keyIdLabel)
		}
		serviceAccountId, exists := resp.GetResource().Labels[uniqueIdLabel]
		if !exists {
			return nil, fmt.Errorf("time series %s missing resource label: %s", metricType, uniqueIdLabel)
		}
		serviceAccountEmail, exists := serviceAccountIds[serviceAccountId]
		if !exists {
			return nil, fmt.Errorf("time series %s labeled with unknown service account id %s", metricType, serviceAccountId)
		}

		for _, p := range resp.GetPoints() {
			startTime := p.Interval.StartTime.AsTime()
			endTime := p.Interval.EndTime.AsTime()
			delta := endTime.Sub(startTime)

			if delta > 12*time.Hour {
				logs.Warn.Printf("metric data point delta is greater at 12 hours; should be ~10 minutes. Did the Cloud Metrics API change?")
			}

			value := p.Value.GetInt64Value()
			if value <= 0 {
				// no authentications in this window
				continue
			}

			mkey := key(serviceAccountEmail, keyId)
			previousTime, exists := lastAuthTimes[mkey]
			if !exists || endTime.After(previousTime) {
				lastAuthTimes[mkey] = endTime
			}
		}
	}

	return lastAuthTimes, nil
}

// for the given project, build a map of all its service account emails, keyed by unique ID
// eg. { "1234567890": "service-account@project" }
func (a *authMetrics) buildServiceAccountUniqueIdMap(project string) (map[string]string, error) {
	m := make(map[string]string)
	err := a.iam.Projects.ServiceAccounts.List("projects/"+project).Pages(context.Background(), func(response *iam.ListServiceAccountsResponse) error {
		for _, account := range response.Accounts {
			m[account.UniqueId] = account.Email
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error listing service accounts in %s: %v", project, err)
	}
	return m, nil
}

// key combine sa email and id into a string key for use in lastAuthTime maps
func key(serviceAccountEmail string, keyID string) string {
	return serviceAccountEmail + "/" + keyID
}
