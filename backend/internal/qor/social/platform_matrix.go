package social

// PlatformRelay defines which relay providers support a given platform.
type PlatformRelay struct {
	Relays   []string
	Warnings map[string]string
}

// PlatformRelayMatrix maps each platform to its supported relays and known issues.
var PlatformRelayMatrix = map[Platform]PlatformRelay{
	PlatformTwitter:     {Relays: []string{"outstand", "postforme", "buffer", "direct"}, Warnings: map[string]string{"outstand": "X/Twitter token refresh may be unreliable"}},
	PlatformInstagram:   {Relays: []string{"outstand", "postforme", "buffer", "direct"}, Warnings: map[string]string{"*": "Business/Creator account required"}},
	PlatformFacebook:    {Relays: []string{"outstand", "postforme", "buffer", "direct"}, Warnings: nil},
	PlatformTikTok:      {Relays: []string{"outstand", "postforme", "buffer", "direct"}, Warnings: map[string]string{"postforme": "Consumer and Business TikTok are separate"}},
	PlatformLinkedIn:    {Relays: []string{"outstand", "postforme", "buffer", "direct"}, Warnings: map[string]string{"postforme": "Personal page analytics unavailable"}},
	PlatformThreads:     {Relays: []string{"outstand", "postforme", "buffer", "direct"}, Warnings: nil},
	PlatformBluesky:     {Relays: []string{"outstand", "postforme", "buffer", "direct"}, Warnings: map[string]string{"postforme": "Uses handle + app password, not OAuth popup"}},
	PlatformYouTube:     {Relays: []string{"outstand", "postforme", "buffer", "direct"}, Warnings: nil},
	PlatformPinterest:   {Relays: []string{"outstand", "postforme", "buffer", "direct"}, Warnings: map[string]string{"outstand": "Requires board_id selection"}},
	PlatformGoogleMyBiz: {Relays: []string{"outstand", "buffer", "direct"}, Warnings: nil},
	PlatformMastodon:    {Relays: []string{"buffer", "direct"}, Warnings: nil},
	PlatformReddit:      {Relays: []string{"direct"}, Warnings: nil},
	PlatformDiscord:     {Relays: []string{"direct"}, Warnings: nil},
	PlatformSlack:       {Relays: []string{"direct"}, Warnings: nil},
	PlatformDevTo:       {Relays: []string{"direct"}, Warnings: nil},
	PlatformMedium:      {Relays: []string{"direct"}, Warnings: nil},
	PlatformWordPress:   {Relays: []string{"direct"}, Warnings: nil},
	PlatformNostr:       {Relays: []string{"direct"}, Warnings: nil},
	PlatformTelegramBot: {Relays: []string{"direct"}, Warnings: nil},
}

// RelayProviderInfo describes a relay provider for the frontend.
type RelayProviderInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Pricing     string `json:"pricing"`
	DocsURL     string `json:"docs_url"`
	KeyPrefix   string `json:"key_prefix"`
	KeyHint     string `json:"key_hint"`
}

// RelayProviderRegistry lists all supported relay providers.
var RelayProviderRegistry = []RelayProviderInfo{
	{ID: "outstand", Name: "Outstand", Description: "Unified social API — handles OAuth, token refresh, rate limits", Pricing: "$5/mo (1000 posts)", DocsURL: "https://www.outstand.so/docs/getting-started", KeyPrefix: "sk_", KeyHint: "Get key from Outstand dashboard → Settings → API Keys"},
	{ID: "postforme", Name: "PostForMe", Description: "Social posting API with Go SDK — white-label OAuth flows", Pricing: "$10/mo (1000 posts)", DocsURL: "https://api.postforme.dev/docs", KeyPrefix: "", KeyHint: "Get key from app.postforme.dev → API Keys"},
	{ID: "buffer", Name: "Buffer", Description: "Schedule and publish via Buffer — connect accounts in Buffer dashboard first", Pricing: "Free (3 channels) or $5/channel/mo", DocsURL: "https://developers.buffer.com", KeyPrefix: "", KeyHint: "Get token from publish.buffer.com → Settings → API"},
	{ID: "pipedream", Name: "Pipedream", Description: "Work tools relay — Gmail, Calendar, Slack, Notion (not social)", Pricing: "Free (100 actions/mo)", DocsURL: "https://pipedream.com/docs", KeyPrefix: "pd_", KeyHint: "Get key from pipedream.com → Settings → API Keys"},
}
