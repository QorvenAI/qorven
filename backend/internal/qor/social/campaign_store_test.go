// Copyright 2026 Qorven AI. All rights reserved.
package social

import "testing"

func TestCampaignStore_MethodSet(t *testing.T) {
	_ = (*Store).CreateCampaign
	_ = (*Store).ListCampaigns
	_ = (*Store).GetCampaign
	_ = (*Store).SetCampaignStatus
}
