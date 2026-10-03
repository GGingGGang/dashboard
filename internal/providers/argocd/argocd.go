package argocd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sync"

	"idp-dashboard/internal/platform"
	"idp-dashboard/internal/providers/internal/httpapi"
)

type ArgoCD struct {
	api          *httpapi.Client
	connection   platform.Connection
	password     string
	mu           sync.Mutex
	sessionToken string
	loginError   error
}

func (a *ArgoCD) login(ctx context.Context) error {
	if a.loginError != nil {
		return a.loginError
	}
	body, err := json.Marshal(map[string]string{"username": a.connection.Username, "password": a.password})
	if err != nil {
		return errors.New("Cannot prepare Argo CD login")
	}
	var response struct {
		Token string `json:"token"`
	}
	if err = a.api.Request(ctx, http.MethodPost, "/api/v1/session", nil, body, &response); err != nil {
		var upstream *platform.APIError
		if errors.As(err, &upstream) && (upstream.Status == 401 || upstream.Status == 403) {
			a.loginError = errors.New("Argo CD 로그인 실패: 사용자명·비밀번호와 로컬 계정 로그인 허용 여부를 확인한 뒤 연결을 다시 저장하세요")
			return a.loginError
		}
		return err
	}
	if response.Token == "" {
		return errors.New("Argo CD login did not return a session token")
	}
	a.sessionToken = response.Token
	return nil
}

func (a *ArgoCD) applications(ctx context.Context, out any) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	client := a.api
	if a.connection.Auth == "argocd-login" {
		if a.sessionToken == "" {
			if err := a.login(ctx); err != nil {
				return err
			}
		}
		client = a.api.WithBearer(a.sessionToken)
	}
	err := client.Get(ctx, "/api/v1/applications", nil, out)
	var upstream *platform.APIError
	if a.connection.Auth == "argocd-login" && errors.As(err, &upstream) && upstream.Status == 401 {
		// Expired sessions get one refresh, never an unbounded login loop.
		a.sessionToken = ""
		if err = a.login(ctx); err != nil {
			return err
		}
		client = a.api.WithBearer(a.sessionToken)
		err = client.Get(ctx, "/api/v1/applications", nil, out)
	}
	if errors.As(err, &upstream) {
		switch upstream.Status {
		case 401:
			return errors.New("Argo CD 인증 실패: Bearer에는 비밀번호가 아닌 유효한 토큰이 필요합니다. 계정 비밀번호는 Argo CD 로그인을 선택하세요")
		case 403:
			return errors.New("Argo CD 조회 권한이 거부되었습니다. 계정 또는 토큰의 applications 조회 권한을 확인하세요")
		}
	}
	return err
}

func (a *ArgoCD) Check(ctx context.Context) error { _, err := a.Deployments(ctx); return err }
func (a *ArgoCD) Discover(ctx context.Context) ([]platform.Target, error) {
	apps, err := a.Deployments(ctx)
	out := []platform.Target{}
	for _, app := range apps {
		out = append(out, platform.Target{ID: app.ID, Name: app.Name})
	}
	return out, err
}
func (a *ArgoCD) Deployments(ctx context.Context) ([]platform.Deployment, error) {
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
	err := a.applications(ctx, &response)
	result := []platform.Deployment{}
	for _, app := range response.Items {
		revs := app.Status.Sync.Revisions
		if len(revs) == 0 && app.Status.Sync.Revision != "" {
			revs = []string{app.Status.Sync.Revision}
		}
		id := app.Metadata.Namespace + "/" + app.Metadata.Name
		result = append(result, platform.Deployment{ID: id, Name: app.Metadata.Name, Project: app.Spec.Project, Sync: app.Status.Sync.Status, Health: app.Status.Health.Status, Revisions: revs, Phase: app.Status.Operation.Phase, Message: app.Status.Operation.Message, Finished: app.Status.Operation.Finished, URL: a.api.Link("/applications/" + url.PathEscape(app.Metadata.Namespace) + "/" + url.PathEscape(app.Metadata.Name))})
	}
	return result, err
}

// Definition registers this adapter and its supported capabilities.
func Definition() platform.Definition {
	return platform.Definition{
		Info:     platform.ProviderInfo{Kind: "argocd", Name: "Argo CD", Category: "cd", Capabilities: []string{"deployments", "sync", "health"}, DefaultAuth: "bearer", AuthMethods: []string{"bearer", "argocd-login", "none"}, PollSeconds: 15},
		Validate: validate,
		Create: func(c platform.Connection, secret string) (platform.Provider, error) {
			client, err := httpapi.New(c, secret)
			if err != nil {
				return nil, err
			}
			return &ArgoCD{api: client, connection: c, password: secret}, nil
		},
	}
}

func validate(c platform.Connection) error {
	if c.Auth == "argocd-login" && c.Username == "" {
		return errors.New("Username is required for this authentication method")
	}
	return nil
}
