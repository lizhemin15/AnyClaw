package scheduler

import (
	"reflect"
	"testing"
)

func TestParseAgentSlugsFromConfigJSON(t *testing.T) {
	t.Run("empty list implies main", func(t *testing.T) {
		slugs, err := ParseAgentSlugsFromConfigJSON([]byte(`{"agents":{"defaults":{},"list":[]}}`))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(slugs, []string{"main"}) {
			t.Fatalf("got %#v", slugs)
		}
	})
	t.Run("multi id", func(t *testing.T) {
		j := `{"agents":{"list":[{"id":"main"},{"id":"worker-a"},{"id":"Worker-B"}]}}`
		slugs, err := ParseAgentSlugsFromConfigJSON([]byte(j))
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"main", "worker-a", "worker-b"}
		if !reflect.DeepEqual(slugs, want) {
			t.Fatalf("got %#v want %#v", slugs, want)
		}
	})
}
