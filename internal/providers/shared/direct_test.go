package shared

import (
	"bytes"
	"context"
	"testing"
	"time"

	core "github.com/openclaw/crabbox/internal/cli"
)

func TestCleanupServersUsesSingleBatchCutoff(t *testing.T) {
	clock := &testCleanupClock{now: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)}
	boundary := clock.now.Add(time.Minute)
	servers := []core.Server{
		testCleanupServer("old", clock.now.Add(-time.Hour)),
		testCleanupServer("boundary-a", boundary),
		testCleanupServer("boundary-b", boundary),
	}

	var deleted []string
	backend := DirectSSHBackend{
		RT: core.Runtime{Stderr: &bytes.Buffer{}, Clock: clock},
		Delete: func(ctx context.Context, cfg core.Config, server core.Server) error {
			deleted = append(deleted, server.Name)
			if server.Name == "old" {
				clock.now = boundary.Add(time.Second)
			}
			return nil
		},
	}

	if err := backend.CleanupServers(context.Background(), core.CleanupRequest{}, servers); err != nil {
		t.Fatalf("CleanupServers returned error: %v", err)
	}

	if len(deleted) != 1 || deleted[0] != "old" {
		t.Fatalf("deleted=%v, want only old", deleted)
	}
}

type testCleanupClock struct {
	now time.Time
}

func (c *testCleanupClock) Now() time.Time {
	return c.now
}

func testCleanupServer(name string, expiresAt time.Time) core.Server {
	return core.Server{
		Name: name,
		Labels: map[string]string{
			"keep":       "false",
			"expires_at": expiresAt.Format(time.RFC3339Nano),
		},
	}
}
