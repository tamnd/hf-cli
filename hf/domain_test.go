package hf

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "hf" {
		t.Errorf("Scheme = %q, want hf", info.Scheme)
	}
	found := false
	for _, h := range info.Hosts {
		if h == Host {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Hosts = %v, want to contain %s", info.Hosts, Host)
	}
	if info.Identity.Binary != "hf" {
		t.Errorf("Identity.Binary = %q, want hf", info.Identity.Binary)
	}
}

func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}
	domains := h.Domains()
	found := false
	for _, d := range domains {
		if d == "hf" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("hf domain not registered; got %v", domains)
	}
}
