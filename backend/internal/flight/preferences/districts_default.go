// Copyright 2026 Tekky AI Academy LLP. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package preferences

import "strings"

// defaultNiceDistricts is a curated map of city -> list of known-nice
// neighborhoods. This applies when the user has not specified their own
// preferred_districts for a city. The intent is to avoid airport hotels
// and distant suburbs when the user is clearly planning a leisure trip.
//
// Substrings are matched case-insensitively against the hotel address.
// Inclusion here is subjective and based on "where a well-traveled person
// would want to stay for a weekend" — central, walkable, interesting,
// with good food and transport options.
var defaultNiceDistricts = map[string][]string{
	"amsterdam": {
		"jordaan", "de pijp", "centrum", "canal", "grachtengordel",
		"oud-west", "oud west", "leidseplein", "rembrandtplein",
		"negen straatjes", "nine streets", "dam square",
	},
	"paris": {
		"le marais", "marais", "saint-germain", "saint germain",
		"latin quarter", "quartier latin", "montmartre", "montparnasse",
		"1er", "4e", "5e", "6e", "7e", "8e", "9e", "11e",
		"1st arrondissement", "4th arrondissement", "5th arrondissement",
		"6th arrondissement", "7th arrondissement",
		"opera", "louvre", "champs-elysees", "champs elysees",
	},
	"prague": {
		"prague 1", "prague 2", "prague 3", "prague 5", "prague 7",
		"praha 1", "praha 2", "praha 3", "praha 5", "praha 7",
		"stare mesto", "old town", "mala strana", "lesser town",
		"nove mesto", "new town", "vinohrady", "zizkov", "smichov",
		"wenceslas", "václavské",
	},
	"krakow": {
		"old town", "stare miasto", "kazimierz", "grzegórzki",
		"podgórze", "podgorze", "główny", "rynek", "piasek",
	},
	"kraków": {
		"old town", "stare miasto", "kazimierz", "grzegórzki",
		"podgórze", "podgorze", "główny", "rynek", "piasek",
	},
	"madrid": {
		"sol", "centro", "malasaña", "malasana", "chueca",
		"la latina", "barrio de las letras", "lavapiés", "lavapies",
		"retiro", "salamanca", "chamberí", "chamberi",
	},
	"helsinki": {
		"punavuori", "kallio", "kamppi", "kluuvi", "kruununhaka",
		"ullanlinna", "eira", "töölö", "toolo", "design district",
	},
	"vienna": {
		"innere stadt", "1. bezirk", "1st district", "neubau",
		"7. bezirk", "7th district", "leopoldstadt",
		"mariahilf", "6. bezirk", "6th district", "josefstadt",
		"8. bezirk", "8th district", "landstraße", "landstrasse",
	},
	"barcelona": {
		"gothic quarter", "gothic", "born", "el born", "barrio gótico",
		"barri gòtic", "raval", "el raval", "eixample", "gràcia",
		"gracia", "barceloneta", "sant antoni", "sant pere",
	},
	"lisbon": {
		"baixa", "chiado", "alfama", "bairro alto", "príncipe real",
		"principe real", "estrela", "avenida", "graça", "graca",
	},
	"rome": {
		"centro storico", "trastevere", "monti", "prati", "testaccio",
		"pigneto", "celio", "aventino", "campo de' fiori", "pantheon",
		"piazza navona", "trevi", "spagna",
	},
	"berlin": {
		"mitte", "prenzlauer berg", "kreuzberg", "friedrichshain",
		"neukölln", "neukolln", "charlottenburg", "schöneberg",
		"schoneberg",
	},
	"lisboa": {
		"baixa", "chiado", "alfama", "bairro alto", "príncipe real",
		"principe real", "estrela", "avenida",
	},
}

// DefaultDistrictsFor returns curated nice-neighborhood substrings for the
// given city when the user hasn't specified their own. Returns nil if no
// default list is known.
func DefaultDistrictsFor(city string) []string {
	if city == "" {
		return nil
	}
	key := strings.ToLower(strings.TrimSpace(city))
	// Try exact first.
	if v, ok := defaultNiceDistricts[key]; ok {
		return v
	}
	// Match first word (handles "Paris, France" -> "paris").
	if parts := strings.FieldsFunc(key, func(r rune) bool {
		return r == ',' || r == ' '
	}); len(parts) > 0 {
		if v, ok := defaultNiceDistricts[parts[0]]; ok {
			return v
		}
	}
	return nil
}
