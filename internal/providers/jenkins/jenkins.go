package jenkins

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"idp-dashboard/internal/platform"
	"idp-dashboard/internal/providers/internal/httpapi"
)

type Jenkins struct {
	api          *httpapi.Client
	connectionID string
}

func (j *Jenkins) Check(ctx context.Context) error {
	var out map[string]any
	return j.api.Get(ctx, "/api/json", url.Values{"tree": {"mode"}}, &out)
}

func (j *Jenkins) Discover(ctx context.Context) ([]platform.Target, error) {
	targets := []platform.Target{}
	folders := []string{"/"}
	for visited := 0; len(folders) > 0; visited++ {
		if visited >= 200 {
			return nil, errors.New("Discovery exceeds 200 folders; use a Jenkins folder base URL")
		}
		path := folders[0]
		folders = folders[1:]
		var out struct {
			Jobs []struct {
				Name     string `json:"name"`
				FullName string `json:"fullName"`
				Class    string `json:"_class"`
			} `json:"jobs"`
		}
		if err := j.api.Get(ctx, path+"api/json", url.Values{"tree": {"jobs[name,fullName,_class]"}}, &out); err != nil {
			return nil, err
		}
		for _, job := range out.Jobs {
			p := path + "job/" + url.PathEscape(job.Name) + "/"
			if strings.Contains(job.Class, "Folder") || strings.Contains(job.Class, "MultiBranchProject") {
				folders = append(folders, p)
				continue
			}
			name := job.FullName
			if name == "" {
				name = job.Name
			}
			targets = append(targets, platform.Target{ID: p, Name: name})
			if len(targets) > 2000 {
				return nil, errors.New("Discovery exceeds 2000 jobs; use a narrower folder")
			}
		}
	}
	return targets, nil
}

func jobPath(path string) (string, error) {
	if !strings.HasPrefix(path, "/job/") || strings.ContainsAny(path, "?#\\") || strings.Contains(path, "..") {
		return "", errors.New("Invalid Jenkins job path")
	}
	return strings.TrimRight(path, "/") + "/", nil
}

type jenkinsBuild struct {
	Number    int64  `json:"number"`
	Timestamp int64  `json:"timestamp"`
	Duration  int64  `json:"duration"`
	Building  bool   `json:"building"`
	Result    string `json:"result"`
	Actions   []struct {
		LastBuiltRevision *struct {
			SHA string `json:"SHA1"`
		} `json:"lastBuiltRevision"`
	} `json:"actions"`
}

func (j *Jenkins) normalize(job string, b jenkinsBuild) platform.Build {
	status := b.Result
	if b.Building {
		status = "RUNNING"
	}
	if status == "" {
		status = "UNKNOWN"
	}
	commit := ""
	for _, a := range b.Actions {
		if a.LastBuiltRevision != nil {
			commit = a.LastBuiltRevision.SHA
			break
		}
	}
	return platform.Build{ConnectionID: j.connectionID, Job: job, Number: b.Number, Started: b.Timestamp, Duration: b.Duration, Status: status, RawStatus: b.Result, Commit: commit, URL: j.api.Link(fmt.Sprintf("%s%d/", job, b.Number)), Observed: time.Now().UnixMilli()}
}

const buildFields = "number,timestamp,duration,building,result,actions[lastBuiltRevision[SHA1]]"

func (j *Jenkins) Builds(ctx context.Context, job string, offset int) ([]platform.Build, bool, error) {
	path, err := jobPath(job)
	if err != nil {
		return nil, false, err
	}
	if offset < 0 {
		return nil, false, errors.New("Invalid page offset")
	}
	var out struct {
		Builds []jenkinsBuild `json:"builds"`
	}
	q := url.Values{"tree": {fmt.Sprintf("builds[%s]{%d,%d}", buildFields, offset, offset+100)}}
	if err = j.api.Get(ctx, path+"api/json", q, &out); err != nil {
		return nil, false, err
	}
	builds := []platform.Build{}
	for _, b := range out.Builds {
		builds = append(builds, j.normalize(path, b))
	}
	return builds, len(out.Builds) == 100, nil
}

func (j *Jenkins) Build(ctx context.Context, job string, number int64) (platform.Build, error) {
	path, err := jobPath(job)
	if err != nil {
		return platform.Build{}, err
	}
	if number < 1 {
		return platform.Build{}, errors.New("Invalid build number")
	}
	var out jenkinsBuild
	err = j.api.Get(ctx, fmt.Sprintf("%s%d/api/json", path, number), url.Values{"tree": {buildFields}}, &out)
	return j.normalize(path, out), err
}

func (j *Jenkins) Queue(ctx context.Context) ([]platform.QueueItem, error) {
	var out struct {
		Items []struct {
			ID    int64  `json:"id"`
			Since int64  `json:"inQueueSince"`
			Why   string `json:"why"`
			Task  struct {
				Name string `json:"name"`
				URL  string `json:"url"`
			} `json:"task"`
		} `json:"items"`
	}
	err := j.api.Get(ctx, "/queue/api/json", url.Values{"tree": {"items[id,inQueueSince,why,task[name,url]]"}}, &out)
	items := []platform.QueueItem{}
	for _, i := range out.Items {
		// Queue URLs can be configured with a different Jenkins root URL.
		u, e := url.Parse(i.Task.URL)
		if e != nil {
			continue
		}
		basePath := strings.TrimRight(j.api.BasePath(), "/")
		job := strings.TrimPrefix(u.EscapedPath(), basePath)
		if _, e = jobPath(job); e != nil {
			continue
		}
		items = append(items, platform.QueueItem{ID: i.ID, Job: job, Since: i.Since, Reason: i.Why, URL: j.api.Link(job)})
	}
	return items, err
}

// Definition registers this adapter and its supported capabilities.
func Definition() platform.Definition {
	return platform.Definition{
		Info: platform.ProviderInfo{Kind: "jenkins", Name: "Jenkins", Category: "ci", Capabilities: []string{"builds", "queue", "history"}, DefaultAuth: "basic", AuthMethods: []string{"basic", "bearer", "none"}, PollSeconds: 15},
		Create: func(c platform.Connection, secret string) (platform.Provider, error) {
			client, err := httpapi.New(c, secret)
			if err != nil {
				return nil, err
			}
			return &Jenkins{api: client, connectionID: c.ID}, nil
		},
	}
}
