package mongodb

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// mongoCollector is the driver-backed collector used in production.
type mongoCollector struct {
	client *mongo.Client
}

// mongoConnect dials a target and returns a collector. Credentials come from the
// environment (never from the URI/config).
func mongoConnect(ctx context.Context, t engine.MongoDBTarget) (collector, error) {
	opts := options.Client().ApplyURI(t.URI)
	if t.Username != "" {
		authSource := t.AuthSource
		if authSource == "" {
			authSource = "admin"
		}
		opts.SetAuth(options.Credential{
			Username:   t.Username,
			Password:   os.Getenv(t.PasswordEnv),
			AuthSource: authSource,
		})
	}
	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, err
	}
	// Fail fast if the deployment is unreachable, rather than on first command.
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(ctx)
		return nil, err
	}
	return &mongoCollector{client: client}, nil
}

func (m *mongoCollector) Close(ctx context.Context) { _ = m.client.Disconnect(ctx) }

// serverStatus is the subset of serverStatus we read.
type serverStatus struct {
	Version     string `bson:"version"`
	Connections struct {
		Current   int64 `bson:"current"`
		Available int64 `bson:"available"`
	} `bson:"connections"`
}

// replStatus is the subset of replSetGetStatus we read.
type replStatus struct {
	Set     string `bson:"set"`
	Members []struct {
		Name       string    `bson:"name"`
		Health     float64   `bson:"health"`
		StateStr   string    `bson:"stateStr"`
		OptimeDate time.Time `bson:"optimeDate"`
	} `bson:"members"`
}

func (m *mongoCollector) Collect(ctx context.Context) (metrics, error) {
	admin := m.client.Database("admin")

	var ss serverStatus
	if err := admin.RunCommand(ctx, bson.D{{Key: "serverStatus", Value: 1}}).Decode(&ss); err != nil {
		return metrics{}, err
	}
	out := metrics{
		Version:     ss.Version,
		Connections: ss.Connections.Current,
		Available:   ss.Connections.Available,
	}

	var rs replStatus
	err := admin.RunCommand(ctx, bson.D{{Key: "replSetGetStatus", Value: 1}}).Decode(&rs)
	if err != nil {
		if isStandalone(err) {
			out.Standalone = true
			return out, nil
		}
		return metrics{}, err
	}
	out.ReplSet = rs.Set
	for _, mem := range rs.Members {
		out.Members = append(out.Members, member{
			Name:     mem.Name,
			Health:   mem.Health,
			StateStr: mem.StateStr,
			Optime:   mem.OptimeDate,
		})
	}
	return out, nil
}

// isStandalone reports whether a replSetGetStatus error means the node simply is
// not part of a replica set (code 76, NoReplicationEnabled), rather than a real
// failure.
func isStandalone(err error) bool {
	var ce mongo.CommandError
	if errors.As(err, &ce) && ce.Code == 76 {
		return true
	}
	return strings.Contains(err.Error(), "not running with --replSet") ||
		strings.Contains(err.Error(), "NoReplicationEnabled")
}
