package fix

// audit_test.go — table-driven equivalence tests against the C oracle.
//
// The oracle (tools/macro-oracle/oracle) was run once with -tags=oracle
// to populate testdata/oracle/<macro>.golden; this test file verifies
// the Go fix.* helpers produce the same outputs without needing a C
// toolchain at CI time.
//
// To regenerate goldens after adding cases:
//   cd tools/macro-oracle && make
//   go test ./internal/silk/fix/ -tags=oracle -run TestRegenOracle
//
// allOracleQueries() lives in this file so it's available in both the
// regen build (-tags=oracle) and the verify build.

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// loadGolden reads testdata/oracle/<macro>.golden and returns the parallel
// queries + results. Lives here (no build tag) so the verify path can use it.
func loadGolden(name string) ([]string, []int64, error) {
	path := filepath.Join("testdata", "oracle", name+".golden")
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	var qs []string
	var rs []int64
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 4096), 64*1024)
	for scan.Scan() {
		line := scan.Text()
		tab := strings.LastIndex(line, "\t")
		if tab < 0 {
			return nil, nil, fmt.Errorf("malformed golden line: %q", line)
		}
		v, err := strconv.ParseInt(line[tab+1:], 10, 64)
		if err != nil {
			return nil, nil, err
		}
		qs = append(qs, line[:tab])
		rs = append(rs, v)
	}
	return qs, rs, scan.Err()
}

// edgeInts32 — curated edge-case int32 values used for many macros.
var edgeInts32 = []int32{
	0, 1, -1, 2, -2,
	int32Max, int32Min, int32Min + 1,
	int16Max, int16Min,
	int16Max + 1,            // 0x8000 — wraps to negative when cast to int16
	int16Min - 1,            // 0xFFFF7FFF — wraps to positive when cast to int16
	0x7FFF8000,              // split-half boundary (high half ~int16 max, low half == int16 min)
	0xCAFE_BABE - (1 << 32), // deterministic random-ish negative
	0x12345678,
	0x76543210,
	0x55555555,
	-0x55555555,
}

// edgeShifts — curated shift counts for 32-bit shift operations.
var edgeShifts = []int32{0, 1, 2, 7, 8, 15, 16, 17, 23, 30, 31}

// edgeShiftsRound — for RShiftRound which requires shift > 0.
var edgeShiftsRound = []int32{1, 2, 7, 8, 15, 16, 17, 23, 30, 31}

// shortInts — small subset for cubic-blowup macros (3-arg with 3 nested loops).
var shortInts = []int32{
	0, 1, -1,
	int16Max, int16Min,
	int16Max + 1, int16Min - 1,
	0x12345678, -0x12345678,
}

// allOracleQueries returns the full set of queries grouped by macro.
// Each macro maps to a list of "OP arg1 arg2 [arg3]" strings (or
// "FIX_CONST C Q") that the oracle understands.
func allOracleQueries() map[string][]string {
	m := map[string][]string{}

	// Add/Sub family
	pairs := []string{}
	for _, a := range edgeInts32 {
		for _, b := range edgeInts32 {
			pairs = append(pairs, fmt.Sprintf("%%s %d %d", a, b))
		}
	}
	for _, op := range []string{
		"ADD32", "SUB32", "ADD32_OVFLW", "SUB32_OVFLW",
		"ADD_SAT32", "SUB_SAT32", "ADD_POS_SAT32",
		"MUL", "MIN", "MAX", "MIN_32", "MAX_32",
		"DIV32",
	} {
		for _, p := range pairs {
			query := fmt.Sprintf(p, op)
			if op == "DIV32" {
				// skip divide-by-zero
				if strings.HasSuffix(query, " 0") {
					continue
				}
			}
			m[op] = append(m[op], query)
		}
	}

	// ADD_SAT16 — int16 inputs only (operands are clipped, but we still
	// pass full int32 to the oracle which casts).
	for _, a := range []int32{0, 1, -1, int16Max, int16Min, int16Max - 1, int16Min + 1} {
		for _, b := range []int32{0, 1, -1, int16Max, int16Min, int16Max - 1, int16Min + 1} {
			m["ADD_SAT16"] = append(m["ADD_SAT16"], fmt.Sprintf("ADD_SAT16 %d %d", a, b))
		}
	}

	// Half-half multiply (BB/BT/TT/WB/WT) — full edge cross-product.
	for _, op := range []string{"SMULBB", "SMULBT", "SMULTT", "SMULWB", "SMULWT", "SMULWW"} {
		for _, a := range edgeInts32 {
			for _, b := range edgeInts32 {
				m[op] = append(m[op], fmt.Sprintf("%s %d %d", op, a, b))
			}
		}
	}

	// 3-operand half-half (SMLA*) use a smaller set so the queries don't blow up.
	for _, op := range []string{
		"SMLABB", "SMLABT", "SMLATT", "SMLAWB", "SMLAWT", "SMLAWW",
		"SMLABB_OVFLW", "SMLATT_OVFLW", "SMLAWB_OVFLW", "SMLAWT_OVFLW",
		"MLA", "MLA_OVFLW",
	} {
		for _, a := range shortInts {
			for _, b := range shortInts {
				for _, c := range shortInts {
					m[op] = append(m[op], fmt.Sprintf("%s %d %d %d", op, a, b, c))
				}
			}
		}
	}

	// Shifts
	for _, op := range []string{"LSHIFT32", "RSHIFT32", "LSHIFT_OVFLW"} {
		for _, a := range edgeInts32 {
			for _, sh := range edgeShifts {
				m[op] = append(m[op], fmt.Sprintf("%s %d 0 %d", op, a, sh))
			}
		}
	}
	for _, a := range edgeInts32 {
		for _, sh := range edgeShifts {
			// LSHIFT_SAT32 spec: shift in [0, 31]
			m["LSHIFT_SAT32"] = append(m["LSHIFT_SAT32"], fmt.Sprintf("LSHIFT_SAT32 %d 0 %d", a, sh))
		}
	}
	for _, a := range edgeInts32 {
		for _, sh := range edgeShiftsRound {
			m["RSHIFT_ROUND"] = append(m["RSHIFT_ROUND"], fmt.Sprintf("RSHIFT_ROUND %d 0 %d", a, sh))
		}
	}

	// ADD/SUB with shift
	for _, op := range []string{"ADD_LSHIFT32", "ADD_RSHIFT32", "SUB_LSHIFT32", "SUB_RSHIFT32"} {
		for _, a := range shortInts {
			for _, b := range shortInts {
				for _, sh := range []int32{0, 1, 8, 15, 31} {
					m[op] = append(m[op], fmt.Sprintf("%s %d %d %d", op, a, b, sh))
				}
			}
		}
	}

	// Sat / Limit / Abs
	for _, a := range edgeInts32 {
		m["SAT16"] = append(m["SAT16"], fmt.Sprintf("SAT16 %d", a))
		m["SAT32"] = append(m["SAT32"], fmt.Sprintf("SAT32 %d", a))
		m["ABS32"] = append(m["ABS32"], fmt.Sprintf("ABS32 %d", a))
		m["ABS_INT32"] = append(m["ABS_INT32"], fmt.Sprintf("ABS_INT32 %d", a))
	}
	for _, a := range shortInts {
		for _, l1 := range shortInts {
			for _, l2 := range shortInts {
				m["LIMIT"] = append(m["LIMIT"], fmt.Sprintf("LIMIT %d %d %d", a, l1, l2))
			}
		}
	}

	// Misc unary
	for _, a := range edgeInts32 {
		m["RAND"] = append(m["RAND"], fmt.Sprintf("RAND %d", a))
		m["CLZ32"] = append(m["CLZ32"], fmt.Sprintf("CLZ32 %d", a))
		m["SQRT_APPROX"] = append(m["SQRT_APPROX"], fmt.Sprintf("SQRT_APPROX %d", a))
		// SinApprox/CosApprox: input in Q24 — limit to a sensible range
		m["LIN2LOG"] = append(m["LIN2LOG"], fmt.Sprintf("LIN2LOG %d", a))
	}
	for _, a := range []int32{0, 1, 1 << 7, 1 << 8, 1 << 15, 1 << 16, 1 << 24, int32Max - 1} {
		m["LOG2LIN"] = append(m["LOG2LIN"], fmt.Sprintf("LOG2LIN %d", a))
	}
	for _, a := range []int32{
		-12 * 32, -6 * 32, -32, 0, 32, 6 * 32, 12 * 32,
		-1, 1, 5, 100, 200,
	} {
		m["SIGM_Q15"] = append(m["SIGM_Q15"], fmt.Sprintf("SIGM_Q15 %d", a))
	}
	// Trig: Q24 input, full revolution = 1<<24 = 16777216
	for _, a := range []int32{0, 1 << 22, 1 << 23, 1 << 24, -1 << 23, 1<<24 + 1, 12345678} {
		m["SIN_APPROX_Q24"] = append(m["SIN_APPROX_Q24"], fmt.Sprintf("SIN_APPROX_Q24 %d", a))
		m["COS_APPROX_Q24"] = append(m["COS_APPROX_Q24"], fmt.Sprintf("COS_APPROX_Q24 %d", a))
	}

	// Smull/Smmul with broader range
	for _, a := range edgeInts32 {
		for _, b := range edgeInts32 {
			m["SMULL"] = append(m["SMULL"], fmt.Sprintf("SMULL %d %d", a, b))
			m["SMMUL"] = append(m["SMMUL"], fmt.Sprintf("SMMUL %d %d", a, b))
		}
	}

	// Ror32: full coverage of rot in {-31..31}
	for _, a := range []int32{0, 1, -1, int32Max, int32Min, 0x12345678, -0x7FFFFFFF /* = 0x80000001 */} {
		for r := int32(-31); r <= 31; r += 4 {
			m["ROR32"] = append(m["ROR32"], fmt.Sprintf("ROR32 %d 0 %d", a, r))
		}
	}

	// FIX_CONST: walk through C values across Q domains.
	cs := []float64{
		0.0, 0.5, -0.5, 0.25, -0.25, 1.0 / 3.0, -1.0 / 3.0,
		2.0 / 3.0, 0.49999999, -0.49999999,
		0.6, -0.6, 0.95, -0.95, 1.0, -1.0, 1.5, -1.5,
		2.7, -2.7, 100.0, -100.0,
		// Specific values pulled from real callsites
		0.45, -0.004, -0.1, 0.15, 0.99, 0.95, 0.7, 1e-3, 1e-5,
		0.6, // Sat path constant
	}
	qs := []int{0, 7, 14, 16, 24, 30}
	for _, c := range cs {
		for _, q := range qs {
			m["FIX_CONST"] = append(m["FIX_CONST"], fmt.Sprintf("FIX_CONST %.17g %d", c, q))
		}
	}

	// CLZ16 — 16-bit input only
	for _, a := range []int32{0, 1, -1, 0x7FFF, 0x8000, 0x4000, 0x0001, 0x00FF, -0x4000} {
		m["CLZ16"] = append(m["CLZ16"], fmt.Sprintf("CLZ16 %d", a))
	}

	// DIV32_VARQ — exercise both Newton-Raphson branches.
	for _, a := range []int32{1, 1 << 14, 1 << 16, 1 << 24, int32Max, -1, -(1 << 16), int32Min + 1} {
		for _, b := range []int32{1, 2, 1 << 14, 1 << 16, 1 << 24, int32Max, -1, -(1 << 16)} {
			for _, q := range []int32{8, 14, 16, 24, 30} {
				m["DIV32_VARQ"] = append(m["DIV32_VARQ"],
					fmt.Sprintf("DIV32_VARQ %d %d %d", a, b, q))
			}
		}
	}
	for _, b := range []int32{1, 2, 1 << 14, 1 << 16, 1 << 24, int32Max, -1, -(1 << 16)} {
		for _, q := range []int32{8, 14, 16, 24, 30} {
			m["INVERSE32_VARQ"] = append(m["INVERSE32_VARQ"],
				fmt.Sprintf("INVERSE32_VARQ %d 0 %d", b, q))
		}
	}

	return m
}

// goCompute runs a single oracle query through the corresponding Go
// helper and returns its int64-cast result. This is the heart of the
// audit: every divergence here is a real bit-mismatch with the C macro.
func goCompute(query string) (int64, error) {
	parts := strings.Fields(query)
	if len(parts) < 2 {
		return 0, fmt.Errorf("bad query: %q", query)
	}
	op := parts[0]

	// FIX_CONST is special: 2 args, double + int.
	if op == "FIX_CONST" {
		c, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return 0, err
		}
		q, err := strconv.Atoi(parts[2])
		if err != nil {
			return 0, err
		}
		return int64(FixConst32(c, uint(q))), nil
	}

	parseI64 := func(s string) int64 {
		v, _ := strconv.ParseInt(s, 10, 64)
		return v
	}

	a := int32(parseI64(parts[1]))
	var b, c int32
	if len(parts) > 2 {
		b = int32(parseI64(parts[2]))
	}
	if len(parts) > 3 {
		c = int32(parseI64(parts[3]))
	}

	switch op {
	// Add/Sub
	case "ADD32":
		return int64(a + b), nil
	case "SUB32":
		return int64(a - b), nil
	case "ADD32_OVFLW":
		return int64(AddOvflw32(a, b)), nil
	case "SUB32_OVFLW":
		return int64(SubOvflw32(a, b)), nil
	case "ADD_SAT32":
		return int64(AddSat32(a, b)), nil
	case "SUB_SAT32":
		return int64(SubSat32(a, b)), nil
	case "ADD_SAT16":
		return int64(AddSat16(a, b)), nil
	case "ADD_POS_SAT32":
		return int64(AddPosSat32(a, b)), nil
	case "ADD_LSHIFT32":
		return int64(AddLShift32(a, b, c)), nil
	case "ADD_RSHIFT32":
		return int64(AddRShift32(a, b, c)), nil
	case "SUB_LSHIFT32":
		return int64(SubLShift32(a, b, c)), nil
	case "SUB_RSHIFT32":
		return int64(SubRShift32(a, b, c)), nil

	// Mul / MLA
	case "MUL":
		return int64(Mul(a, b)), nil
	case "MLA":
		return int64(Mla(a, b, c)), nil
	case "MLA_OVFLW":
		return int64(MlaOvflw(a, b, c)), nil
	case "SMULL":
		return Smull(a, b), nil
	case "SMMUL":
		return int64(Smmul(a, b)), nil

	// Half-half
	case "SMULBB":
		return int64(SmulBB(a, b)), nil
	case "SMLABB":
		return int64(SmlaBB(a, b, c)), nil
	case "SMLABB_OVFLW":
		return int64(SmlaBBOvflw(a, b, c)), nil
	case "SMULBT":
		return int64(SmulBT(a, b)), nil
	case "SMLABT":
		return int64(SmlaBT(a, b, c)), nil
	case "SMULTT":
		return int64(SmulTT(a, b)), nil
	case "SMLATT":
		return int64(SmlaTT(a, b, c)), nil
	case "SMLATT_OVFLW":
		return int64(SmlaTTOvflw(a, b, c)), nil
	case "SMULWB":
		return int64(SmulWB(a, b)), nil
	case "SMLAWB":
		return int64(SmlaWB(a, b, c)), nil
	case "SMLAWB_OVFLW":
		return int64(SmlaWBOvflw(a, b, c)), nil
	case "SMULWT":
		return int64(SmulWT(a, b)), nil
	case "SMLAWT":
		return int64(SmlaWT(a, b, c)), nil
	case "SMLAWT_OVFLW":
		return int64(SmlaWTOvflw(a, b, c)), nil
	case "SMULWW":
		return int64(SmulWW(a, b)), nil
	case "SMLAWW":
		return int64(SmlaWW(a, b, c)), nil

	// Shifts
	case "LSHIFT32":
		return int64(LShift32(a, c)), nil
	case "RSHIFT32":
		return int64(RShift32(a, c)), nil
	case "LSHIFT_OVFLW":
		return int64(LShiftOvflw32(a, c)), nil
	case "LSHIFT_SAT32":
		return int64(LShiftSat32(a, c)), nil
	case "RSHIFT_ROUND":
		return int64(RShiftRound(a, c)), nil

	// Sat/Limit/Abs
	case "SAT16":
		return int64(Sat16(a)), nil
	case "SAT32":
		return int64(Sat32From64(int64(a))), nil
	case "ABS32":
		return int64(Abs32(a)), nil
	case "ABS_INT32":
		return int64(AbsInt32(a)), nil
	case "LIMIT":
		// C SKP_LIMIT(a, l1, l2) handles l1>l2 by swapping roles.
		// Our fix.Limit(a, l1, l2) requires l1 <= l2, so we replicate
		// the C swap here.
		l1, l2 := b, c
		if l1 > l2 {
			l1, l2 = l2, l1
		}
		return int64(Limit(a, l1, l2)), nil

	// Min/Max
	case "MIN", "MIN_32", "MIN_INT":
		return int64(MinInt(a, b)), nil
	case "MAX", "MAX_32", "MAX_INT":
		return int64(MaxInt(a, b)), nil
	case "MAX_16":
		return int64(Max16(int16(a), int16(b))), nil

	// Misc
	case "RAND":
		return int64(Rand(a)), nil
	case "ROR32":
		return int64(Ror32(a, c)), nil
	case "DIV32":
		if b == 0 {
			return 0, nil
		}
		return int64(Div32(a, b)), nil
	case "DIV32_16":
		return int64(Div32By16(a, b)), nil
	case "CLZ32":
		return int64(Clz32(a)), nil
	case "CLZ16":
		return int64(Clz16(int16(a))), nil
	case "DIV32_VARQ":
		return int64(Div32VarQ(a, b, c)), nil
	case "INVERSE32_VARQ":
		return int64(Inverse32VarQ(a, c)), nil
	case "SQRT_APPROX":
		return int64(SqrtApprox(a)), nil
	case "SIN_APPROX_Q24":
		return int64(SinApproxQ24(a)), nil
	case "COS_APPROX_Q24":
		return int64(CosApproxQ24(a)), nil

	// lin2log / log2lin / sigm — these live in dsp, not fix; oracle needs
	// to compare against dsp.{Lin2Log, Log2Lin, SigmQ15}. Those tests run
	// in the dsp package. Skip here.
	case "LIN2LOG", "LOG2LIN", "SIGM_Q15":
		return 0, fmt.Errorf("skip: dsp-package macro %s", op)

	default:
		return 0, fmt.Errorf("unsupported op: %s", op)
	}
}

// TestAuditAllMacros walks every golden file and verifies the Go helper
// produces the same value.
func TestAuditAllMacros(t *testing.T) {
	macros := []string{
		"ADD32", "SUB32", "ADD32_OVFLW", "SUB32_OVFLW",
		"ADD_SAT32", "SUB_SAT32", "ADD_POS_SAT32", "ADD_SAT16",
		"ADD_LSHIFT32", "ADD_RSHIFT32", "SUB_LSHIFT32", "SUB_RSHIFT32",
		"MUL", "MLA", "MLA_OVFLW", "SMULL", "SMMUL",
		"SMULBB", "SMLABB", "SMLABB_OVFLW",
		"SMULBT", "SMLABT",
		"SMULTT", "SMLATT", "SMLATT_OVFLW",
		"SMULWB", "SMLAWB", "SMLAWB_OVFLW",
		"SMULWT", "SMLAWT", "SMLAWT_OVFLW",
		"SMULWW", "SMLAWW",
		"LSHIFT32", "RSHIFT32", "LSHIFT_OVFLW", "LSHIFT_SAT32", "RSHIFT_ROUND",
		"SAT16", "SAT32", "ABS32", "ABS_INT32", "LIMIT",
		"MIN", "MAX", "MIN_32", "MAX_32",
		"RAND", "ROR32",
		"DIV32",
		"CLZ32", "CLZ16",
		"DIV32_VARQ", "INVERSE32_VARQ",
		"SQRT_APPROX",
		"SIN_APPROX_Q24", "COS_APPROX_Q24",
		"FIX_CONST",
	}
	for _, macro := range macros {
		macro := macro
		t.Run(macro, func(t *testing.T) {
			queries, results, err := loadGolden(macro)
			if err != nil {
				t.Skipf("no golden for %s: %v", macro, err)
				return
			}
			if len(queries) == 0 {
				t.Skipf("empty golden for %s", macro)
				return
			}
			divergences := 0
			var firstFail string
			for i, q := range queries {
				got, err := goCompute(q)
				if err != nil {
					t.Fatalf("goCompute(%q): %v", q, err)
				}
				if got != results[i] {
					divergences++
					if firstFail == "" {
						firstFail = fmt.Sprintf("query=%q  go=%d  c=%d", q, got, results[i])
					}
				}
			}
			if divergences > 0 {
				t.Errorf("%d/%d divergences (first: %s)", divergences, len(queries), firstFail)
			}
		})
	}
}
