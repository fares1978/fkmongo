package fkmongo

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

var (
	// ErrNotFound matches (via errors.Is) every error returned when no
	// document matched the filter.
	ErrNotFound = errors.New("not found")
	// ErrDuplicate matches (via errors.Is) every error returned when a write
	// violated a unique index.
	ErrDuplicate = errors.New("already exists")
)

// NotFoundError reports that no document of Entity matched.
type NotFoundError struct {
	Entity string
}

func (e *NotFoundError) Error() string { return entityName(e.Entity) + " not found" }

func (e *NotFoundError) Is(target error) bool { return target == ErrNotFound }

// DuplicateKeyError reports a unique index violation.
type DuplicateKeyError struct {
	Entity string
	// Fields are the indexed fields that collided, best effort; may be empty.
	Fields []string
	// Err is the original driver error.
	Err error
}

func (e *DuplicateKeyError) Error() string {
	if len(e.Fields) == 0 {
		return entityName(e.Entity) + " already exists"
	}
	return fmt.Sprintf("%s with this %s already exists", entityName(e.Entity), strings.Join(e.Fields, ", "))
}

func (e *DuplicateKeyError) Is(target error) bool { return target == ErrDuplicate }

func (e *DuplicateKeyError) Unwrap() error { return e.Err }

func entityName(e string) string {
	if e == "" {
		return "document"
	}
	return e
}

// mapError converts driver errors into this package's error types.
func mapError(entity string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, mongo.ErrNoDocuments):
		return &NotFoundError{Entity: entity}
	case mongo.IsDuplicateKeyError(err):
		return &DuplicateKeyError{Entity: entity, Fields: duplicateKeyFields(err), Err: err}
	}
	return err
}

var (
	dupKeyRe      = regexp.MustCompile(`dup key: \{(.*)\}`)
	dupKeyFieldRe = regexp.MustCompile(`(?:^|,)\s*([^\s:,"{}]+):`)
	quotedRe      = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)
)

// duplicateKeyFields returns the fields of the violated unique index. It reads
// keyPattern from the server response and falls back to parsing the message.
func duplicateKeyFields(err error) []string {
	for _, doc := range serverDocs(err) {
		val, lookupErr := doc.LookupErr("keyPattern")
		if lookupErr != nil {
			continue
		}
		kp, ok := val.DocumentOK()
		if !ok {
			continue
		}
		elems, elemsErr := kp.Elements()
		if elemsErr != nil || len(elems) == 0 {
			continue
		}
		fields := make([]string, 0, len(elems))
		for _, el := range elems {
			fields = append(fields, el.Key())
		}
		return fields
	}

	m := dupKeyRe.FindStringSubmatch(err.Error())
	if m == nil {
		return nil
	}
	var fields []string
	// Blank out string values so commas and colons inside them aren't parsed as keys.
	keys := quotedRe.ReplaceAllString(m[1], `""`)
	for _, f := range dupKeyFieldRe.FindAllStringSubmatch(keys, -1) {
		fields = append(fields, f[1])
	}
	return fields
}

func serverDocs(err error) []bson.Raw {
	var docs []bson.Raw
	var we mongo.WriteException
	if errors.As(err, &we) {
		for _, e := range we.WriteErrors {
			docs = append(docs, e.Raw)
		}
	}
	var bwe mongo.BulkWriteException
	if errors.As(err, &bwe) {
		for _, e := range bwe.WriteErrors {
			docs = append(docs, e.Raw)
		}
	}
	var ce mongo.CommandError
	if errors.As(err, &ce) {
		docs = append(docs, ce.Raw)
	}
	return docs
}
