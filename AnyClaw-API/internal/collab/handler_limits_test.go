package collab

import (
	"testing"

	"github.com/anyclaw/anyclaw-api/internal/db"
)

func TestCollaborationLimitsPayload(t *testing.T) {
	p := collaborationLimitsPayload()
	want := map[string]int{
		"max_agents":                         db.MaxCollaborationAgentsPerInstance,
		"max_edges":                          db.MaxCollaborationEdgesPerInstance,
		"max_thread_id_runes":                db.MaxInternalMailThreadRunes,
		"max_internal_mail_subject_runes":    db.MaxInternalMailSubjectRunes,
		"max_internal_mail_body_kb":          db.MaxInternalMailBodyBytes / 1024,
		"max_agent_slug_runes":               db.MaxInstanceAgentSlugRunes,
		"max_agent_display_name_runes":       db.MaxInstanceAgentDisplayRunes,
		"max_internal_mail_list_limit":       db.MaxInternalMailListLimit,
		"max_internal_mail_list_offset":      db.MaxInternalMailListOffset,
		"max_instance_message_body_kb":       db.MaxUserInstanceMessageBodyBytes / 1024,
		"max_instance_message_list_limit":    db.MaxUserInstanceMessageListLimit,
		"max_instance_message_list_offset":   db.MaxUserInstanceMessageListOffset,
	}
	if len(p) != len(want) {
		t.Fatalf("limits key count: got %d want %d", len(p), len(want))
	}
	for k, v := range want {
		got, ok := p[k]
		if !ok {
			t.Fatalf("missing limits key %q", k)
		}
		if got != v {
			t.Fatalf("limits[%q]: got %d want %d", k, got, v)
		}
	}
}
