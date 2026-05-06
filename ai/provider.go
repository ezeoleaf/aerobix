package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type AIProvider interface {
	Chat(prompt string) (string, error)
}

type OpenAIProvider struct {
	APIKey  string
	BaseURL string
	Model   string
	HTTP    *http.Client
}

func (p OpenAIProvider) Chat(prompt string) (string, error) {
	if strings.TrimSpace(p.APIKey) == "" {
		return "", fmt.Errorf("openai api key is empty")
	}
	base := strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	if base == "" {
		base = "https://api.openai.com"
	}
	model := strings.TrimSpace(p.Model)
	if model == "" {
		model = "gpt-5.4-mini"
	}
	reqBody := map[string]any{
		"model": model,
		"input": prompt,
	}
	raw, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest(http.MethodPost, base+"/v1/responses", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	cl := p.HTTP
	if cl == nil {
		cl = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := cl.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("openai request failed: %s", resp.Status)
	}
	var out struct {
		OutputText string `json:"output_text"`
	}
	if err := json.Unmarshal(body, &out); err == nil && strings.TrimSpace(out.OutputText) != "" {
		return strings.TrimSpace(out.OutputText), nil
	}
	// fallback decode shape
	var alt struct {
		Output []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &alt); err == nil {
		for _, o := range alt.Output {
			for _, c := range o.Content {
				if strings.TrimSpace(c.Text) != "" {
					return strings.TrimSpace(c.Text), nil
				}
			}
		}
	}
	return "", fmt.Errorf("openai response had no text")
}

type LocalOllamaProvider struct {
	BaseURL string
	Model   string
	HTTP    *http.Client
}

func (p LocalOllamaProvider) Chat(prompt string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	if base == "" {
		base = "http://localhost:11434"
	}
	model := strings.TrimSpace(p.Model)
	if model == "" {
		model = "llama3.1"
	}
	reqBody := map[string]any{
		"model":  model,
		"stream": false,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	raw, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest(http.MethodPost, base+"/api/chat", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	cl := p.HTTP
	if cl == nil {
		cl = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := cl.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("ollama request failed: %s", resp.Status)
	}
	var out struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.Message.Content) != "" {
		return strings.TrimSpace(out.Message.Content), nil
	}
	if strings.TrimSpace(out.Response) != "" {
		return strings.TrimSpace(out.Response), nil
	}
	return "", fmt.Errorf("ollama response had no text")
}

type AnthropicProvider struct {
	APIKey  string
	BaseURL string
	Model   string
	HTTP    *http.Client
}

func (p AnthropicProvider) Chat(prompt string) (string, error) {
	if strings.TrimSpace(p.APIKey) == "" {
		return "", fmt.Errorf("anthropic api key is empty")
	}
	base := strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	if base == "" {
		base = "https://api.anthropic.com"
	}
	model := strings.TrimSpace(p.Model)
	if model == "" {
		model = "claude-sonnet-4.5"
	}
	reqBody := map[string]any{
		"model":      model,
		"max_tokens": 900,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	raw, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest(http.MethodPost, base+"/v1/messages", bytes.NewReader(raw))
	req.Header.Set("x-api-key", p.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	cl := p.HTTP
	if cl == nil {
		cl = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := cl.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("anthropic request failed: %s", resp.Status)
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	for _, c := range out.Content {
		if strings.TrimSpace(c.Text) != "" {
			return strings.TrimSpace(c.Text), nil
		}
	}
	return "", fmt.Errorf("anthropic response had no text")
}
