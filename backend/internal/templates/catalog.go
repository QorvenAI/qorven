// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package templates

func Catalog() []WorkspaceTemplate {
	return []WorkspaceTemplate{
		{ID: "crm", Name: "CRM & Sales", Description: "AI-powered sales pipeline with prospecting, outreach, and deal tracking", Icon: "📊", Category: "business",
			Agents: []AgentSpec{
				{Key: "sales-director", Name: "Sales Director", Role: "leader", SystemPrompt: "You are a Sales Director. Manage the pipeline, assign leads, track deals, and generate weekly forecasts."},
				{Key: "prospector", Name: "Prospector", Role: "specialist", ReportsTo: "sales-director", SystemPrompt: "You find and qualify leads via web research.", Tools: []string{"web_search", "web_fetch"}},
				{Key: "outreach", Name: "Outreach Agent", Role: "specialist", ReportsTo: "sales-director", SystemPrompt: "You draft personalized outreach emails and follow-ups.", Tools: []string{"email_send"}},
				{Key: "analyst", Name: "Sales Analyst", Role: "specialist", ReportsTo: "sales-director", SystemPrompt: "You analyze sales data and generate reports."},
			},
			Dashboard: DashboardSpec{Layout: "grid-2col", Blocks: []BlockSpec{{Type: "stat-row"}, {Type: "pipeline", Title: "Sales Pipeline"}, {Type: "data-table", Title: "Deals"}, {Type: "chart", Title: "Revenue"}, {Type: "contacts", Title: "Contacts"}, {Type: "feed", Title: "Activity"}}},
			Skills: []string{"email_templates", "lead_scoring"}, Connectors: []string{"gmail", "linkedin", "stripe"},
		},
		{ID: "social", Name: "Social Media", Description: "Multi-platform content creation, scheduling, and analytics", Icon: "📱", Category: "marketing",
			Agents: []AgentSpec{
				{Key: "content-director", Name: "Content Director", Role: "leader", SystemPrompt: "You plan content strategy across platforms."},
				{Key: "twitter-agent", Name: "Twitter/X Agent", Role: "specialist", ReportsTo: "content-director", SystemPrompt: "You write tweets, monitor mentions, engage with replies.", Tools: []string{"web_search"}},
				{Key: "linkedin-agent", Name: "LinkedIn Agent", Role: "specialist", ReportsTo: "content-director", SystemPrompt: "You write LinkedIn posts and grow professional network.", Tools: []string{"web_search"}},
				{Key: "analytics-agent", Name: "Analytics Agent", Role: "specialist", ReportsTo: "content-director", SystemPrompt: "You track cross-platform metrics and generate reports."},
			},
			Dashboard: DashboardSpec{Layout: "grid-2col", Blocks: []BlockSpec{{Type: "stat-row"}, {Type: "calendar", Title: "Content Calendar"}, {Type: "kanban", Title: "Post Queue"}, {Type: "chart", Title: "Engagement"}, {Type: "feed", Title: "Posts"}}},
			Skills: []string{"content_writing", "hashtag_research"}, Connectors: []string{"twitter", "linkedin", "instagram"},
		},
		{ID: "research", Name: "Research & Analysis", Description: "Deep research with cited sources and report generation", Icon: "🔬", Category: "knowledge",
			Agents: []AgentSpec{
				{Key: "research-lead", Name: "Research Lead", Role: "leader", SystemPrompt: "You coordinate research projects and synthesize findings."},
				{Key: "web-researcher", Name: "Web Researcher", Role: "specialist", ReportsTo: "research-lead", SystemPrompt: "You search the web and provide cited sources.", Tools: []string{"web_search", "web_fetch"}},
				{Key: "data-analyst", Name: "Data Analyst", Role: "specialist", ReportsTo: "research-lead", SystemPrompt: "You analyze data and create visualizations.", Tools: []string{"exec"}},
			},
			Dashboard: DashboardSpec{Layout: "sidebar-right", Blocks: []BlockSpec{{Type: "feed", Title: "Activity"}, {Type: "data-table", Title: "Sources"}, {Type: "timeline", Title: "Timeline"}}},
			Skills: []string{"academic_search", "citation_format"}, Connectors: []string{"google_scholar"},
		},
		{ID: "trading", Name: "Trading & Crypto", Description: "Portfolio tracking, market analysis, and alerts", Icon: "📈", Category: "finance",
			Agents: []AgentSpec{
				{Key: "portfolio-mgr", Name: "Portfolio Manager", Role: "leader", SystemPrompt: "You manage portfolio allocation and risk."},
				{Key: "market-analyst", Name: "Market Analyst", Role: "specialist", ReportsTo: "portfolio-mgr", SystemPrompt: "You analyze market trends and indicators.", Tools: []string{"web_search", "web_fetch"}},
				{Key: "alert-agent", Name: "Alert Agent", Role: "specialist", ReportsTo: "portfolio-mgr", SystemPrompt: "You monitor prices and send alerts.", Tools: []string{"web_search"}},
			},
			Dashboard: DashboardSpec{Layout: "grid-2col", Blocks: []BlockSpec{{Type: "stat-row"}, {Type: "chart", Title: "Performance"}, {Type: "data-table", Title: "Holdings"}, {Type: "feed", Title: "News"}}},
			Skills: []string{"market_data"}, Connectors: []string{"coingecko"},
		},
		{ID: "invoicing", Name: "Invoicing & Finance", Description: "Invoice generation, payment tracking, and reports", Icon: "💰", Category: "business",
			Agents: []AgentSpec{
				{Key: "finance-mgr", Name: "Finance Manager", Role: "leader", SystemPrompt: "You oversee invoicing and financial reporting."},
				{Key: "invoice-agent", Name: "Invoice Agent", Role: "specialist", ReportsTo: "finance-mgr", SystemPrompt: "You generate invoices and track payments.", Tools: []string{"email_send"}},
			},
			Dashboard: DashboardSpec{Layout: "grid-2col", Blocks: []BlockSpec{{Type: "stat-row"}, {Type: "data-table", Title: "Invoices"}, {Type: "chart", Title: "Revenue"}, {Type: "pipeline", Title: "Payments"}}},
			Skills: []string{"invoice_gen"}, Connectors: []string{"stripe"},
		},
		{ID: "devops", Name: "DevOps & Engineering", Description: "CI/CD monitoring, incident response, infrastructure", Icon: "🛠️", Category: "engineering",
			Agents: []AgentSpec{
				{Key: "devops-lead", Name: "DevOps Lead", Role: "leader", SystemPrompt: "You coordinate infrastructure and incident response."},
				{Key: "monitor-agent", Name: "Monitor Agent", Role: "specialist", ReportsTo: "devops-lead", SystemPrompt: "You monitor services and alert on anomalies.", Tools: []string{"web_fetch", "exec"}},
			},
			Dashboard: DashboardSpec{Layout: "grid-2col", Blocks: []BlockSpec{{Type: "stat-row"}, {Type: "timeline", Title: "Deployments"}, {Type: "data-table", Title: "Services"}, {Type: "feed", Title: "Incidents"}}},
			Skills: []string{"docker_ops", "git_ops"}, Connectors: []string{"github"},
		},

		// ── Phase 2 Templates ───────────────────────────────────────────────────

		{ID: "support", Name: "Customer Support", Description: "Ticket triage, resolution tracking, and customer satisfaction", Icon: "🎧", Category: "business",
			Agents: []AgentSpec{
				{Key: "support-lead", Name: "Support Lead", Role: "leader", SystemPrompt: "You manage the support queue, prioritize tickets, and track resolution metrics."},
				{Key: "triage-agent", Name: "Triage Agent", Role: "specialist", ReportsTo: "support-lead", SystemPrompt: "You categorize incoming tickets by urgency and route them appropriately.", Tools: []string{"web_fetch"}},
				{Key: "resolver-agent", Name: "Resolver Agent", Role: "specialist", ReportsTo: "support-lead", SystemPrompt: "You research solutions and draft detailed responses to customer issues.", Tools: []string{"web_search", "web_fetch"}},
				{Key: "satisfaction-agent", Name: "CSAT Agent", Role: "specialist", ReportsTo: "support-lead", SystemPrompt: "You track customer satisfaction trends and identify improvement opportunities."},
			},
			Dashboard: DashboardSpec{Layout: "grid-2col", Blocks: []BlockSpec{{Type: "stat-row"}, {Type: "pipeline", Title: "Ticket Queue"}, {Type: "data-table", Title: "Open Tickets"}, {Type: "chart", Title: "Resolution Time"}, {Type: "feed", Title: "Recent Activity"}}},
			Skills: []string{"email_templates"}, Connectors: []string{"gmail", "slack", "zendesk"},
		},

		{ID: "hr", Name: "HR & People Ops", Description: "Recruiting, onboarding, performance tracking, and team management", Icon: "👥", Category: "business",
			Agents: []AgentSpec{
				{Key: "hr-director", Name: "HR Director", Role: "leader", SystemPrompt: "You oversee hiring, onboarding, and employee performance management."},
				{Key: "recruiter", Name: "Recruiter", Role: "specialist", ReportsTo: "hr-director", SystemPrompt: "You source candidates, review applications, and schedule interviews.", Tools: []string{"web_search", "email_send"}},
				{Key: "onboarding-agent", Name: "Onboarding Agent", Role: "specialist", ReportsTo: "hr-director", SystemPrompt: "You create onboarding plans and track new hire progress.", Tools: []string{"email_send"}},
				{Key: "performance-agent", Name: "Performance Agent", Role: "specialist", ReportsTo: "hr-director", SystemPrompt: "You track performance metrics and prepare review reports."},
			},
			Dashboard: DashboardSpec{Layout: "grid-2col", Blocks: []BlockSpec{{Type: "stat-row"}, {Type: "kanban", Title: "Hiring Pipeline"}, {Type: "data-table", Title: "Team Directory"}, {Type: "timeline", Title: "Onboarding"}, {Type: "feed", Title: "HR Activity"}}},
			Skills: []string{"email_templates"}, Connectors: []string{"gmail", "linkedin", "slack"},
		},

		{ID: "ecommerce", Name: "E-Commerce & Retail", Description: "Order management, inventory tracking, customer insights, and growth", Icon: "🛒", Category: "business",
			Agents: []AgentSpec{
				{Key: "store-manager", Name: "Store Manager", Role: "leader", SystemPrompt: "You oversee orders, inventory, and store performance metrics."},
				{Key: "inventory-agent", Name: "Inventory Agent", Role: "specialist", ReportsTo: "store-manager", SystemPrompt: "You monitor stock levels, predict shortages, and manage reorders.", Tools: []string{"web_fetch"}},
				{Key: "customer-agent", Name: "Customer Agent", Role: "specialist", ReportsTo: "store-manager", SystemPrompt: "You analyze customer behavior and generate personalized recommendations."},
				{Key: "growth-agent", Name: "Growth Agent", Role: "specialist", ReportsTo: "store-manager", SystemPrompt: "You identify growth opportunities through competitive analysis and trend research.", Tools: []string{"web_search"}},
			},
			Dashboard: DashboardSpec{Layout: "grid-2col", Blocks: []BlockSpec{{Type: "stat-row"}, {Type: "pipeline", Title: "Order Status"}, {Type: "data-table", Title: "Products"}, {Type: "chart", Title: "Sales Trend"}, {Type: "contacts", Title: "Customers"}, {Type: "feed", Title: "Recent Orders"}}},
			Skills: []string{"product_descriptions"}, Connectors: []string{"shopify", "stripe", "gmail"},
		},

		{ID: "legal", Name: "Legal & Compliance", Description: "Contract review, compliance monitoring, and document management", Icon: "⚖️", Category: "professional",
			Agents: []AgentSpec{
				{Key: "legal-lead", Name: "Legal Lead", Role: "leader", SystemPrompt: "You oversee contract review, compliance, and legal risk management."},
				{Key: "contract-reviewer", Name: "Contract Reviewer", Role: "specialist", ReportsTo: "legal-lead", SystemPrompt: "You review contracts for risk clauses, obligations, and non-standard terms.", Tools: []string{"web_fetch"}},
				{Key: "compliance-agent", Name: "Compliance Agent", Role: "specialist", ReportsTo: "legal-lead", SystemPrompt: "You monitor regulatory changes and ensure policy compliance.", Tools: []string{"web_search"}},
			},
			Dashboard: DashboardSpec{Layout: "sidebar-right", Blocks: []BlockSpec{{Type: "stat-row"}, {Type: "data-table", Title: "Contracts"}, {Type: "timeline", Title: "Deadlines"}, {Type: "feed", Title: "Compliance Updates"}}},
			Skills: []string{"document_analysis"}, Connectors: []string{"gmail", "google_drive"},
		},

		{ID: "content", Name: "Content Studio", Description: "Blog writing, SEO optimization, content calendar, and distribution", Icon: "✍️", Category: "marketing",
			Agents: []AgentSpec{
				{Key: "content-lead", Name: "Content Lead", Role: "leader", SystemPrompt: "You plan the content strategy, editorial calendar, and oversee all content production."},
				{Key: "writer-agent", Name: "Writer", Role: "specialist", ReportsTo: "content-lead", SystemPrompt: "You write high-quality blog posts, articles, and long-form content.", Tools: []string{"web_search"}},
				{Key: "seo-agent", Name: "SEO Agent", Role: "specialist", ReportsTo: "content-lead", SystemPrompt: "You optimize content for search engines and research target keywords.", Tools: []string{"web_search"}},
				{Key: "editor-agent", Name: "Editor", Role: "specialist", ReportsTo: "content-lead", SystemPrompt: "You proofread, edit for clarity, and ensure brand voice consistency."},
			},
			Dashboard: DashboardSpec{Layout: "grid-2col", Blocks: []BlockSpec{{Type: "stat-row"}, {Type: "calendar", Title: "Content Calendar"}, {Type: "kanban", Title: "Articles"}, {Type: "chart", Title: "Traffic"}, {Type: "feed", Title: "Published"}}},
			Skills: []string{"content_writing", "seo_analysis"}, Connectors: []string{"wordpress", "ghost", "twitter"},
		},

		{ID: "freelance", Name: "Freelance & Agency", Description: "Project tracking, client management, time billing, and proposals", Icon: "💼", Category: "professional",
			Agents: []AgentSpec{
				{Key: "project-manager", Name: "Project Manager", Role: "leader", SystemPrompt: "You track all client projects, deadlines, budgets, and deliverables."},
				{Key: "proposal-agent", Name: "Proposal Agent", Role: "specialist", ReportsTo: "project-manager", SystemPrompt: "You write compelling project proposals and scopes of work.", Tools: []string{"web_search"}},
				{Key: "billing-agent", Name: "Billing Agent", Role: "specialist", ReportsTo: "project-manager", SystemPrompt: "You track time, generate invoices, and monitor payment status.", Tools: []string{"email_send"}},
			},
			Dashboard: DashboardSpec{Layout: "grid-2col", Blocks: []BlockSpec{{Type: "stat-row"}, {Type: "pipeline", Title: "Project Status"}, {Type: "data-table", Title: "Clients"}, {Type: "data-table", Title: "Invoices"}, {Type: "chart", Title: "Revenue"}, {Type: "feed", Title: "Activity"}}},
			Skills: []string{"proposal_writing", "invoice_gen"}, Connectors: []string{"stripe", "gmail"},
		},

		{ID: "education", Name: "Education & Training", Description: "Curriculum creation, student tracking, quiz generation, and learning analytics", Icon: "🎓", Category: "education",
			Agents: []AgentSpec{
				{Key: "curriculum-lead", Name: "Curriculum Director", Role: "leader", SystemPrompt: "You design learning paths, oversee content creation, and track student progress."},
				{Key: "content-creator", Name: "Content Creator", Role: "specialist", ReportsTo: "curriculum-lead", SystemPrompt: "You create engaging educational content, lessons, and exercises.", Tools: []string{"web_search"}},
				{Key: "assessment-agent", Name: "Assessment Agent", Role: "specialist", ReportsTo: "curriculum-lead", SystemPrompt: "You generate quizzes, grade assignments, and provide detailed feedback."},
				{Key: "analytics-agent", Name: "Analytics Agent", Role: "specialist", ReportsTo: "curriculum-lead", SystemPrompt: "You analyze learning outcomes and identify students who need support."},
			},
			Dashboard: DashboardSpec{Layout: "grid-2col", Blocks: []BlockSpec{{Type: "stat-row"}, {Type: "data-table", Title: "Students"}, {Type: "chart", Title: "Progress"}, {Type: "kanban", Title: "Curriculum"}, {Type: "feed", Title: "Activity"}}},
			Skills: []string{"quiz_generation", "content_writing"}, Connectors: []string{"gmail"},
		},

		{ID: "analytics", Name: "Business Intelligence", Description: "KPI tracking, data analysis, automated reporting, and forecasting", Icon: "📈", Category: "analytics",
			Agents: []AgentSpec{
				{Key: "analytics-lead", Name: "Analytics Lead", Role: "leader", SystemPrompt: "You coordinate data collection, analysis, and executive reporting."},
				{Key: "data-collector", Name: "Data Collector", Role: "specialist", ReportsTo: "analytics-lead", SystemPrompt: "You gather data from various sources and maintain data pipelines.", Tools: []string{"web_fetch", "exec"}},
				{Key: "analyst-agent", Name: "Business Analyst", Role: "specialist", ReportsTo: "analytics-lead", SystemPrompt: "You analyze data trends, identify patterns, and produce actionable insights."},
				{Key: "reporter-agent", Name: "Reporter", Role: "specialist", ReportsTo: "analytics-lead", SystemPrompt: "You create executive-level reports and data visualizations.", Tools: []string{"email_send"}},
			},
			Dashboard: DashboardSpec{Layout: "grid-3col", Blocks: []BlockSpec{{Type: "stat-row"}, {Type: "chart", Title: "Revenue Trend"}, {Type: "chart", Title: "User Growth"}, {Type: "data-table", Title: "KPIs"}, {Type: "chart", Title: "Conversion"}, {Type: "feed", Title: "Insights"}}},
			Skills: []string{"data_analysis", "reporting"}, Connectors: []string{"google_analytics", "stripe"},
		},
	}
}
