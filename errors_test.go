package fkmongo

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestDuplicateKeyFieldsFromMessage(t *testing.T) {
	tests := []struct {
		msg  string
		want []string
	}{
		{`E11000 duplicate key error collection: db.users index: email_1 dup key: { email: "a@b.c" }`, []string{"email"}},
		{`E11000 duplicate key error collection: db.users index: a_1_b_1 dup key: { a: 1, b: "x, y: z" }`, []string{"a", "b"}},
		{`some other error`, nil},
	}
	for _, tt := range tests {
		if got := duplicateKeyFields(errors.New(tt.msg)); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("duplicateKeyFields(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}

func TestErrorMatching(t *testing.T) {
	nf := fmt.Errorf("wrapped: %w", &NotFoundError{Entity: "supplier"})
	if !errors.Is(nf, ErrNotFound) || errors.Is(nf, ErrDuplicate) {
		t.Errorf("NotFoundError matched wrongly")
	}
	if got := (&NotFoundError{}).Error(); got != "document not found" {
		t.Errorf("empty entity message = %q", got)
	}

	cause := errors.New("driver error")
	dup := &DuplicateKeyError{Entity: "supplier", Fields: []string{"email"}, Err: cause}
	if !errors.Is(dup, ErrDuplicate) || !errors.Is(dup, cause) {
		t.Errorf("DuplicateKeyError should match ErrDuplicate and its cause")
	}
	if got, want := dup.Error(), "supplier with this email already exists"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
