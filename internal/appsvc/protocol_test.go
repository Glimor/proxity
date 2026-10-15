//go:build windows

package appsvc

import (
	"context"
	"testing"

	"github.com/Glimor/proxity/internal/domain"
)

func TestProfileProtocolsAndExportImport(t *testing.T) {
	svc, eng, _ := newTestService(t)
	ctx := context.Background()
	for _, protocol := range []domain.ProxyProtocol{domain.ProxyHTTP, domain.ProxyHTTPS, domain.ProxySOCKS4, domain.ProxySOCKS5, ""} {
		p, err := svc.CreateProfile(ctx, ProfileInput{Name: string(protocol) + " test", Protocol: protocol, Host: "localhost", Port: 1080, Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		if p.Protocol != protocol.Normalized() {
			t.Fatalf("create: %s", p.Protocol)
		}
		p, err = svc.UpdateProfile(ctx, p.ID, ProfileInput{Name: p.Name, Protocol: domain.ProxyHTTPS, Host: p.Host, Port: 443, Enabled: true})
		if err != nil || p.Protocol != domain.ProxyHTTPS {
			t.Fatalf("update: %+v %v", p, err)
		}
		if len(eng.profiles) == 0 {
			t.Fatal("engine not refreshed")
		}
	}
	// Import each supported type and legacy JSON, then export and load it into another service.
	list := []ExportProfile{}
	for _, protocol := range []domain.ProxyProtocol{domain.ProxyHTTP, domain.ProxyHTTPS, domain.ProxySOCKS4, domain.ProxySOCKS5, ""} {
		list = append(list, ExportProfile{Name: "import " + string(protocol), Protocol: protocol, Host: "localhost", Port: 1080})
	}
	if _, err := svc.ImportProfiles(ctx, list, false); err != nil {
		t.Fatal(err)
	}
	exported, err := svc.ExportProfiles(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	other, _, _ := newTestService(t)
	if _, err := other.ImportProfiles(ctx, exported, false); err != nil {
		t.Fatal(err)
	}
	for _, want := range exported {
		got, err := other.d.Profiles.GetProfile(ctx, want.ID)
		if err != nil || got.Protocol != want.Protocol {
			t.Fatalf("export/import: %+v %v", got, err)
		}
	}
}

func TestInvalidProtocolAndCredentials(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	secret := "secret"
	for _, in := range []ProfileInput{
		{Protocol: "ftp"}, {Protocol: domain.ProxySOCKS4, Password: &secret},
		{Protocol: domain.ProxyHTTP, Username: "bad:user"}, {Protocol: domain.ProxySOCKS4, Username: "bad\x00user"},
	} {
		in.Name = "test"
		in.Host = "localhost"
		in.Port = 1080
		if _, err := svc.CreateProfile(ctx, in); err == nil {
			t.Fatalf("accepted %+v", in)
		}
	}
	if _, err := svc.ImportProfiles(ctx, []ExportProfile{{Name: "test", Protocol: "invalid", Host: "localhost", Port: 1080}}, false); err == nil {
		t.Fatal("invalid import accepted")
	}
	if _, err := svc.ImportProfiles(ctx, []ExportProfile{{Name: "test", Protocol: domain.ProxySOCKS4, Host: "localhost", Port: 1080, PasswordCiphertext: []byte{1}}}, false); err == nil {
		t.Fatal("SOCKS4 password import accepted")
	}
	p, err := svc.CreateProfile(ctx, ProfileInput{Name: "test", Host: "localhost", Port: 1080, Password: &secret})
	if err != nil {
		t.Fatal(err)
	}
	p, err = svc.UpdateProfile(ctx, p.ID, ProfileInput{Name: p.Name, Host: p.Host, Port: p.Port, Protocol: domain.ProxySOCKS4})
	if err != nil || len(p.PasswordCiphertext) != 0 {
		t.Fatalf("SOCKS4 retained password: %+v %v", p, err)
	}
}
