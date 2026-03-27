package db

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestErrInternalMailListOffsetTooLarge(t *testing.T) {
	wrapped := fmt.Errorf("detail: %w", ErrInternalMailListOffsetTooLarge)
	if !errors.Is(wrapped, ErrInternalMailListOffsetTooLarge) {
		t.Fatal("errors.Is should match wrapped sentinel")
	}
}

func TestErrInternalMailInvalidThreadFilter(t *testing.T) {
	inner := validateInternalMailThreadID(strings.Repeat("x", MaxInternalMailThreadRunes+1))
	if inner == nil {
		t.Fatal("inner error expected")
	}
	wrapped := fmt.Errorf("%w: %v", ErrInternalMailInvalidThreadFilter, inner)
	if !errors.Is(wrapped, ErrInternalMailInvalidThreadFilter) {
		t.Fatal("errors.Is should match thread filter sentinel")
	}
}

func TestCanonicalEdge(t *testing.T) {
	cases := []struct {
		a, b       string
		wantLo, hi string
	}{
		{"a", "b", "a", "b"},
		{"b", "a", "a", "b"},
		{"  x ", "y", "x", "y"},
	}
	for _, tc := range cases {
		lo, hi := canonicalEdge(tc.a, tc.b)
		if lo != tc.wantLo || hi != tc.hi {
			t.Fatalf("canonicalEdge(%q,%q) = (%q,%q), want (%q,%q)", tc.a, tc.b, lo, hi, tc.wantLo, tc.hi)
		}
	}
}

func TestValidateInternalMailReplyAgainstParent(t *testing.T) {
	parent := &InternalMailRow{
		ThreadID: "th1",
		FromSlug: "alice",
		ToSlug:   "bob",
	}
	err := validateInternalMailReplyAgainstParent(nil, "th1", "bob", "alice")
	if err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("nil parent: got %v", err)
	}
	if err := validateInternalMailReplyAgainstParent(parent, "th2", "bob", "alice"); err == nil {
		t.Fatal("thread mismatch: want error")
	}
	if err := validateInternalMailReplyAgainstParent(parent, "th1", "alice", "bob"); err == nil {
		t.Fatal("wrong replier: want error")
	}
	if err := validateInternalMailReplyAgainstParent(parent, "th1", "bob", "bob"); err == nil {
		t.Fatal("wrong recipient: want error")
	}
	if err := validateInternalMailReplyAgainstParent(parent, "th1", "bob", "alice"); err != nil {
		t.Fatalf("valid reply: %v", err)
	}
	if err := validateInternalMailReplyAgainstParent(parent, " th1 ", " bob ", " alice "); err != nil {
		t.Fatalf("trimmed fields: %v", err)
	}
}

func TestValidateInternalMailThreadForPost(t *testing.T) {
	var d *DB
	s, err := d.ValidateInternalMailThreadForPost("  th1  ")
	if err != nil || s != "th1" {
		t.Fatalf("trim: got %q err=%v", s, err)
	}
	if _, err := d.ValidateInternalMailThreadForPost(""); err == nil {
		t.Fatal("empty: want error")
	}
	long := strings.Repeat("x", MaxInternalMailThreadRunes+1)
	if _, err := d.ValidateInternalMailThreadForPost(long); err == nil {
		t.Fatal("long: want error")
	}
}

func TestValidateInternalMailContent(t *testing.T) {
	if err := validateInternalMailContent("", strings.Repeat("a", MaxInternalMailBodyBytes)); err != nil {
		t.Fatalf("max body ok: %v", err)
	}
	if err := validateInternalMailContent("", strings.Repeat("a", MaxInternalMailBodyBytes+1)); err == nil {
		t.Fatal("oversized body: want error")
	}
	sub512 := strings.Repeat("a", MaxInternalMailSubjectRunes)
	if err := validateInternalMailContent(sub512, "x"); err != nil {
		t.Fatal(err)
	}
	if err := validateInternalMailContent(sub512+"b", "x"); err == nil {
		t.Fatal("oversized subject: want error")
	}
	if err := validateInternalMailThreadID(strings.Repeat("世", MaxInternalMailThreadRunes)); err != nil {
		t.Fatal(err)
	}
	if err := validateInternalMailThreadID(strings.Repeat("世", MaxInternalMailThreadRunes+1)); err == nil {
		t.Fatal("long thread_id: want error")
	}
	if utf8.RuneCountInString(strings.Repeat("世", MaxInternalMailThreadRunes+1)) != MaxInternalMailThreadRunes+1 {
		t.Fatal("test string rune count")
	}
}

func TestCollabResolveNameInput(t *testing.T) {
	if _, err := collabResolveNameInput("  "); err == nil {
		t.Fatal("blank: want error")
	}
	ok := strings.Repeat("a", MaxInstanceAgentDisplayRunes)
	if q, err := collabResolveNameInput(ok); err != nil || q != ok {
		t.Fatalf("max len: %v %q", err, q)
	}
	if _, err := collabResolveNameInput(ok + "b"); err == nil {
		t.Fatal("too long: want error")
	}
}

func TestValidateCollabAgentSlugSyntax(t *testing.T) {
	if err := validateCollabAgentSlugSyntax(" main "); err != nil {
		t.Fatal(err)
	}
	if err := validateCollabAgentSlugSyntax(""); err == nil {
		t.Fatal("empty: want error")
	}
	if err := validateCollabAgentSlugSyntax(strings.Repeat("世", MaxInstanceAgentSlugRunes+1)); err == nil {
		t.Fatal("long: want error")
	}
}

func TestValidateInstanceAgentRows(t *testing.T) {
	if err := validateInstanceAgentRows(nil); err == nil {
		t.Fatal("nil slice: want error")
	}
	if err := validateInstanceAgentRows([]InstanceAgentRow{}); err == nil {
		t.Fatal("empty: want error")
	}
	if err := validateInstanceAgentRows([]InstanceAgentRow{{AgentSlug: "a", DisplayName: "n"}}); err != nil {
		t.Fatal(err)
	}
	if err := validateInstanceAgentRows([]InstanceAgentRow{
		{AgentSlug: "a", DisplayName: "n1"},
		{AgentSlug: "a", DisplayName: "n2"},
	}); err == nil {
		t.Fatal("dup slug: want error")
	}
	if err := validateInstanceAgentRows([]InstanceAgentRow{
		{AgentSlug: "a", DisplayName: "n"},
		{AgentSlug: "b", DisplayName: "n"},
	}); err == nil {
		t.Fatal("dup name: want error")
	}
	if err := validateInstanceAgentRows([]InstanceAgentRow{{AgentSlug: " ", DisplayName: "x"}}); err == nil {
		t.Fatal("blank slug: want error")
	}
	tooMany := make([]InstanceAgentRow, MaxCollaborationAgentsPerInstance+1)
	for i := range tooMany {
		tooMany[i] = InstanceAgentRow{AgentSlug: "a" + strconv.Itoa(i), DisplayName: "n" + strconv.Itoa(i)}
	}
	if err := validateInstanceAgentRows(tooMany); err == nil {
		t.Fatal("too many agents: want error")
	}
	longSlug := strings.Repeat("x", MaxInstanceAgentSlugRunes)
	if err := validateInstanceAgentRows([]InstanceAgentRow{{AgentSlug: longSlug, DisplayName: "n"}}); err != nil {
		t.Fatal(err)
	}
	if err := validateInstanceAgentRows([]InstanceAgentRow{{AgentSlug: longSlug + "y", DisplayName: "n"}}); err == nil {
		t.Fatal("long slug: want error")
	}
	longName := strings.Repeat("世", MaxInstanceAgentDisplayRunes)
	if err := validateInstanceAgentRows([]InstanceAgentRow{{AgentSlug: "a", DisplayName: longName}}); err != nil {
		t.Fatal(err)
	}
	if err := validateInstanceAgentRows([]InstanceAgentRow{{AgentSlug: "a", DisplayName: longName + "世"}}); err == nil {
		t.Fatal("long display: want error")
	}
}

func TestValidateInternalMailReply_skipsDB(t *testing.T) {
	var d *DB
	if err := d.ValidateInternalMailReply(1, "t", "a", "b", nil); err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	if err := d.ValidateInternalMailReply(1, "t", "a", "b", &zero); err != nil {
		t.Fatal(err)
	}
}
