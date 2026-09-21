package cli

// Read-only operation table for packet 2 coverage, derived from
// api/operations.json (the pinned OpenAPI inventory). read_ops_test.go fails if
// this table drifts from the inventory or leaves a GET operation uncovered.

// readSpecs maps every implemented GET operation to a CLI command.
var readSpecs = []readSpec{
	{
		group: "activity", action: "list-task",
		short:       "Get task activity",
		operationID: "getActivities",
		path:        "/activity/{taskId}",
		params: []readParam{
			{name: "taskId", in: paramPath, flag: "task-id", required: true},
		},
	},
	{
		group: "asset", action: "download",
		short:       "Download asset",
		operationID: "getAsset",
		path:        "/asset/{id}",
		public:      true,
		binary:      true,
		params: []readParam{
			{name: "id", in: paramPath, flag: "id", required: true},
		},
	},
	{
		group: "column", action: "list",
		short:       "Get columns",
		operationID: "getColumns",
		path:        "/column/{projectId}",
		params: []readParam{
			{name: "projectId", in: paramPath, flag: "project-id", required: true},
		},
	},
	{
		group: "comment", action: "list-task",
		short:       "Get task comments",
		operationID: "getTaskComments",
		path:        "/comment/{taskId}",
		params: []readParam{
			{name: "taskId", in: paramPath, flag: "task-id", required: true},
		},
	},
	{
		group: "custom-field", action: "list-project",
		short:       "Get custom fields",
		operationID: "getCustomFieldsByProject",
		path:        "/custom-field/project/{projectId}",
		params: []readParam{
			{name: "projectId", in: paramPath, flag: "project-id", required: true},
		},
	},
	{
		group: "custom-field", action: "list-project-filter-values",
		short:       "Get custom field filter values",
		operationID: "getCustomFieldFilterValues",
		path:        "/custom-field/project/{projectId}/filter-values",
		params: []readParam{
			{name: "projectId", in: paramPath, flag: "project-id", required: true},
		},
	},
	{
		group: "custom-field", action: "list-project-values",
		short:       "Get project custom field values",
		operationID: "getCustomFieldValuesByProject",
		path:        "/custom-field/project/{projectId}/values",
		params: []readParam{
			{name: "projectId", in: paramPath, flag: "project-id", required: true},
		},
	},
	{
		group: "custom-field", action: "list-task-values",
		short:       "Get task custom field values",
		operationID: "getCustomFieldValuesByTask",
		path:        "/custom-field/task/{taskId}",
		params: []readParam{
			{name: "taskId", in: paramPath, flag: "task-id", required: true},
		},
	},
	{
		group: "discord", action: "get-integration",
		short:       "Get Discord integration",
		operationID: "getDiscordIntegration",
		path:        "/discord-integration/project/{projectId}",
		params: []readParam{
			{name: "projectId", in: paramPath, flag: "project-id", required: true},
		},
	},
	{
		group: "external-link", action: "list-task",
		short:       "Get task external links",
		operationID: "getExternalLinksByTask",
		path:        "/external-link/task/{taskId}",
		params: []readParam{
			{name: "taskId", in: paramPath, flag: "task-id", required: true},
		},
	},
	{
		group: "gitea", action: "get-integration",
		short:       "Get Gitea integration",
		operationID: "getGiteaIntegration",
		path:        "/gitea-integration/project/{projectId}",
		params: []readParam{
			{name: "projectId", in: paramPath, flag: "project-id", required: true},
		},
	},
	{
		group: "github", action: "get-app-info",
		short:       "Get GitHub app info",
		operationID: "getGitHubAppInfo",
		path:        "/github-integration/app-info",
	},
	{
		group: "github", action: "get-integration",
		short:       "Get GitHub integration",
		operationID: "getGitHubIntegration",
		path:        "/github-integration/project/{projectId}",
		params: []readParam{
			{name: "projectId", in: paramPath, flag: "project-id", required: true},
		},
	},
	{
		group: "github", action: "list-repositories",
		short:       "List GitHub repositories",
		operationID: "listGitHubRepositories",
		path:        "/github-integration/repositories/{projectId}",
		params: []readParam{
			{name: "projectId", in: paramPath, flag: "project-id", required: true},
		},
	},
	{
		group: "invitation", action: "get",
		short:       "Get invitation details",
		operationID: "getInvitationDetails",
		path:        "/invitation/{id}",
		params: []readParam{
			{name: "id", in: paramPath, flag: "id", required: true},
		},
	},
	{
		group: "invitation", action: "list-pending",
		short:       "Get pending invitations",
		operationID: "getUserPendingInvitations",
		path:        "/invitation/pending",
	},
	{
		group: "label", action: "get",
		short:       "Get label",
		operationID: "getLabel",
		path:        "/label/{id}",
		params: []readParam{
			{name: "id", in: paramPath, flag: "id", required: true},
		},
	},
	{
		group: "label", action: "list-task",
		short:       "Get task labels",
		operationID: "getTaskLabels",
		path:        "/label/task/{taskId}",
		params: []readParam{
			{name: "taskId", in: paramPath, flag: "task-id", required: true},
		},
	},
	{
		group: "label", action: "list-workspace",
		short:       "Get workspace labels",
		operationID: "getWorkspaceLabels",
		path:        "/label/workspace/{workspaceId}",
		params: []readParam{
			{name: "workspaceId", in: paramPath, flag: "workspace-id", required: true},
		},
	},
	{
		group: "mattermost", action: "get-integration",
		short:       "Get Mattermost integration",
		operationID: "getMattermostIntegration",
		path:        "/mattermost-integration/project/{projectId}",
		params: []readParam{
			{name: "projectId", in: paramPath, flag: "project-id", required: true},
		},
	},
	{
		group: "mcp", action: "get-authorization-request",
		short:       "Get consent request",
		operationID: "getMcpAuthorizationRequest",
		path:        "/mcp/authorize/request/{requestId}",
		public:      true,
		params: []readParam{
			{name: "requestId", in: paramPath, flag: "request-id", required: true},
		},
	},
	{
		group: "notification", action: "list",
		short:       "List notifications",
		operationID: "listNotifications",
		path:        "/notification",
	},
	{
		group: "notification-preference", action: "get",
		short:       "Get notification preferences",
		operationID: "getNotificationPreferences",
		path:        "/notification-preferences",
	},
	{
		group: "oauth", action: "get-id-token",
		short:       "Get OAuth id token",
		operationID: "getOAuthIdToken",
		path:        "/oauth/id-token",
	},
	{
		group: "org", action: "get-active-member",
		short:       "Get Organization Active Member",
		operationID: "getOrganizationActiveMember",
		path:        "/auth/organization/get-active-member",
	},
	{
		group: "org", action: "get-active-member-role",
		short:       "Get Organization Active Member Role",
		operationID: "getOrganizationActiveMemberRole",
		path:        "/auth/organization/get-active-member-role",
	},
	{
		group: "org", action: "get-full",
		short:       "Get Organization Full Organization",
		operationID: "getOrganizationFullOrganization",
		path:        "/auth/organization/get-full-organization",
	},
	{
		group: "org", action: "get-invitation",
		short:       "Get Organization Invitation",
		operationID: "getOrganizationInvitation",
		path:        "/auth/organization/get-invitation",
		params: []readParam{
			{name: "id", in: paramQuery, flag: "id", required: true},
		},
	},
	{
		group: "org", action: "get-role",
		short:       "Get Organization Role",
		operationID: "getOrganizationRole",
		path:        "/auth/organization/get-role",
	},
	{
		group: "org", action: "list",
		short:       "List Organization",
		operationID: "listOrganization",
		path:        "/auth/organization/list",
	},
	{
		group: "org", action: "list-invitations",
		short:       "List Organization Invitations",
		operationID: "listOrganizationInvitations",
		path:        "/auth/organization/list-invitations",
	},
	{
		group: "org", action: "list-members",
		short:       "List Organization Members",
		operationID: "listOrganizationMembers",
		path:        "/auth/organization/list-members",
	},
	{
		group: "org", action: "list-roles",
		short:       "List Organization Roles",
		operationID: "listOrganizationRoles",
		path:        "/auth/organization/list-roles",
	},
	{
		group: "org", action: "list-team-members",
		short:       "List Organization Team Members",
		operationID: "listOrganizationTeamMembers",
		path:        "/auth/organization/list-team-members",
	},
	{
		group: "org", action: "list-teams",
		short:       "List Organization Teams",
		operationID: "listOrganizationTeams",
		path:        "/auth/organization/list-teams",
	},
	{
		group: "org", action: "list-user-invitations",
		short:       "List Organization User Invitations",
		operationID: "listOrganizationUserInvitations",
		path:        "/auth/organization/list-user-invitations",
	},
	{
		group: "org", action: "list-user-teams",
		short:       "List Organization User Teams",
		operationID: "listOrganizationUserTeams",
		path:        "/auth/organization/list-user-teams",
	},
	{
		group: "project", action: "get",
		short:       "Get project",
		operationID: "getProject",
		path:        "/project/{id}",
		params: []readParam{
			{name: "id", in: paramPath, flag: "id", required: true},
		},
	},
	{
		group: "project", action: "list",
		short:       "List projects",
		operationID: "listProjects",
		path:        "/project",
		params: []readParam{
			{name: "workspaceId", in: paramQuery, flag: "workspace-id", required: true},
			{name: "includeArchived", in: paramQuery, flag: "include-archived", help: "Pass \"true\" to include archived projects in the list."},
		},
	},
	{
		group: "search", action: "global",
		short:       "Global search",
		operationID: "globalSearch",
		path:        "/search",
		params: []readParam{
			{name: "q", in: paramQuery, flag: "q", required: true},
			{name: "type", in: paramQuery, flag: "type", enum: []string{"all", "tasks", "projects", "workspaces", "comments", "activities"}},
			{name: "workspaceId", in: paramQuery, flag: "workspace-id", required: true},
			{name: "projectId", in: paramQuery, flag: "project-id"},
			{name: "limit", in: paramQuery, flag: "limit"},
			{name: "userEmail", in: paramQuery, flag: "user-email"},
		},
	},
	{
		group: "slack", action: "get-integration",
		short:       "Get Slack integration",
		operationID: "getSlackIntegration",
		path:        "/slack-integration/project/{projectId}",
		params: []readParam{
			{name: "projectId", in: paramPath, flag: "project-id", required: true},
		},
	},
	{
		group: "task", action: "export",
		short:       "Export tasks",
		operationID: "exportTasks",
		path:        "/task/export/{projectId}",
		params: []readParam{
			{name: "projectId", in: paramPath, flag: "project-id", required: true},
		},
	},
	{
		group: "task", action: "get",
		short:       "Get task",
		operationID: "getTask",
		path:        "/task/{id}",
		params: []readParam{
			{name: "id", in: paramPath, flag: "id", required: true},
		},
	},
	{
		group: "task", action: "list",
		short:       "List tasks",
		operationID: "listTasks",
		path:        "/task/tasks/{projectId}",
		params: []readParam{
			{name: "projectId", in: paramPath, flag: "project-id", required: true},
			{name: "status", in: paramQuery, flag: "status"},
			{name: "priority", in: paramQuery, flag: "priority"},
			{name: "assigneeId", in: paramQuery, flag: "assignee-id"},
			{name: "page", in: paramQuery, flag: "page", numeric: true},
			{name: "limit", in: paramQuery, flag: "limit", numeric: true},
			{name: "sortBy", in: paramQuery, flag: "sort-by", enum: []string{"createdAt", "priority", "dueDate", "position", "title", "number"}},
			{name: "sortOrder", in: paramQuery, flag: "sort-order", enum: []string{"asc", "desc"}},
			{name: "dueBefore", in: paramQuery, flag: "due-before"},
			{name: "dueAfter", in: paramQuery, flag: "due-after"},
		},
	},
	{
		group: "task-relation", action: "list-task",
		short:       "Get task relations",
		operationID: "getTaskRelations",
		path:        "/task-relation/{taskId}",
		params: []readParam{
			{name: "taskId", in: paramPath, flag: "task-id", required: true},
		},
	},
	{
		group: "telegram", action: "get-integration",
		short:       "Get Telegram integration",
		operationID: "getTelegramIntegration",
		path:        "/telegram-integration/project/{projectId}",
		params: []readParam{
			{name: "projectId", in: paramPath, flag: "project-id", required: true},
		},
	},
	{
		group: "time-entry", action: "get",
		short:       "Get time entry",
		operationID: "getTimeEntry",
		path:        "/time-entry/{id}",
		params: []readParam{
			{name: "id", in: paramPath, flag: "id", required: true},
		},
	},
	{
		group: "time-entry", action: "list-task",
		short:       "Get task time entries",
		operationID: "getTaskTimeEntries",
		path:        "/time-entry/task/{taskId}",
		params: []readParam{
			{name: "taskId", in: paramPath, flag: "task-id", required: true},
		},
	},
	{
		group: "user", action: "download-avatar",
		short:       "Download avatar",
		operationID: "getUserAvatar",
		path:        "/user/avatar/{id}",
		public:      true,
		binary:      true,
		params: []readParam{
			{name: "id", in: paramPath, flag: "id", required: true},
		},
	},
	{
		group: "webhook", action: "get-integration",
		short:       "Get webhook integration",
		operationID: "getGenericWebhookIntegration",
		path:        "/generic-webhook-integration/project/{projectId}",
		params: []readParam{
			{name: "projectId", in: paramPath, flag: "project-id", required: true},
		},
	},
	{
		group: "workflow-rule", action: "list-project",
		short:       "Get workflow rules",
		operationID: "getWorkflowRules",
		path:        "/workflow-rule/{projectId}",
		params: []readParam{
			{name: "projectId", in: paramPath, flag: "project-id", required: true},
		},
	},
	{
		group: "workspace", action: "list-members",
		short:       "Get workspace members",
		operationID: "getWorkspaceMembers",
		path:        "/workspace/{workspaceId}/members",
		params: []readParam{
			{name: "workspaceId", in: paramPath, flag: "workspace-id", required: true},
		},
	},
}

// groupShorts describes each read command group.
var groupShorts = map[string]string{
	"activity":                "Task activity",
	"asset":                   "Asset downloads",
	"column":                  "Board columns",
	"comment":                 "Task comments",
	"custom-field":            "Custom fields",
	"discord":                 "Discord integration",
	"external-link":           "Task external links",
	"gitea":                   "Gitea integration",
	"github":                  "GitHub integration",
	"invitation":              "Invitations",
	"label":                   "Labels",
	"mattermost":              "Mattermost integration",
	"mcp":                     "MCP authorization",
	"notification":            "Notifications",
	"notification-preference": "Notification preferences",
	"oauth":                   "OAuth",
	"org":                     "Organization management",
	"project":                 "Projects",
	"search":                  "Search",
	"slack":                   "Slack integration",
	"task":                    "Tasks",
	"task-relation":           "Task relations",
	"telegram":                "Telegram integration",
	"time-entry":              "Time entries",
	"user":                    "User",
	"webhook":                 "Generic webhook integration",
	"workflow-rule":           "Workflow rules",
	"workspace":               "Workspace",
}
