package kafka

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// fakeCluster returns canned metadata and per-group lag.
type fakeCluster struct {
	meta    metadata
	metaErr error
	lags    map[string]int64
	lagErr  error
}

func (f *fakeCluster) Metadata(context.Context) (metadata, error) { return f.meta, f.metaErr }
func (f *fakeCluster) GroupLag(_ context.Context, g string) (int64, error) {
	if f.lagErr != nil {
		return 0, f.lagErr
	}
	return f.lags[g], nil
}
func (f *fakeCluster) Close() {}

func checkWith(cfg engine.KafkaConfig, cl *fakeCluster, connErr error) *Check {
	c := New(cfg)
	c.connect = func(context.Context, engine.KafkaConfig) (cluster, error) {
		if connErr != nil {
			return nil, connErr
		}
		return cl, nil
	}
	return c
}

func run(t *testing.T, c *Check) map[string]engine.Finding {
	t.Helper()
	m := map[string]engine.Finding{}
	for _, f := range c.Run(context.Background()) {
		m[f.Target] = f
	}
	return m
}

func baseCfg() engine.KafkaConfig {
	return engine.KafkaConfig{Brokers: []string{"b:9092"}, ExpectBrokers: 3, Groups: []string{"g1"}, LagWarn: 1000, LagCrit: 100000}
}

func TestHealthy(t *testing.T) {
	cl := &fakeCluster{meta: metadata{Brokers: 3, ControllerPresent: true, UnderReplicated: 0}, lags: map[string]int64{"g1": 10}}
	f := run(t, checkWith(baseCfg(), cl, nil))
	for _, k := range []string{"cluster", "partitions", "group/g1"} {
		if f[k].Status != engine.OK {
			t.Errorf("%s: want OK, got %s (%s)", k, f[k].Status, f[k].Message)
		}
	}
}

func TestNoControllerIsBad(t *testing.T) {
	cl := &fakeCluster{meta: metadata{Brokers: 2, ControllerPresent: false}, lags: map[string]int64{}}
	if got := run(t, checkWith(baseCfg(), cl, nil))["cluster"]; got.Status != engine.BAD {
		t.Errorf("no controller: want BAD, got %s (%s)", got.Status, got.Message)
	}
}

func TestFewBrokersIsWarn(t *testing.T) {
	cl := &fakeCluster{meta: metadata{Brokers: 2, ControllerPresent: true}, lags: map[string]int64{}}
	if got := run(t, checkWith(baseCfg(), cl, nil))["cluster"]; got.Status != engine.WARN {
		t.Errorf("2/3 brokers: want WARN, got %s (%s)", got.Status, got.Message)
	}
}

func TestUnderReplicatedIsBad(t *testing.T) {
	cl := &fakeCluster{meta: metadata{Brokers: 3, ControllerPresent: true, UnderReplicated: 4}, lags: map[string]int64{}}
	if got := run(t, checkWith(baseCfg(), cl, nil))["partitions"]; got.Status != engine.BAD {
		t.Errorf("under-replicated: want BAD, got %s (%s)", got.Status, got.Message)
	}
}

func TestGroupLagThresholds(t *testing.T) {
	cfg := baseCfg()
	cfg.Groups = []string{"warn", "bad"}
	cl := &fakeCluster{meta: metadata{Brokers: 3, ControllerPresent: true}, lags: map[string]int64{"warn": 5000, "bad": 200000}}
	f := run(t, checkWith(cfg, cl, nil))
	if f["group/warn"].Status != engine.WARN {
		t.Errorf("lag 5000: want WARN, got %s", f["group/warn"].Status)
	}
	if f["group/bad"].Status != engine.BAD {
		t.Errorf("lag 200000: want BAD, got %s", f["group/bad"].Status)
	}
}

func TestConnectErrorIsError(t *testing.T) {
	if got := run(t, checkWith(baseCfg(), nil, errors.New("no brokers")))["cluster"]; got.Status != engine.ERROR {
		t.Errorf("connection failed: want ERROR, got %s (%s)", got.Status, got.Message)
	}
}

func TestMetadataErrorIsError(t *testing.T) {
	cl := &fakeCluster{metaErr: errors.New("timeout")}
	if got := run(t, checkWith(baseCfg(), cl, nil))["cluster"]; got.Status != engine.ERROR || !strings.Contains(got.Message, "metadata") {
		t.Errorf("metadata failed: want ERROR, got %s (%s)", got.Status, got.Message)
	}
}
