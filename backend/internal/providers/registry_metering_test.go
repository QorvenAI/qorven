// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import "testing"

func TestRegistry_WrapsInMeteredWhenConfigured(t *testing.T) {
	r := NewRegistry()
	r.SetMetering(&fakeEnforcer{}, &fakeRecorder{})
	r.RegisterProvider("p1", &fakeProvider{})

	got, ok := r.Get("p1")
	if !ok {
		t.Fatal("provider not found")
	}
	if _, isMetered := got.(*MeteredProvider); !isMetered {
		t.Fatalf("registry must return *MeteredProvider when metering configured, got %T", got)
	}
}

func TestRegistry_NoMeteringMeansPassThrough(t *testing.T) {
	r := NewRegistry()
	r.RegisterProvider("p1", &fakeProvider{})
	got, _ := r.Get("p1")
	if _, isMetered := got.(*MeteredProvider); isMetered {
		t.Fatal("without SetMetering the provider must not be metered-wrapped")
	}
}

func TestRegistry_SetMeteringRewrapsExisting(t *testing.T) {
	r := NewRegistry()
	r.RegisterProvider("p1", &fakeProvider{}) // registered BEFORE metering configured
	r.SetMetering(&fakeEnforcer{}, &fakeRecorder{})
	got, _ := r.Get("p1")
	if _, isMetered := got.(*MeteredProvider); !isMetered {
		t.Fatalf("SetMetering must re-wrap already-registered providers, got %T", got)
	}
}

func TestRegistry_MeterIsIdempotent(t *testing.T) {
	r := NewRegistry()
	r.SetMetering(&fakeEnforcer{}, &fakeRecorder{})
	r.RegisterProvider("p1", &fakeProvider{})
	r.SetMetering(&fakeEnforcer{}, &fakeRecorder{}) // again
	got, _ := r.Get("p1")
	mp, isMetered := got.(*MeteredProvider)
	if !isMetered {
		t.Fatalf("expected metered, got %T", got)
	}
	if mp.Name() != "fake" {
		t.Fatalf("metered wrapper must still delegate Name() to the original fake, got %q", mp.Name())
	}
}
