// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package store

// Stores is the top-level container for all storage backends.
// Each field holds an interface that is implemented by the pg/ package.
type Stores struct {
	DB               *DB
	Sessions         SessionStore
	Memory           MemoryStore
	Cron             CronStore
	Skills           SkillStore
	Agents           AgentStore
	Providers        ProviderStore
	Tracing          TracingStore
	MCP              MCPServerStore
	ChannelInstances ChannelInstanceStore
	ConfigSecrets    ConfigSecretsStore
	AgentLinks       AgentLinkStore
	Teams            TeamStore
	BuiltinTools     BuiltinToolStore
	PendingMessages  PendingMessageStore
	KnowledgeGraph   KnowledgeGraphStore
	Contacts         ContactStore
	Activity         ActivityStore
	Snapshots        SnapshotStore
	APIKeys          APIKeyStore
	Heartbeats       HeartbeatStore
	ConfigPermissions     ConfigPermissionStore
	Tenants               TenantStore
	BuiltinToolTenantCfgs BuiltinToolTenantConfigStore
	SkillTenantCfgs       SkillTenantConfigStore
	SystemConfigs         SystemConfigStore
	SubagentTasks         SubagentTaskStore
}
