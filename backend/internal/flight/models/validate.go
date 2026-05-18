// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package models

import (
	"fmt"
	"regexp"
	"time"
)

var iataRegex = regexp.MustCompile(`^[A-Z]{3}$`)

// ValidateIATA checks that code is a valid 3-letter uppercase IATA code.
func ValidateIATA(code string) error {
	if !iataRegex.MatchString(code) {
		return fmt.Errorf("invalid IATA code %q: must be exactly 3 uppercase letters (e.g. HEL, NRT)", code)
	}
	return nil
}

// ValidateDate checks that date is a valid YYYY-MM-DD string and is not in the past.
func ValidateDate(date string) error {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return fmt.Errorf("invalid date %q: expected YYYY-MM-DD format", date)
	}
	today := time.Now().Truncate(24 * time.Hour)
	if t.Before(today) {
		return fmt.Errorf("date %q is in the past", date)
	}
	return nil
}

// ValidateDateRange checks that from and to are valid dates and from <= to.
func ValidateDateRange(from, to string) error {
	fromT, err := time.Parse("2006-01-02", from)
	if err != nil {
		return fmt.Errorf("invalid start date %q: expected YYYY-MM-DD format", from)
	}
	toT, err := time.Parse("2006-01-02", to)
	if err != nil {
		return fmt.Errorf("invalid end date %q: expected YYYY-MM-DD format", to)
	}
	if toT.Before(fromT) {
		return fmt.Errorf("end date %s is before start date %s", to, from)
	}
	return nil
}
