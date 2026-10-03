package platform

import (
	"context"
	"net/url"
)

type ArgoCD struct{ *api }

func (a *ArgoCD) Check(ctx context.Context) error { _, err := a.Deployments(ctx); return err }
func (a *ArgoCD) Discover(ctx context.Context) ([]Target, error) {
	apps, err := a.Deployments(ctx)
	out := []Target{}
	for _, app := range apps {
		out = append(out, Target{ID: app.ID, Name: app.Name})
	}
	return out, err
}
func (a *ArgoCD) Deployments(ctx context.Context) ([]Deployment, error) {
	var response struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Spec struct {
				Project string `json:"project"`
			} `json:"spec"`
			Status struct {
				Sync struct {
					Status    string   `json:"status"`
					Revision  string   `json:"revision"`
					Revisions []string `json:"revisions"`
				} `json:"sync"`
				Health struct {
					Status  string `json:"status"`
					Message string `json:"message"`
				} `json:"health"`
				Operation struct {
					Phase    string `json:"phase"`
					Message  string `json:"message"`
					Finished string `json:"finishedAt"`
				} `json:"operationState"`
			} `json:"status"`
		} `json:"items"`
	}
	err := a.get(ctx, "/api/v1/applications", url.Values{}, &response)
	result := []Deployment{}
	for _, app := range response.Items {
		revs := app.Status.Sync.Revisions
		if len(revs) == 0 && app.Status.Sync.Revision != "" {
			revs = []string{app.Status.Sync.Revision}
		}
		id := app.Metadata.Namespace + "/" + app.Metadata.Name
		result = append(result, Deployment{ID: id, Name: app.Metadata.Name, Project: app.Spec.Project, Sync: app.Status.Sync.Status, Health: app.Status.Health.Status, Revisions: revs, Phase: app.Status.Operation.Phase, Message: app.Status.Operation.Message, Finished: app.Status.Operation.Finished, URL: a.link("/applications/" + url.PathEscape(app.Metadata.Namespace) + "/" + url.PathEscape(app.Metadata.Name))})
	}
	return result, err
}
