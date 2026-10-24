// Package licence activates and checks an ActNow app's licence on this
// Mac, against actnow.dev (protocol: docs/licensing.md of the site).
//
// A code is activated once online: the site answers with a token signed
// with Ed25519, bound to this Mac's device ID (IOPlatformUUID with the
// app's salt, hashed) and stored in the app's own Keychain item. The token
// is then checked offline with the public key the app embeds
// (ProductionKeys), refreshed every 7 days, and works up to 30 days without
// a connection. Early-access (whitelist) tokens stop at the app's
// paid_since version.
//
//	lic, err := licence.New(licence.Config{
//		App:        "captioncraft",
//		Version:    mygo.App.Version(),
//		DeviceSalt: "dev.hehehai.captioncraft/1f6c…",
//		Keys:       licence.ProductionKeys(),
//		Store:      licence.NewKeychainStore("dev.hehehai.captioncraft"),
//	})
//	wake := lic.Run(ctx, func(st licence.Status) { /* update the UI */ })
//	st, err := lic.Activate(ctx, code) // from the activation page
//
// Without a code, the app has a free trial of 15 days per Mac, counted from
// its first launch (no code needed; the start is kept in the Keychain item
// "actnow.trial"/<app slug>, so it survives reinstalls). Online, StartTrial
// (called by Check) takes the site's trial token; offline, the Mac's own
// record decides. A valid code always wins. Trial states: trial_valid (a
// trial token), trial_local (offline, within 15 days), trial_expired (over,
// or a code was refused: then the code's own state, with no trial). Use
// Status.IsTrial and Status.Licensed to show them.
//
// Removing a Mac happens on the site only (ManageURL). VerifySelf checks
// the app's own code signature (Team ID 779M533DL5) in release builds.
//
// Everything that touches the system is replaceable in Config (clock,
// device ID, store, HTTP client), so the logic is tested on Linux.
package licence
