package dbtx

import (
	"context"
	"testing"
)

func TestAfterCommitWithoutHooksReportsFalse(t *testing.T) {
	if AfterCommit(context.Background(), func(context.Context) {}) {
		t.Fatal("no hook list: AfterCommit must report false")
	}
}

func TestCommitHooksRunOnceInOrder(t *testing.T) {
	ctx, hooks := WithCommitHooks(context.Background())
	var got []int
	for i := 1; i <= 3; i++ {
		if !AfterCommit(ctx, func(context.Context) { got = append(got, i) }) {
			t.Fatal("AfterCommit refused with a hook list")
		}
	}
	hooks.Run(context.Background())
	hooks.Run(context.Background())
	if len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Fatalf("hooks ran %v, want [1 2 3] once", got)
	}
	var nilHooks *CommitHooks
	nilHooks.Run(context.Background()) // must not panic
}
