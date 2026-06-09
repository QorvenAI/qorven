package reachuser

import "testing"

func TestDecide_LowUrgency_InAppOnly_NeverEscalates(t *testing.T) {
	d := Decide(Input{Urgency: "low", CurrentRung: 0, Online: false})
	if d.DeliverRung != 1 || d.Channel != ChannelInApp {
		t.Fatalf("expected deliver rung 1 in-app, got %+v", d)
	}
	if d.WaitSeconds != 0 {
		t.Errorf("low urgency must not schedule a next advance, got wait=%d", d.WaitSeconds)
	}
	d2 := Decide(Input{Urgency: "low", CurrentRung: 1, Online: false})
	if !d2.Done {
		t.Errorf("low urgency after rung 1 should be Done, got %+v", d2)
	}
}

func TestDecide_Normal_OpensInApp_ThenIM_ThenEmail(t *testing.T) {
	d1 := Decide(Input{Urgency: "normal", CurrentRung: 0, Online: false})
	if d1.DeliverRung != 1 || d1.Channel != ChannelInApp || d1.WaitSeconds != 300 {
		t.Fatalf("normal open: want rung1 in-app wait 300, got %+v", d1)
	}
	d2 := Decide(Input{Urgency: "normal", CurrentRung: 1, Online: false})
	if d2.DeliverRung != 2 || d2.Channel != ChannelIM || d2.WaitSeconds != 1800 {
		t.Fatalf("normal advance1: want rung2 IM wait 1800, got %+v", d2)
	}
	d3 := Decide(Input{Urgency: "normal", CurrentRung: 2, Online: false})
	if d3.DeliverRung != 3 || d3.Channel != ChannelEmail || d3.WaitSeconds != 0 {
		t.Fatalf("normal advance2: want rung3 email wait 0, got %+v", d3)
	}
	if !Decide(Input{Urgency: "normal", CurrentRung: 3, Online: false}).Done {
		t.Errorf("normal after rung 3 should be Done")
	}
}

func TestDecide_NormalOnlineUser_StillDeliversInAppAndWaits(t *testing.T) {
	d := Decide(Input{Urgency: "normal", CurrentRung: 0, Online: true})
	if d.DeliverRung != 1 || d.Channel != ChannelInApp || d.WaitSeconds != 300 {
		t.Fatalf("online normal open: want rung1 in-app wait 300, got %+v", d)
	}
}

func TestDecide_Urgent_OpensInAppAndImmediatelyIM(t *testing.T) {
	d1 := Decide(Input{Urgency: "urgent", CurrentRung: 0, Online: false})
	if d1.DeliverRung != 1 || d1.Channel != ChannelInApp || d1.WaitSeconds != 0 {
		t.Fatalf("urgent open: want rung1 in-app wait 0 (immediate climb), got %+v", d1)
	}
	d2 := Decide(Input{Urgency: "urgent", CurrentRung: 1, Online: false})
	if d2.DeliverRung != 2 || d2.Channel != ChannelIM || d2.WaitSeconds != 0 {
		t.Fatalf("urgent advance1: want rung2 IM wait 0, got %+v", d2)
	}
	d3 := Decide(Input{Urgency: "urgent", CurrentRung: 2, Online: false})
	if d3.DeliverRung != 3 || d3.Channel != ChannelEmail {
		t.Fatalf("urgent advance2: want rung3 email, got %+v", d3)
	}
}
