package licence

import "github.com/hehehai/actnowkit/family"

// Reference texts of the licence screens, in English and Simplified
// Chinese. actnowkit does no i18n: apps copy them into their catalogues.
// Placeholders: {app}, {date}, {version}, {count}, {max}.
var (
	TextActivateTitle  = family.Text{EN: "Activate {app}", ZH: "激活 {app}"}
	TextActivatePrompt = family.Text{
		EN: "Enter the licence code from your email or your ActNow account.",
		ZH: "输入邮件或 ActNow 账户里的授权码。",
	}
	TextCodePlaceholder = family.Text{EN: "CC-XXXXXX-XXXXXX-XXXXXX-XXXXXX", ZH: "CC-XXXXXX-XXXXXX-XXXXXX-XXXXXX"}
	TextActivate        = family.Text{EN: "Activate", ZH: "激活"}
	TextRequestAccess   = family.Text{EN: "Request early access", ZH: "申请体验资格"}
	TextHowItWorks      = family.Text{EN: "How activation works", ZH: "激活说明"}
	TextManageOnline    = family.Text{EN: "Manage on actnow.dev", ZH: "在 actnow.dev 管理"}
	// TextRemoveThisMac: the app's Remove button only opens the site.
	TextRemoveThisMac = family.Text{EN: "Remove a Mac…", ZH: "移除设备…"}
	TextRemoveHint    = family.Text{
		EN: "Macs are removed on actnow.dev, with a link we email you. A code can remove 3 Macs every 30 days.",
		ZH: "移除设备在 actnow.dev 上操作，需要点击我们发到你邮箱的链接。每个码 30 天内最多移除 3 台。",
	}

	TextEditionWhitelist = family.Text{EN: "Early access", ZH: "体验版"}
	TextEditionPaid      = family.Text{EN: "Licensed", ZH: "已授权"}
	TextValidUntil       = family.Text{EN: "Valid until {date}", ZH: "有效期至 {date}"}
	TextNeverExpires     = family.Text{EN: "Never expires", ZH: "永久有效"}
	TextDevicesUsed      = family.Text{EN: "{count} of {max} Macs", ZH: "已用 {count}/{max} 台 Mac"}

	TextStateValid = family.Text{EN: "{app} is activated on this Mac.", ZH: "{app} 已在这台 Mac 上激活。"}
	TextStateGrace = family.Text{
		EN: "{app} could not check its licence: connect to the internet before {date}.",
		ZH: "{app} 暂时无法联网校验授权，请在 {date} 前连接网络。",
	}
	TextStateUpdated = family.Text{
		EN: "{app} was updated: connect to the internet once to confirm your licence.",
		ZH: "{app} 已更新，请联网一次以确认授权。",
	}
	TextStateClock = family.Text{
		EN: "This Mac's clock went back in time. Set the date and time automatically, then connect to the internet.",
		ZH: "这台 Mac 的时间被调回过。请打开\"自动设置日期与时间\"，然后联网。",
	}
	TextStateExpired = family.Text{
		EN: "Your {app} licence ended on {date}. Renew it to keep using {app}; your projects are safe.",
		ZH: "你的 {app} 授权已于 {date} 到期。续期后即可继续使用，你的项目和数据都不会丢失。",
	}
	TextStateOffline = family.Text{
		EN: "{app} has not checked its licence for 30 days. Connect to the internet to continue.",
		ZH: "{app} 已有 30 天没有联网校验授权，请联网后继续使用。",
	}
	TextStateWhitelist = family.Text{
		EN: "Your early-access code works with versions before {version}. This version needs a paid licence.",
		ZH: "你的体验码适用于 {version} 之前的版本。这个版本需要付费授权。",
	}
	TextStateDeviceLimit = family.Text{
		EN: "This code is already used on {max} Macs. Remove one on actnow.dev, then try again.",
		ZH: "这个码已在 {max} 台 Mac 上使用。请先在 actnow.dev 移除一台，再重试。",
	}
	TextStateRemoved = family.Text{
		EN: "This Mac was removed from the licence. Enter a code to activate it again.",
		ZH: "这台 Mac 已从授权中移除。输入授权码可以重新激活。",
	}
	TextStateRevoked = family.Text{
		EN: "This licence code is no longer valid. Contact support@actnow.dev if this is a mistake.",
		ZH: "这个授权码已失效。如有疑问请联系 support@actnow.dev。",
	}
	TextStateInactive = family.Text{EN: "{app} is not activated on this Mac.", ZH: "{app} 尚未在这台 Mac 上激活。"}

	// Activation errors (APIError.Code).
	TextErrInvalidFormat = family.Text{EN: "This is not a valid code: check it for typos.", ZH: "授权码格式不对，请检查是否输错。"}
	TextErrNotFound      = family.Text{EN: "No such code.", ZH: "没有这个授权码。"}
	TextErrWrongApp      = family.Text{EN: "This code is for another app.", ZH: "这个授权码属于另一个 app。"}
	TextErrRateLimited   = family.Text{EN: "Too many tries. Wait a few minutes and try again.", ZH: "尝试次数过多，请几分钟后再试。"}
	TextErrNetwork       = family.Text{EN: "Cannot reach actnow.dev. Check your connection.", ZH: "无法连接 actnow.dev，请检查网络。"}

	// TextUpdateNeedsPaid: the update panel, early-access licence and an
	// update at or above paid_since (docs/licensing.md §7).
	TextUpdateNeedsPaid = family.Text{
		EN: "This update needs a paid licence: after updating, your early-access code will no longer work. You can keep the version you have.",
		ZH: "这个更新需要付费授权，更新后你的体验码将不能继续使用。你也可以继续使用当前版本。",
	}
)

// StateText is the reference text of a status (fill {app}, {date},
// {version}, {max} with Fill).
func StateText(st Status) family.Text {
	switch st.State {
	case Valid:
		return TextStateValid
	case Grace:
		return TextStateGrace
	case NeedsRefresh:
		switch st.Reason {
		case ReasonUpdated:
			return TextStateUpdated
		case ReasonClock:
			return TextStateClock
		}
		return TextStateValid
	case Expired:
		if st.Reason == ReasonOffline {
			return TextStateOffline
		}
		return TextStateExpired
	case WhitelistNotApplicable:
		return TextStateWhitelist
	case DeviceLimit:
		return TextStateDeviceLimit
	case Revoked:
		if st.Reason == ReasonRemoved {
			return TextStateRemoved
		}
		return TextStateRevoked
	}
	return TextStateInactive
}

// ErrorText is the reference text of an Activate error.
func ErrorText(code string) family.Text {
	switch code {
	case "invalid_code_format":
		return TextErrInvalidFormat
	case "code_not_found":
		return TextErrNotFound
	case "wrong_app":
		return TextErrWrongApp
	case "code_revoked":
		return TextStateRevoked
	case "code_expired":
		return TextStateExpired
	case "whitelist_not_applicable":
		return TextStateWhitelist
	case "rate_limited":
		return TextErrRateLimited
	}
	return TextErrNetwork
}
