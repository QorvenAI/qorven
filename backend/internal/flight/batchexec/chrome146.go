// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package batchexec

import utls "github.com/refraction-networking/utls"

// Chrome146Spec returns a utls.ClientHelloSpec matching Chrome 146's TLS ClientHello.
//
// Key differences from Chrome 133 (HelloChrome_Auto) as shipped in utls v1.8.2:
//   - The spec is functionally identical to Chrome 133 at the utls level: both use
//     X25519MLKEM768 for Post-Quantum key exchange, BoringGREASEECH for ECH, and
//     ApplicationSettingsExtensionNew (ALPS, codepoint 17613). utls v1.8.2 does not
//     yet have a HelloChrome_146 parrot, so we define the spec explicitly here.
//   - Extension order is randomised by ShuffleChromeTLSExtensions, matching Chrome's
//     own extension-shuffling behaviour (GREASE and SNI are pinned first/last).
//
// Datadome fingerprints the JA3/JA4 hash of the TLS ClientHello. Using HelloChrome_Auto
// (which resolves to Chrome 133) on a build of Chrome that has already moved to 146 will
// produce a fingerprint mismatch and trigger 403s.
//
// Supported_groups order: GREASE, X25519MLKEM768 (4588), X25519, P-256, P-384.
// Key_share groups sent:  GREASE (1 byte), X25519MLKEM768, X25519.
func Chrome146Spec() utls.ClientHelloSpec {
	return utls.ClientHelloSpec{
		CipherSuites: []uint16{
			utls.GREASE_PLACEHOLDER,
			utls.TLS_AES_128_GCM_SHA256,
			utls.TLS_AES_256_GCM_SHA384,
			utls.TLS_CHACHA20_POLY1305_SHA256,
			utls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			utls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			utls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			utls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			utls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			utls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			utls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
			utls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			utls.TLS_RSA_WITH_AES_128_GCM_SHA256,
			utls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			utls.TLS_RSA_WITH_AES_128_CBC_SHA,
			utls.TLS_RSA_WITH_AES_256_CBC_SHA,
		},
		CompressionMethods: []byte{
			0x00, // compressionNone
		},
		Extensions: utls.ShuffleChromeTLSExtensions([]utls.TLSExtension{
			// GREASE extension (pinned first by ShuffleChromeTLSExtensions).
			&utls.UtlsGREASEExtension{},
			// SNI — server name indication.
			&utls.SNIExtension{},
			// Extended master secret (RFC 7627).
			&utls.ExtendedMasterSecretExtension{},
			// Renegotiation info.
			&utls.RenegotiationInfoExtension{Renegotiation: utls.RenegotiateOnceAsClient},
			// Supported groups: GREASE + X25519MLKEM768 (PQ) + classical curves.
			&utls.SupportedCurvesExtension{Curves: []utls.CurveID{
				utls.GREASE_PLACEHOLDER,
				utls.X25519MLKEM768, // CurveID 4588 — Post-Quantum hybrid key exchange
				utls.X25519,
				utls.CurveP256,
				utls.CurveP384,
			}},
			// EC point formats.
			&utls.SupportedPointsExtension{SupportedPoints: []byte{
				0x00, // pointFormatUncompressed
			}},
			// Session ticket (TLS 1.2 resumption).
			&utls.SessionTicketExtension{},
			// ALPN: h2 first (Chrome always prefers HTTP/2).
			&utls.ALPNExtension{AlpnProtocols: []string{"h2", "http/1.1"}},
			// OCSP stapling.
			&utls.StatusRequestExtension{},
			// Signature algorithms.
			&utls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []utls.SignatureScheme{
				utls.ECDSAWithP256AndSHA256,
				utls.PSSWithSHA256,
				utls.PKCS1WithSHA256,
				utls.ECDSAWithP384AndSHA384,
				utls.PSSWithSHA384,
				utls.PKCS1WithSHA384,
				utls.PSSWithSHA512,
				utls.PKCS1WithSHA512,
			}},
			// Signed Certificate Timestamps.
			&utls.SCTExtension{},
			// Key shares: GREASE + X25519MLKEM768 (PQ, ~1200 bytes) + X25519.
			&utls.KeyShareExtension{KeyShares: []utls.KeyShare{
				{Group: utls.CurveID(utls.GREASE_PLACEHOLDER), Data: []byte{0}},
				{Group: utls.X25519MLKEM768},
				{Group: utls.X25519},
			}},
			// PSK key exchange modes.
			&utls.PSKKeyExchangeModesExtension{Modes: []uint8{
				utls.PskModeDHE,
			}},
			// Supported TLS versions.
			&utls.SupportedVersionsExtension{Versions: []uint16{
				utls.GREASE_PLACEHOLDER,
				utls.VersionTLS13,
				utls.VersionTLS12,
			}},
			// Compressed certificate (brotli).
			&utls.UtlsCompressCertExtension{Algorithms: []utls.CertCompressionAlgo{
				utls.CertCompressionBrotli,
			}},
			// ALPS (Application-Layer Protocol Settings), new codepoint 17613.
			// Chrome 116+ uses this codepoint; the old 17513 codepoint is Chrome <=115.
			&utls.ApplicationSettingsExtensionNew{SupportedProtocols: []string{"h2"}},
			// GREASE ECH — Chrome sends this when no real ECH config is available.
			utls.BoringGREASEECH(),
			// Second GREASE extension (pinned last by ShuffleChromeTLSExtensions).
			&utls.UtlsGREASEExtension{},
		}),
	}
}
