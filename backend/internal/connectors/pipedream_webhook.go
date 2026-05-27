package connectors

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

type PipedreamWebhookPayload struct {
	Type           string           `json:"type"`
	ExternalUserID string           `json:"external_user_id"`
	Account        PipedreamAccount `json:"account"`
}

func HandlePipedreamWebhook(ctx context.Context, store *RelayStore, tenantID string, payload PipedreamWebhookPayload) error {
	switch payload.Type {
	case "account_authenticated":
		return store.UpsertAccount(ctx, tenantID, ConnectedAccountRecord{
			RelayProvider:     "pipedream",
			ExternalAccountID: payload.Account.ID,
			PlatformID:        payload.Account.App,
			DisplayName:       payload.Account.Name,
			AuthorizedScopes:  payload.Account.AuthorizedScopes,
			Healthy:           payload.Account.Healthy,
		})
	case "account_deleted":
		accounts, err := store.ListAccounts(ctx, tenantID)
		if err != nil {
			return err
		}
		for _, a := range accounts {
			if a.ExternalAccountID == payload.Account.ID {
				return store.DeleteAccount(ctx, tenantID, a.ID)
			}
		}
		return nil
	default:
		return nil
	}
}

func VerifyWebhookSignature(body []byte, signature, signingKey string) bool {
	if signingKey == "" || signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(signingKey))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
