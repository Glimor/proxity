package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Glimor/proxity/internal/domain"
)

func TestProtocolRoundTrip(t *testing.T) {
	db, _ := openTestDB(t)
	ctx := context.Background()
	for _, protocol := range []domain.ProxyProtocol{domain.ProxyHTTP, domain.ProxyHTTPS, domain.ProxySOCKS4, domain.ProxySOCKS5, ""} {
		p := &domain.ProxyProfile{ID: string(protocol) + "id", Name: "test", Protocol: protocol, Host: "localhost", Port: 1080}
		if err := db.Profiles().CreateProfile(ctx, p); err != nil {
			t.Fatal(err)
		}
		got, err := db.Profiles().GetProfile(ctx, p.ID)
		if err != nil || got.Protocol != protocol.Normalized() {
			t.Fatalf("protocol=%s, profile=%+v, err=%v", protocol, got, err)
		}
		got.Protocol = domain.ProxyHTTPS
		if err := db.Profiles().UpdateProfile(ctx, got); err != nil {
			t.Fatal(err)
		}
		got, err = db.Profiles().GetProfile(ctx, p.ID)
		if err != nil || got.Protocol != domain.ProxyHTTPS {
			t.Fatalf("update: %+v, %v", got, err)
		}
	}
}

func TestLegacyProfileMigration(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	initial, err := migrationsFS.ReadFile("migrations/0001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.applyMigration(ctx, "0001_init.sql", string(initial)); err != nil {
		t.Fatal(err)
	}
	_, err = db.SQL.ExecContext(ctx, `INSERT INTO proxy_profiles(id,name,host,port,created_at,updated_at) VALUES('legacy','legacy','localhost',1080,'2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := db.Profiles().GetProfile(ctx, "legacy")
	if err != nil || p.Protocol != domain.ProxySOCKS5 {
		t.Fatalf("legacy: %+v, %v", p, err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
}
