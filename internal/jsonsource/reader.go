package jsonsource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"idp-dashboard/internal/platform"
)

const maxInput = 8 << 20
const maxOutput = 1 << 20

type Reader struct{}

// Read fetches public JSON only; it does not send credentials or execute commands.
func (Reader) Read(ctx context.Context, request platform.JSONRequest) (platform.JSONResult, error) {
	if request.Source != "http" {
		return platform.JSONResult{}, errors.New("현재 인증 없는 HTTP API 조회만 지원합니다")
	}
	if len(request.Filter) > 16384 || strings.TrimSpace(request.Filter) == "" {
		return platform.JSONResult{}, errors.New("jq 식을 입력하세요 (최대 16KiB)")
	}
	query, err := gojq.Parse(request.Filter)
	if err != nil {
		return platform.JSONResult{}, fmt.Errorf("jq: %w", err)
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return platform.JSONResult{}, fmt.Errorf("jq: %w", err)
	}
	raw, err := fetch(ctx, request.URL)
	if err != nil {
		return platform.JSONResult{}, err
	}
	return evaluate(ctx, code, raw)
}

func fetch(ctx context.Context, address string) ([]byte, error) {
	u, err := url.Parse(address)
	if err != nil || len(address) > 8192 || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" {
		return nil, errors.New("계정 정보가 없는 유효한 HTTP(S) URL을 입력하세요")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, errors.New("API 요청을 만들 수 없습니다")
	}
	req.Header.Set("Accept", "application/json")
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return nil, errors.New("API 요청 실패: 주소, 인증서 또는 제한 시간을 확인하세요")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("API HTTP %d (리다이렉트는 따라가지 않습니다)", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxInput+1))
	if err != nil {
		return nil, errors.New("API 응답을 읽지 못했습니다")
	}
	if len(raw) > maxInput {
		return nil, errors.New("JSON 응답은 최대 8MiB까지 지원합니다")
	}
	return raw, nil
}

func evaluate(ctx context.Context, code *gojq.Code, raw []byte) (platform.JSONResult, error) {
	decoder := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return platform.JSONResult{}, fmt.Errorf("JSON 응답 해석 실패: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return platform.JSONResult{}, errors.New("응답은 하나의 JSON 값이어야 합니다")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	iter := code.RunWithContext(ctx, value)
	outputs := make([]json.RawMessage, 0)
	size := 0
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			return platform.JSONResult{}, fmt.Errorf("jq 실행 실패: %w", err)
		}
		data, err := gojq.Marshal(v)
		if err != nil {
			return platform.JSONResult{}, err
		}
		size += len(data)
		if size > maxOutput || len(outputs) >= 200 {
			return platform.JSONResult{}, errors.New("jq 결과 제한 초과: 1MiB / 200개. 조회식을 좁혀주세요")
		}
		outputs = append(outputs, data)
	}
	data, err := json.Marshal(outputs)
	if err != nil {
		return platform.JSONResult{}, err
	}
	if len(outputs) == 1 {
		data = outputs[0]
	}
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, data, "", "  "); err != nil {
		return platform.JSONResult{}, err
	}
	if formatted.Len() > maxOutput {
		return platform.JSONResult{}, errors.New("jq 표시 결과가 1MiB를 초과합니다")
	}
	result := platform.JSONResult{JSON: formatted.String(), Text: formatted.String(), Count: len(outputs), ObservedAt: time.Now().UTC()}
	result.Scalar = len(outputs) == 1 && len(data) > 0 && data[0] != '[' && data[0] != '{'
	if result.Scalar && data[0] == '"' {
		_ = json.Unmarshal(data, &result.Text)
	}
	return result, nil
}
