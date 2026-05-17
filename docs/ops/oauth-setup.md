# OAuth setup

Qorven connects to third-party services (Google, Slack, GitHub, etc.) via OAuth 2.0. The callback URL in the OAuth handshake must point at your Qorven instance, so you register an OAuth app on each provider's developer console once — your Qorven install then uses those credentials for every user on it.

This doc is the recipe for every supported provider.

## The pattern

For every provider:

1. Go to the provider's developer console.
2. Create a new OAuth 2.0 app (sometimes called "Client", "Integration", or "Connected App").
3. Add your Qorven callback URL as an authorised redirect URI. You can copy it from **Settings → Connectors → OAuth apps** in Qorven — each provider shows its exact URL.
4. Note the client ID and client secret the provider generates.
5. Paste both into Qorven under **Settings → Connectors → OAuth apps** and click **Save**.

Qorven stores the credentials encrypted (with your gateway's encryption key) in the same vault as user tokens. They are never written to plaintext files.

You only register each app **once per Qorven instance** — the same credentials serve every user on that install.

## Providers

### Google (Gmail, Drive, Sheets, Calendar, Docs)

Console: https://console.cloud.google.com/apis/credentials

1. Create a project if you don't have one.
2. Enable the APIs you'll use: Gmail API, Google Drive API, Sheets API, Calendar API, Docs API.
3. **APIs & Services → OAuth consent screen** — fill in the app name, support email, and add your user email as a test user (you can leave the app in Testing mode for personal use).
4. **APIs & Services → Credentials → Create Credentials → OAuth client ID** — pick *Web application*. Paste the callback URL into **Authorized redirect URIs**.
5. Copy the **Client ID** and **Client secret** into Qorven.

Scopes Qorven requests: `gmail.modify`, `spreadsheets`, `drive`, `calendar`, `documents`.

### Microsoft (Graph: Outlook, Calendar, OneDrive, Teams)

Console: https://portal.azure.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade

1. **Azure Active Directory → App registrations → New registration**.
2. Name it whatever you like. Supported account types: **Accounts in any organizational directory and personal Microsoft accounts** unless you have a reason otherwise.
3. Add your callback URL under **Redirect URI** as type *Web*.
4. After registration: **Certificates & secrets → New client secret** — copy the *value* (not the ID) immediately, Azure only shows it once.
5. **API permissions** — add the delegated scopes: `Mail.ReadWrite`, `Mail.Send`, `Calendars.ReadWrite`, `Files.ReadWrite.All`, `Team.ReadBasic.All`, and `offline_access` for refresh tokens.
6. Paste the **Application (client) ID** and **client secret** into Qorven.

### Slack

Console: https://api.slack.com/apps

1. **Create New App → From scratch**. Pick your workspace.
2. **OAuth & Permissions**:
   - Add your callback URL under **Redirect URLs** and save.
   - Scroll to **Scopes → Bot Token Scopes** and add: `chat:write`, `channels:read`, `channels:history`, `users:read`.
3. **Install App** into your workspace.
4. Copy **Basic Information → App Credentials** — the **Client ID** and **Client Secret**.

Paste both into Qorven.

### GitHub

Console: https://github.com/settings/developers

1. **OAuth Apps → New OAuth App**.
2. Fill the name + homepage URL (pointing at your Qorven install is fine).
3. Paste the callback URL into **Authorization callback URL**.
4. After creating: copy the **Client ID**. Click **Generate a new client secret** and copy it immediately.

Paste both into Qorven. Scopes Qorven requests: `repo`, `read:org`, `read:user`.

### Twitter / X

Console: https://developer.twitter.com/en/portal/dashboard

1. Create a **Project**, then an **App** inside that project.
2. **User authentication settings** — set type to *Web App, Automated App or Bot*.
3. Paste the callback URL into **Callback URI / Redirect URL** and a website URL (any valid URL — your Qorven install is fine).
4. Scopes: `tweet.read`, `tweet.write`, `users.read`, `offline.access`.
5. Copy the **Client ID** and **Client Secret** from the OAuth 2.0 section.

### LinkedIn

Console: https://www.linkedin.com/developers/apps

1. **Create app**, link a LinkedIn Page you manage (can be a test page).
2. **Auth** tab → add your callback URL.
3. **Products** tab → request *Sign In with LinkedIn using OpenID Connect* and *Share on LinkedIn* (review is instant for most).
4. Copy **Client ID** and **Primary Client Secret** from the Auth tab.

Scopes: `openid`, `profile`, `w_member_social`.

### Salesforce

Docs: https://help.salesforce.com/s/articleView?id=sf.connected_app_create.htm

1. **Setup → App Manager → New Connected App**.
2. Enable **OAuth Settings**.
3. Paste the callback URL into **Callback URL**.
4. Pick scopes: *Manage user data via APIs (api)* and *Perform requests on your behalf at any time (refresh_token, offline_access)*.
5. After saving, Salesforce generates a **Consumer Key** (= client_id) and **Consumer Secret** (= client_secret) under the app's API settings. Note: the secret is hidden by default behind a "Click to reveal" link.

### Shopify

Console: https://partners.shopify.com/

1. **Apps → Create app → Create app manually**.
2. Set the **App URL** to your Qorven install.
3. Add the callback URL under **Allowed redirection URLs**.
4. Copy the **API key** (= client_id) and **API secret key** (= client_secret).

Scopes Qorven requests: `read_products`, `write_products`, `read_orders`, `write_orders`.

### HubSpot / Notion / Airtable / Linear / Intercom / Zendesk / Jira

These providers use API-key or bearer-token auth, not OAuth 2.0 — so they don't need the OAuth-apps registration flow. Instead:

1. Go to the provider's developer or account settings page.
2. Generate a personal access token / API key.
3. In Qorven, open **Settings → Connectors**, click **Connect** on the card, and paste the token.

Qorven handles the auth header format automatically.

## Verifying

After saving credentials for a provider:

1. Go to **Settings → Connectors** (Catalog tab).
2. Find the provider's card. It should show **Connect**.
3. Click **Connect** → the provider's consent screen opens in a popup.
4. Approve. On success Qorven stores the access + refresh tokens and the card flips to **Connected**.

If you get an error like *"redirect_uri_mismatch"* at this step, the callback URL in your provider's OAuth app doesn't match the one Qorven is actually using. Paste the exact URL shown in **Settings → Connectors → OAuth apps**.

## Troubleshooting

| Error | Cause |
|---|---|
| `redirect_uri_mismatch` | Copy the exact URL from Qorven's OAuth apps page into your provider console. Trailing slashes and `http` vs `https` matter. |
| `invalid_client` | Client ID or secret is wrong. Re-copy both; some consoles truncate the secret in the UI. |
| `access_denied` | User cancelled the consent screen. Retry. |
| Token exchange succeeds but calls fail with 401 | The provider's OAuth app needs additional scopes. Check the provider's console → OAuth settings and add the scopes Qorven requested. |
| Silently loops back to the catalog after consent | Cookies / popup blocker issue. Disable any browser extension that intercepts `window.opener` and retry. |

## Revoking

Remove credentials from **Settings → Connectors → OAuth apps**. Disconnect a user's authorised token from **Settings → Connectors → Catalog** on that user's card. You can also revoke access from the provider's side (e.g. https://myaccount.google.com/connections) — Qorven will detect the revocation on the next refresh attempt and mark the connection as expired.
