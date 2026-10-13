package storage

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"idp-dashboard/internal/platform"
)

func TestConnectionEditPreservesUnknownSettingsAndRejectsFutureSchema(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	raw := `{"id":"c","kind":"missing-provider","name":"before","url":"https://example.invalid","schemaVersion":1,"tenant":"old","extension":{"large":"9223372036854775807"},"targets":[{"id":"keep","name":"old","capabilities":["builds"],"future":true},{"id":"remove","future":true}],"rules":[{"id":"rule","future":{"a":1}}]}`
	if _, err = s.db.Exec("INSERT INTO connections VALUES(?,?,?)", "c", raw, ""); err != nil {
		t.Fatal(err)
	}
	connections, err := s.Connections()
	if err != nil {
		t.Fatal(err)
	}
	c := connections[0].Connection
	c.Name = "after"
	c.Tenant = ""
	c.Targets = []platform.Target{{ID: "keep", Name: "new"}}
	c.Rules = []platform.Rule{{ID: "rule", Name: "new"}}
	if err = s.SaveConnection(c, ""); err != nil {
		t.Fatal(err)
	}
	var saved string
	if err = s.db.QueryRow("SELECT config FROM connections WHERE id='c'").Scan(&saved); err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal([]byte(saved), &fields)
	if !strings.Contains(saved, `"large":"9223372036854775807"`) || !strings.Contains(saved, `"future":true`) || !strings.Contains(saved, `"future":{"a":1}`) || strings.Contains(saved, `"remove"`) || fields["tenant"] != nil || strings.Contains(saved, `"capabilities"`) {
		t.Fatalf("settings lost or cleared field restored: %s", saved)
	}
	future := strings.Replace(saved, `"schemaVersion":1`, `"schemaVersion":2`, 1)
	if _, err = s.db.Exec("UPDATE connections SET config=? WHERE id='c'", future); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveConnection(c, ""); err == nil {
		t.Fatal("future config overwritten")
	}
	if err = s.db.QueryRow("SELECT config FROM connections WHERE id='c'").Scan(&saved); err != nil || saved != future {
		t.Fatal("refused edit changed stored config")
	}
}
