// Package feedback is the apps' in-app feedback: what the user writes,
// screenshots and a screen recording, and diagnostics (this Mac, the app,
// the newest log lines and the last crash, redacted), sent to the site
// that serves the app — actnow.dev, or refto.one for Refto — over the
// feedback protocol (docs/feedback.md of the sites), and the user's own
// history of feedback with the replies.
//
// Like updates and licence, the package has no UI: the app draws Settings
// → Feedback with the Manager's state and the reference texts (Text*).
//
//	fb, err := feedback.New(feedback.Config{
//		App: "dicta", Version: version, Channel: "beta",
//		BaseURL: "https://actnow.dev",
//		Dir:     filepath.Join(dataDir, "feedback"),
//		LicenceToken: func() string {
//			if t := lic.Status().Token; t != nil {
//				return t.Raw
//			}
//			return ""
//		},
//		DeviceID:     func() string { id, _ := licence.DeviceID(salt); return id },
//		Collect: feedback.CollectConfig{LogDir: logDir, Logs: []feedback.LogSpec{{Name: "dicta.log"}}},
//	})
//	fb.Subscribe(func(st feedback.State) { /* redraw */ })
//	go fb.Resume(ctx)                       // a submission left by the last run
//	who, _ := fb.Identity(ctx)               // is the email optional?
//	problems := fb.Validate(draft, who)      // before Submit
//	fb.Submit(ctx, draft)                    // prepares, saves, sends
//	fb.Refresh(ctx)                          // the history's statuses
//
// The protocol (v1), every request a POST of JSON to
// <BaseURL>/api/feedback/v1/<endpoint>, every error {"error": code,
// "message": …, "details": …} with a 4xx status:
//
//   - identity {app, licenceToken?} → {linked, emailHint?}: whether the
//     site knows an address to answer (the licence code's owner).
//   - create {app, version, build, channel, locale, kind, body, email?,
//     licenceToken?, deviceId?, system, attachments: [{ref, role, name,
//     mime, bytes, sha256}]} → {id, number, viewToken, webUrl, uploads:
//     [{ref, attachmentId, method, url, headers, expiresAt, fallbackUrl}]}.
//     Errors: email_required, invalid_email, invalid_body,
//     attachment_too_large, attachment_type, too_many_attachments,
//     rate_limited (details.retryAfter), disabled.
//   - each file: PUT url with headers (straight to the storage bucket); on
//     a network failure PUT fallbackUrl with "Authorization: Feedback
//     <viewToken>" (through the site).
//   - uploads {id, viewToken} → {uploads}: new URLs for the files not yet
//     stored (the first ones expired).
//   - complete {id, viewToken} → {id, number, status, webUrl}; error
//     upload_missing (details.missing: attachment ids).
//   - list {items: [{id, viewToken}]} → {items: [Item]}.
//   - get {id, viewToken} → Thread; reply {id, viewToken, body} → Message.
//
// Nothing is uploaded before the user submits. Submit copies what will be
// sent (images re-encoded: at most MaxImages of MaxImageBytes, their
// metadata gone; a video of MaxVideoBytes, compressed with macOS's
// avconvert when larger; diagnostics.json) into Dir/outbox/<local id>/, so
// a send that fails or is cut by quitting resumes later (Resume, Retry).
package feedback
