//go:build windows

package wfp

import (
	"reflect"
	"testing"

	"golang.org/x/sys/windows"
)

func TestFilterKeysForSublayer(t *testing.T) {
	target := windows.GUID{Data1: 1}
	other := windows.GUID{Data1: 2}
	first := windows.GUID{Data1: 10}
	second := windows.GUID{Data1: 20}
	filters := []*wtFwpmFilter0{
		{filterKey: first, subLayerKey: target},
		{filterKey: windows.GUID{Data1: 11}, subLayerKey: other},
		nil,
		{filterKey: second, subLayerKey: target},
	}

	got := filterKeysForSublayer(filters, target)
	want := []windows.GUID{first, second}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterKeysForSublayer() = %v, want %v", got, want)
	}
}
