package provider

import (
	"net/http"

	"nofx/mcp"
)

const (
	DefaultNineRouterBaseURL = "http://localhost:20128/v1"
	DefaultNineRouterModel   = "FRIDA"
)

func init() {
	mcp.RegisterProvider(mcp.ProviderNineRouter, func(opts ...mcp.ClientOption) mcp.AIClient {
		return NewNineRouterClientWithOptions(opts...)
	})
}

type NineRouterClient struct {
	*mcp.Client
}

func (c *NineRouterClient) BaseClient() *mcp.Client { return c.Client }

// NewNineRouterClient creates NineRouter client
func NewNineRouterClient() mcp.AIClient {
	return NewNineRouterClientWithOptions()
}

// NewNineRouterClientWithOptions creates NineRouter client (supports options pattern)
func NewNineRouterClientWithOptions(opts ...mcp.ClientOption) mcp.AIClient {
	ninerouterOpts := []mcp.ClientOption{
		mcp.WithProvider(mcp.ProviderNineRouter),
		mcp.WithModel(DefaultNineRouterModel),
		mcp.WithBaseURL(DefaultNineRouterBaseURL),
	}

	allOpts := append(ninerouterOpts, opts...)
	baseClient := mcp.NewClient(allOpts...).(*mcp.Client)

	ninerouterClient := &NineRouterClient{
		Client: baseClient,
	}

	baseClient.Hooks = ninerouterClient
	return ninerouterClient
}

func (c *NineRouterClient) SetAPIKey(apiKey string, customURL string, customModel string) {
	c.APIKey = apiKey

	if len(apiKey) > 8 {
		c.Log.Infof("🔧 [MCP] NineRouter API Key set")
	}
	if customURL != "" {
		c.BaseURL = customURL
		c.Log.Infof("🔧 [MCP] NineRouter using custom BaseURL: %s", customURL)
	} else {
		c.Log.Infof("🔧 [MCP] NineRouter using default BaseURL: %s", c.BaseURL)
	}
	if customModel != "" {
		c.Model = customModel
		c.Log.Infof("🔧 [MCP] NineRouter using custom Model: %s", customModel)
	} else {
		c.Log.Infof("🔧 [MCP] NineRouter using default Model: %s", c.Model)
	}
}

// NineRouter uses standard Bearer auth (OpenAI compatible)
func (c *NineRouterClient) SetAuthHeader(reqHeaders http.Header) {
	c.Client.SetAuthHeader(reqHeaders)
}
