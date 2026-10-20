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
// Removing a Mac happens on the site only (ManageURL). VerifySelf checks
// the app's own code signature (Team ID 779M533DL5) in release builds.
//
// Everything that touches the system is replaceable in Config (clock,
// device ID, store, HTTP client), so the logic is tested on Linux.
package licence
