// Package remote talks to a relay's model-listing endpoint so the menu can be
// refreshed without hand-editing JSON.
package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Shippuurl/a2v/internal/config"
)

var nonChat = regexp.MustCompile(`(?i)image|embed|tts|whisper|audio|rerank|moderation|dall-e|sora|video|ocr|vision-ocr`)

type listing struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// client is shared so a sync uses one connection set.
var client = &http.Client{Timeout: 20 * time.Second}

// Sync fetches the model list for a proxy.
func Sync(ctx context.Context, p *config.Proxy) ([]config.Model, error) {
	base := strings.TrimRight(p.URL, "/")
	candidates := []struct {
		url  string
		auth string
	}{
		{base + "/v1/models", "x-api-key"},
		{base + "/v1/models", "bearer"},
		{base + "/models", "bearer"},
	}

	var lastErr error
	for _, c := range candidates {
		ids, err := fetch(ctx, c.url, c.auth, p.Key)
		if err != nil {
			lastErr = err
			continue
		}
		if len(ids) == 0 {
			lastErr = fmt.Errorf("%s 没有返回模型", c.url)
			continue
		}
		return Build(ids), nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("没有可用的模型列表端点")
	}
	return nil, lastErr
}

func fetch(ctx context.Context, url, auth, key string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	switch auth {
	case "x-api-key":
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	default:
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s -> HTTP %d", url, resp.StatusCode)
	}
	var l listing
	if err := json.NewDecoder(resp.Body).Decode(&l); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var ids []string
	for _, m := range l.Data {
		if m.ID == "" || seen[m.ID] || nonChat.MatchString(m.ID) {
			continue
		}
		seen[m.ID] = true
		ids = append(ids, m.ID)
	}
	return ids, nil
}

// Build converts raw ids into catalogue entries with friendly names.
func Build(ids []string) []config.Model {
	out := make([]config.Model, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, config.Model{ID: id, Name: NameOf(id), Desc: DescOf(id)})
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := rank(out[i].ID), rank(out[j].ID)
		if ri != rj {
			return ri < rj
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func rank(id string) int {
	l := strings.ToLower(id)
	switch {
	case strings.Contains(l, "claude"):
		return 0
	case strings.Contains(l, "gpt"), regexp.MustCompile(`(^|[/-])o\d`).MatchString(l):
		return 1
	case strings.Contains(l, "gemini"):
		return 2
	case strings.Contains(l, "deepseek"):
		return 3
	case strings.Contains(l, "qwen"), strings.Contains(l, "glm"), strings.Contains(l, "kimi"):
		return 4
	default:
		return 9
	}
}

var knownNames = []struct {
	re   *regexp.Regexp
	name string
}{
	{regexp.MustCompile(`claude-opus-4-8`), "Opus 4.8"},
	{regexp.MustCompile(`claude-opus-4-6`), "Opus 4.6"},
	{regexp.MustCompile(`claude-opus-5`), "Opus 5"},
	{regexp.MustCompile(`claude-opus-4`), "Opus 4"},
	{regexp.MustCompile(`claude-sonnet-5`), "Sonnet 5"},
	{regexp.MustCompile(`claude-sonnet-4-6`), "Sonnet 4.6"},
	{regexp.MustCompile(`claude-sonnet-4-5`), "Sonnet 4.5"},
	{regexp.MustCompile(`claude-haiku-4-5`), "Haiku 4.5"},
	{regexp.MustCompile(`gpt-5\.6-sol`), "GPT 5.6 Sol"},
	{regexp.MustCompile(`gpt-5\.6-luna`), "GPT 5.6 Luna"},
	{regexp.MustCompile(`gpt-5\.6-terra`), "GPT 5.6 Terra"},
	{regexp.MustCompile(`deepseek-reasoner`), "DeepSeek R1"},
	{regexp.MustCompile(`deepseek-chat`), "DeepSeek V3"},
}

var knownDescs = []struct {
	re   *regexp.Regexp
	desc string
}{
	{regexp.MustCompile(`opus-4-8|opus-5`), "最强"},
	{regexp.MustCompile(`opus`), "Opus 平衡"},
	{regexp.MustCompile(`sonnet-5`), "最新 Sonnet"},
	{regexp.MustCompile(`sonnet-4-6`), "日常主力"},
	{regexp.MustCompile(`sonnet`), "Sonnet"},
	{regexp.MustCompile(`haiku`), "极速响应"},
	{regexp.MustCompile(`gpt-5\.6-sol`), "代码"},
	{regexp.MustCompile(`gpt-5\.6-luna`), "轻量"},
	{regexp.MustCompile(`gpt-5\.6-terra`), "重量"},
	{regexp.MustCompile(`(^|[/-])o[0-9]`), "推理"},
	{regexp.MustCompile(`gemini`), "Google"},
	{regexp.MustCompile(`deepseek-chat`), "DeepSeek"},
	{regexp.MustCompile(`deepseek`), "DS 推理"},
}

// NameOf produces a short display name for a model id.
func NameOf(id string) string {
	base := id
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	for _, k := range knownNames {
		if k.re.MatchString(base) {
			return k.name
		}
	}
	words := strings.FieldsFunc(base, func(r rune) bool { return r == '-' || r == '_' })
	for i, w := range words {
		if w == "" {
			continue
		}
		if fixed, ok := acronyms[strings.ToLower(w)]; ok {
			words[i] = fixed
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

var acronyms = map[string]string{
	"gpt": "GPT", "glm": "GLM", "ds": "DS", "ai": "AI", "api": "API",
	"llm": "LLM", "v": "V", "ui": "UI", "id": "ID", "hf": "HF",
	"deepseek": "DeepSeek", "claude": "Claude", "gemini": "Gemini",
}

// NaiveTitle is the plain "capitalise each dash-separated word" transform,
// without acronym fixes. The TUI uses it to spot catalogue names that are
// nothing but a re-cased id.
func NaiveTitle(id string) string {
	base := id
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	words := strings.FieldsFunc(base, func(r rune) bool { return r == '-' || r == '_' })
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// DescOf is the one-word hint shown next to a model.
func DescOf(id string) string {
	for _, k := range knownDescs {
		if k.re.MatchString(id) {
			return k.desc
		}
	}
	return ""
}

// Health pings the relay's model endpoint; used by `a2v doctor`.
func Health(ctx context.Context, p *config.Proxy) (int, error) {
	base := strings.TrimRight(p.URL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/models", nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+p.Key)
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}
