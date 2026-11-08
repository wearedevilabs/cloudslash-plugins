// SPDX-License-Identifier: Apache-2.0
// Copyright 2024-2026 CloudSlash Contributors

package aws

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"gopkg.in/ini.v1"
)

// Client wraps the AWS SDK client.
type Client struct {
	Config aws.Config
	STS    *sts.Client
	Logger *slog.Logger
}

// NewClient initializes a new AWS SDK client.
func NewClient(ctx context.Context, region, profile string, verbose bool, logger *slog.Logger) (*Client, error) {
	if logger == nil {
		logger = slog.Default()
	}
	
	// Build load options
	var opts []func(*config.LoadOptions) error
	
	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}
	if profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(profile))
	}

	// Check for environment variable overrides.
	if endpoint := os.Getenv("AWS_ENDPOINT_URL"); endpoint != "" {
		opts = append(opts, config.WithBaseEndpoint(endpoint))
	}

	// App signature.
	const signature = "CS-v1-7f8a9d-AGPL"

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %v", err)
	}

	// Inject custom User-Agent.
	cfg.APIOptions = append(cfg.APIOptions, func(stack *middleware.Stack) error {
		return stack.Build.Add(middleware.BuildMiddlewareFunc("UserAgentTrap", func(ctx context.Context, input middleware.BuildInput, next middleware.BuildHandler) (
			middleware.BuildOutput, middleware.Metadata, error,
		) {
			req, ok := input.Request.(*smithyhttp.Request)
			if ok {
				currentUA := req.Header.Get("User-Agent")
				if currentUA == "" {
					currentUA = "CloudSlash/v1.1"
				}
				req.Header.Set("User-Agent", fmt.Sprintf("%s (%s)", currentUA, signature))
			}
			return next.HandleBuild(ctx, input)
		}), middleware.After)
	})

	// Enable verbose logging if requested.
	if verbose {
		cfg.APIOptions = append(cfg.APIOptions, func(stack *middleware.Stack) error {
			return stack.Initialize.Add(middleware.InitializeMiddlewareFunc("MatrixLogger", func(ctx context.Context, input middleware.InitializeInput, next middleware.InitializeHandler) (
				middleware.InitializeOutput, middleware.Metadata, error,
			) {
				opName := middleware.GetOperationName(ctx)
				logger.Info("AWS SDK API Call", "operation", opName)
				return next.HandleInitialize(ctx, input)
			}), middleware.Before)
		})
	}

	return &Client{
		Config: cfg,
		STS:    sts.NewFromConfig(cfg),
		Logger: logger,
	}, nil
}

// VerifyIdentity checks credentials and returns the AWS Account ID.
func (c *Client) VerifyIdentity(ctx context.Context) (string, error) {
	input := &sts.GetCallerIdentityInput{}
	result, err := c.STS.GetCallerIdentity(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to get caller identity: %v", err)
	}
	return *result.Account, nil
}

// GetConfigForRegion returns a config copy for the specified region.
func (c *Client) GetConfigForRegion(region string) aws.Config {
	cfg := c.Config.Copy()
	cfg.Region = region
	return cfg
}

// ListProfiles retrieves available AWS CLI profiles using ini parsing.
func ListProfiles() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	profiles := make(map[string]bool)
	paths := []string{}
	
	// Determine configuration paths.
	if cfgPath := os.Getenv("AWS_CONFIG_FILE"); cfgPath != "" {
		paths = append(paths, cfgPath)
	} else {
		paths = append(paths, filepath.Join(home, ".aws", "config"))
	}

	if credPath := os.Getenv("AWS_SHARED_CREDENTIALS_FILE"); credPath != "" {
		paths = append(paths, credPath)
	} else {
		paths = append(paths, filepath.Join(home, ".aws", "credentials"))
	}

	foundFile := false
	for _, path := range paths {
		// Use ini.Load to parse the file robustly
		cfg, err := ini.Load(path)
		if err != nil {
			continue // Skip if file doesn't exist or is unreadable
		}
		foundFile = true

		for _, section := range cfg.Sections() {
			name := section.Name()
			if name == "DEFAULT" {
				continue
			}
			
			// Handle [profile name] vs [name]
			// config file uses [profile my-profile], credentials file uses [my-profile]
			if len(name) > 8 && name[:8] == "profile " {
				profiles[name[8:]] = true
			} else {
				profiles[name] = true
			}
		}
	}

	var list []string
	for p := range profiles {
		list = append(list, p)
	}
	sort.Strings(list)

	if len(list) == 0 {
		// If we found files but no profiles, maybe there's a default implicit one?
		// Check for environment variables or web identity as a fallback
		if os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE") != "" || os.Getenv("AWS_ACCESS_KEY_ID") != "" {
			return []string{"default"}, nil
		}
		
		if !foundFile {
			return nil, fmt.Errorf("no AWS config or credentials files found at standard locations")
		}
		
		return nil, fmt.Errorf("no profiles found in configuration files")
	}

	return list, nil
}
