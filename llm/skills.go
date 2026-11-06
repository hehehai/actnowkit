package llm

// Skill is a task the app asks of the model ("summarize", "translate"…).
// The app names its skills and their defaults (Client.Defaults); the
// Client fills a request's unset temperature and reply length from them
// and the Meter accounts usage under the skill.
type Skill string

// SkillDefaults are what a skill's requests use when they set nothing.
type SkillDefaults struct {
	// Temperature: low for faithful work. JSON replies are decoded
	// greedily by the engine's grammar whatever it is.
	Temperature float64
	// MaxTokens bounds the reply. Callers that plan a context count it
	// in: prompt + MaxTokens must fit the model's context.
	MaxTokens int
}

// withDefaults fills what req leaves unset from its skill's defaults.
func withDefaults(req Request, defaults func(Skill) SkillDefaults) Request {
	if req.Skill == "" || defaults == nil {
		return req
	}
	d := defaults(req.Skill)
	if req.Temperature == nil {
		req.Temperature = F64(d.Temperature)
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = d.MaxTokens
	}
	return req
}
