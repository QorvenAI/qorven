// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sagemakerruntime"
)

// TypeSageMaker is defined in types.go.

// SageMakerProvider calls an Amazon SageMaker real-time inference endpoint.
// The endpoint is expected to expose an OpenAI-compatible /v1/chat/completions
// schema (common when deploying vLLM or TGI containers on SageMaker).
type SageMakerProvider struct {
	client       *sagemakerruntime.Client
	endpointName string // SageMaker endpoint name (stored in cfg.APIBase or settings)
	providerName string
}

// NewSageMakerProvider creates a SageMakerProvider.
// cfg.APIBase holds the endpoint name; region comes from settings.region (default us-east-1).
// Static credentials are used when cfg.AWSAccessKey is set; otherwise SDK credential chain.
func NewSageMakerProvider(cfg ProviderConfig) (*SageMakerProvider, error) {
	region := "us-east-1"
	endpointName := cfg.APIBase

	if cfg.Settings != nil {
		var s map[string]string
		if json.Unmarshal(cfg.Settings, &s) == nil {
			if r := s["region"]; r != "" {
				region = r
			}
			if e := s["endpoint_name"]; e != "" {
				endpointName = e
			}
		}
	}
	if endpointName == "" {
		return nil, fmt.Errorf("sagemaker: endpoint_name required (set api_base or settings.endpoint_name)")
	}

	var opts []func(*awsconfig.LoadOptions) error
	opts = append(opts, awsconfig.WithRegion(region))
	if cfg.AWSAccessKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AWSAccessKey, cfg.AWSSecretKey, cfg.AWSSessionToken),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("sagemaker: aws config: %w", err)
	}

	return &SageMakerProvider{
		client:       sagemakerruntime.NewFromConfig(awsCfg),
		endpointName: endpointName,
		providerName: cfg.Name,
	}, nil
}

func (p *SageMakerProvider) Name() string { return p.providerName }

// DefaultModel returns the endpoint name as the model identifier.
// SageMaker has no /v1/models discovery endpoint — the endpoint name IS the model.
func (p *SageMakerProvider) DefaultModel() string { return p.endpointName }

func (p *SageMakerProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	bodyMap := map[string]any{
		"model":    req.Model,
		"messages": req.Messages,
		"stream":   false,
	}
	if t, ok := req.Options["temperature"]; ok {
		bodyMap["temperature"] = t
	}
	body, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, err
	}

	out, err := p.client.InvokeEndpoint(ctx, &sagemakerruntime.InvokeEndpointInput{
		EndpointName: aws.String(p.endpointName),
		ContentType:  aws.String("application/json"),
		Accept:       aws.String("application/json"),
		Body:         body,
	})
	if err != nil {
		return nil, fmt.Errorf("sagemaker invoke: %w", err)
	}

	raw, err := io.ReadAll(bytes.NewReader(out.Body))
	if err != nil {
		return nil, fmt.Errorf("sagemaker read body: %w", err)
	}

	return parseSageMakerResponse(raw)
}

func (p *SageMakerProvider) ChatStream(ctx context.Context, req ChatRequest, onChunk func(StreamChunk)) (*ChatResponse, error) {
	resp, err := p.Chat(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.Content != "" {
		onChunk(StreamChunk{Content: resp.Content})
	}
	return resp, nil
}

func parseSageMakerResponse(raw []byte) (*ChatResponse, error) {
	// OpenAI-compat response
	var oai struct {
		Choices []struct {
			Message struct {
				Content   string     `json:"content"`
				ToolCalls []ToolCall `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &oai); err != nil {
		return nil, fmt.Errorf("sagemaker parse: %w", err)
	}
	if len(oai.Choices) == 0 {
		return nil, fmt.Errorf("sagemaker: empty choices in response")
	}

	resp := &ChatResponse{
		Content:      oai.Choices[0].Message.Content,
		FinishReason: oai.Choices[0].FinishReason,
		ToolCalls:    oai.Choices[0].Message.ToolCalls,
		Usage: &Usage{
			PromptTokens:     oai.Usage.PromptTokens,
			CompletionTokens: oai.Usage.CompletionTokens,
			TotalTokens:      oai.Usage.TotalTokens,
		},
	}

	// Trim stop sequences that some frameworks echo back
	resp.Content = strings.TrimRight(resp.Content, "\x00")
	return resp, nil
}
