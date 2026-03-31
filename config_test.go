package main

import (
	"testing"
)

func TestAllLocationsDedup(t *testing.T) {
	locs := allLocations()
	seen := make(map[string]bool, len(locs))
	for _, loc := range locs {
		if seen[loc] {
			t.Errorf("duplicate location: %q", loc)
		}
		seen[loc] = true
	}
}

func TestAllLocationsCount(t *testing.T) {
	locs := allLocations()
	expected := len(baseLocations) + len(numberedLocations)
	// May be less due to dedup
	if len(locs) > expected {
		t.Errorf("allLocations returned %d, expected <= %d", len(locs), expected)
	}
	if len(locs) == 0 {
		t.Error("allLocations returned empty")
	}
}

func TestNodeTypesNonEmpty(t *testing.T) {
	if len(nodeTypes) == 0 {
		t.Fatal("nodeTypes is empty")
	}
	for _, nt := range nodeTypes {
		if nt.Name == "" {
			t.Error("nodeType with empty name")
		}
		if len(nt.ISPs) == 0 {
			t.Errorf("nodeType %q has no ISPs", nt.Name)
		}
	}
}

func TestGotchaDataNonEmpty(t *testing.T) {
	if len(gotchaPrefixes) == 0 {
		t.Error("gotchaPrefixes empty")
	}
	if len(gotchaMiddles) == 0 {
		t.Error("gotchaMiddles empty")
	}
	if len(gotchaRegions) == 0 {
		t.Error("gotchaRegions empty")
	}
	if len(gotchaSuffixes) == 0 {
		t.Error("gotchaSuffixes empty")
	}
	if gotchaNumberStart > gotchaNumberEnd {
		t.Errorf("gotcha range invalid: %d > %d", gotchaNumberStart, gotchaNumberEnd)
	}
}

func TestUPOSNodesNonEmpty(t *testing.T) {
	if len(uposNodes) == 0 {
		t.Error("uposNodes empty")
	}
	for _, node := range uposNodes {
		if node == "" {
			t.Error("empty string in uposNodes")
		}
	}
}

func TestExternalNodesAreFullDomains(t *testing.T) {
	for _, node := range externalNodes {
		if node == "" {
			t.Error("empty string in externalNodes")
		}
		// External nodes should contain a dot (full domain)
		found := false
		for _, c := range node {
			if c == '.' {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("external node %q looks like a subdomain prefix, not a full domain", node)
		}
	}
}

func TestDNSServerListsNonEmpty(t *testing.T) {
	if len(dnsOverseas) == 0 {
		t.Error("dnsOverseas empty")
	}
	if len(dnsDomestic) == 0 {
		t.Error("dnsDomestic empty")
	}
	for _, s := range dnsOverseas {
		if s.QPS <= 0 {
			t.Errorf("overseas server %q has non-positive QPS %d", s.Addr, s.QPS)
		}
	}
	for _, s := range dnsDomestic {
		if s.QPS <= 0 {
			t.Errorf("domestic server %q has non-positive QPS %d", s.Addr, s.QPS)
		}
	}
}

func TestStandardISPsNonEmpty(t *testing.T) {
	if len(standardISPs) == 0 {
		t.Error("standardISPs empty")
	}
}
