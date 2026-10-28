package platform

import (
	"errors"
	sdk "github.com/GGingGGang/dashboard/sdk"
	"slices"
)

func validateContributions(m sdk.Manifest) error {
	check := func(id, title, capability, renderer string) error {
		if !extensionID.MatchString(id) || title == "" || len(title) > 256 {
			return errors.New("Invalid contribution identity")
		}
		if !slices.Contains([]string{"builds", "queue", "deployments", "metrics", "logs", "traces", "data"}, capability) {
			return errors.New("Unknown contribution capability")
		}
		if !slices.Contains([]string{"table", "scalar", "timeseries"}, renderer) {
			return errors.New("Unknown contribution renderer")
		}
		if capability != "data" && ((capability == "metrics" && renderer != "timeseries") || (capability != "metrics" && renderer != "table")) {
			return errors.New("Renderer does not match capability")
		}
		return nil
	}
	cards := map[string]bool{}
	queries := map[string]bool{}
	dashboards := map[string]bool{}
	for _, p := range m.Providers {
		if _, err := sdk.ParseFormSchema(p.ConfigSchema); err != nil {
			return err
		}
	}
	for _, c := range m.Cards {
		if err := check(c.ID, c.Title, c.Capability, c.Renderer); err != nil {
			return err
		}
		if cards[c.ID] {
			return errors.New("Duplicate card")
		}
		cards[c.ID] = true
		if _, err := sdk.ParseFormSchema(c.ConfigSchema); err != nil {
			return err
		}
	}
	for _, q := range m.Queries {
		if err := check(q.ID, q.Title, q.Capability, q.Renderer); err != nil {
			return err
		}
		if queries[q.ID] {
			return errors.New("Duplicate query")
		}
		queries[q.ID] = true
		if _, err := sdk.ParseFormSchema(q.InputSchema); err != nil {
			return err
		}
	}
	for _, d := range m.Dashboards {
		if !extensionID.MatchString(d.ID) || d.Title == "" || len(d.Title) > 256 || dashboards[d.ID] {
			return errors.New("Invalid dashboard")
		}
		dashboards[d.ID] = true
		seen := map[string]bool{}
		for _, id := range d.Cards {
			if !cards[id] || seen[id] {
				return errors.New("Invalid dashboard card reference")
			}
			seen[id] = true
		}
	}
	return nil
}
