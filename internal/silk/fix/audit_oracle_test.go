//go:build oracle

// audit_oracle_test.go runs only with `-tags=oracle` and regenerates
// the testdata/oracle/<macro>.golden files by piping curated edge inputs
// through the C oracle binary.
//
// Usage:
//
//	cd tools/macro-oracle && make
//	go test ./internal/silk/fix/ -tags=oracle -run TestRegenOracle
//
// The golden files are committed; the non-oracle build (default) loads
// them via loadGolden() in audit_test.go and compares Go output against
// them — no C toolchain needed at CI time.
package fix

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const oracleBinary = "../../../tools/macro-oracle/oracle"

// askOracle pipes a list of "OP a b c" lines (or "FIX_CONST C Q") to the
// oracle binary and returns the parallel decimal-int64 results.
func askOracle(t *testing.T, queries []string) []int64 {
	t.Helper()
	cmd := exec.Command(oracleBinary)
	cmd.Stdin = strings.NewReader(strings.Join(queries, "\n") + "\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("oracle exec: %v\nstderr: %s", err, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Logf("oracle stderr: %s", stderr.String())
	}
	results := []int64{}
	scan := bufio.NewScanner(&stdout)
	for scan.Scan() {
		v, err := strconv.ParseInt(scan.Text(), 10, 64)
		if err != nil {
			t.Fatalf("oracle line %q: %v", scan.Text(), err)
		}
		results = append(results, v)
	}
	if len(results) != len(queries) {
		t.Fatalf("oracle returned %d lines, expected %d", len(results), len(queries))
	}
	return results
}

// writeGolden saves "<query>\t<result>\n" lines to a golden file.
func writeGolden(t *testing.T, name string, queries []string, results []int64) {
	t.Helper()
	dir := filepath.Join("testdata", "oracle")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	for i, q := range queries {
		fmt.Fprintf(&buf, "%s\t%d\n", q, results[i])
	}
	path := filepath.Join(dir, name+".golden")
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestRegenOracle regenerates every golden file by querying the C oracle.
func TestRegenOracle(t *testing.T) {
	if _, err := os.Stat(oracleBinary); err != nil {
		t.Fatalf("oracle binary not found at %s — run `cd tools/macro-oracle && make` first", oracleBinary)
	}
	for macro, queries := range allOracleQueries() {
		results := askOracle(t, queries)
		writeGolden(t, macro, queries, results)
		t.Logf("regen %s: %d cases", macro, len(queries))
	}
}
