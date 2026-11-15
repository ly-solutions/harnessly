package limits

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// What is left of a vendor's subscription, for the profiles a chat runs on besides Claude and Codex (Kimi Code, GLM,
// Grok, the Moonshot platform). Each vendor has one request, the one its own CLI or plugin makes with the credential
// the machine already holds, and a parser that turns the answer into the entry rate-limits.json keeps for Claude: the
// windows `five_hour`, `seven_day`, `month` with `used_percentage` and `resets_at`, a `plan`, a `balance`. A parser
// takes only what it can read as a number: a window the answer does not carry is not there.

// Sources of the vendors.
const (
	SourceKimiUsages  = "kimi-usages"
	SourceZaiQuota    = "zai-quota"
	SourceGrokBilling = "grok-billing"
	SourceMoonshot    = "moonshot-balance"
	SourceDeepSeek    = "deepseek-balance"
	SourceOpenRouter  = "openrouter-credits"
)

// APIProfile is the profile of a provider of the registry by API (internal/extproviders) made from that preset: its
// entries stand apart from the chat profiles', which may share a vendor's name.
func APIProfile(preset string) string { return "api:" + preset }

// WindowMonth is the monthly pool some vendors have besides the two windows of Claude.
const WindowMonth = "month"

const (
	secFiveHour = 5 * 3600
	secSevenDay = 7 * 86400
	secMonth    = 30 * 86400
)

// Credential is what a request for the limits is authorised with: the saved key of the profile, or the token of the
// vendor CLI's sign-in (Login).
type Credential struct {
	Token string
	Login bool
	// Account is the account of the sign-in the vendor wants named beside the token (Vendor.AccountHeader); "" for none.
	Account string
}

// Vendor is how one vendor's limits are asked.
type Vendor struct {
	Source string
	URL    string
	// Auth puts the credential on the request the way the vendor's own client does.
	Auth func(req *http.Request, token string)
	// Parse is the entry of an answer; nil when it has nothing to show.
	Parse func(body any, now float64) Data
	// Refused says an answer that came as a success refuses the credential (Z.ai answers 200 with its own code); nil
	// when the vendor refuses by the HTTP status alone.
	Refused func(body any) bool
	// LoginOnly: the request works with the CLI's sign-in only, a saved API key has no such answer.
	LoginOnly bool
	// Unofficial: the request is the vendor's internal one, known from outside its own sources. Its entry says so, and
	// a failure leaves nothing of the last answer: what such a source said before it broke is not to be trusted.
	Unofficial bool
	// AccountHeader is the header the account of a sign-in goes in; "" when the vendor wants none.
	AccountHeader string
	// Host is the host a provider of the registry must point at for the request to be made with its key; empty for a
	// chat profile, whose address the runtime owns.
	Host string
}

func bearer(req *http.Request, token string) { req.Header.Set("Authorization", "Bearer "+token) }

// The vendors by the id of the profile (providers.Preset.ID).
var vendors = map[string]Vendor{
	// `/usage` of Kimi CLI: <the Kimi Code base>/usages with the key as a Bearer token.
	"kimi-code": {Source: SourceKimiUsages, URL: "https://api.kimi.com/coding/v1/usages", Auth: bearer, Parse: ParseKimiUsage},
	// The quota request of Z.ai's own Claude Code plugin (glm-plan-usage): the key goes as it is, with no scheme.
	"glm":    {Source: SourceZaiQuota, URL: "https://api.z.ai/api/monitor/usage/quota/limit", Auth: rawKey, Parse: ParseZaiQuota, Refused: zaiRefused},
	"glm-cn": {Source: SourceZaiQuota, URL: "https://open.bigmodel.cn/api/monitor/usage/quota/limit", Auth: rawKey, Parse: ParseZaiQuota, Refused: zaiRefused},
	// The billing request of the Grok CLI, with the token of its sign-in. A key of the xAI API has no pool to ask.
	"xai": {Source: SourceGrokBilling, URL: "https://cli-chat-proxy.grok.com/v1/billing?format=credits", Auth: grokAuth, Parse: ParseGrokCredits, LoginOnly: true, Unofficial: true},
	// The Moonshot platform is paid by the token: its documented balance, no windows.
	"kimi": {Source: SourceMoonshot, URL: "https://api.moonshot.ai/v1/users/me/balance", Auth: bearer, Parse: moonshotUSD},
	// The providers of the registry by API whose vendor documents a balance for the key itself.
	// The same vendors as accounts of the registry: each key asks for itself.
	"api:kimi-code":  {Source: SourceKimiUsages, URL: "https://api.kimi.com/coding/v1/usages", Auth: bearer, Parse: ParseKimiUsage, Host: "api.kimi.com"},
	"api:kimi":       {Source: SourceMoonshot, URL: "https://api.moonshot.ai/v1/users/me/balance", Auth: bearer, Parse: moonshotUSD, Host: "api.moonshot.ai"},
	"api:glm":        {Source: SourceZaiQuota, URL: "https://api.z.ai/api/monitor/usage/quota/limit", Auth: rawKey, Parse: ParseZaiQuota, Refused: zaiRefused, Host: "api.z.ai"},
	"api:glm-cn":     {Source: SourceZaiQuota, URL: "https://open.bigmodel.cn/api/monitor/usage/quota/limit", Auth: rawKey, Parse: ParseZaiQuota, Refused: zaiRefused, Host: "open.bigmodel.cn"},
	"api:deepseek":   {Source: SourceDeepSeek, URL: "https://api.deepseek.com/user/balance", Auth: bearer, Parse: ParseDeepSeekBalance, Host: "api.deepseek.com"},
	"api:openrouter": {Source: SourceOpenRouter, URL: "https://openrouter.ai/api/v1/credits", Auth: bearer, Parse: ParseOpenRouterCredits, Host: "openrouter.ai"},
}

func rawKey(req *http.Request, token string) {
	req.Header.Set("Authorization", token)
	req.Header.Set("Accept-Language", "en-US,en")
}

func grokAuth(req *http.Request, token string) {
	bearer(req, token)
	req.Header.Set("x-xai-token-auth", "xai-grok-cli")
}

func moonshotUSD(body any, now float64) Data { return ParseMoonshotBalance(body, "USD", now) }

// CodexLoginPrefix starts the id of a profile that is one more sign-in of the Codex CLI.
const CodexLoginPrefix = "cx-"

// codexLogin asks the windows of a ChatGPT plan the way the Codex CLI does, with the token of that sign-in.
var codexLogin = Vendor{Source: SourceCodexUsage, URL: CodexUsageURL, Auth: codexAuth, Parse: profileCodexUsage,
	LoginOnly: true, AccountHeader: "ChatGPT-Account-Id"}

func codexAuth(req *http.Request, token string) {
	bearer(req, token)
	req.Header.Set("originator", "codex_cli_rs")
}

// profileCodexUsage is ParseCodexUsage without the field only the entry of the CLI's own sign-in has.
func profileCodexUsage(body any, now float64) Data {
	data := ParseCodexUsage(body, now)
	if data != nil {
		delete(data, "codexbarError")
	}
	return data
}

// VendorOf is how the limits of a profile are asked; ok is false for a profile that has no such request.
func VendorOf(profile string) (Vendor, bool) {
	if strings.HasPrefix(profile, CodexLoginPrefix) {
		return codexLogin, true
	}
	v, ok := vendors[profile]
	return v, ok
}

// numberOf is a JSON number, or a string that holds one (Kimi sends its counts as strings).
func numberOf(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, !math.IsNaN(x) && !math.IsInf(x, 0)
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f, err == nil && !math.IsNaN(f) && !math.IsInf(f, 0)
	}
	return 0, false
}

// timeOf is an ISO time (Kimi writes nanoseconds, Grok an offset) as epoch seconds.
func timeOf(v any) (float64, bool) {
	s, ok := v.(string)
	if !ok || s == "" {
		return 0, false
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return 0, false
	}
	return float64(t.UnixMilli()) / 1000, true
}

func firstOf(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v := m[k]; v != nil {
			return v
		}
	}
	return nil
}

// windowOfSeconds names a window by its length; empty for a length that is none of the three.
func windowOfSeconds(s float64) string {
	if k := windowKeyOfSeconds(s); k != "" {
		return k
	}
	if math.Abs(s-secMonth) <= 3*86400 {
		return WindowMonth
	}
	return ""
}

var windowSeconds = map[string]float64{"five_hour": secFiveHour, "seven_day": secSevenDay, WindowMonth: secMonth}

func clampPct(p float64) float64 { return math.Max(0, math.Min(100, p)) }

// window is a stored window. A vendor names no reset for a window nothing was spent of: it starts with the first
// request, so it ends no sooner than its length from now. A spent window without a reset is not a window.
func window(key string, usedPct, reset float64, hasReset bool, source string, now float64) map[string]any {
	if !hasReset {
		if usedPct > 0 {
			return nil
		}
		reset = jsRound(now + windowSeconds[key])
	}
	return map[string]any{"used_percentage": clampPct(usedPct), "resets_at": reset, "at": now, "source": source}
}

func entry(source string, now float64) Data {
	return Data{"ts": now, "source": source, "five_hour": nil, "seven_day": nil, WindowMonth: nil, "error": nil}
}

func hasAny(d Data) bool {
	return d["five_hour"] != nil || d["seven_day"] != nil || d[WindowMonth] != nil || d["balance"] != nil
}

// usedOf is the spent share of a count {limit, used | remaining}, per cent.
func usedOf(detail map[string]any) (float64, bool) {
	limit, ok := numberOf(detail["limit"])
	if !ok || limit <= 0 {
		return 0, false
	}
	used, ok := numberOf(detail["used"])
	if !ok {
		remaining, ok := numberOf(detail["remaining"])
		if !ok {
			return 0, false
		}
		used = limit - remaining
	}
	return used / limit * 100, true
}

func kimiCount(detail map[string]any, key, source string, now float64) map[string]any {
	pct, ok := usedOf(detail)
	if !ok {
		return nil
	}
	reset, has := timeOf(firstOf(detail, "resetTime", "reset_time", "resetAt", "reset_at"))
	return window(key, pct, reset, has, source, now)
}

var kimiTimeUnits = map[string]float64{"TIME_UNIT_SECOND": 1, "TIME_UNIT_MINUTE": 60, "TIME_UNIT_HOUR": 3600, "TIME_UNIT_DAY": 86400}

var kimiPools = map[string]string{"limit_5h": "five_hour", "limit_7d": "seven_day", "limit_month_total": WindowMonth}

// ParseKimiUsage reads the answer of Kimi Code's usages: the weekly count in `usage`, the counted windows in `limits`
// (the five hours as 300 minutes), and on newer plans the shares in `usages` (`limit_5h`, `limit_7d`,
// `limit_month_total` with `used_ratio` 0..1), which replace a count of the same window.
func ParseKimiUsage(body any, now float64) Data {
	b, ok := object(body)
	if !ok {
		return nil
	}
	data := entry(SourceKimiUsages, now)
	if usage, ok := object(b["usage"]); ok {
		if w := kimiCount(usage, "seven_day", SourceKimiUsages, now); w != nil {
			data["seven_day"] = w
		}
	}
	if list, ok := b["limits"].([]any); ok {
		for _, raw := range list {
			item, ok := object(raw)
			if !ok {
				continue
			}
			win, _ := object(item["window"])
			duration, ok := numberOf(win["duration"])
			unit, _ := win["timeUnit"].(string)
			if !ok || kimiTimeUnits[unit] == 0 {
				continue
			}
			key := windowOfSeconds(duration * kimiTimeUnits[unit])
			detail, ok := object(item["detail"])
			if key == "" || !ok {
				continue
			}
			if w := kimiCount(detail, key, SourceKimiUsages, now); w != nil {
				data[key] = w
			}
		}
	}
	if pools, ok := object(b["usages"]); ok {
		for name, key := range kimiPools {
			pool, ok := object(pools[name])
			if !ok {
				continue
			}
			ratio, ok := numberOf(pool["used_ratio"])
			if !ok {
				continue
			}
			reset, has := timeOf(pool["reset_time"])
			if w := window(key, ratio*100, reset, has, SourceKimiUsages, now); w != nil {
				data[key] = w
			}
		}
	}
	if !hasAny(data) {
		return nil
	}
	return data
}

// zaiUnits are the lengths of the `unit` codes of a limit, in seconds.
var zaiUnits = map[float64]float64{1: 86400, 3: 3600, 5: 60, 6: 7 * 86400}

// epochSeconds reads a time a vendor gives in milliseconds (or, on older answers, in seconds).
func epochSeconds(v float64) float64 {
	if v > 1e11 {
		return v / 1000
	}
	return v
}

// ParseZaiQuota reads the quota of a GLM Coding Plan (Z.ai and bigmodel.cn answer alike): `data.limits`, of which the
// TOKENS_LIMIT entries are the plan's windows, `unit` 3 × `number` 5 being the five hours and `unit` 6 × 1 the week,
// with `percentage` spent and `nextResetTime` in milliseconds. TIME_LIMIT is the monthly count of the plan's MCP
// tools, not of the model, and is left out.
func ParseZaiQuota(body any, now float64) Data {
	b, ok := object(body)
	if !ok {
		return nil
	}
	d, ok := object(b["data"])
	if !ok {
		return nil
	}
	list, ok := d["limits"].([]any)
	if !ok {
		return nil
	}
	data := entry(SourceZaiQuota, now)
	for _, raw := range list {
		item, ok := object(raw)
		if !ok || item["type"] != "TOKENS_LIMIT" {
			continue
		}
		unit, _ := numberOf(item["unit"])
		number, _ := numberOf(item["number"])
		pct, ok := numberOf(item["percentage"])
		key := windowOfSeconds(zaiUnits[unit] * number)
		if !ok || key == "" {
			continue
		}
		reset, has := numberOf(item["nextResetTime"])
		if w := window(key, pct, epochSeconds(reset), has && reset > 0, SourceZaiQuota, now); w != nil {
			data[key] = w
		}
	}
	if !hasAny(data) {
		return nil
	}
	if plan, ok := firstOf(d, "planName", "level").(string); ok && plan != "" {
		data["plan"] = map[string]any{"name": "GLM " + plan, "at": now}
	}
	return data
}

// zaiRefused: `success` false with a code of the 1000s (1001: no credential in the header) or 401.
func zaiRefused(body any) bool {
	b, ok := object(body)
	if !ok || b["success"] != false {
		return false
	}
	code, _ := numberOf(b["code"])
	return code == 401 || (code >= 1000 && code < 1100)
}

// ParseGrokCredits reads the billing of a Grok subscription as the Grok CLI asks it: `config.creditUsagePercent` (else
// the spent over the cap) of the current period, weekly on today's plans. A period whose length is told and is a
// month goes to the monthly window.
func ParseGrokCredits(body any, now float64) Data {
	b, ok := object(body)
	if !ok {
		return nil
	}
	cfg, ok := object(b["config"])
	if !ok {
		return nil
	}
	pct, ok := numberOf(cfg["creditUsagePercent"])
	if !ok {
		used, _ := object(cfg["onDemandUsed"])
		limit, _ := object(cfg["onDemandCap"])
		u, uok := numberOf(used["val"])
		l, lok := numberOf(limit["val"])
		if !uok || !lok || l <= 0 {
			return nil
		}
		pct = u / l * 100
	}
	period, _ := object(cfg["currentPeriod"])
	end, ok := timeOf(period["end"])
	if !ok {
		if end, ok = timeOf(cfg["billingPeriodEnd"]); !ok {
			return nil
		}
	}
	key := "seven_day"
	if start, ok := timeOf(period["start"]); ok && windowOfSeconds(end-start) == WindowMonth {
		key = WindowMonth
	}
	data := entry(SourceGrokBilling, now)
	data[key] = window(key, pct, end, true, SourceGrokBilling, now)
	return data
}

// ParseMoonshotBalance reads the balance of a Moonshot platform account: what is left to spend, in the currency of the
// platform the key is of.
func ParseMoonshotBalance(body any, currency string, now float64) Data {
	b, ok := object(body)
	if !ok {
		return nil
	}
	d, ok := object(b["data"])
	if !ok {
		return nil
	}
	amount, ok := numberOf(d["available_balance"])
	if !ok {
		return nil
	}
	data := entry(SourceMoonshot, now)
	data["balance"] = map[string]any{"amount": amount, "currency": currency, "at": now}
	return data
}

func balanceEntry(source string, amount float64, currency string, now float64) Data {
	data := entry(source, now)
	data["balance"] = map[string]any{"amount": amount, "currency": currency, "at": now}
	return data
}

// ParseDeepSeekBalance reads DeepSeek's balance of a key: `balance_infos`, of which the dollar one is taken, else the
// first.
func ParseDeepSeekBalance(body any, now float64) Data {
	b, ok := object(body)
	if !ok {
		return nil
	}
	list, _ := b["balance_infos"].([]any)
	var found Data
	for _, raw := range list {
		info, ok := object(raw)
		if !ok {
			continue
		}
		amount, aok := numberOf(info["total_balance"])
		currency, cok := info["currency"].(string)
		if !aok || !cok || currency == "" {
			continue
		}
		if found == nil || currency == "USD" {
			found = balanceEntry(SourceDeepSeek, amount, currency, now)
		}
	}
	return found
}

// ParseOpenRouterCredits reads OpenRouter's credits: what was bought less what was spent, in dollars.
func ParseOpenRouterCredits(body any, now float64) Data {
	b, ok := object(body)
	if !ok {
		return nil
	}
	d, ok := object(b["data"])
	if !ok {
		return nil
	}
	total, tok := numberOf(d["total_credits"])
	used, uok := numberOf(d["total_usage"])
	if !tok || !uok {
		return nil
	}
	return balanceEntry(SourceOpenRouter, total-used, "USD", now)
}
