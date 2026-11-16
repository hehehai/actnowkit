package feedback

import "github.com/hehehai/actnowkit/family"

// Reference texts of Settings → Feedback, in English and Simplified
// Chinese. actnowkit does no i18n: apps copy them into their catalogues.
// Placeholders: {app}, {number}, {email}, {count}, {max}, {size}, {site}.
var (
	TextSection         = family.Text{EN: "Feedback", ZH: "反馈"}
	TextNew             = family.Text{EN: "New feedback", ZH: "新反馈"}
	TextMenuItem        = family.Text{EN: "Send Feedback…", ZH: "发送反馈…"}
	TextHistory         = family.Text{EN: "Your feedback", ZH: "我的反馈"}
	TextOnTheWeb        = family.Text{EN: "Send feedback on the web", ZH: "在网页上反馈"}
	TextViewOnWeb       = family.Text{EN: "View on {site}", ZH: "在 {site} 上查看"}
	TextKind            = family.Text{EN: "Type", ZH: "类型"}
	TextKindBug         = family.Text{EN: "Problem", ZH: "问题"}
	TextKindIdea        = family.Text{EN: "Suggestion", ZH: "建议"}
	TextKindQuestion    = family.Text{EN: "Question", ZH: "提问"}
	TextKindOther       = family.Text{EN: "Other", ZH: "其他"}
	TextBody            = family.Text{EN: "Description", ZH: "描述"}
	TextBodyPlaceholder = family.Text{
		EN: "What happened, and what did you expect? Steps to reproduce help.",
		ZH: "发生了什么？你期望的结果是什么？附上复现步骤会更快定位。",
	}
	TextEmail            = family.Text{EN: "Email", ZH: "邮箱"}
	TextEmailPlaceholder = family.Text{EN: "So we can reply", ZH: "用于接收回复"}
	TextEmailLinked      = family.Text{
		EN: "We'll reply to {email}, the address of your licence. Enter another to use it instead.",
		ZH: "回复会发到授权邮箱 {email}；也可以填写其他邮箱。",
	}
	TextAttachments = family.Text{EN: "Attachments", ZH: "附件"}
	TextAddImages   = family.Text{EN: "Add screenshots…", ZH: "添加截图…"}
	TextAddVideo    = family.Text{EN: "Add a screen recording…", ZH: "添加录屏…"}
	TextAttachHint  = family.Text{
		EN: "Up to 3 images (made smaller to 5 MB each) and one video up to 20 MB (larger ones are compressed). Uploaded only when you send.",
		ZH: "最多 3 张图片（每张自动压缩到 5 MB 以内）和 1 段 20 MB 以内的视频（更大的会自动压缩）。只在提交时上传。",
	}
	TextDiagnostics     = family.Text{EN: "Include diagnostics", ZH: "附上诊断信息"}
	TextDiagnosticsHint = family.Text{
		EN: "This Mac's model and macOS version, {app}'s version and settings, and its recent log lines and last crash, with keys, passwords, names, email and IP addresses removed.",
		ZH: "包括这台 Mac 的型号和 macOS 版本、{app} 的版本和设置、最近的日志和最后一次崩溃记录；密钥、密码、姓名、邮箱和 IP 地址已去除。",
	}
	TextPreview = family.Text{EN: "See what is sent…", ZH: "查看将发送的内容…"}
	TextPrivacy = family.Text{
		EN: "Your feedback goes to {site} and is used only to answer you and improve {app}.",
		ZH: "反馈会发送到 {site}，只用于回复你和改进 {app}。",
	}
	TextPrivacyLink = family.Text{EN: "Privacy policy", ZH: "隐私政策"}
	TextSend        = family.Text{EN: "Send", ZH: "提交"}
	TextPreparing   = family.Text{EN: "Preparing attachments…", ZH: "正在处理附件…"}
	TextSending     = family.Text{EN: "Sending… {size}", ZH: "正在发送… {size}"}
	TextSent        = family.Text{
		EN: "Thanks! Your feedback {number} was sent. We'll reply by email and here.",
		ZH: "感谢反馈！编号 {number} 已提交，我们会通过邮件和这里回复你。",
	}
	TextFailed = family.Text{
		EN: "Your feedback could not be sent. It is kept on this Mac and sent when you're online again.",
		ZH: "反馈未能发送，已保存在这台 Mac 上，联网后会自动重新发送。",
	}
	TextRetry     = family.Text{EN: "Try again", ZH: "重试"}
	TextDiscard   = family.Text{EN: "Discard", ZH: "放弃"}
	TextPending   = family.Text{EN: "Waiting to send", ZH: "等待发送"}
	TextNoHistory = family.Text{EN: "Feedback you send from this Mac appears here.", ZH: "在这台 Mac 上提交的反馈会显示在这里。"}
	TextReply     = family.Text{EN: "Reply", ZH: "回复"}
	TextSupport   = family.Text{EN: "Support", ZH: "客服"}
	TextYou       = family.Text{EN: "You", ZH: "你"}
	TextNewReply  = family.Text{EN: "New reply", ZH: "有新回复"}

	TextStatusNew         = family.Text{EN: "Received", ZH: "已收到"}
	TextStatusTriaged     = family.Text{EN: "Being looked at", ZH: "处理中"}
	TextStatusInProgress  = family.Text{EN: "In progress", ZH: "处理中"}
	TextStatusWaitingUser = family.Text{EN: "Waiting for you", ZH: "等待你的回复"}
	TextStatusResolved    = family.Text{EN: "Resolved", ZH: "已解决"}
	TextStatusClosed      = family.Text{EN: "Closed", ZH: "已关闭"}

	TextProblemBodyShort     = family.Text{EN: "Write a few words about it.", ZH: "请简单描述一下。"}
	TextProblemBodyLong      = family.Text{EN: "At most {max} characters.", ZH: "最多 {max} 个字符。"}
	TextProblemEmailRequired = family.Text{EN: "Enter an email so we can reply.", ZH: "请填写邮箱，以便我们回复。"}
	TextProblemEmailInvalid  = family.Text{EN: "This email doesn't look right.", ZH: "邮箱格式不正确。"}
	TextProblemTooManyImages = family.Text{EN: "At most {max} images.", ZH: "最多 {max} 张图片。"}
	TextProblemImage         = family.Text{EN: "This image cannot be used (missing, too large or not an image).", ZH: "这张图片无法使用（文件不存在、过大或不是图片）。"}
	TextProblemVideo         = family.Text{EN: "This video cannot be used: an MP4 or MOV file up to 1 GB.", ZH: "这段视频无法使用：仅支持 1 GB 以内的 MP4 或 MOV。"}
	TextProblemVideoTooLarge = family.Text{EN: "The video is still over 20 MB after compressing: trim it and try again.", ZH: "视频压缩后仍超过 20 MB，请剪短后再试。"}
	TextRateLimited          = family.Text{EN: "Too much feedback from this Mac for now: it will be sent later.", ZH: "短时间内提交过多，稍后会自动发送。"}
)

// The feedback window (feedbackui) and the "Report this problem" link.
var (
	TextWindowTitle   = family.Text{EN: "Send Feedback", ZH: "发送反馈"}
	TextWindowSub     = family.Text{EN: "to the {app} team · {site}", ZH: "发给 {app} 团队 · {site}"}
	TextReportTitle   = family.Text{EN: "Report this problem", ZH: "报告此问题"}
	TextReportLink    = family.Text{EN: "Report this problem…", ZH: "报告此问题…"}
	TextIncludeReport = family.Text{
		EN: "Include this error report and its {count} log lines",
		ZH: "附上这份错误报告和相关的 {count} 行日志",
	}
	TextContextError  = family.Text{EN: "Error", ZH: "错误"}
	TextContextWhere  = family.Text{EN: "Where", ZH: "位置"}
	TextContextWhen   = family.Text{EN: "When", ZH: "时间"}
	TextContextSteps  = family.Text{EN: "Steps", ZH: "最近操作"}
	TextContextReport = family.Text{EN: "Report", ZH: "错误报告"}
	TextAdd           = family.Text{EN: "Add", ZH: "添加"}
	TextAddSub        = family.Text{EN: "images or a video", ZH: "图片或视频"}
	TextAttachCount   = family.Text{EN: "{images} of 3 images · {videos} of 1 video", ZH: "图片 {images}/3 · 视频 {videos}/1"}
	TextRemove        = family.Text{EN: "Remove", ZH: "移除"}
	TextOpenWith      = family.Text{EN: "Open in QuickTime Player", ZH: "用 QuickTime Player 打开"}
	TextHistoryCount  = family.Text{EN: "Your feedback ({count})", ZH: "我的反馈（{count}）"}
	TextCancel        = family.Text{EN: "Cancel", ZH: "取消"}
	TextClose         = family.Text{EN: "Close", ZH: "关闭"}
	TextViewIt        = family.Text{EN: "View it", ZH: "查看"}
	TextSentTitle     = family.Text{EN: "Thanks — feedback sent", ZH: "感谢反馈"}
	TextSentBody      = family.Text{
		EN: "Your feedback {number} is with us. We reply by email and in Your feedback.",
		ZH: "反馈 {number} 已收到，我们会通过邮件和“我的反馈”回复你。",
	}
	TextOpenWindow = family.Text{EN: "Open the feedback window", ZH: "打开反馈窗口"}
)

// KindText is the reference text of a kind.
func KindText(k Kind) family.Text {
	switch k {
	case KindBug:
		return TextKindBug
	case KindIdea:
		return TextKindIdea
	case KindQuestion:
		return TextKindQuestion
	}
	return TextKindOther
}

// StatusText is the reference text of a status.
func StatusText(s Status) family.Text {
	switch s {
	case StatusTriaged:
		return TextStatusTriaged
	case StatusInProgress:
		return TextStatusInProgress
	case StatusWaitingUser:
		return TextStatusWaitingUser
	case StatusResolved:
		return TextStatusResolved
	case StatusClosed:
		return TextStatusClosed
	}
	return TextStatusNew
}

// ProblemText is the reference text of a form problem ({max} filled).
func ProblemText(p Problem) family.Text {
	switch p {
	case ProblemBodyShort, ProblemKind:
		return TextProblemBodyShort
	case ProblemBodyLong:
		return TextProblemBodyLong.Fill("max", "10000")
	case ProblemEmailRequired:
		return TextProblemEmailRequired
	case ProblemEmailInvalid:
		return TextProblemEmailInvalid
	case ProblemTooManyImages:
		return TextProblemTooManyImages.Fill("max", "3")
	case ProblemImageMissing, ProblemImageTooLarge, ProblemImageType:
		return TextProblemImage
	case ProblemVideoTooLarge:
		return TextProblemVideo
	}
	return TextProblemVideo
}
