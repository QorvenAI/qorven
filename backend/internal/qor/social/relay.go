package social

import "context"

// PublishOpts contains optional parameters for publishing through a relay.
type PublishOpts struct {
	ScheduledAt string
	MediaURLs   []string
	Metadata    map[string]string
}

// RelayPublisher can publish content through a relay provider.
type RelayPublisher interface {
	Publish(ctx context.Context, accountID string, content string, opts PublishOpts) (*PostResult, error)
	GetAccountHealth(ctx context.Context, accountID string) (bool, error)
}

// RelayAccount represents a social account on a relay provider.
type RelayAccount struct {
	ID          string `json:"id"`
	Platform    string `json:"platform"`
	AccountName string `json:"account_name"`
	Username    string `json:"username"`
	Healthy     bool   `json:"healthy"`
}

// RelayConnector handles OAuth connection flows through a relay provider.
type RelayConnector interface {
	GetAuthURL(ctx context.Context, platform, tenantID, redirectURL string) (string, error)
	FinalizeConnection(ctx context.Context, sessionToken string) (*RelayAccount, error)
	ListAccounts(ctx context.Context, tenantID string) ([]RelayAccount, error)
	DeleteAccount(ctx context.Context, accountID string) error
	TestConnection(ctx context.Context) error
}

// RelayClient combines publishing and connection management.
type RelayClient interface {
	RelayPublisher
	RelayConnector
}
