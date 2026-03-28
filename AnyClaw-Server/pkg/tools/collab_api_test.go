package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCollabAPIClient_GetRoster_GetTopology(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/instances/1/collab/bridge/roster", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != "tok" || r.Header.Get("Authorization") != "Bearer tok" {
			t.Error("expected token query and Bearer header")
		}
		fmt.Fprint(w, `{"agents":[{"id":1,"instance_id":1,"user_id":10,"agent_slug":"main","display_name":"主理"}],"peer_instances":[{"instance_id":2,"name":"B"}],"instance_topology_version":7,"limits":{"max_agents":64,"max_edges":4096,"max_thread_id_runes":64,"max_internal_mail_subject_runes":512,"max_internal_mail_body_kb":256,"max_agent_slug_runes":128,"max_agent_display_name_runes":255,"max_internal_mail_list_limit":500,"max_internal_mail_list_offset":500000}}`)
	})
	mux.HandleFunc("/instances/1/collab/bridge/topology", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"edges":[["a","b"]],"version":3,"peer_instances":[{"instance_id":2,"name":"B"}],"instance_topology_version":7,"limits":{"max_agents":64,"max_edges":4096,"max_thread_id_runes":64,"max_internal_mail_subject_runes":512,"max_internal_mail_body_kb":256,"max_agent_slug_runes":128,"max_agent_display_name_runes":255,"max_internal_mail_list_limit":500,"max_internal_mail_list_offset":500000}}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewCollabAPIClient(srv.URL, "1", "tok")
	ctx := context.Background()

	roster, err := c.GetRoster(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(roster.Agents) != 1 || roster.Agents[0].AgentSlug != "main" {
		t.Fatalf("roster agents: %+v", roster.Agents)
	}
	if len(roster.PeerInstances) != 1 || roster.PeerInstances[0].InstanceID != 2 || roster.PeerInstances[0].Name != "B" || roster.InstanceTopologyVersion != 7 {
		t.Fatalf("roster peers/version: %+v ver=%d", roster.PeerInstances, roster.InstanceTopologyVersion)
	}
	if roster.Limits == nil || roster.Limits.MaxAgents != 64 || roster.Limits.MaxEdges != 4096 || roster.Limits.MaxThreadIDRunes != 64 ||
		roster.Limits.MaxInternalMailSubjectRunes != 512 || roster.Limits.MaxInternalMailBodyKB != 256 ||
		roster.Limits.MaxAgentSlugRunes != 128 || roster.Limits.MaxAgentDisplayNameRunes != 255 ||
		roster.Limits.MaxInternalMailListLimit != 500 || roster.Limits.MaxInternalMailListOffset != 500000 {
		t.Fatalf("roster limits: %+v", roster.Limits)
	}

	topo, err := c.GetTopology(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(topo.Edges) != 1 || topo.Edges[0][0] != "a" || topo.Edges[0][1] != "b" || topo.Version != 3 {
		t.Fatalf("topology: edges=%v version=%d", topo.Edges, topo.Version)
	}
	if len(topo.PeerInstances) != 1 || topo.PeerInstances[0].InstanceID != 2 || topo.InstanceTopologyVersion != 7 {
		t.Fatalf("topology peers: %+v ver=%d", topo.PeerInstances, topo.InstanceTopologyVersion)
	}
	if topo.Limits == nil || topo.Limits.MaxEdges != 4096 || topo.Limits.MaxThreadIDRunes != 64 ||
		topo.Limits.MaxInternalMailSubjectRunes != 512 || topo.Limits.MaxInternalMailBodyKB != 256 ||
		topo.Limits.MaxAgentSlugRunes != 128 || topo.Limits.MaxAgentDisplayNameRunes != 255 ||
		topo.Limits.MaxInternalMailListLimit != 500 || topo.Limits.MaxInternalMailListOffset != 500000 {
		t.Fatalf("topology limits: %+v", topo.Limits)
	}
}

func TestCollabAPIClient_SyncRosterSlugs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/instances/1/collab/bridge/roster/sync", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Query().Get("token") != "tok" || r.Header.Get("Authorization") != "Bearer tok" {
			t.Error("expected token query and Bearer header")
		}
		fmt.Fprint(w, `{"status":"ok","added":2,"limits":{"max_agents":64}}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewCollabAPIClient(srv.URL, "1", "tok")
	added, err := c.SyncRosterSlugs(context.Background(), []string{"main", "a"})
	if err != nil {
		t.Fatal(err)
	}
	if added != 2 {
		t.Fatalf("added=%d", added)
	}
}

func TestCollabAPIClient_GetInternalMailList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/instances/1/collab/bridge/mails", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != "tok" {
			t.Error("expected token query")
		}
		if !strings.Contains(r.URL.RawQuery, "limit=50") {
			t.Errorf("expected limit in query: %q", r.URL.RawQuery)
		}
		fmt.Fprint(w, `{"mails":[],"total":0,"limits":{"max_internal_mail_list_limit":500}}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewCollabAPIClient(srv.URL, "1", "tok")
	out, err := c.GetInternalMailList(context.Background(), "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	total, ok := out["total"].(float64)
	if !ok || total != 0 {
		t.Fatalf("total: %#v", out["total"])
	}
}

func TestCollabAPIClient_GetInternalMail(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/instances/1/collab/bridge/mail/42", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != "tok" || r.Header.Get("Authorization") != "Bearer tok" {
			t.Error("expected token query and Bearer header")
		}
		fmt.Fprint(w, `{"id":42,"instance_id":1,"thread_id":"t","from_slug":"a","to_slug":"b","subject":"s","body":"x","topology_version":1,"created_at":"2020-01-01T00:00:00Z","limits":{"max_agents":64,"max_internal_mail_list_limit":500}}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewCollabAPIClient(srv.URL, "1", "tok")
	out, err := c.GetInternalMail(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if id, _ := out["id"].(float64); id != 42 {
		t.Fatalf("id: %#v", out["id"])
	}
	lim, ok := out["limits"].(map[string]any)
	if !ok || lim["max_agents"].(float64) != 64 {
		t.Fatalf("limits: %#v", out["limits"])
	}
}

func TestCollabAPIClient_InstanceMessagesBridge(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/instances/1/collab/bridge/instance-mail", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != "tok" {
			t.Error("expected token query")
		}
		switch r.Method {
		case http.MethodGet:
			fmt.Fprint(w, `{"messages":[],"total":0,"limits":{"max_instance_message_list_limit":500}}`)
		case http.MethodPost:
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("expected JSON body: %q", r.Header.Get("Content-Type"))
			}
			fmt.Fprint(w, `{"ok":true,"id":9,"limits":{"max_instance_message_body_kb":256}}`)
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewCollabAPIClient(srv.URL, "1", "tok")
	ctx := context.Background()

	list, err := c.GetInstanceMessageList(ctx, 20, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if tot, _ := list["total"].(float64); tot != 0 {
		t.Fatalf("total: %#v", list["total"])
	}

	post, err := c.PostInstanceMessage(ctx, 2, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := post["ok"].(bool); !ok {
		t.Fatalf("post: %#v", post)
	}
}
