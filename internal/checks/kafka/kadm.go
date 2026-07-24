package kafka

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

// connectKadm is the default cluster: a franz-go client + kadm admin.
func connectKadm(_ context.Context, cfg engine.KafkaConfig) (cluster, error) {
	opts := []kgo.Opt{kgo.SeedBrokers(cfg.Brokers...)}
	if cfg.TLS {
		opts = append(opts, kgo.DialTLSConfig(&tls.Config{}))
	}
	if cfg.SASLUser != "" {
		pw := os.Getenv(cfg.SASLPasswordEnv)
		switch strings.ToLower(cfg.SASLMechanism) {
		case "scram-sha-512":
			opts = append(opts, kgo.SASL(scram.Auth{User: cfg.SASLUser, Pass: pw}.AsSha512Mechanism()))
		case "scram-sha-256":
			opts = append(opts, kgo.SASL(scram.Auth{User: cfg.SASLUser, Pass: pw}.AsSha256Mechanism()))
		default:
			opts = append(opts, kgo.SASL(plain.Auth{User: cfg.SASLUser, Pass: pw}.AsMechanism()))
		}
	}
	cl, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	return &kadmCluster{cl: cl, adm: kadm.NewClient(cl)}, nil
}

type kadmCluster struct {
	cl  *kgo.Client
	adm *kadm.Client
}

func (k *kadmCluster) Metadata(ctx context.Context) (metadata, error) {
	m, err := k.adm.Metadata(ctx)
	if err != nil {
		return metadata{}, err
	}
	under := 0
	for _, td := range m.Topics {
		if td.Err != nil {
			continue
		}
		for _, pd := range td.Partitions {
			if pd.Err == nil && len(pd.ISR) < len(pd.Replicas) {
				under++
			}
		}
	}
	return metadata{
		Brokers:           len(m.Brokers),
		ControllerPresent: m.Controller >= 0,
		UnderReplicated:   under,
	}, nil
}

func (k *kadmCluster) GroupLag(ctx context.Context, group string) (int64, error) {
	lags, err := k.adm.Lag(ctx, group)
	if err != nil {
		return 0, err
	}
	dl, ok := lags[group]
	if !ok {
		return 0, fmt.Errorf("gruppo %q non trovato", group)
	}
	if err := dl.Error(); err != nil {
		return 0, err
	}
	return dl.Lag.Total(), nil
}

func (k *kadmCluster) Close() { k.cl.Close() }
