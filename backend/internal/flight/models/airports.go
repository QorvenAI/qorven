// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package models

import "strings"

// AirportNames maps IATA airport codes to city/airport names for the top 200
// airports worldwide. Used as a fallback when the API response does not include
// city names (e.g., in explore results).
var AirportNames = map[string]string{
	// Europe
	"HEL": "Helsinki",
	"LHR": "London Heathrow",
	"LGW": "London Gatwick",
	"STN": "London Stansted",
	"LTN": "London Luton",
	"SEN": "London Southend",
	"BRS": "Bristol",
	"CDG": "Paris CDG",
	"ORY": "Paris Orly",
	"AMS": "Amsterdam",
	"FRA": "Frankfurt",
	"MUC": "Munich",
	"BER": "Berlin",
	"DUS": "Dusseldorf",
	"HAM": "Hamburg",
	"MAD": "Madrid",
	"BCN": "Barcelona",
	"AGP": "Malaga",
	"PMI": "Palma de Mallorca",
	"ALC": "Alicante",
	"IBZ": "Ibiza",
	"VLC": "Valencia",
	"SVQ": "Seville",
	"FCO": "Rome Fiumicino",
	"MXP": "Milan Malpensa",
	"NAP": "Naples",
	"VCE": "Venice",
	"LIN": "Milan Linate",
	"BGY": "Milan Bergamo",
	"BLQ": "Bologna",
	"OLB": "Olbia",
	"CTA": "Catania",
	"PSA": "Pisa",
	"ZRH": "Zurich",
	"GVA": "Geneva",
	"VIE": "Vienna",
	"BRU": "Brussels",
	"CPH": "Copenhagen",
	"OSL": "Oslo",
	"ARN": "Stockholm",
	"GOT": "Gothenburg",
	"DUB": "Dublin",
	"LIS": "Lisbon",
	"OPO": "Porto",
	"ATH": "Athens",
	"GDN": "Gdansk",
	"WRO": "Wroclaw",
	"KTW": "Katowice",
	"FAO": "Faro",
	"SKG": "Thessaloniki",
	"WAW": "Warsaw",
	"KRK": "Krakow",
	"PRG": "Prague",
	"BUD": "Budapest",
	"OTP": "Bucharest",
	"SOF": "Sofia",
	"IST": "Istanbul",
	"SAW": "Istanbul Sabiha",
	"AYT": "Antalya",
	"ZAG": "Zagreb",
	"BEG": "Belgrade",
	"TLL": "Tallinn",
	"RIX": "Riga",
	"VNO": "Vilnius",
	"KEF": "Reykjavik",
	"EDI": "Edinburgh",
	"MAN": "Manchester",
	"BHX": "Birmingham",
	"NCE": "Nice",
	"LYS": "Lyon",
	"TLS": "Toulouse",
	"MRS": "Marseille",
	"DBV": "Dubrovnik",
	"SPU": "Split",
	"TIV": "Tivat",
	"CFU": "Corfu",
	"HER": "Heraklion",
	"RHO": "Rhodes",
	"JTR": "Santorini",
	"TFS": "Tenerife South",
	"LPA": "Gran Canaria",
	"ACE": "Lanzarote",
	"FUE": "Fuerteventura",

	// North America
	"JFK": "New York JFK",
	"EWR": "Newark",
	"LGA": "New York LaGuardia",
	"LAX": "Los Angeles",
	"SFO": "San Francisco",
	"ORD": "Chicago O'Hare",
	"MDW": "Chicago Midway",
	"ATL": "Atlanta",
	"DFW": "Dallas/Fort Worth",
	"DEN": "Denver",
	"SEA": "Seattle",
	"MIA": "Miami",
	"FLL": "Fort Lauderdale",
	"MCO": "Orlando",
	"TPA": "Tampa",
	"BOS": "Boston",
	"IAD": "Washington Dulles",
	"DCA": "Washington Reagan",
	"PHL": "Philadelphia",
	"MSP": "Minneapolis",
	"DTW": "Detroit",
	"CLT": "Charlotte",
	"PHX": "Phoenix",
	"SAN": "San Diego",
	"IAH": "Houston",
	"AUS": "Austin",
	"SLC": "Salt Lake City",
	"PDX": "Portland",
	"BNA": "Nashville",
	"RDU": "Raleigh-Durham",
	"HNL": "Honolulu",
	"OGG": "Maui",
	"YYZ": "Toronto Pearson",
	"YVR": "Vancouver",
	"YUL": "Montreal",
	"YYC": "Calgary",
	"YOW": "Ottawa",
	"MEX": "Mexico City",
	"CUN": "Cancun",
	"SJD": "San Jose del Cabo",
	"GDL": "Guadalajara",

	// Asia
	"NRT": "Tokyo Narita",
	"HND": "Tokyo Haneda",
	"KIX": "Osaka Kansai",
	"ICN": "Seoul Incheon",
	"PEK": "Beijing Capital",
	"PKX": "Beijing Daxing",
	"PVG": "Shanghai Pudong",
	"HKG": "Hong Kong",
	"TPE": "Taipei",
	"SIN": "Singapore",
	"BKK": "Bangkok Suvarnabhumi",
	"DMK": "Bangkok Don Mueang",
	"KUL": "Kuala Lumpur",
	"CGK": "Jakarta",
	"MNL": "Manila",
	"SGN": "Ho Chi Minh City",
	"HAN": "Hanoi",
	"DEL": "New Delhi",
	"BOM": "Mumbai",
	"BLR": "Bangalore",
	"MAA": "Chennai",
	"CCU": "Kolkata",
	"CMB": "Colombo",
	"KTM": "Kathmandu",
	"DPS": "Bali Denpasar",
	"REP": "Siem Reap",
	"RGN": "Yangon",
	"PNH": "Phnom Penh",

	// Middle East
	"DXB": "Dubai",
	"AUH": "Abu Dhabi",
	"DOH": "Doha",
	"RUH": "Riyadh",
	"JED": "Jeddah",
	"TLV": "Tel Aviv",
	"AMM": "Amman",
	"BAH": "Bahrain",
	"MCT": "Muscat",
	"KWI": "Kuwait City",

	// Africa
	"JNB": "Johannesburg",
	"CPT": "Cape Town",
	"NBO": "Nairobi",
	"CAI": "Cairo",
	"CMN": "Casablanca",
	"RAK": "Marrakech",
	"ADD": "Addis Ababa",
	"LOS": "Lagos",
	"ACC": "Accra",
	"DSS": "Dakar",
	"TUN": "Tunis",

	// Oceania
	"SYD": "Sydney",
	"MEL": "Melbourne",
	"BNE": "Brisbane",
	"PER": "Perth",
	"AKL": "Auckland",
	"CHC": "Christchurch",
	"WLG": "Wellington",
	"NAN": "Nadi Fiji",
	"PPT": "Tahiti",

	// South America
	"GRU": "Sao Paulo",
	"GIG": "Rio de Janeiro",
	"EZE": "Buenos Aires",
	"SCL": "Santiago",
	"BOG": "Bogota",
	"LIM": "Lima",
	"UIO": "Quito",
	"CCS": "Caracas",
	"MVD": "Montevideo",
	"PTY": "Panama City",
	"SJO": "San Jose Costa Rica",
	"HAV": "Havana",
	"SDQ": "Santo Domingo",
	"MBJ": "Montego Bay",
}

// airportSearchCities maps airport codes with airport-qualified display names to
// provider-friendly city names for broader ground searches.
var airportSearchCities = map[string]string{
	"CDG": "Paris",
	"ORY": "Paris",
	"LHR": "London",
	"LGW": "London",
	"STN": "London",
	"LTN": "London",
	"SEN": "London",
	"FCO": "Rome",
	"MXP": "Milan",
	"LIN": "Milan",
	"BGY": "Milan",
	"SAW": "Istanbul",
	"JFK": "New York",
	"LGA": "New York",
	"NRT": "Tokyo",
	"HND": "Tokyo",
	"KIX": "Osaka",
	"ICN": "Seoul",
	"PEK": "Beijing",
	"PKX": "Beijing",
	"PVG": "Shanghai",
	"BKK": "Bangkok",
	"DMK": "Bangkok",
	"DPS": "Denpasar",
}

// LookupAirportName returns the city/airport name for an IATA code.
// Returns the code itself if no match is found.
func LookupAirportName(code string) string {
	if name, ok := AirportNames[code]; ok {
		return name
	}
	return code
}

// ResolveAirportCity converts an airport code into a provider-friendly city
// name for broader ground transport searches.
func ResolveAirportCity(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return ""
	}
	if city, ok := airportSearchCities[code]; ok {
		return city
	}
	return LookupAirportName(code)
}

// ResolveLocationName converts a string that might be an IATA code into a
// city name suitable for hotel/ground searches. If the input is already a
// city name (not a 3-letter uppercase IATA code), it is returned as-is.
// This prevents "PRG" being sent to Google Hotels (which needs "Prague").
func ResolveLocationName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	// Only resolve if it looks like an IATA code (2-3 uppercase letters).
	upper := strings.ToUpper(s)
	if len(upper) <= 3 && upper == s {
		if name, ok := AirportNames[upper]; ok {
			return name
		}
	}
	return s
}

// ResolveHotelCity returns the best city name for hotel search. For airport
// codes, this returns the broader city (e.g. "Paris" for CDG) instead of the
// airport-qualified name ("Paris CDG") which tends to bias hotel search
// toward airport-area properties.
func ResolveHotelCity(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	upper := strings.ToUpper(s)
	if len(upper) <= 3 && upper == s {
		// Prefer the broader city for hotel search.
		if city, ok := airportSearchCities[upper]; ok {
			return city
		}
		if name, ok := AirportNames[upper]; ok {
			return name
		}
	}
	return s
}
