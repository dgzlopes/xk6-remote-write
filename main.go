package remotewrite

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/dgzlopes/prometheus_remote_client_golang/promremote"
	"go.k6.io/k6/js/common"
	"go.k6.io/k6/js/modules"
	"go.k6.io/k6/lib"
	"go.k6.io/k6/stats"
)

type dp promremote.Datapoint

// Register the extension on module initialization, available to
// import from JS as "k6/x/remotewrite".
func init() {
	modules.Register("k6/x/remotewrite", new(RemoteWrite))
}

// RemoteWrite is the k6 extension for interacting with Kubernetes jobs.
type RemoteWrite struct {
}

// Client is the Kubernetes client wrapper.
type Client struct {
	client promremote.Client
}

type Config struct {
	Endpoint  string
	UserAgent string
}

type Response struct {
	Status int
}

// XClient represents
func (r *RemoteWrite) XClient(ctxPtr *context.Context, config Config) interface{} {
	if config.UserAgent == "" {
		config.UserAgent = "k6-remote-write/0.0.1"
	}
	rt := common.GetRuntime(*ctxPtr)
	cfg := promremote.NewConfig(
		promremote.WriteURLOption(config.Endpoint),
		promremote.UserAgent(config.UserAgent),
	)
	client, err := promremote.NewClient(cfg)
	if err != nil {
		log.Fatal(fmt.Errorf("unable to construct client: %v", err))
	}
	return common.Bind(rt, &Client{client: client}, ctxPtr)
}

// Store a metric
func (c *Client) Store(ctx context.Context, labels map[string]string, value float64, timestamp time.Time) (Response, error) {
	var labelList []promremote.Label

	for labelName, labelValue := range labels {
		labelList = append(labelList, promremote.Label{
			Name:  labelName,
			Value: labelValue,
		})
	}

	tsList := promremote.TSList{
		{
			Labels: labelList,
			Datapoint: promremote.Datapoint(promremote.Datapoint{
				Value:     value,
				Timestamp: timestamp,
			}),
		},
	}

	writeOptions := promremote.WriteOptions{}

	state := lib.GetState(ctx)
	err := errors.New("State is nil")

	if state == nil {
		return Response{}, err
	}

	stats.PushIfNotDone(ctx, state.Samples, stats.Sample{
		Metric: RequestsTotal,
		Time:   time.Time{},
		Value:  float64(1),
	})

	stats.PushIfNotDone(ctx, state.Samples, stats.Sample{
		Metric: DataSent,
		Time:   time.Time{},
		Value:  float64(binary.Size(tsList) + binary.Size(writeOptions)),
	})

	start := time.Now()
	result, writeErr := c.client.WriteTimeSeries(context.Background(), tsList, writeOptions)
	elapsed := time.Since(start)

	if err := error(writeErr); err != nil {
		stats.PushIfNotDone(ctx, state.Samples, stats.Sample{
			Metric: RequestsDuration,
			Tags:   stats.IntoSampleTags(&map[string]string{"status": strconv.Itoa(writeErr.StatusCode())}),
			Time:   time.Time{},
			Value:  float64(elapsed.Milliseconds()),
		})
		return Response{Status: writeErr.StatusCode()}, err
	}

	stats.PushIfNotDone(ctx, state.Samples, stats.Sample{
		Metric: RequestsDuration,
		Tags:   stats.IntoSampleTags(&map[string]string{"status": strconv.Itoa(result.StatusCode)}),
		Time:   time.Time{},
		Value:  float64(elapsed.Milliseconds()),
	})

	return Response{Status: result.StatusCode}, nil
}

// StoreNow a metric
func (c *Client) StoreNow(ctx context.Context, labels map[string]string, value float64) (Response, error) {
	return c.Store(ctx, labels, value, time.Now())
}
