package core

import (
	"context"
	"testing"
)

func collectPages(ctx context.Context, reports ...int) []int {
	var got []int
	ctx = WithPageCounter(ctx, func(n int) { got = append(got, n) })
	for _, n := range reports {
		ReportPageCount(ctx, n)
	}
	return got
}

func TestReportPageCountDeliversPositiveCounts(t *testing.T) {
	got := collectPages(context.Background(), 3, 3)
	if len(got) != 2 || got[0] != 3 || got[1] != 3 {
		t.Fatalf("reports = %v, want [3 3]", got)
	}
}

func TestReportPageCountIgnoresNonPositiveCounts(t *testing.T) {
	if got := collectPages(context.Background(), 0, -1); len(got) != 0 {
		t.Fatalf("reports = %v, want none", got)
	}
}

func TestReportPageCountWithoutCounterIsNoOp(t *testing.T) {
	ReportPageCount(context.Background(), 5)
	var nilContext context.Context
	ReportPageCount(nilContext, 5)
	ctx := WithPageCounter(context.Background(), nil)
	ReportPageCount(ctx, 5)
}

func TestReportPageCountSurvivesDerivedContexts(t *testing.T) {
	var got int
	ctx := WithPageCounter(context.Background(), func(n int) { got = n })
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	ReportPageCount(child, 7)
	if got != 7 {
		t.Fatalf("got %d, want 7", got)
	}
}

func TestWithPageCounterInnermostWins(t *testing.T) {
	var outer, inner int
	ctx := WithPageCounter(context.Background(), func(n int) { outer = n })
	ctx = WithPageCounter(ctx, func(n int) { inner = n })
	ReportPageCount(ctx, 2)
	if outer != 0 || inner != 2 {
		t.Fatalf("outer=%d inner=%d, want 0 and 2", outer, inner)
	}
}
