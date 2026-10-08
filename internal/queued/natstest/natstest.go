package natstest

import (
	"testing"
	"time"

	"github.com/adrianozp/platform/internal/queued/model"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

func Start(t *testing.T) *nats.Conn {
	t.Helper()
	srv, err := server.NewServer(&server.Options{Port: -1, JetStream: true, StoreDir: t.TempDir(), NoSigs: true})
	require.NoError(t, err)
	go srv.Start()
	require.True(t, srv.ReadyForConnections(5*time.Second))
	conn, err := nats.Connect(srv.ClientURL())
	require.NoError(t, err)
	t.Cleanup(func() {
		conn.Close()
		srv.Shutdown()
	})
	return conn
}

func Queue(name string) model.Queue {
	return model.Queue{
		ID: "q-" + name, Name: name, Environment: "dev", Version: 1, Target: "http://consumer/hooks",
		Settings: model.Settings{
			Path: "/hooks", RatePerSecond: 100, MaxInFlight: 4, MaxAttempts: 3,
			Backoff:         model.Backoff{Kind: model.BackoffFixed, Initial: 100 * time.Millisecond, Max: 100 * time.Millisecond},
			DeliveryTimeout: time.Second, Retention: time.Hour, MaxMessageBytes: 1 << 16,
		},
		SigningKey: "key",
	}
}
