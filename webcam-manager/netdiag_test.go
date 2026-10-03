package main

import (
	"context"
	"strings"
	"testing"
)

func TestNetworkDiagnosisRuns(t *testing.T) {
	rep := networkDiagnosis(context.Background(), "127.0.0.1:1")
	t.Log(rep)
	if !strings.Contains(rep, "porta 1:") {
		t.Fatal(rep)
	}
}
