package updates

import "github.com/hehehai/actnowkit/family"

// Reference texts of the update UI, in English and Simplified Chinese.
// Placeholders: {app}, {version}, {percent}, {error}.
var (
	TextUpdateAvailable = family.Text{EN: "Update available", ZH: "有新版本"}
	TextNewVersion      = family.Text{EN: "{app} {version} is available", ZH: "{app} {version} 已发布"}
	TextInstall         = family.Text{EN: "Download and Install", ZH: "下载并安装"}
	TextDownloading     = family.Text{EN: "Downloading… {percent}", ZH: "正在下载… {percent}"}
	TextRestart         = family.Text{EN: "Restart to Finish the Update", ZH: "重新启动以完成更新"}
	TextLater           = family.Text{EN: "Later", ZH: "稍后"}
	TextKeepVersion     = family.Text{EN: "Keep This Version", ZH: "继续使用当前版本"}
	TextUpToDate        = family.Text{EN: "{app} is up to date ({version})", ZH: "{app} 已是最新版本（{version}）"}
	TextChecking        = family.Text{EN: "Checking for updates…", ZH: "正在检查更新…"}
	TextCheckNow        = family.Text{EN: "Check for Updates", ZH: "检查更新"}
	TextFailed          = family.Text{EN: "The update did not install: {error}. {app} is unchanged.", ZH: "更新没有安装成功：{error}。{app} 未做任何改动。"}
	TextCheckFailed     = family.Text{EN: "Cannot check for updates: {error}", ZH: "无法检查更新：{error}"}
	TextChangelog       = family.Text{EN: "Changelog", ZH: "更新日志"}
	TextAutoCheck       = family.Text{EN: "Check for updates automatically", ZH: "自动检查更新"}
	TextBetaUpdates     = family.Text{EN: "Get beta updates", ZH: "接收测试版更新"}
	TextNotesInEnglish  = family.Text{EN: "", ZH: "这个版本暂无中文说明，以下为英文。"}
	// TextNeedsPaid is licence.TextUpdateNeedsPaid, for the update panel.
	TextNeedsPaid = family.Text{
		EN: "This update needs a paid licence: after updating, your early-access code will no longer work. You can keep the version you have.",
		ZH: "这个更新需要付费授权，更新后你的体验码将不能继续使用。你也可以继续使用当前版本。",
	}
	TextInstallAnyway = family.Text{EN: "Update Anyway", ZH: "仍然更新"}
)
