package review

import (
	"context"
	"errors"
	"testing"
)

func TestCreateValidatesChecksAndCopiesCollections(t *testing.T) {
	store := NewStore()
	checks := []string{"ingredients", "allergens"}
	record, err := store.Create(context.Background(), "CP-APPLE-12", checks)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	checks[0] = "changed"
	record.Checks[1].Name = "changed"

	got, err := store.Get(record.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Checks[0].Name != "ingredients" || got.Checks[1].Name != "allergens" {
		t.Fatalf("store did not retain defensive copies: %+v", got.Checks)
	}

	for name, input := range map[string][]string{
		"empty":     {},
		"blank":     {"  "},
		"duplicate": {"ingredients", "ingredients"},
	} {
		if _, err := store.Create(context.Background(), "CP-OTHER", input); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: Create() error = %v, want ErrInvalidInput", name, err)
		}
	}
	if _, err := store.Create(context.Background(), " ", []string{"ingredients"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("blank SKU: Create() error = %v, want ErrInvalidInput", err)
	}
}

func TestRecordVerdictPreservesOptionalNoteAndRejectsSecondVerdict(t *testing.T) {
	store := NewStore()
	record, err := store.Create(context.Background(), "CP-APPLE-12", []string{"ingredients"})
	if err != nil {
		t.Fatal(err)
	}
	emptyNote := ""
	updated, err := store.RecordVerdict(context.Background(), record.ID, "ingredients", false, &emptyNote)
	if err != nil {
		t.Fatalf("RecordVerdict() error = %v", err)
	}
	verdict := updated.Checks[0].Verdict
	if verdict == nil || verdict.Passed || verdict.Note == nil || *verdict.Note != "" {
		t.Fatalf("empty supplied note was not retained: %+v", verdict)
	}
	if _, err := store.RecordVerdict(context.Background(), record.ID, "ingredients", true, nil); !errors.Is(err, ErrVerdictExists) {
		t.Fatalf("second verdict error = %v, want ErrVerdictExists", err)
	}
}

func TestFinalizeApprovedOrBlockedAndKeepsCompletedRecordsImmutable(t *testing.T) {
	t.Run("approved", func(t *testing.T) {
		store := NewStore()
		record, err := store.Create(context.Background(), "CP-APPROVED", []string{"ingredients", "barcode"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.RecordVerdict(context.Background(), record.ID, "ingredients", true, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := store.RecordVerdict(context.Background(), record.ID, "barcode", true, nil); err != nil {
			t.Fatal(err)
		}
		finalized, err := store.Finalize(context.Background(), record.ID)
		if err != nil {
			t.Fatalf("Finalize() error = %v", err)
		}
		if finalized.Status != StatusApproved || finalized.CompletedAt == nil {
			t.Fatalf("finalized review = %+v", finalized)
		}
		finalized.Status = StatusBlocked
		finalized.Checks[0].Verdict.Passed = false
		got, err := store.Get(record.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != StatusApproved || !got.Checks[0].Verdict.Passed {
			t.Fatalf("completed record changed through returned copy: %+v", got)
		}
		if _, err := store.Finalize(context.Background(), record.ID); !errors.Is(err, ErrCompleted) {
			t.Fatalf("second Finalize() error = %v, want ErrCompleted", err)
		}
	})

	t.Run("blocked", func(t *testing.T) {
		store := NewStore()
		record, err := store.Create(context.Background(), "CP-BLOCKED", []string{"ingredients"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.RecordVerdict(context.Background(), record.ID, "ingredients", false, nil); err != nil {
			t.Fatal(err)
		}
		finalized, err := store.Finalize(context.Background(), record.ID)
		if err != nil {
			t.Fatal(err)
		}
		if finalized.Status != StatusBlocked {
			t.Fatalf("status = %q, want blocked", finalized.Status)
		}
	})
}

func TestFinalizeRequiresEveryCheck(t *testing.T) {
	store := NewStore()
	record, err := store.Create(context.Background(), "CP-INCOMPLETE", []string{"ingredients", "barcode"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordVerdict(context.Background(), record.ID, "ingredients", true, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Finalize(context.Background(), record.ID); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("Finalize() error = %v, want ErrIncomplete", err)
	}
}

func TestCanceledMutationsDoNotCommit(t *testing.T) {
	store := NewStore()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Create(canceled, "CP-CANCELED", []string{"ingredients"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Create() error = %v", err)
	}
	if _, err := store.Get("review-000001"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("canceled Create() left a record: %v", err)
	}

	record, err := store.Create(context.Background(), "CP-CANCELED", []string{"ingredients"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordVerdict(canceled, record.ID, "ingredients", true, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled RecordVerdict() error = %v", err)
	}
	got, err := store.Get(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Checks[0].Verdict != nil {
		t.Fatal("canceled RecordVerdict() committed a verdict")
	}
	if _, err := store.Finalize(canceled, record.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Finalize() error = %v", err)
	}
	got, err = store.Get(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusOpen {
		t.Fatalf("canceled Finalize() changed status to %q", got.Status)
	}
}
