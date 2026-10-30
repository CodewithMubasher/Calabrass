package store

import (
	"context"
	"path/filepath"
	"testing"
)

func newLedgerStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestLedgerListsNewestFirstAndPerPerson(t *testing.T) {
	ctx := context.Background()
	s := newLedgerStore(t)
	for i, e := range []LedgerEntry{
		{UserID: "a", At: 100, Tool: "run_command", Tier: 2, Summary: "Run: npm test", Outcome: "approved"},
		{UserID: "a", At: 300, Tool: "run_command", Tier: 3, Summary: "Run: rm -rf x", Outcome: "refused"},
		{UserID: "a", At: 200, Tool: "run_command", Tier: 2, Summary: "Run: go build", Outcome: "allowed", Unattended: true, ChatID: "c1"},
		{UserID: "b", At: 250, Tool: "run_command", Tier: 2, Summary: "someone else's", Outcome: "approved"},
	} {
		if err := s.AddLedgerEntry(ctx, e); err != nil {
			t.Fatalf("%d: %v", i, err)
		}
	}

	got, err := s.ListLedger(ctx, "a", LedgerQuery{})
	if err != nil || len(got) != 3 || got[0].At != 300 || got[2].At != 100 {
		t.Fatalf("a's ledger: %+v %v", got, err)
	}
	for _, e := range got {
		if e.Summary == "someone else's" {
			t.Fatal("another person's row leaked")
		}
	}

	if got, _ := s.ListLedger(ctx, "a", LedgerQuery{Unattended: true}); len(got) != 1 || got[0].ChatID != "c1" {
		t.Fatalf("unattended filter: %+v", got)
	}
	if got, _ := s.ListLedger(ctx, "a", LedgerQuery{Refused: true}); len(got) != 1 || got[0].Outcome != "refused" {
		t.Fatalf("refused filter: %+v", got)
	}
	if got, _ := s.ListLedger(ctx, "a", LedgerQuery{ChatID: "c1"}); len(got) != 1 {
		t.Fatalf("chat filter: %+v", got)
	}
	// Paging: only rows older than the last one seen.
	if got, _ := s.ListLedger(ctx, "a", LedgerQuery{Before: 300, Limit: 1}); len(got) != 1 || got[0].At != 200 {
		t.Fatalf("paging: %+v", got)
	}
}

func TestLedgerKeepsOnlyTheNewestRows(t *testing.T) {
	ctx := context.Background()
	s := newLedgerStore(t)
	old := ledgerKeep
	ledgerKeep = 20
	t.Cleanup(func() { ledgerKeep = old })

	for i := 1; i <= 45; i++ {
		if err := s.AddLedgerEntry(ctx, LedgerEntry{UserID: "a", At: int64(i), Tool: "t", Outcome: "allowed"}); err != nil {
			t.Fatal(err)
		}
	}
	// Another person's rows are not counted against this person's limit.
	_ = s.AddLedgerEntry(ctx, LedgerEntry{UserID: "b", At: 1, Tool: "t"})

	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM ledger WHERE user_id = 'a'`).Scan(&n); err != nil || n != 20 {
		t.Fatalf("kept %d rows, want 20 (%v)", n, err)
	}
	var oldest int64
	_ = s.db.QueryRow(`SELECT MIN(at) FROM ledger WHERE user_id = 'a'`).Scan(&oldest)
	if oldest != 26 {
		t.Fatalf("the oldest kept row is %d, want 26 (the 25 oldest dropped)", oldest)
	}
	var other int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM ledger WHERE user_id = 'b'`).Scan(&other)
	if other != 1 {
		t.Fatal("trimming one person's ledger touched another's")
	}
}

func TestLedgerNeedsAUser(t *testing.T) {
	if err := newLedgerStore(t).AddLedgerEntry(context.Background(), LedgerEntry{Tool: "x"}); err == nil {
		t.Fatal("an entry with no user was accepted")
	}
}
